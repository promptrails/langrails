package friendli

import (
	"context"
	"net/http"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/llm/compat"
)

const defaultBaseURL = "https://api.friendli.ai/serverless/v1/chat/completions"

// Provider implements langrails.Provider for Friendli AI's API.
type Provider struct{ inner *compat.Provider }

// Option configures the provider.
type Option func(*compat.Config)

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) Option { return func(c *compat.Config) { c.BaseURL = url } }

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *compat.Config) { c.HTTPClient = client }
}

// New creates a new Friendli AI provider.
func New(apiKey string, opts ...Option) *Provider {
	cfg := compat.Config{Name: "friendli", BaseURL: defaultBaseURL, APIKey: apiKey}
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
