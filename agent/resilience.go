package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/promptrails/langrails"
)

// ToolRetry retries failed tool executions with exponential backoff. Only
// the final error reaches the model.
type ToolRetry struct {
	BaseMiddleware
	maxRetries int
	baseDelay  time.Duration
	retryable  func(error) bool
	names      map[string]bool
}

// ToolRetryOption configures ToolRetry.
type ToolRetryOption func(*ToolRetry)

// WithToolRetryDelay sets the delay before the first retry; it doubles
// with each further retry. Default is 500ms.
func WithToolRetryDelay(d time.Duration) ToolRetryOption {
	return func(r *ToolRetry) { r.baseDelay = d }
}

// WithToolRetryIf retries only errors for which fn returns true. By default
// every error is retried.
func WithToolRetryIf(fn func(error) bool) ToolRetryOption {
	return func(r *ToolRetry) { r.retryable = fn }
}

// WithToolRetryOn limits retries to the named tools.
func WithToolRetryOn(names ...string) ToolRetryOption {
	return func(r *ToolRetry) {
		r.names = make(map[string]bool, len(names))
		for _, n := range names {
			r.names[n] = true
		}
	}
}

// NewToolRetry retries each failing tool call up to maxRetries times.
func NewToolRetry(maxRetries int, opts ...ToolRetryOption) *ToolRetry {
	r := &ToolRetry{maxRetries: maxRetries, baseDelay: 500 * time.Millisecond}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// WrapToolCall retries the call on error.
func (r *ToolRetry) WrapToolCall(next ToolFunc) ToolFunc {
	return func(ctx context.Context, call langrails.ToolCall) (string, error) {
		out, err := next(ctx, call)
		if r.names != nil && !r.names[call.Name] {
			return out, err
		}
		delay := r.baseDelay
		for attempt := 0; err != nil && attempt < r.maxRetries; attempt++ {
			if r.retryable != nil && !r.retryable(err) {
				break
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return "", err
			case <-timer.C:
			}
			delay *= 2
			out, err = next(ctx, call)
		}
		return out, err
	}
}

// ModelFallback retries a failed model call with other models on the same
// provider, in order, and returns the first success. To fall back to a
// different provider, wrap the provider with langrails.WithFallback
// instead.
type ModelFallback struct {
	BaseMiddleware
	models []string
}

// NewModelFallback tries models in order after the agent's own model fails.
func NewModelFallback(models ...string) *ModelFallback {
	return &ModelFallback{models: models}
}

// WrapModelCall retries with each fallback model. The agent's request is
// not modified: each fallback gets its own copy with Model replaced.
func (f *ModelFallback) WrapModelCall(next CallFunc) CallFunc {
	return func(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
		resp, err := next(ctx, req)
		for _, model := range f.models {
			if err == nil || ctx.Err() != nil {
				break
			}
			r := *req
			r.Model = model
			var ferr error
			resp, ferr = next(ctx, &r)
			if ferr != nil {
				err = fmt.Errorf("%w; fallback %s: %v", err, model, ferr)
			} else {
				err = nil
			}
		}
		return resp, err
	}
}
