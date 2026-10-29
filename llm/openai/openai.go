package openai

import (
	"context"
	"net/http"
	"strings"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/llm/compat"
)

const (
	defaultBaseURL = "https://api.openai.com/v1/chat/completions"

	chatPath      = "/chat/completions"
	responsesPath = "/responses"
)

// Provider implements langrails.Provider for OpenAI's API.
type Provider struct {
	inner     *compat.Provider
	responses *responsesClient
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

// WithResponsesAPI sends requests to the Responses API (/v1/responses)
// instead of Chat Completions. It brings reasoning summaries (Thinking),
// keeps a reasoning model's encrypted reasoning across tool calls, and
// offers web search on every model that supports it.
//
// It rewrites a base URL ending in /chat/completions, so put it after
// WithBaseURL, or give WithBaseURL a URL ending in /responses, which
// selects the Responses API by itself.
func WithResponsesAPI() Option {
	return func(c *compat.Config) {
		if base, ok := strings.CutSuffix(c.BaseURL, chatPath); ok {
			c.BaseURL = base + responsesPath
		}
	}
}

// New creates a new OpenAI provider with the given API key and options.
func New(apiKey string, opts ...Option) *Provider {
	cfg := compat.Config{
		Name:    "openai",
		BaseURL: defaultBaseURL,
		APIKey:  apiKey,
		// OpenAI documents a top-level reasoning_effort string and names that
		// parameter in its own errors; the {"reasoning":{...}} object is the
		// OpenRouter form and OpenAI does not read it.
		ReasoningStyle: compat.ReasoningStyleEffortField,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if strings.HasSuffix(cfg.BaseURL, responsesPath) {
		return &Provider{responses: newResponsesClient(cfg)}
	}
	return &Provider{inner: compat.New(cfg)}
}

// Complete sends a completion request and returns the full response.
func (p *Provider) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	if p.responses != nil {
		return p.responses.Complete(ctx, req)
	}
	return p.inner.Complete(ctx, req)
}

// Stream sends a completion request and returns a channel of streaming events.
func (p *Provider) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	if p.responses != nil {
		return p.responses.Stream(ctx, req)
	}
	return p.inner.Stream(ctx, req)
}
