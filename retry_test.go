package langrails

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockProvider struct {
	completeFunc func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	streamFunc   func(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)
}

func (m *mockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	return m.completeFunc(ctx, req)
}

func (m *mockProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	return m.streamFunc(ctx, req)
}

func TestRetryProvider_SucceedsFirstAttempt(t *testing.T) {
	calls := 0
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			calls++
			return &CompletionResponse{Content: "hello"}, nil
		},
	}

	provider := WithRetry(inner, 3, WithBaseDelay(time.Millisecond))
	resp, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("expected 'hello', got %q", resp.Content)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryProvider_RetriesOnServerError(t *testing.T) {
	calls := 0
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			calls++
			if calls < 3 {
				return nil, &APIError{StatusCode: 500, Message: "server error", Provider: "test"}
			}
			return &CompletionResponse{Content: "ok"}, nil
		},
	}

	provider := WithRetry(inner, 3, WithBaseDelay(time.Millisecond))
	resp, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("expected 'ok', got %q", resp.Content)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryProvider_DoesNotRetryAuthError(t *testing.T) {
	calls := 0
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			calls++
			return nil, &APIError{StatusCode: 401, Message: "unauthorized", Provider: "test"}
		},
	}

	provider := WithRetry(inner, 3, WithBaseDelay(time.Millisecond))
	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls)
	}
}

func TestRetryProvider_RespectsContextCancellation(t *testing.T) {
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			return nil, &APIError{StatusCode: 429, Message: "rate limited", Provider: "test"}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	provider := WithRetry(inner, 3, WithBaseDelay(time.Second))
	_, err := provider.Complete(ctx, &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRetryProvider_ExhaustsRetries(t *testing.T) {
	calls := 0
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			calls++
			return nil, &APIError{StatusCode: 500, Message: "always fails", Provider: "test"}
		},
	}

	provider := WithRetry(inner, 2, WithBaseDelay(time.Millisecond))
	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 { // 1 initial + 2 retries
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetryProvider_DoesNotRetryNonAPIError(t *testing.T) {
	calls := 0
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			calls++
			return nil, errors.New("network error")
		},
	}

	provider := WithRetry(inner, 3, WithBaseDelay(time.Millisecond))
	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry for non-API errors), got %d", calls)
	}
}

func TestRetryProvider_Stream_Success(t *testing.T) {
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: EventContent, Content: "hi"}
	close(ch)

	inner := &mockProvider{
		streamFunc: func(_ context.Context, _ *CompletionRequest) (<-chan StreamEvent, error) {
			return ch, nil
		},
	}

	provider := WithRetry(inner, 3, WithBaseDelay(time.Millisecond))
	result, err := provider.Stream(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	event := <-result
	if event.Content != "hi" {
		t.Errorf("expected 'hi', got %q", event.Content)
	}
}

func TestRetryProvider_RespectsContextCancellationDuringBackoff(t *testing.T) {
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			return nil, &APIError{StatusCode: 500, Message: "server error", Provider: "test"}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	provider := WithRetry(inner, 3, WithBaseDelay(5*time.Second))

	go func() {
		_, err := provider.Complete(ctx, &CompletionRequest{})
		errCh <- err
	}()

	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("expected error from cancelled context")
		}
	case <-time.After(time.Second):
		t.Fatal("retry did not abort promptly after context cancellation")
	}
}

func TestRetryProvider_Stream_RetriesOnError(t *testing.T) {
	calls := 0
	ch := make(chan StreamEvent, 1)
	ch <- StreamEvent{Type: EventDone}
	close(ch)

	inner := &mockProvider{
		streamFunc: func(_ context.Context, _ *CompletionRequest) (<-chan StreamEvent, error) {
			calls++
			if calls < 2 {
				return nil, &APIError{StatusCode: 500, Message: "fail", Provider: "test"}
			}
			return ch, nil
		},
	}

	provider := WithRetry(inner, 3, WithBaseDelay(time.Millisecond))
	_, err := provider.Stream(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestRetryProvider_HonorsRetryAfter(t *testing.T) {
	var times []time.Time
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			times = append(times, time.Now())
			if len(times) == 1 {
				return nil, &APIError{StatusCode: 429, Provider: "test", RetryAfter: 50 * time.Millisecond}
			}
			return &CompletionResponse{Content: "ok"}, nil
		},
	}

	// Base delay far larger than Retry-After: the header must win.
	provider := WithRetry(inner, 1, WithBaseDelay(time.Hour))
	if _, err := provider.Complete(context.Background(), &CompletionRequest{}); err != nil {
		t.Fatal(err)
	}
	if gap := times[1].Sub(times[0]); gap < 50*time.Millisecond || gap > time.Second {
		t.Errorf("waited %v, want ~50ms", gap)
	}
}

func TestRetryProvider_GivesUpWhenRetryAfterExceedsMaxDelay(t *testing.T) {
	calls := 0
	inner := &mockProvider{
		completeFunc: func(_ context.Context, _ *CompletionRequest) (*CompletionResponse, error) {
			calls++
			return nil, &APIError{StatusCode: 429, Provider: "test", RetryAfter: time.Hour}
		},
	}

	provider := WithRetry(inner, 3, WithMaxDelay(time.Second))
	start := time.Now()
	_, err := provider.Complete(context.Background(), &CompletionRequest{})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		t.Fatalf("err = %v", err)
	}
	if calls != 1 || time.Since(start) > 500*time.Millisecond {
		t.Errorf("calls = %d, elapsed %v: should give up immediately", calls, time.Since(start))
	}
}

func TestRetryProvider_BackoffJitterAndCap(t *testing.T) {
	r := WithRetry(nil, 3, WithBaseDelay(100*time.Millisecond), WithMaxDelay(time.Second))
	for range 50 {
		d := r.backoff(1) // nominal 200ms
		if d < 100*time.Millisecond || d > 200*time.Millisecond {
			t.Fatalf("jittered delay %v outside [100ms, 200ms]", d)
		}
	}
	if d := r.backoff(10); d > time.Second {
		t.Errorf("delay %v exceeds max", d)
	}

	fixed := WithRetry(nil, 3, WithBaseDelay(100*time.Millisecond), WithoutJitter())
	if d := fixed.backoff(2); d != 400*time.Millisecond {
		t.Errorf("deterministic delay = %v, want 400ms", d)
	}
	if d := WithRetry(nil, 3, WithBaseDelay(0)).backoff(3); d != 0 {
		t.Errorf("zero base delay = %v", d)
	}
}
