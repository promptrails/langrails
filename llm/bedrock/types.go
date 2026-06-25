package bedrock

import "encoding/json"

// Request types for the Bedrock Converse API.
// The model identifier is carried in the URL path, not the body.

type request struct {
	Messages                     []message        `json:"messages"`
	System                       []systemBlock    `json:"system,omitempty"`
	InferenceConfig              *inferenceConfig `json:"inferenceConfig,omitempty"`
	ToolConfig                   *toolConfig      `json:"toolConfig,omitempty"`
	AdditionalModelRequestFields json.RawMessage  `json:"additionalModelRequestFields,omitempty"`
}

// systemBlock is a Converse SystemContentBlock — exactly one of text or
// cachePoint. The cachePoint form (text omitted) marks a cache breakpoint on the
// system prefix when prompt caching is on.
type systemBlock struct {
	Text       string      `json:"text,omitempty"`
	CachePoint *cachePoint `json:"cachePoint,omitempty"`
}

type inferenceConfig struct {
	MaxTokens     *int     `json:"maxTokens,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"topP,omitempty"`
	StopSequences []string `json:"stopSequences,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock is a Converse content block. Exactly one field is set per block.
type contentBlock struct {
	Text       string      `json:"text,omitempty"`
	Image      *imageBlock `json:"image,omitempty"`
	ToolUse    *toolUse    `json:"toolUse,omitempty"`
	ToolResult *toolResult `json:"toolResult,omitempty"`
	CachePoint *cachePoint `json:"cachePoint,omitempty"`
}

type cachePoint struct {
	Type string `json:"type"` // "default"
}

type imageBlock struct {
	Format string           `json:"format"` // png, jpeg, gif, webp
	Source imageSourceBytes `json:"source"`
}

type imageSourceBytes struct {
	Bytes string `json:"bytes"` // base64-encoded image bytes
}

type toolUse struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

type toolResult struct {
	ToolUseID string                  `json:"toolUseId"`
	Content   []toolResultContentItem `json:"content"`
	Status    string                  `json:"status,omitempty"`
}

type toolResultContentItem struct {
	Text string `json:"text,omitempty"`
}

type toolConfig struct {
	Tools      []toolEntry `json:"tools"`
	ToolChoice *toolChoice `json:"toolChoice,omitempty"`
}

// toolEntry is a Converse tool list item — a toolSpec, or a cachePoint that
// marks a cache breakpoint on the tool-definition prefix.
type toolEntry struct {
	ToolSpec   *toolSpec   `json:"toolSpec,omitempty"`
	CachePoint *cachePoint `json:"cachePoint,omitempty"`
}

type toolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema toolInputSchema `json:"inputSchema"`
}

type toolInputSchema struct {
	JSON json.RawMessage `json:"json"`
}

type toolChoice struct {
	Auto *struct{}       `json:"auto,omitempty"`
	Any  *struct{}       `json:"any,omitempty"`
	Tool *toolChoiceName `json:"tool,omitempty"`
}

type toolChoiceName struct {
	Name string `json:"name"`
}

// Response types for the non-streaming Converse API.

type response struct {
	Output     responseOutput `json:"output"`
	StopReason string         `json:"stopReason"`
	Usage      usage          `json:"usage"`
}

type responseOutput struct {
	Message responseMessage `json:"message"`
}

type responseMessage struct {
	Role    string                 `json:"role"`
	Content []responseContentBlock `json:"content"`
}

type responseContentBlock struct {
	Text             string                 `json:"text,omitempty"`
	ToolUse          *responseToolUse       `json:"toolUse,omitempty"`
	ReasoningContent *reasoningContentBlock `json:"reasoningContent,omitempty"`
}

type reasoningContentBlock struct {
	ReasoningText *struct {
		Text      string `json:"text"`
		Signature string `json:"signature,omitempty"`
	} `json:"reasoningText,omitempty"`
}

type responseToolUse struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

type usage struct {
	InputTokens           int `json:"inputTokens"`
	OutputTokens          int `json:"outputTokens"`
	TotalTokens           int `json:"totalTokens"`
	CacheReadInputTokens  int `json:"cacheReadInputTokens,omitempty"`
	CacheWriteInputTokens int `json:"cacheWriteInputTokens,omitempty"`
}

// Streaming event payload types (converse-stream). Each event-stream frame's
// payload is one of these, distinguished by the ":event-type" header.

type streamContentBlockStart struct {
	Start struct {
		ToolUse *struct {
			ToolUseID string `json:"toolUseId"`
			Name      string `json:"name"`
		} `json:"toolUse,omitempty"`
	} `json:"start"`
	ContentBlockIndex int `json:"contentBlockIndex"`
}

type streamContentBlockDelta struct {
	Delta struct {
		Text    string `json:"text,omitempty"`
		ToolUse *struct {
			Input string `json:"input"`
		} `json:"toolUse,omitempty"`
		ReasoningContent *struct {
			Text string `json:"text"`
		} `json:"reasoningContent,omitempty"`
	} `json:"delta"`
	ContentBlockIndex int `json:"contentBlockIndex"`
}

type streamMetadata struct {
	Usage usage `json:"usage"`
}

// streamException is the payload for exception frames.
type streamException struct {
	Message string `json:"message"`
}
