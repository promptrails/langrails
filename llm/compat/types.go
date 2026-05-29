package compat

import "encoding/json"

// Request types

type request struct {
	Model            string            `json:"model"`
	Messages         []message         `json:"messages"`
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxTokens        *int              `json:"max_tokens,omitempty"`
	TopP             *float64          `json:"top_p,omitempty"`
	FrequencyPenalty *float64          `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64          `json:"presence_penalty,omitempty"`
	Stop             []string          `json:"stop,omitempty"`
	Seed             *int              `json:"seed,omitempty"`
	Stream           bool              `json:"stream"`
	Tools            []tool            `json:"tools,omitempty"`
	ToolChoice       interface{}       `json:"tool_choice,omitempty"` // string or toolChoiceFunction
	ResponseFormat   *responseFormat   `json:"response_format,omitempty"`
	Reasoning        *reasoningParam   `json:"reasoning,omitempty"`
	WebSearchOptions *webSearchOptions `json:"web_search_options,omitempty"`
}

// webSearchOptions enables OpenAI-style built-in web search. The empty form
// uses provider defaults; SearchContextSize is optional ("low"/"medium"/"high").
type webSearchOptions struct {
	SearchContextSize string `json:"search_context_size,omitempty"`
}

// toolChoiceFunction is the object form of tool_choice that forces a named tool.
type toolChoiceFunction struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function"`
}

type reasoningParam struct {
	Effort string `json:"effort"`
}

type message struct {
	Role       string      `json:"role"`
	Content    interface{} `json:"content"` // string or []contentPart
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall  `json:"tool_calls,omitempty"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type tool struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type toolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function functionCall `json:"function"`
}

type functionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type responseFormat struct {
	Type       string           `json:"type"`
	JSONSchema *jsonSchemaParam `json:"json_schema,omitempty"`
}

type jsonSchemaParam struct {
	Name   string          `json:"name"`
	Schema json.RawMessage `json:"schema"`
	Strict bool            `json:"strict"`
}

// Response types

type response struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   usage    `json:"usage"`
	// Citations is the Perplexity-style top-level list of source URLs.
	Citations []string `json:"citations,omitempty"`
}

type choice struct {
	Message      choiceMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type choiceMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []toolCall `json:"tool_calls,omitempty"`
	// Reasoning text: providers diverge on the field name (DeepSeek/most use
	// reasoning_content; OpenRouter uses reasoning).
	ReasoningContent string       `json:"reasoning_content,omitempty"`
	Reasoning        string       `json:"reasoning,omitempty"`
	Annotations      []annotation `json:"annotations,omitempty"` // OpenAI url_citation annotations
}

type annotation struct {
	Type        string `json:"type"`
	URLCitation *struct {
		URL        string `json:"url"`
		Title      string `json:"title"`
		StartIndex int    `json:"start_index"`
		EndIndex   int    `json:"end_index"`
	} `json:"url_citation,omitempty"`
}

type usage struct {
	PromptTokens            int                      `json:"prompt_tokens"`
	CompletionTokens        int                      `json:"completion_tokens"`
	TotalTokens             int                      `json:"total_tokens"`
	PromptTokensDetails     *promptTokensDetails     `json:"prompt_tokens_details,omitempty"`
	CompletionTokensDetails *completionTokensDetails `json:"completion_tokens_details,omitempty"`
}

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type completionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// Error response

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Streaming types

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
	Usage   *usage         `json:"usage,omitempty"`
}

type streamChoice struct {
	Delta        streamDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type streamDelta struct {
	Content          string           `json:"content"`
	ToolCalls        []streamToolCall `json:"tool_calls,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	Reasoning        string           `json:"reasoning,omitempty"`
}

type streamToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Function functionCall `json:"function"`
}
