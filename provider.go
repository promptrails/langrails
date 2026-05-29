package langrails

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

	// TopK limits the number of tokens considered at each step.
	// Supported by Anthropic and Gemini. Ignored by OpenAI-compatible providers.
	TopK *int

	// FrequencyPenalty penalizes tokens based on their frequency in the output so far.
	// Range is typically -2 to 2. Supported by OpenAI-compatible providers.
	FrequencyPenalty *float64

	// PresencePenalty penalizes tokens based on whether they appear in the output so far.
	// Range is typically -2 to 2. Supported by OpenAI-compatible providers.
	PresencePenalty *float64

	// Stop is a list of sequences where the model should stop generating.
	// The model will stop at the first occurrence of any stop sequence.
	Stop []string

	// Seed enables deterministic output when supported by the provider.
	// Same seed + same request = same output (best effort).
	Seed *int

	// Tools defines the functions/tools available for the model to call.
	// Not all providers support tool calling.
	Tools []ToolDefinition

	// OutputSchema is an optional JSON schema for structured output.
	// When set, the provider will attempt to constrain the output to match
	// this schema. Support varies by provider.
	OutputSchema *[]byte

	// Thinking enables extended thinking / chain-of-thought mode.
	// When true, Anthropic returns thinking blocks, and OpenAI uses
	// reasoning effort for o-series models.
	Thinking bool

	// ThinkingBudget limits the number of thinking tokens (Anthropic only).
	// Ignored when Thinking is false.
	ThinkingBudget *int

	// ReasoningEffort selects reasoning intensity in a provider-agnostic way
	// ("minimal", "low", "medium", "high"). Empty means off / provider default.
	// When set, it takes precedence over Thinking/ThinkingBudget. Providers that
	// use a token budget (Anthropic, Gemini) derive one from the effort level.
	ReasoningEffort ReasoningEffort

	// ToolChoice controls whether and which tool the model must call. When nil,
	// the provider default (usually "auto") applies. Ignored by providers that
	// don't support tool choice. When OutputSchema is set, structured output
	// takes precedence over ToolChoice.
	ToolChoice *ToolChoice

	// ServerTools enables provider-hosted tools that the provider executes
	// itself (e.g. web search), as opposed to Tools, which the caller executes.
	// Support varies by provider; unsupported server tools are ignored.
	ServerTools []ServerTool

	// ResponseFormat selects output formatting. Empty means plain text.
	// ResponseFormatJSONObject requests JSON mode without a schema. When
	// OutputSchema is set it takes precedence (schema-constrained JSON).
	ResponseFormat ResponseFormatType

	// CacheControl enables the provider's prompt caching for eligible content
	// (e.g. Anthropic cache_control breakpoints, Bedrock cachePoint). Providers
	// with implicit caching ignore this but still report cached tokens in Usage.
	CacheControl bool
}

// CompletionResponse represents the response from an LLM provider.
type CompletionResponse struct {
	// Content is the generated text content.
	Content string

	// Thinking contains the model's internal reasoning when Thinking mode
	// is enabled. Only populated by providers that support extended thinking
	// (e.g., Anthropic).
	Thinking string

	// ToolCalls contains any tool/function calls the model wants to make.
	// When non-empty, the caller should execute the tools and send the
	// results back in a follow-up request.
	ToolCalls []ToolCall

	// Citations contains sources returned by provider-native web search or
	// grounding. Empty when no server-side search was used.
	Citations []Citation

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

	// CachedTokens is the number of prompt tokens served from cache (read hits).
	// Reported by providers with prompt caching; 0 otherwise.
	CachedTokens int

	// CacheCreationTokens is the number of tokens written to the cache by this
	// request. Reported by Anthropic/Bedrock; 0 otherwise.
	CacheCreationTokens int

	// ReasoningTokens is the number of tokens spent on reasoning/thinking.
	// Most providers count these within CompletionTokens; 0 when unavailable.
	ReasoningTokens int
}
