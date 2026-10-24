package graph

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrNodeTimeout is wrapped by the error of a node that ran longer than
// its WithNodeTimeout.
var ErrNodeTimeout = errors.New("graph: node timed out")

// ErrorHandler recovers from a node's failure once its retries are
// exhausted. It receives the state the node was given and the error, and
// returns the state to continue with and, optionally, the node to go to
// next ("" follows the node's edges as if it had succeeded). Returning an
// error fails the run with that error.
//
// For a fan-out branch only the state is used; the branch's result is
// replaced by it and next is ignored.
type ErrorHandler[S any] func(ctx context.Context, state S, err error) (recovered S, next string, handlerErr error)

// NodeOption configures one node. Pass it to AddNodeWithOptions.
type NodeOption[S any] func(*nodeConfig[S])

type nodeConfig[S any] struct {
	timeout   time.Duration
	retries   int
	backoff   time.Duration
	onFailure ErrorHandler[S]
}

// WithNodeTimeout caps each attempt of the node at d. The node's context
// is cancelled at the deadline and the attempt fails with an error
// wrapping ErrNodeTimeout. A node that ignores its context keeps running
// in the background, but its result is discarded.
func WithNodeTimeout[S any](d time.Duration) NodeOption[S] {
	return func(c *nodeConfig[S]) { c.timeout = d }
}

// WithNodeRetry re-runs a failed node up to n more times, waiting backoff
// before the first retry and doubling it after each one. Interrupts and a
// cancelled run are never retried.
func WithNodeRetry[S any](n int, backoff time.Duration) NodeOption[S] {
	return func(c *nodeConfig[S]) {
		c.retries = n
		c.backoff = backoff
	}
}

// WithErrorHandler recovers from the node's failure once its retries are
// exhausted; see ErrorHandler. Interrupts are not failures and never reach
// it.
func WithErrorHandler[S any](h ErrorHandler[S]) NodeOption[S] {
	return func(c *nodeConfig[S]) { c.onFailure = h }
}

// AddNodeWithOptions registers a node with per-node behavior: a timeout,
// retries and an error handler.
//
//	g.AddNodeWithOptions("fetch", fetch,
//	    graph.WithNodeTimeout[State](10*time.Second),
//	    graph.WithNodeRetry[State](2, time.Second),
//	    graph.WithErrorHandler(func(ctx context.Context, s State, err error) (State, string, error) {
//	        s.Error = err.Error()
//	        return s, "apologize", nil
//	    }),
//	)
func (g *Graph[S]) AddNodeWithOptions(name string, fn NodeFunc[S], opts ...NodeOption[S]) {
	g.nodes[name] = fn
	cfg := &nodeConfig[S]{}
	for _, opt := range opts {
		opt(cfg)
	}
	g.nodeConfigs[name] = cfg
}

// execNode runs a node with its retries and timeout, then hands a final
// failure to its error handler. next is the handler's routing override.
func (g *Graph[S]) execNode(ctx context.Context, cfg *runConfig[S], name string, state S) (out S, next string, err error) {
	fn := g.nodes[name]
	nc := g.nodeConfigs[name]
	if nc == nil {
		out, err = cfg.callNode(ctx, name, fn, state)
		return out, "", err
	}

	backoff := nc.backoff
	for attempt := 0; ; attempt++ {
		out, err = cfg.callNode(ctx, name, nc.wrap(name, fn), state)
		if err == nil {
			return out, "", nil
		}
		if _, ok := asInterrupt(err); ok || ctx.Err() != nil || attempt >= nc.retries {
			break
		}
		if !sleep(ctx, backoff) {
			break
		}
		backoff *= 2
	}

	if _, ok := asInterrupt(err); ok || nc.onFailure == nil || ctx.Err() != nil {
		return out, "", err
	}
	out, next, herr := nc.onFailure(ctx, state, err)
	if herr != nil {
		return out, "", herr
	}
	return out, next, nil
}

// wrap applies the node's timeout.
func (c *nodeConfig[S]) wrap(name string, fn NodeFunc[S]) NodeFunc[S] {
	if c.timeout <= 0 {
		return fn
	}
	return func(ctx context.Context, state S) (S, error) {
		tctx, cancel := context.WithTimeoutCause(ctx, c.timeout, ErrNodeTimeout)
		defer cancel()

		type result struct {
			s   S
			err error
		}
		done := make(chan result, 1)
		go func() {
			s, err := fn(tctx, state)
			done <- result{s, err}
		}()
		select {
		case r := <-done:
			if r.err != nil && errors.Is(context.Cause(tctx), ErrNodeTimeout) && ctx.Err() == nil {
				return r.s, fmt.Errorf("%w: %q after %v: %w", ErrNodeTimeout, name, c.timeout, r.err)
			}
			return r.s, r.err
		case <-tctx.Done():
			var zero S
			if ctx.Err() != nil {
				return zero, ctx.Err()
			}
			return zero, fmt.Errorf("%w: %q after %v", ErrNodeTimeout, name, c.timeout)
		}
	}
}

// sleep waits d or until ctx is done, reporting whether it waited.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
