package langrails

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func okProvider() *mockProvider {
	return &mockProvider{
		completeFunc: func(context.Context, *CompletionRequest) (*CompletionResponse, error) {
			return &CompletionResponse{Content: "ok"}, nil
		},
		streamFunc: func(context.Context, *CompletionRequest) (<-chan StreamEvent, error) {
			ch := make(chan StreamEvent)
			close(ch)
			return ch, nil
		},
	}
}

func TestRateLimit_ReserveSchedule(t *testing.T) {
	r := WithRateLimit(okProvider(), 10, 2) // one token per 100ms
	clock := time.Unix(0, 0)
	r.now = func() time.Time { return clock }
	r.last = clock

	// Burst of two is free, the third waits one interval, the fourth two.
	want := []time.Duration{0, 0, 100 * time.Millisecond, 200 * time.Millisecond}
	for i, w := range want {
		if got := r.reserve(); got != w {
			t.Errorf("reserve %d: got %v, want %v", i, got, w)
		}
	}

	// After a long quiet period the bucket refills only up to burst.
	clock = clock.Add(10 * time.Second)
	for i, w := range []time.Duration{0, 0, 100 * time.Millisecond} {
		if got := r.reserve(); got != w {
			t.Errorf("after refill %d: got %v, want %v", i, got, w)
		}
	}
}

func TestRateLimit_BlocksAndPasses(t *testing.T) {
	p := WithRateLimit(okProvider(), 20, 1) // 50ms per request
	start := time.Now()
	for range 3 {
		if _, err := p.Complete(context.Background(), &CompletionRequest{}); err != nil {
			t.Fatal(err)
		}
	}
	if el := time.Since(start); el < 90*time.Millisecond {
		t.Errorf("3 requests at 20/s took %v, want >= ~100ms", el)
	}

	ch, err := p.Stream(context.Background(), &CompletionRequest{})
	if err != nil || ch == nil {
		t.Errorf("stream: %v", err)
	}
}

func TestRateLimit_ContextCancelReturnsToken(t *testing.T) {
	p := WithRateLimit(okProvider(), 1, 1)
	if _, err := p.Complete(context.Background(), &CompletionRequest{}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := p.Complete(ctx, &CompletionRequest{})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
	p.mu.Lock()
	tokens := p.tokens
	p.mu.Unlock()
	if tokens < -0.1 {
		t.Errorf("cancelled wait kept its token: tokens = %v", tokens)
	}
}

func TestRateLimit_Concurrent(t *testing.T) {
	p := WithRateLimit(okProvider(), 100, 5)
	var wg sync.WaitGroup
	start := time.Now()
	for range 15 {
		wg.Go(func() {
			if _, err := p.Complete(context.Background(), &CompletionRequest{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	// 5 free, 10 more at 10ms each.
	if el := time.Since(start); el < 90*time.Millisecond {
		t.Errorf("took %v, want >= ~100ms", el)
	}
}

func TestRateLimit_Defaults(t *testing.T) {
	p := WithRateLimit(okProvider(), 0, 0)
	if p.burst != 1 || p.interval <= 0 {
		t.Errorf("burst = %v, interval = %v", p.burst, p.interval)
	}
}
