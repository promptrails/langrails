package langrails

import "encoding/json"

// Role identifies who sent a message.
type Role string

// Message roles.
const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message represents a single message in a conversation.
type Message struct {
	// Role is the role of the message sender: RoleSystem, RoleUser,
	// RoleAssistant or RoleTool. Untyped string constants such as "user"
	// still work.
	Role Role

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

// Content part types.
const (
	ContentText     = "text"
	ContentImage    = "image"
	ContentAudio    = "audio"
	ContentDocument = "document"
)

// ContentPart represents a part of a multimodal message.
// A message can contain multiple parts, mixing text, images, audio and
// documents. Use the constructors (TextPart, ImageURLPart, AudioPart,
// DocumentPart, ...) rather than filling the fields by hand.
//
// Not every provider accepts every type; a provider rejects a request with
// a part it cannot send with an error wrapping ErrUnsupportedContent,
// before anything is sent.
type ContentPart struct {
	// Type is the content type: ContentText, ContentImage, ContentAudio or
	// ContentDocument.
	Type string

	// Text is the text content. Only used when Type is "text".
	Text string

	// ImageURL is the URL of the image. Only used when Type is "image".
	// Can be an HTTP(S) URL or a base64 data URI (data:image/png;base64,...).
	ImageURL string

	// MediaType is the MIME type of an audio or document part, e.g.
	// "audio/wav", "audio/mpeg", "application/pdf", "text/plain".
	MediaType string

	// Data is the base64-encoded content of an audio or document part.
	Data string

	// URL references a document by URL instead of inline Data.
	URL string

	// Filename optionally names a document part. Some providers show it to
	// the model or require one; a name is generated when it is empty.
	Filename string
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

// AudioPart creates an audio input part from base64-encoded data.
// mediaType is e.g. "audio/wav" or "audio/mpeg".
func AudioPart(data string, mediaType string) ContentPart {
	return ContentPart{Type: ContentAudio, Data: data, MediaType: mediaType}
}

// DocumentPart creates a document input part (for example a PDF) from
// base64-encoded data. mediaType is e.g. "application/pdf"; filename may be
// empty.
func DocumentPart(data, mediaType, filename string) ContentPart {
	return ContentPart{Type: ContentDocument, Data: data, MediaType: mediaType, Filename: filename}
}

// DocumentURLPart creates a document input part the provider fetches from a
// URL. mediaType may be empty when the provider can infer it.
func DocumentURLPart(url, mediaType string) ContentPart {
	return ContentPart{Type: ContentDocument, URL: url, MediaType: mediaType}
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

	// Metadata holds provider-specific data that must be round-tripped through
	// the conversation history. For example, Gemini requires thoughtSignature
	// to be returned with function call parts in subsequent requests.
	Metadata map[string]string
}
