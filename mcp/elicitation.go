package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/promptrails/langrails/graph"
)

// Elicitation actions.
const (
	ElicitAccept  = "accept"
	ElicitDecline = "decline"
	ElicitCancel  = "cancel"
)

// ElicitRequest is a server asking the user for input in the middle of a
// tool call.
type ElicitRequest struct {
	// Message tells the user what is being asked and why.
	Message string `json:"message"`

	// Schema is a flat JSON object schema (string, number, integer,
	// boolean and enum properties) describing the input to collect.
	Schema json.RawMessage `json:"requestedSchema"`
}

// ElicitResult is the user's response.
type ElicitResult struct {
	// Action is ElicitAccept (Content holds the input), ElicitDecline
	// (the user said no) or ElicitCancel (the user dismissed the request).
	Action string `json:"action"`

	// Content is the input, matching the request's schema, when accepted.
	Content map[string]any `json:"content,omitempty"`
}

// AcceptElicitation answers with the user's input.
func AcceptElicitation(content map[string]any) ElicitResult {
	return ElicitResult{Action: ElicitAccept, Content: content}
}

// ElicitationHandler collects the input a server asks for. It runs while
// the server waits, so it may block on a human. An error answers the
// server with a cancel and makes the tool call return that error; return
// a graph interrupt (see AwaitElicitation) to pause a graph instead.
type ElicitationHandler func(ctx context.Context, req ElicitRequest) (ElicitResult, error)

// WithElicitation lets servers ask the user for input during tool calls
// (the elicitation capability), answered by h.
func WithElicitation(h ElicitationHandler) Option {
	return func(c *Client) {
		c.elicit = h
	}
}

// elicitErrs collects handler errors for the tool calls in flight, so the
// call an elicitation belongs to can return the handler's error.
type elicitErrs struct {
	mu       sync.Mutex
	inflight map[*elicitSlot]bool
}

type elicitSlot struct {
	mu  sync.Mutex
	err error
}

type elicitKey struct{}

// begin registers a tool call; the returned context carries its slot.
func (e *elicitErrs) begin(ctx context.Context) (context.Context, *elicitSlot) {
	s := &elicitSlot{}
	e.mu.Lock()
	if e.inflight == nil {
		e.inflight = map[*elicitSlot]bool{}
	}
	e.inflight[s] = true
	e.mu.Unlock()
	return context.WithValue(ctx, elicitKey{}, s), s
}

func (e *elicitErrs) end(s *elicitSlot) {
	e.mu.Lock()
	delete(e.inflight, s)
	e.mu.Unlock()
}

// record attributes a handler error to the call whose context it ran in
// or, when the transport has no call context (stdio), to the only call in
// flight.
func (e *elicitErrs) record(ctx context.Context, err error) {
	s, ok := ctx.Value(elicitKey{}).(*elicitSlot)
	if !ok {
		e.mu.Lock()
		if len(e.inflight) == 1 {
			for only := range e.inflight {
				s = only
			}
		}
		e.mu.Unlock()
	}
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

func (s *elicitSlot) get() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// handleElicit answers an elicitation/create request.
func (c *Client) handleElicit(ctx context.Context, params json.RawMessage) (any, *jsonRPCError) {
	var req ElicitRequest
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, &jsonRPCError{Code: -32602, Message: "invalid elicitation request"}
	}
	res, err := c.elicit(ctx, req)
	if err != nil {
		c.elicitErrs.record(ctx, err)
		return ElicitResult{Action: ElicitCancel}, nil
	}
	if res.Action == "" {
		res.Action = ElicitCancel
	}
	return res, nil
}

// PendingElicitation is the interrupt payload AwaitElicitation pauses a
// graph with.
type PendingElicitation struct {
	// Key identifies the request in Elicitations.
	Key string

	// Request is what the server asked.
	Request ElicitRequest

	// Answered holds the answers the run was resumed with. Resume with
	// Answered.With(p, result) so earlier requests stay answered.
	Answered Elicitations
}

// Elicitations are answers keyed by PendingElicitation.Key. Resume a
// graph paused by AwaitElicitation with one, via graph.WithResumeValue.
type Elicitations map[string]ElicitResult

// With returns a copy of e that also holds r as the answer to p.
func (e Elicitations) With(p PendingElicitation, r ElicitResult) Elicitations {
	out := make(Elicitations, len(e)+1)
	for k, v := range e {
		out[k] = v
	}
	out[p.Key] = r
	return out
}

// AwaitElicitation returns a handler for tool calls made inside a graph
// node with a checkpointer. Instead of blocking, it cancels the server's
// request and pauses the graph with a PendingElicitation; resume with the
// answers so far plus the new one:
//
//	p := res.Interruption.Payload.(mcp.PendingElicitation)
//	res, err = g.Resume(ctx, append(opts, graph.WithResumeValue[State](
//	    p.Answered.With(p, mcp.AcceptElicitation(map[string]any{"name": "prod"}))))...)
//
// The node runs again on resume and makes the tool call again; when the
// server asks the same question (same message and schema) it gets the
// stored answer. An agent running in the node ends its run on the pause,
// so the graph can stop.
func AwaitElicitation() ElicitationHandler {
	return func(ctx context.Context, req ElicitRequest) (ElicitResult, error) {
		key := req.Message + "\x00" + string(req.Schema)
		answered, _ := graph.ResumeValue[Elicitations](ctx)
		if r, ok := answered[key]; ok {
			return r, nil
		}
		return ElicitResult{}, graph.Interrupt(PendingElicitation{Key: key, Request: req, Answered: answered})
	}
}

// elicitationError wraps a handler error for the tool call's caller.
func elicitationError(name string, err error) error {
	return fmt.Errorf("mcp: tool %q: elicitation: %w", name, err)
}
