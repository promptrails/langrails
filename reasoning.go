package langrails

// ReasoningEffort selects how much effort the model spends on reasoning, in a
// provider-agnostic way. Providers that expose an effort level (OpenAI) use it
// directly; providers that use a token budget (Anthropic, Gemini, Bedrock)
// derive a budget from it.
type ReasoningEffort string

const (
	// ReasoningOff disables reasoning (the zero value).
	ReasoningOff ReasoningEffort = ""
	// ReasoningMinimal requests the least reasoning effort.
	ReasoningMinimal ReasoningEffort = "minimal"
	// ReasoningLow requests low reasoning effort.
	ReasoningLow ReasoningEffort = "low"
	// ReasoningMedium requests medium reasoning effort.
	ReasoningMedium ReasoningEffort = "medium"
	// ReasoningHigh requests high reasoning effort.
	ReasoningHigh ReasoningEffort = "high"
)

// ResponseFormatType selects how the model formats its output.
type ResponseFormatType string

const (
	// ResponseFormatText is plain text output (the zero value / default).
	ResponseFormatText ResponseFormatType = ""
	// ResponseFormatJSONObject requests valid JSON output without a schema.
	// When CompletionRequest.OutputSchema is also set, the schema takes
	// precedence (schema-constrained JSON).
	ResponseFormatJSONObject ResponseFormatType = "json_object"
)

// BudgetTokens maps a reasoning effort level to an approximate thinking-token
// budget, for providers that take a token budget rather than an effort level.
// It returns 0 for ReasoningOff.
func (e ReasoningEffort) BudgetTokens() int {
	switch e {
	case ReasoningMinimal:
		return 1024
	case ReasoningLow:
		return 4096
	case ReasoningMedium:
		return 8192
	case ReasoningHigh:
		return 16384
	default:
		return 0
	}
}
