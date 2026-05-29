package langrails

// EventType represents the type of a streaming event.
type EventType string

const (
	// EventContent indicates a text content chunk.
	EventContent EventType = "content"

	// EventReasoning indicates a reasoning/thinking text chunk. Emitted before
	// the corresponding EventContent chunks by providers that stream reasoning.
	EventReasoning EventType = "reasoning"

	// EventCitation indicates a citation/source was emitted during streaming.
	EventCitation EventType = "citation"

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

	// Reasoning contains the reasoning/thinking text chunk for EventReasoning events.
	Reasoning string

	// Citation contains the source for EventCitation events.
	Citation *Citation

	// ToolCall contains tool call data for EventToolCall events.
	ToolCall *ToolCall

	// Error contains error details for EventError events.
	Error error

	// Usage contains token usage data, typically sent with the final event.
	Usage *TokenUsage
}
