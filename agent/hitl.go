package agent

import (
	"context"
	"fmt"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

// Decision is an approver's verdict on a tool call. When Approve is false,
// Reason is surfaced to the model as the tool result so it can adapt.
type Decision struct {
	Approve bool
	Reason  string
}

// Approve is a convenience constructor for an approving Decision.
func Approve() Decision { return Decision{Approve: true} }

// Reject is a convenience constructor for a rejecting Decision with a
// reason returned to the model.
func Reject(reason string) Decision { return Decision{Approve: false, Reason: reason} }

// Approver decides whether a tool call may run. It is consulted before the
// underlying tool executes. It may block while waiting for a human (for
// example on a channel or HTTP callback) and may return an error to abort
// the run — pair that with a graph checkpointer to pause durably and
// resume later.
type Approver func(ctx context.Context, call langrails.ToolCall) (Decision, error)

// HumanInLoop wraps a tools.Executor with an approval gate. Before a
// guarded tool runs, the approver is consulted; approved calls run the
// wrapped executor, while rejected calls return a rejection result to the
// model instead of executing. This is the interrupt / human-in-the-loop
// pattern: a human reviews and approves (or denies) tool calls before they
// take effect.
type HumanInLoop struct {
	executor tools.Executor
	approve  Approver
	guarded  map[string]bool // nil/empty means every tool is guarded
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
			return "", fmt.Errorf("approval for %q: %w", name, err)
		}
		if !dec.Approve {
			reason := dec.Reason
			if reason == "" {
				reason = "tool call rejected by human reviewer"
			}
			return fmt.Sprintf(`{"error": %q}`, reason), nil
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
