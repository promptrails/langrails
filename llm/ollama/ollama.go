package ollama

import (
	"context"
	"net/http"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/llm/compat"
)

const defaultBaseURL = "http://localhost:11434/v1/chat/completions"

// Provider implements langrails.Provider for Ollama's OpenAI-compatible API.
type Provider struct{ inner *compat.Provider }

// Option configures the provider.
type Option func(*compat.Config)

// WithBaseURL sets a custom Ollama server URL.
// Default is http://localhost:11434/v1/chat/completions.
func WithBaseURL(url string) Option { return func(c *compat.Config) { c.BaseURL = url } }

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *compat.Config) { c.HTTPClient = client }
}

// New creates a new Ollama provider. The apiKey can be empty for local
// Ollama instances that don't require authentication.
func New(opts ...Option) *Provider {
	cfg := compat.Config{Name: "ollama", BaseURL: defaultBaseURL, APIKey: "ollama"}
	for _, opt := range opts {
		opt(&cfg)
	}
	return &Provider{inner: compat.New(cfg)}
}

// Complete sends a completion request and returns the full response.
func (p *Provider) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	return p.inner.Complete(ctx, req)
}

// Stream sends a completion request and returns a channel of streaming events.
func (p *Provider) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return p.inner.Stream(ctx, req)
}
