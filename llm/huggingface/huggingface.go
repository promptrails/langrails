package huggingface

import (
	"context"
	"net/http"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/llm/compat"
)

const defaultBaseURL = "https://router.huggingface.co/v1/chat/completions"

// Provider implements langrails.Provider for the Hugging Face Inference Providers
// router. The router proxies chat-completion traffic to a network of partner
// providers (Cerebras, SambaNova, Together, Fireworks, ...) selected
// automatically or via a per-model suffix.
type Provider struct{ inner *compat.Provider }

// Option configures the provider.
type Option func(*compat.Config)

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) Option { return func(c *compat.Config) { c.BaseURL = url } }

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *compat.Config) { c.HTTPClient = client }
}

// New creates a new Hugging Face Router provider.
func New(apiKey string, opts ...Option) *Provider {
	cfg := compat.Config{Name: "huggingface", BaseURL: defaultBaseURL, APIKey: apiKey}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{inner: compat.New(cfg)}
}

func (p *Provider) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	return p.inner.Complete(ctx, req)
}

func (p *Provider) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return p.inner.Stream(ctx, req)
}
