package unillm

// EventType represents the type of a streaming event.
type EventType string

const (
	// EventContent indicates a text content chunk.
	EventContent EventType = "content"

	// EventToolCall indicates a tool/function call event.
	EventToolCall EventType = "tool_call"

	// EventDone indicates the stream has completed successfully.
	EventDone EventType = "done"

	// EventError indicates an error occurred during streaming.
	EventError EventType = "error"
)

// StreamEvent represents a single event in a streaming response.
type StreamEvent struct {
	// Type indicates the kind of event.
	Type EventType

	// Content contains the text chunk for EventContent events.
	Content string

	// ToolCall contains tool call data for EventToolCall events.
	ToolCall *ToolCall

	// Error contains error details for EventError events.
	Error error

	// Usage contains token usage data, typically sent with the final event.
	Usage *TokenUsage
}
