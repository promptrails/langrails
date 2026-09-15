package langrails

// ReasoningEffort selects how much effort the model spends on reasoning, in a
// provider-agnostic way. Providers that expose an effort level (OpenAI) use it
// directly; providers that use a token budget (Anthropic, Gemini, Bedrock)
// derive a budget from it.
type ReasoningEffort string

const (
	// ReasoningOff leaves reasoning unset (the zero value): the provider is
	// told nothing and applies its own default.
	ReasoningOff ReasoningEffort = ""
	// ReasoningNone explicitly turns reasoning OFF, which is not the same as
	// leaving it unset. Models that reason by default need to be told, and
	// OpenAI's chat/completions rejects function tools on such a model unless
	// the effort is explicitly "none". Providers with no way to express this
	// treat it as unset.
	ReasoningNone ReasoningEffort = "none"
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

// Requested reports whether the caller asked for reasoning to be ON. Both the
// unset zero value and an explicit "none" return false, so a provider that
// enables thinking on any non-empty effort does not switch it on for a value
// that means the opposite.
func (e ReasoningEffort) Requested() bool {
	return e != ReasoningOff && e != ReasoningNone
}

// BudgetTokens maps a reasoning effort level to an approximate thinking-token
// budget, for providers that take a token budget rather than an effort level.
// It returns 0 for ReasoningOff and ReasoningNone.
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
