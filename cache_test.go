package langrails

import (
	"context"
	"errors"
	"testing"
	"time"
)

func countingProvider(calls *int) *mockProvider {
	return &mockProvider{
		completeFunc: func(_ context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			*calls++
			if req.Model == "fail" {
				return nil, errors.New("boom")
			}
			return &CompletionResponse{Content: req.Model, ToolCalls: []ToolCall{{ID: "1"}}}, nil
		},
		streamFunc: func(context.Context, *CompletionRequest) (<-chan StreamEvent, error) {
			*calls++
			return nil, nil
		},
	}
}

func TestCacheProvider_HitsAndMisses(t *testing.T) {
	var calls int
	p := WithCache(countingProvider(&calls), NewMemoryCache(10, 0))
	ctx := context.Background()

	r1, _ := p.Complete(ctx, &CompletionRequest{Model: "a"})
	r2, _ := p.Complete(ctx, &CompletionRequest{Model: "a"})
	if calls != 1 || r2.Content != "a" {
		t.Fatalf("calls = %d, content = %q", calls, r2.Content)
	}

	// Mutating a returned response must not poison the cache.
	r1.Content = "changed"
	r2.ToolCalls[0].ID = "changed"
	r3, _ := p.Complete(ctx, &CompletionRequest{Model: "a"})
	if r3.Content != "a" || r3.ToolCalls[0].ID != "1" {
		t.Errorf("cache was mutated through a returned response: %+v", r3)
	}

	temp := 0.5
	if _, _ = p.Complete(ctx, &CompletionRequest{Model: "a", Temperature: &temp}); calls != 2 {
		t.Errorf("different request should miss, calls = %d", calls)
	}

	if _, err := p.Complete(ctx, &CompletionRequest{Model: "fail"}); err == nil {
		t.Error("expected error")
	}
	if _, _ = p.Complete(ctx, &CompletionRequest{Model: "fail"}); calls != 4 {
		t.Errorf("errors must not be cached, calls = %d", calls)
	}

	_, _ = p.Stream(ctx, &CompletionRequest{Model: "a"})
	if calls != 5 {
		t.Errorf("stream should pass through, calls = %d", calls)
	}
}

type brokenCache struct{}

func (brokenCache) Get(context.Context, string) (*CompletionResponse, bool, error) {
	return nil, false, errors.New("down")
}

func (brokenCache) Set(context.Context, string, *CompletionResponse) error { return errors.New("down") }

func TestCacheProvider_CacheErrorsAreMisses(t *testing.T) {
	var calls int
	p := WithCache(countingProvider(&calls), brokenCache{})
	resp, err := p.Complete(context.Background(), &CompletionRequest{Model: "a"})
	if err != nil || resp.Content != "a" {
		t.Fatalf("resp = %+v, err = %v", resp, err)
	}
}

func TestMemoryCache_LRUAndTTL(t *testing.T) {
	ctx := context.Background()
	c := NewMemoryCache(2, time.Minute)
	clock := time.Unix(0, 0)
	c.now = func() time.Time { return clock }

	_ = c.Set(ctx, "a", &CompletionResponse{Content: "a"})
	_ = c.Set(ctx, "b", &CompletionResponse{Content: "b"})
	_, _, _ = c.Get(ctx, "a") // a is now most recent
	_ = c.Set(ctx, "c", &CompletionResponse{Content: "c"})

	if _, ok, _ := c.Get(ctx, "b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok, _ := c.Get(ctx, "a"); !ok {
		t.Error("a should still be cached")
	}
	if c.Len() != 2 {
		t.Errorf("len = %d", c.Len())
	}

	_ = c.Set(ctx, "a", &CompletionResponse{Content: "a2"})
	if r, _, _ := c.Get(ctx, "a"); r.Content != "a2" {
		t.Errorf("overwrite: %q", r.Content)
	}

	clock = clock.Add(2 * time.Minute)
	if _, ok, _ := c.Get(ctx, "a"); ok {
		t.Error("a should have expired")
	}
	if c.Len() != 1 {
		t.Errorf("expired entry not removed, len = %d", c.Len())
	}
}

func TestCacheKey(t *testing.T) {
	k1, _ := CacheKey(&CompletionRequest{Model: "a", Messages: []Message{{Role: "user", Content: "hi"}}})
	k2, _ := CacheKey(&CompletionRequest{Model: "a", Messages: []Message{{Role: "user", Content: "hi"}}})
	k3, _ := CacheKey(&CompletionRequest{Model: "a", Messages: []Message{{Role: "user", Content: "hey"}}})
	if k1 != k2 || k1 == k3 || len(k1) != 64 {
		t.Errorf("keys: %s %s %s", k1, k2, k3)
	}
}
