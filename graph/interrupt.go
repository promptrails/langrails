package graph

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Interruption describes a run paused by a node waiting for outside input,
// typically a human decision. It is returned in RunResult and saved in the
// checkpoint, so the run can be resumed later — minutes or days — from
// another process.
type Interruption struct {
	// Node is the node that paused. It runs again on resume.
	Node string

	// Payload is what the node asked with: a question, the action to
	// approve, a draft to review. Checkpointers that serialize state should
	// be able to serialize it too.
	Payload any
}

// InterruptError is the error a node returns to pause the run. Create it
// with Interrupt or Await rather than directly.
type InterruptError struct {
	Payload any
}

func (e *InterruptError) Error() string { return "graph: interrupted" }

// Interrupt returns an error that pauses the run when a node returns it.
// Prefer Await, which also hands the node the resume value.
func Interrupt(payload any) error {
	return &InterruptError{Payload: payload}
}

// resumeSlot carries a resume value to the interrupted node. It is consumed
// by the first Await that reads it, so on resume it reaches the node that
// asked, even if that node sits inside a subgraph that re-runs earlier
// nodes first.
type resumeSlot struct {
	mu    sync.Mutex
	value any
	used  bool
}

type resumeKey struct{}

// Await pauses the run until a value is supplied, then returns it.
//
// The first time a node calls Await, it returns an interrupt error carrying
// payload; the node must return that error. The graph saves a checkpoint
// and Run returns with RunResult.Interruption set. When the run is resumed
// with WithResumeValue(v), the node runs again from the top and this time
// Await returns v.
//
//	g.AddNode("approve", func(ctx context.Context, s State) (State, error) {
//	    ok, err := graph.Await[bool](ctx, "Deploy "+s.Version+"?")
//	    if err != nil {
//	        return s, err
//	    }
//	    s.Approved = ok
//	    return s, nil
//	})
//
// Because the node re-runs, code before Await runs twice; keep side effects
// after it. A resume value whose type is not T is an error.
func Await[T any](ctx context.Context, payload any) (T, error) {
	var zero T
	slot, ok := ctx.Value(resumeKey{}).(*resumeSlot)
	if ok {
		slot.mu.Lock()
		defer slot.mu.Unlock()
		if !slot.used {
			slot.used = true
			v, ok := slot.value.(T)
			if !ok {
				return zero, fmt.Errorf("graph: resume value is %T, want %T", slot.value, zero)
			}
			return v, nil
		}
	}
	return zero, Interrupt(payload)
}

// WithResumeValue supplies the answer to a pending interruption when
// resuming. The interrupted node's Await returns it.
func WithResumeValue[S any](v any) Option[S] {
	return func(c *runConfig[S]) {
		c.resume = &resumeSlot{value: v}
	}
}

// asInterrupt reports whether err is (or wraps) a node interrupt.
func asInterrupt(err error) (*InterruptError, bool) {
	var ie *InterruptError
	if errors.As(err, &ie) {
		return ie, true
	}
	return nil, false
}
