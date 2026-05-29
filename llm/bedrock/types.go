package bedrock

import "encoding/json"

// Request types for the Bedrock Converse API.
// The model identifier is carried in the URL path, not the body.

type request struct {
	Messages        []message        `json:"messages"`
	System          []systemBlock    `json:"system,omitempty"`
	InferenceConfig *inferenceConfig `json:"inferenceConfig,omitempty"`
	ToolConfig      *toolConfig      `json:"toolConfig,omitempty"`
}

type systemBlock struct {
	Text string `json:"text"`
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
	ToolUse    *toolUse    `json:"toolUse,omitempty"`
	ToolResult *toolResult `json:"toolResult,omitempty"`
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

type toolEntry struct {
	ToolSpec toolSpec `json:"toolSpec"`
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
	Text    string           `json:"text,omitempty"`
	ToolUse *responseToolUse `json:"toolUse,omitempty"`
}

type responseToolUse struct {
	ToolUseID string          `json:"toolUseId"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
}

type usage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
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
	} `json:"delta"`
	ContentBlockIndex int `json:"contentBlockIndex"`
}

type streamMessageStop struct {
	StopReason string `json:"stopReason"`
}

type streamMetadata struct {
	Usage usage `json:"usage"`
}

// streamException is the payload for exception frames.
type streamException struct {
	Message string `json:"message"`
}
