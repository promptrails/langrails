package agent

import (
	"context"

	"github.com/promptrails/langrails"
)

// CallFunc performs a single model call. Middleware wraps it with
// WrapModelCall to add retries, timing, caching, or request/response
// rewriting around the underlying provider call.
type CallFunc func(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error)

// State is the mutable per-iteration context passed to middleware hooks.
// BeforeModel hooks may mutate Request in place (for example to trim or
// redact messages); AfterModel hooks may inspect or mutate Response and
// may call Stop to end the loop after this iteration.
type State struct {
	// Request is the request about to be sent (BeforeModel) or that was
	// just sent (AfterModel). Mutate it in place to change what the model
	// sees.
	Request *langrails.CompletionRequest

	// Response is the model's response. It is nil in BeforeModel and set in
	// AfterModel.
	Response *langrails.CompletionResponse

	// Iteration is the 1-indexed agent loop iteration.
	Iteration int

	stop bool
}

// Stop requests that the agent loop end after the current iteration,
// returning the current response without executing further tool calls.
func (s *State) Stop() { s.stop = true }

// Stopped reports whether Stop has been called.
func (s *State) Stopped() bool { return s.stop }

// Middleware intercepts the agent loop at key points. BeforeModel hooks
// run in registration order before each model call; AfterModel hooks run
// in reverse registration order after each model call; WrapModelCall
// composes around the model call with the first registered middleware
// outermost.
//
// Embed BaseMiddleware to implement only the hooks you need.
type Middleware interface {
	BeforeModel(ctx context.Context, state *State) error
	AfterModel(ctx context.Context, state *State) error
	WrapModelCall(next CallFunc) CallFunc
}

// BaseMiddleware provides no-op implementations of every Middleware hook.
// Embed it in a custom middleware and override only the hooks you need.
type BaseMiddleware struct{}

// BeforeModel is a no-op.
func (BaseMiddleware) BeforeModel(context.Context, *State) error { return nil }

// AfterModel is a no-op.
func (BaseMiddleware) AfterModel(context.Context, *State) error { return nil }

// WrapModelCall returns next unchanged.
func (BaseMiddleware) WrapModelCall(next CallFunc) CallFunc { return next }
