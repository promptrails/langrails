package langrails

import (
	"context"
	"sync"
	"time"
)

// RateLimitProvider wraps a Provider so that requests are sent no faster
// than a fixed rate, using a token bucket. Callers over the limit block until
// a token is available or their context ends. It is safe for concurrent use,
// and one RateLimitProvider shared by many goroutines limits them together.
type RateLimitProvider struct {
	inner Provider

	mu       sync.Mutex
	interval time.Duration // time to earn one token
	burst    float64
	tokens   float64
	last     time.Time
	now      func() time.Time
}

// WithRateLimit wraps a provider with a client-side rate limit of
// requestsPerSecond, allowing bursts of up to burst requests. Rates below
// one per second are fine (0.5 = one request every two seconds); zero or
// negative means unlimited. burst < 1 is treated as 1.
//
// Example:
//
//	// At most 5 requests per second, up to 10 at once after a quiet period.
//	provider := langrails.WithRateLimit(openai.New("sk-..."), 5, 10)
//
// Put it inside WithRetry so that retries are rate limited too:
//
//	provider := langrails.WithRetry(langrails.WithRateLimit(p, 5, 10), 3)
func WithRateLimit(provider Provider, requestsPerSecond float64, burst int) *RateLimitProvider {
	if burst < 1 {
		burst = 1
	}
	interval := time.Duration(float64(time.Second) / requestsPerSecond)
	if requestsPerSecond <= 0 || interval <= 0 {
		interval = time.Nanosecond
	}
	r := &RateLimitProvider{
		inner:    provider,
		interval: interval,
		burst:    float64(burst),
		tokens:   float64(burst),
		now:      time.Now,
	}
	r.last = r.now()
	return r
}

// Complete waits for a token, then sends the request.
func (r *RateLimitProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if err := r.Wait(ctx); err != nil {
		return nil, err
	}
	return r.inner.Complete(ctx, req)
}

// Stream waits for a token, then opens the stream.
func (r *RateLimitProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	if err := r.Wait(ctx); err != nil {
		return nil, err
	}
	return r.inner.Stream(ctx, req)
}

// Wait blocks until a request may be sent, and consumes that slot. It
// returns the context's error if the context ends first; the slot is then
// given back.
func (r *RateLimitProvider) Wait(ctx context.Context) error {
	delay := r.reserve()
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		r.mu.Lock()
		r.tokens++
		r.mu.Unlock()
		return ctx.Err()
	}
}

// reserve takes a token, possibly driving the bucket negative, and returns
// how long the caller must wait for the token it took to have been earned.
// Reserving up front keeps waiters in arrival order without a queue.
func (r *RateLimitProvider) reserve() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	elapsed := now.Sub(r.last)
	r.last = now
	r.tokens += float64(elapsed) / float64(r.interval)
	if r.tokens > r.burst {
		r.tokens = r.burst
	}

	r.tokens--
	if r.tokens >= 0 {
		return 0
	}
	return time.Duration(-r.tokens * float64(r.interval))
}
