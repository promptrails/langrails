package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/promptrails/langrails"
)

// ErrCallLimit is wrapped by the error a limit middleware returns when a
// run exceeds its budget and the middleware is set to fail rather than end.
var ErrCallLimit = errors.New("call limit exceeded")

// ModelCallLimit caps the number of model calls in one run. When the model
// still wants to call tools after the last allowed call, the run either ends
// gracefully with that response (the default) or fails with ErrCallLimit.
//
// Unlike WithMaxIterations, which always fails, the graceful mode returns
// the last response, so the caller gets whatever the model produced.
type ModelCallLimit struct {
	BaseMiddleware
	limit int
	fail  bool
}

// NewModelCallLimit allows at most limit model calls per run.
func NewModelCallLimit(limit int) *ModelCallLimit {
	return &ModelCallLimit{limit: limit}
}

// FailOnLimit makes the run fail with ErrCallLimit instead of ending
// gracefully.
func (m *ModelCallLimit) FailOnLimit() *ModelCallLimit {
	m.fail = true
	return m
}

// AfterModel stops (or fails) the run once the limit is reached and the
// model asks for more.
func (m *ModelCallLimit) AfterModel(_ context.Context, s *State) error {
	if s.Iteration < m.limit || len(s.Response.ToolCalls) == 0 {
		return nil
	}
	if m.fail {
		return fmt.Errorf("model call limit %d: %w", m.limit, ErrCallLimit)
	}
	s.Stop()
	return nil
}

// ToolCallLimit caps how many times tools may run in one run: all tools
// together, or only the named ones. A call over the limit is not executed;
// the model receives an error result telling it the limit was reached, so
// it can answer with what it has.
type ToolCallLimit struct {
	BaseMiddleware
	limit int
	names map[string]bool
}

// NewToolCallLimit allows at most limit tool executions per run. With
// names, each named tool gets its own limit and other tools are unlimited.
func NewToolCallLimit(limit int, names ...string) *ToolCallLimit {
	l := &ToolCallLimit{limit: limit}
	if len(names) > 0 {
		l.names = make(map[string]bool, len(names))
		for _, n := range names {
			l.names[n] = true
		}
	}
	return l
}

// WrapToolCall refuses calls beyond the limit.
func (l *ToolCallLimit) WrapToolCall(next ToolFunc) ToolFunc {
	return func(ctx context.Context, call langrails.ToolCall) (string, error) {
		count := 0
		switch {
		case l.names == nil:
			count = toolCallCount(ctx, "")
		case l.names[call.Name]:
			count = toolCallCount(ctx, call.Name)
		}
		if count > l.limit {
			return "", fmt.Errorf("tool %q not run: limit of %d calls reached; answer with the information you have: %w",
				call.Name, l.limit, ErrCallLimit)
		}
		return next(ctx, call)
	}
}
