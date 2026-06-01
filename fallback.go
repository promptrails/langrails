package langrails

import "context"

// FallbackProvider wraps two providers, trying the primary first and
// falling back to the secondary if the primary returns an error.
//
// Fallback providers can be chained to create a priority list:
//
//	provider := langrails.WithFallback(
//		openai.New("sk-..."),
//		anthropic.New("sk-ant-..."),
//	)
type FallbackProvider struct {
	primary  Provider
	fallback Provider
}

// WithFallback wraps two providers in a fallback chain. If the primary
// provider returns an error, the fallback provider is tried.
func WithFallback(primary, fallback Provider) *FallbackProvider {
	return &FallbackProvider{
		primary:  primary,
		fallback: fallback,
	}
}

// Complete tries the primary provider first, then falls back to the
// secondary provider on any error. If the context is cancelled between
// the primary failure and the fallback attempt, the cancelled error is
// returned immediately.
func (f *FallbackProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	resp, err := f.primary.Complete(ctx, req)
	if err == nil {
		return resp, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return f.fallback.Complete(ctx, req)
}

// Stream tries the primary provider first, then falls back to the
// secondary provider on any error. If the context is cancelled between
// the primary failure and the fallback attempt, the cancelled error is
// returned immediately.
func (f *FallbackProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	ch, err := f.primary.Stream(ctx, req)
	if err == nil {
		return ch, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return f.fallback.Stream(ctx, req)
}
