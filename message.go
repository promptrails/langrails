package unillm

import "encoding/json"

// Message represents a single message in a conversation.
type Message struct {
	// Role is the role of the message sender.
	// Valid values: "system", "user", "assistant", "tool".
	Role string

	// Content is the text content of the message.
	Content string

	// ToolCallID is the ID of the tool call this message is responding to.
	// Only used when Role is "tool".
	ToolCallID string

	// ToolCalls contains tool/function calls made by the assistant.
	// Only present when Role is "assistant" and the model wants to call tools.
	ToolCalls []ToolCall
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
