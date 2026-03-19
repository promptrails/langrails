package openai

import (
	"context"
	"net/http"

	"github.com/promptrails/llmrails"
	"github.com/promptrails/llmrails/compat"
)

const (
	defaultBaseURL = "https://api.openai.com/v1/chat/completions"
)

// Provider implements llmrails.Provider for OpenAI's API.
type Provider struct {
	inner *compat.Provider
}

// Option configures the OpenAI provider.
type Option func(*compat.Config)

// WithBaseURL sets a custom base URL (useful for Azure OpenAI or proxies).
func WithBaseURL(url string) Option {
	return func(c *compat.Config) {
		c.BaseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *compat.Config) {
		c.HTTPClient = client
	}
}

// New creates a new OpenAI provider with the given API key and options.
func New(apiKey string, opts ...Option) *Provider {
	cfg := compat.Config{
		Name:    "openai",
		BaseURL: defaultBaseURL,
		APIKey:  apiKey,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{inner: compat.New(cfg)}
}

// Complete sends a completion request and returns the full response.
func (p *Provider) Complete(ctx context.Context, req *llmrails.CompletionRequest) (*llmrails.CompletionResponse, error) {
	return p.inner.Complete(ctx, req)
}

// Stream sends a completion request and returns a channel of streaming events.
func (p *Provider) Stream(ctx context.Context, req *llmrails.CompletionRequest) (<-chan llmrails.StreamEvent, error) {
	return p.inner.Stream(ctx, req)
}
