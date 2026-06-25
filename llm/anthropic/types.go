package anthropic

import "encoding/json"

// Request types

type request struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	// System is a plain string normally, or []systemBlock when prompt caching is
	// on — the array form lets us attach a cache_control breakpoint to the system
	// prompt. Typed as any so both shapes marshal under the same JSON field.
	System      any         `json:"system,omitempty"`
	MaxTokens   int         `json:"max_tokens"`
	Temperature *float64    `json:"temperature,omitempty"`
	TopP        *float64    `json:"top_p,omitempty"`
	TopK        *int        `json:"top_k,omitempty"`
	Stop        []string    `json:"stop_sequences,omitempty"`
	Stream      bool        `json:"stream"`
	Tools       []tool      `json:"tools,omitempty"`
	ToolChoice  *toolChoice `json:"tool_choice,omitempty"`
	Thinking    *thinking   `json:"thinking,omitempty"`
}

type thinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type toolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type message struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type         string              `json:"type"`
	Text         string              `json:"text,omitempty"`
	ID           string              `json:"id,omitempty"`
	Name         string              `json:"name,omitempty"`
	Input        json.RawMessage     `json:"input,omitempty"`
	ToolUseID    string              `json:"tool_use_id,omitempty"`
	Content      string              `json:"content,omitempty"`
	Source       *imageSource        `json:"source,omitempty"`
	Citations    []anthropicCitation `json:"citations,omitempty"`
	CacheControl *cacheControl       `json:"cache_control,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`                 // "base64" or "url"
	MediaType string `json:"media_type,omitempty"` // for base64
	Data      string `json:"data,omitempty"`       // base64 payload
	URL       string `json:"url,omitempty"`        // for url
}

type tool struct {
	Type        string          `json:"type,omitempty"` // set for server tools (e.g. web_search_20250305)
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	// Web search server tool options.
	MaxUses        int           `json:"max_uses,omitempty"`
	AllowedDomains []string      `json:"allowed_domains,omitempty"`
	BlockedDomains []string      `json:"blocked_domains,omitempty"`
	UserLocation   *userLocation `json:"user_location,omitempty"`
	// Set on the last tool to cache the whole tool-definition prefix.
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

// systemBlock is the array form of the system prompt, used only when prompt
// caching is on so a cache_control breakpoint can sit on the system prefix.
type systemBlock struct {
	Type         string        `json:"type"` // "text"
	Text         string        `json:"text"`
	CacheControl *cacheControl `json:"cache_control,omitempty"`
}

type userLocation struct {
	Type    string `json:"type"` // "approximate"
	Country string `json:"country,omitempty"`
}

type cacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type anthropicCitation struct {
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	Title     string `json:"title,omitempty"`
	CitedText string `json:"cited_text,omitempty"`
}

// Response types

type response struct {
	ID         string         `json:"id"`
	Model      string         `json:"model"`
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      usage          `json:"usage"`
}

type usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// Error response

type errorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Streaming types

type streamEvent struct {
	Type         string         `json:"type"`
	Index        int            `json:"index,omitempty"`
	ContentBlock *contentBlock  `json:"content_block,omitempty"`
	Delta        *streamDelta   `json:"delta,omitempty"`
	Message      *streamMessage `json:"message,omitempty"`
	Usage        *usage         `json:"usage,omitempty"`
}

type streamDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

type streamMessage struct {
	Usage *usage `json:"usage,omitempty"`
}
