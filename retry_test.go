package llmrails

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
