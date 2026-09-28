package langrails

import (
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"time"
)

// DefaultMaxRetryDelay caps a single retry wait unless WithMaxDelay says
// otherwise.
const DefaultMaxRetryDelay = time.Minute

// RetryProvider wraps a Provider with automatic retry logic using
// exponential backoff with jitter. Only retryable errors (rate limits,
// server errors) are retried. When the provider says how long to wait
// (APIError.RetryAfter), that wait is used instead of the backoff.
type RetryProvider struct {
	inner      Provider
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	jitter     bool
}

// RetryOption configures the retry behavior.
type RetryOption func(*RetryProvider)

// WithBaseDelay sets the base delay for exponential backoff.
// Default is 1 second. The actual delay doubles with each retry:
// 1s, 2s, 4s, 8s, etc.
func WithBaseDelay(d time.Duration) RetryOption {
	return func(r *RetryProvider) {
		r.baseDelay = d
	}
}

// WithMaxDelay caps a single wait between attempts. Default is
// DefaultMaxRetryDelay. When a provider's Retry-After asks for longer than
// this, the error is returned instead of retrying early into another
// rejection.
func WithMaxDelay(d time.Duration) RetryOption {
	return func(r *RetryProvider) {
		r.maxDelay = d
	}
}

// WithoutJitter makes the backoff deterministic. By default each backoff
// delay is randomized between half and all of its nominal value, so many
// clients failing at once do not retry in lockstep.
func WithoutJitter() RetryOption {
	return func(r *RetryProvider) {
		r.jitter = false
	}
}

// WithRetry wraps a provider with retry logic. maxRetries is the maximum
// number of retry attempts (not including the initial attempt).
//
// Example:
//
//	provider := langrails.WithRetry(openai.New("sk-..."), 3)
func WithRetry(provider Provider, maxRetries int, opts ...RetryOption) *RetryProvider {
	r := &RetryProvider{
		inner:      provider,
		maxRetries: maxRetries,
		baseDelay:  time.Second,
		maxDelay:   DefaultMaxRetryDelay,
		jitter:     true,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Complete sends a completion request with automatic retries.
func (r *RetryProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		resp, err := r.inner.Complete(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}

		if attempt < r.maxRetries {
			if !r.wait(ctx, attempt, err) {
				return nil, lastErr
			}
		}
	}

	return nil, lastErr
}

// Stream sends a streaming request with automatic retries.
// Note: only the initial connection is retried, not mid-stream failures.
func (r *RetryProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		ch, err := r.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}

		if attempt < r.maxRetries {
			if !r.wait(ctx, attempt, err) {
				return nil, lastErr
			}
		}
	}

	return nil, lastErr
}

// wait sleeps before the next attempt. It returns false when the caller
// should give up instead: the context ended, or the provider asked for a
// longer wait than maxDelay allows.
func (r *RetryProvider) wait(ctx context.Context, attempt int, err error) bool {
	delay := r.backoff(attempt)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		if apiErr.RetryAfter > r.maxDelay {
			return false
		}
		delay = apiErr.RetryAfter
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (r *RetryProvider) backoff(attempt int) time.Duration {
	delay := time.Duration(float64(r.baseDelay) * math.Pow(2, float64(attempt)))
	if delay > r.maxDelay || (delay < 0 && r.baseDelay > 0) { // < 0: overflow
		delay = r.maxDelay
	}
	if r.jitter && delay > 1 {
		half := delay / 2
		delay = half + rand.N(delay-half+1) // #nosec G404 -- backoff jitter, not security-sensitive
	}
	return delay
}

func isRetryable(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.IsRetryable()
	}
	return false
}
