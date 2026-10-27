package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/graph"
	"github.com/promptrails/langrails/tools"
)

// ErrInvalidEdit is wrapped by the error that aborts a run when an
// approver's edited arguments do not match the tool's schema.
var ErrInvalidEdit = errors.New("agent: edited arguments do not match the tool's schema")

// Decision is an approver's verdict on a tool call. When Approve is false,
// Reason is surfaced to the model as the tool result so it can adapt.
type Decision struct {
	Approve bool
	Reason  string

	// Arguments, when set on an approving decision, replace the call's
	// arguments: the reviewer edited the call before letting it run. The
	// tool cannot be changed. With WithToolSchemas the edit is checked
	// against the tool's schema first.
	Arguments string
}

// Approve is a convenience constructor for an approving Decision.
func Approve() Decision { return Decision{Approve: true} }

// Reject is a convenience constructor for a rejecting Decision with a
// reason returned to the model.
func Reject(reason string) Decision { return Decision{Approve: false, Reason: reason} }

// Edit approves the call with the given JSON arguments instead of the
// model's.
func Edit(arguments string) Decision { return Decision{Approve: true, Arguments: arguments} }

// Approver decides whether a tool call may run. It is consulted before the
// underlying tool executes. It may block while waiting for a human (for
// example on a channel or HTTP callback). An error aborts the agent run;
// AwaitApproval uses that to pause a graph durably instead of blocking.
type Approver func(ctx context.Context, call langrails.ToolCall) (Decision, error)

// AbortError marks a tool error that must end the agent run instead of
// being sent to the model as the tool's result. Return Abort(err) from an
// executor or tool wrapper to stop the run; the agent returns an error
// wrapping err. Graph interrupts (graph.Await, graph.Interrupt) abort the
// run the same way without wrapping, so a node running the agent pauses.
type AbortError struct {
	Err error
}

func (e *AbortError) Error() string { return e.Err.Error() }

func (e *AbortError) Unwrap() error { return e.Err }

// Abort wraps err so that it ends the agent run.
func Abort(err error) error {
	return &AbortError{Err: err}
}

// aborts reports whether a tool error ends the run.
func aborts(err error) bool {
	var ae *AbortError
	var ie *graph.InterruptError
	return errors.As(err, &ae) || errors.As(err, &ie)
}

// HumanInLoop wraps a tools.Executor with an approval gate. Before a
// guarded tool runs, the approver is consulted; approved calls run the
// wrapped executor (with the reviewer's edits, if any), while rejected
// calls return a rejection result to the model instead of executing. This
// is the interrupt / human-in-the-loop pattern: a human reviews and
// approves, edits or denies tool calls before they take effect.
type HumanInLoop struct {
	executor tools.Executor
	approve  Approver
	guarded  map[string]bool // nil/empty means every tool is guarded
	schemas  map[string]json.RawMessage
}

// HumanInLoopOption configures a HumanInLoop.
type HumanInLoopOption func(*HumanInLoop)

// WithInterruptOn restricts approval to the named tools. Tools not listed
// run without approval. By default every tool requires approval.
func WithInterruptOn(names ...string) HumanInLoopOption {
	return func(h *HumanInLoop) {
		h.guarded = make(map[string]bool, len(names))
		for _, n := range names {
			h.guarded[n] = true
		}
	}
}

// WithToolSchemas checks edited arguments against the tools' parameter
// schemas before the edited call runs. An edit that does not match aborts
// the run with an error wrapping ErrInvalidEdit, rather than letting a bad
// call reach the tool.
func WithToolSchemas(defs []langrails.ToolDefinition) HumanInLoopOption {
	return func(h *HumanInLoop) {
		h.schemas = make(map[string]json.RawMessage, len(defs))
		for _, d := range defs {
			h.schemas[d.Name] = d.Parameters
		}
	}
}

// NewHumanInLoop wraps executor so that guarded tool calls require approval
// before they run. Pass it to agent.WithTools in place of the bare
// executor.
func NewHumanInLoop(executor tools.Executor, approve Approver, opts ...HumanInLoopOption) *HumanInLoop {
	h := &HumanInLoop{executor: executor, approve: approve}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Execute consults the approver for guarded tools, then runs the wrapped
// executor for approved calls or returns a rejection result otherwise.
func (h *HumanInLoop) Execute(ctx context.Context, name, arguments string) (string, error) {
	if h.requiresApproval(name) {
		dec, err := h.approve(ctx, langrails.ToolCall{Name: name, Arguments: arguments})
		if err != nil {
			return "", Abort(fmt.Errorf("approval for %q: %w", name, err))
		}
		if !dec.Approve {
			reason := dec.Reason
			if reason == "" {
				reason = "tool call rejected by human reviewer"
			}
			return fmt.Sprintf(`{"error": %q}`, reason), nil
		}
		if dec.Arguments != "" {
			if schema, ok := h.schemas[name]; ok {
				if err := ValidateArguments(schema, dec.Arguments); err != nil {
					return "", Abort(fmt.Errorf("%w: %q: %w", ErrInvalidEdit, name, err))
				}
			}
			arguments = dec.Arguments
		}
	}
	return h.executor.Execute(ctx, name, arguments)
}

func (h *HumanInLoop) requiresApproval(name string) bool {
	if len(h.guarded) == 0 {
		return true
	}
	return h.guarded[name]
}

// ApprovalRequest is the interrupt payload AwaitApproval pauses a graph
// with: the call to review, the tool's schema (so a UI can build an edit
// form), and the decisions given so far.
type ApprovalRequest struct {
	// Key identifies the call in Approvals.
	Key string

	// Tool and Arguments are the call awaiting a decision.
	Tool      string
	Arguments string

	// Schema is the tool's parameter schema, when known.
	Schema json.RawMessage

	// Decided holds the decisions the run was resumed with. Resume with
	// Decided.With(req, decision) so earlier calls stay answered.
	Decided Approvals
}

// Approvals are decisions keyed by ApprovalRequest.Key. Resume a graph
// paused by AwaitApproval with one, via graph.WithResumeValue.
type Approvals map[string]Decision

// With returns a copy of a that also holds d as the answer to req.
func (a Approvals) With(req ApprovalRequest, d Decision) Approvals {
	out := make(Approvals, len(a)+1)
	for k, v := range a {
		out[k] = v
	}
	out[req.Key] = d
	return out
}

// AwaitApproval returns an Approver for agents that run inside a graph
// node with a checkpointer. Instead of blocking, it pauses the graph with
// an ApprovalRequest as the interrupt payload; resume with the Approvals
// so far plus the new decision:
//
//	req := res.Interruption.Payload.(agent.ApprovalRequest)
//	res, err = g.Resume(ctx, append(opts,
//	    graph.WithResumeValue[State](req.Decided.With(req, agent.Approve())))...)
//
// The node runs again from the top on resume, so the agent replays its
// run; calls already decided find their decision and go through. Calls
// are keyed by tool name and arguments, so the replay should make the same
// calls: put a response cache in front of the provider (langrails.WithCache)
// to make model calls repeat exactly. Tools that ran before the pause run
// again in the replay, so use it with tools that tolerate that (reads,
// idempotent writes), or block in the approver instead. Pass defs to include each tool's
// schema in the request; with WithToolSchemas on the HumanInLoop, edits
// are also checked against it.
func AwaitApproval(defs []langrails.ToolDefinition) Approver {
	schemas := make(map[string]json.RawMessage, len(defs))
	for _, d := range defs {
		schemas[d.Name] = d.Parameters
	}
	return func(ctx context.Context, call langrails.ToolCall) (Decision, error) {
		key := call.Name + "\x00" + call.Arguments
		decided, _ := graph.ResumeValue[Approvals](ctx)
		if d, ok := decided[key]; ok {
			return d, nil
		}
		return Decision{}, graph.Interrupt(ApprovalRequest{
			Key:       key,
			Tool:      call.Name,
			Arguments: call.Arguments,
			Schema:    schemas[call.Name],
			Decided:   decided,
		})
	}
}
