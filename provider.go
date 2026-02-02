package unillm

import "context"

// Provider is the unified interface that all LLM providers must implement.
// It provides both synchronous and streaming completion methods.
type Provider interface {
	// Complete sends a completion request and blocks until the full response
	// is received. It returns the complete response or an error.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)

	// Stream sends a completion request and returns a channel that emits
	// events as they arrive. The channel is closed when the response is
	// complete or an error occurs. Callers should range over the channel
	// and check each event's Type field.
	Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamEvent, error)
}

// CompletionRequest represents a request to an LLM provider.
type CompletionRequest struct {
	// Model is the provider-specific model identifier (e.g., "gpt-4o", "claude-sonnet-4-20250514").
	Model string

	// Messages is the conversation history to send to the model.
	Messages []Message

	// SystemPrompt is an optional system instruction. Some providers handle
	// this as a separate field rather than a system message.
	SystemPrompt string

	// Temperature controls randomness in the output. Range is typically 0-2.
	// nil means the provider's default is used.
	Temperature *float64

	// MaxTokens limits the maximum number of tokens in the response.
	// nil means the provider's default is used.
	MaxTokens *int

	// TopP controls nucleus sampling. Range is typically 0-1.
	// nil means the provider's default is used.
	TopP *float64

	// Tools defines the functions/tools available for the model to call.
	// Not all providers support tool calling.
	Tools []ToolDefinition

	// OutputSchema is an optional JSON schema for structured output.
	// When set, the provider will attempt to constrain the output to match
	// this schema. Support varies by provider.
	OutputSchema *[]byte
}

// CompletionResponse represents the response from an LLM provider.
type CompletionResponse struct {
	// Content is the generated text content.
	Content string

	// ToolCalls contains any tool/function calls the model wants to make.
	// When non-empty, the caller should execute the tools and send the
	// results back in a follow-up request.
	ToolCalls []ToolCall

	// Usage contains token usage statistics for this request.
	Usage TokenUsage

	// FinishReason indicates why the model stopped generating.
	// Common values: "stop", "tool_calls", "length", "content_filter".
	FinishReason string

	// Model is the actual model that was used (may differ from the requested model).
	Model string
}

// TokenUsage contains token consumption statistics for a completion request.
type TokenUsage struct {
	// PromptTokens is the number of tokens in the input/prompt.
	PromptTokens int

	// CompletionTokens is the number of tokens in the generated output.
	CompletionTokens int

	// TotalTokens is the sum of PromptTokens and CompletionTokens.
	TotalTokens int
}
