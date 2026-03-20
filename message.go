package langrails

import "encoding/json"

// Message represents a single message in a conversation.
type Message struct {
	// Role is the role of the message sender.
	// Valid values: "system", "user", "assistant", "tool".
	Role string

	// Content is the text content of the message.
	// For simple text-only messages, set this field.
	Content string

	// ContentParts is an optional list of content parts for multimodal messages.
	// When set, this takes precedence over Content. Use this to send images
	// alongside text. If nil, Content is used as a single text part.
	ContentParts []ContentPart

	// ToolCallID is the ID of the tool call this message is responding to.
	// Only used when Role is "tool".
	ToolCallID string

	// ToolCalls contains tool/function calls made by the assistant.
	// Only present when Role is "assistant" and the model wants to call tools.
	ToolCalls []ToolCall
}

// ContentPart represents a part of a multimodal message.
// A message can contain multiple parts, mixing text and images.
type ContentPart struct {
	// Type is the content type: "text" or "image".
	Type string

	// Text is the text content. Only used when Type is "text".
	Text string

	// ImageURL is the URL of the image. Only used when Type is "image".
	// Can be an HTTP(S) URL or a base64 data URI (data:image/png;base64,...).
	ImageURL string
}

// TextPart creates a text content part.
func TextPart(text string) ContentPart {
	return ContentPart{Type: "text", Text: text}
}

// ImageURLPart creates an image content part from a URL.
func ImageURLPart(url string) ContentPart {
	return ContentPart{Type: "image", ImageURL: url}
}

// ImageBase64Part creates an image content part from base64-encoded data.
// mediaType should be "image/png", "image/jpeg", etc.
func ImageBase64Part(data string, mediaType string) ContentPart {
	return ContentPart{Type: "image", ImageURL: "data:" + mediaType + ";base64," + data}
}

// ToolDefinition describes a tool/function that the model can call.
type ToolDefinition struct {
	// Name is the unique identifier for this tool.
	Name string

	// Description explains what the tool does, helping the model
	// decide when and how to use it.
	Description string

	// Parameters is a JSON schema describing the tool's input parameters.
	Parameters json.RawMessage
}

// ToolCall represents a request from the model to call a specific tool.
type ToolCall struct {
	// ID is a unique identifier for this tool call, used to match
	// tool results back to the original call.
	ID string

	// Name is the name of the tool to call.
	Name string

	// Arguments is a JSON-encoded string of the arguments to pass to the tool.
	Arguments string
}
