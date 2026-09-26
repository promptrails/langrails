package langrails

import (
	"context"
	"strings"
	"time"
)

// Hooks observe the requests a provider handles, for logging, metrics and
// tracing. Every field is optional. Hooks run synchronously on the calling
// goroutine (the stream-forwarding goroutine for Stream), so keep them fast.
type Hooks struct {
	// OnRequest runs before each Complete or Stream call. The context it
	// returns is used for the call and passed to the other hooks, so a
	// tracer can start a span here and end it in OnResponse/OnError. Return
	// ctx unchanged when there is nothing to attach.
	OnRequest func(ctx context.Context, req *CompletionRequest) context.Context

	// OnResponse runs after a successful call. For Stream it runs when the
	// stream ends, with a response assembled from the streamed events
	// (content, reasoning, tool calls, citations, usage).
	OnResponse func(ctx context.Context, req *CompletionRequest, resp *CompletionResponse, elapsed time.Duration)

	// OnError runs when a call fails, including a stream that ends with an
	// error event.
	OnError func(ctx context.Context, req *CompletionRequest, err error, elapsed time.Duration)

	// OnStreamEvent runs for every event of a stream, before it is
	// delivered to the caller.
	OnStreamEvent func(ctx context.Context, req *CompletionRequest, ev StreamEvent)
}

// HooksProvider wraps a Provider and reports each call to Hooks.
type HooksProvider struct {
	inner Provider
	hooks Hooks
}

// WithHooks wraps a provider so every call is reported to hooks.
//
// Example:
//
//	provider := langrails.WithHooks(openai.New("sk-..."), langrails.Hooks{
//	    OnResponse: func(ctx context.Context, req *langrails.CompletionRequest, resp *langrails.CompletionResponse, d time.Duration) {
//	        log.Printf("%s: %d tokens in %v", req.Model, resp.Usage.TotalTokens, d)
//	    },
//	})
func WithHooks(provider Provider, hooks Hooks) *HooksProvider {
	return &HooksProvider{inner: provider, hooks: hooks}
}

// Complete calls the wrapped provider and reports the outcome.
func (h *HooksProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	ctx = h.start(ctx, req)
	start := time.Now()
	resp, err := h.inner.Complete(ctx, req)
	if err != nil {
		h.fail(ctx, req, err, time.Since(start))
		return nil, err
	}
	if h.hooks.OnResponse != nil {
		h.hooks.OnResponse(ctx, req, resp, time.Since(start))
	}
	return resp, nil
}

// Stream calls the wrapped provider and reports each event and the outcome.
func (h *HooksProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	ctx = h.start(ctx, req)
	start := time.Now()
	in, err := h.inner.Stream(ctx, req)
	if err != nil {
		h.fail(ctx, req, err, time.Since(start))
		return nil, err
	}

	out := make(chan StreamEvent)
	go func() {
		defer close(out)
		var acc streamAccumulator
		var streamErr error
		for ev := range in {
			if h.hooks.OnStreamEvent != nil {
				h.hooks.OnStreamEvent(ctx, req, ev)
			}
			acc.add(ev)
			if ev.Type == EventError && ev.Error != nil {
				streamErr = ev.Error
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				// The caller stopped reading; drain so the provider's
				// goroutine can finish, but report the cancellation.
				for range in {
				}
				h.fail(ctx, req, ctx.Err(), time.Since(start))
				return
			}
		}
		if streamErr != nil {
			h.fail(ctx, req, streamErr, time.Since(start))
		} else if h.hooks.OnResponse != nil {
			h.hooks.OnResponse(ctx, req, acc.response(), time.Since(start))
		}
	}()
	return out, nil
}

func (h *HooksProvider) start(ctx context.Context, req *CompletionRequest) context.Context {
	if h.hooks.OnRequest != nil {
		if c := h.hooks.OnRequest(ctx, req); c != nil {
			return c
		}
	}
	return ctx
}

func (h *HooksProvider) fail(ctx context.Context, req *CompletionRequest, err error, elapsed time.Duration) {
	if h.hooks.OnError != nil {
		h.hooks.OnError(ctx, req, err, elapsed)
	}
}

// streamAccumulator assembles a CompletionResponse from stream events.
type streamAccumulator struct {
	content   strings.Builder
	reasoning strings.Builder
	resp      CompletionResponse
}

func (a *streamAccumulator) add(ev StreamEvent) {
	switch ev.Type {
	case EventContent:
		a.content.WriteString(ev.Content)
	case EventReasoning:
		a.reasoning.WriteString(ev.Reasoning)
	case EventToolCall:
		if ev.ToolCall != nil {
			a.resp.ToolCalls = append(a.resp.ToolCalls, *ev.ToolCall)
		}
	case EventCitation:
		if ev.Citation != nil {
			a.resp.Citations = append(a.resp.Citations, *ev.Citation)
		}
	}
	if ev.Usage != nil {
		a.resp.Usage = *ev.Usage
	}
}

func (a *streamAccumulator) response() *CompletionResponse {
	r := a.resp
	r.Content = a.content.String()
	r.Thinking = a.reasoning.String()
	return &r
}
