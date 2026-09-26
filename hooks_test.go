package langrails

import (
	"context"
	"errors"
	"testing"
	"time"
)

type ctxKey struct{}

func TestHooks_Complete(t *testing.T) {
	var got []string
	var sawCtx bool
	p := WithHooks(&mockProvider{
		completeFunc: func(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
			sawCtx = ctx.Value(ctxKey{}) == "span"
			if req.Model == "bad" {
				return nil, errors.New("boom")
			}
			return &CompletionResponse{Content: "hi"}, nil
		},
	}, Hooks{
		OnRequest: func(ctx context.Context, req *CompletionRequest) context.Context {
			got = append(got, "req:"+req.Model)
			return context.WithValue(ctx, ctxKey{}, "span")
		},
		OnResponse: func(ctx context.Context, _ *CompletionRequest, resp *CompletionResponse, d time.Duration) {
			if ctx.Value(ctxKey{}) != "span" || d < 0 {
				t.Error("OnResponse did not get the OnRequest context")
			}
			got = append(got, "resp:"+resp.Content)
		},
		OnError: func(_ context.Context, _ *CompletionRequest, err error, _ time.Duration) {
			got = append(got, "err:"+err.Error())
		},
	})

	if _, err := p.Complete(context.Background(), &CompletionRequest{Model: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(context.Background(), &CompletionRequest{Model: "bad"}); err == nil {
		t.Fatal("expected error")
	}
	want := []string{"req:ok", "resp:hi", "req:bad", "err:boom"}
	if len(got) != len(want) {
		t.Fatalf("hooks = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hook %d = %q, want %q", i, got[i], want[i])
		}
	}
	if !sawCtx {
		t.Error("provider did not receive the OnRequest context")
	}
}

func streamOf(events ...StreamEvent) *mockProvider {
	return &mockProvider{
		streamFunc: func(context.Context, *CompletionRequest) (<-chan StreamEvent, error) {
			ch := make(chan StreamEvent, len(events))
			for _, ev := range events {
				ch <- ev
			}
			close(ch)
			return ch, nil
		},
	}
}

func TestHooks_StreamAssemblesResponse(t *testing.T) {
	var events int
	var final *CompletionResponse
	p := WithHooks(streamOf(
		StreamEvent{Type: EventReasoning, Reasoning: "think"},
		StreamEvent{Type: EventContent, Content: "Hel"},
		StreamEvent{Type: EventContent, Content: "lo"},
		StreamEvent{Type: EventToolCall, ToolCall: &ToolCall{ID: "1", Name: "t"}},
		StreamEvent{Type: EventCitation, Citation: &Citation{URL: "u"}},
		StreamEvent{Type: EventDone, Usage: &TokenUsage{TotalTokens: 7}},
	), Hooks{
		OnStreamEvent: func(context.Context, *CompletionRequest, StreamEvent) { events++ },
		OnResponse: func(_ context.Context, _ *CompletionRequest, r *CompletionResponse, _ time.Duration) {
			final = r
		},
	})

	ch, err := p.Stream(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var n int
	for range ch {
		n++
	}
	if n != 6 || events != 6 {
		t.Errorf("delivered %d, hooked %d", n, events)
	}
	if final == nil || final.Content != "Hello" || final.Thinking != "think" ||
		len(final.ToolCalls) != 1 || len(final.Citations) != 1 || final.Usage.TotalTokens != 7 {
		t.Errorf("assembled response = %+v", final)
	}
}

func TestHooks_StreamErrors(t *testing.T) {
	var errs []error
	h := Hooks{OnError: func(_ context.Context, _ *CompletionRequest, err error, _ time.Duration) {
		errs = append(errs, err)
	}}

	p := WithHooks(streamOf(StreamEvent{Type: EventError, Error: errors.New("mid")}), h)
	ch, _ := p.Stream(context.Background(), &CompletionRequest{})
	for range ch {
	}

	p = WithHooks(&mockProvider{streamFunc: func(context.Context, *CompletionRequest) (<-chan StreamEvent, error) {
		return nil, errors.New("open")
	}}, h)
	if _, err := p.Stream(context.Background(), &CompletionRequest{}); err == nil {
		t.Error("expected open error")
	}

	if len(errs) != 2 || errs[0].Error() != "mid" || errs[1].Error() != "open" {
		t.Errorf("errors = %v", errs)
	}
}

func TestHooks_StreamCancelledReader(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	failed := make(chan error, 1)
	p := WithHooks(streamOf(
		StreamEvent{Type: EventContent, Content: "a"},
		StreamEvent{Type: EventContent, Content: "b"},
	), Hooks{OnError: func(_ context.Context, _ *CompletionRequest, err error, _ time.Duration) {
		failed <- err
	}})

	ch, _ := p.Stream(ctx, &CompletionRequest{})
	<-ch
	cancel()
	select {
	case err := <-failed:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnError not called after cancellation")
	}
}

func TestHooks_NilHooksAreNoops(t *testing.T) {
	p := WithHooks(streamOf(StreamEvent{Type: EventContent, Content: "x"}), Hooks{
		OnRequest: func(context.Context, *CompletionRequest) context.Context { return nil },
	})
	ch, err := p.Stream(context.Background(), &CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
}
