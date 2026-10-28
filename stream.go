package langrails

import "strings"

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

	// EventToolCallDelta indicates a fragment of a tool call while the model
	// is still writing it. The complete call follows as an EventToolCall, so
	// consumers that only need finished calls can ignore these.
	EventToolCallDelta EventType = "tool_call_delta"

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

	// ToolCallDelta contains the fragment for EventToolCallDelta events.
	ToolCallDelta *ToolCallDelta

	// Error contains error details for EventError events.
	Error error

	// Usage contains token usage data, typically sent with the final event.
	Usage *TokenUsage
}

// ToolCallDelta is a fragment of a tool call streamed while the model
// writes it, for showing a call (say, the file an agent is about to write)
// before it is complete.
type ToolCallDelta struct {
	// Index is the call's position among the response's tool calls,
	// starting at 0. Fragments with the same Index belong to the same call.
	Index int

	// ID and Name are set on a call's first fragment (and, for some
	// providers, repeated on later ones).
	ID   string
	Name string

	// Arguments is the next piece of the call's JSON arguments. Joined in
	// order, the pieces of one call form its complete arguments.
	Arguments string
}

// StreamAccumulator assembles a CompletionResponse from stream events, for
// code that forwards a stream and also needs the complete response. Add
// each event in order, then call Response. The zero value is ready to use.
type StreamAccumulator struct {
	content   strings.Builder
	reasoning strings.Builder
	resp      CompletionResponse
	err       error
}

// Add records one event.
func (a *StreamAccumulator) Add(ev StreamEvent) {
	switch ev.Type {
	case EventContent:
		a.content.WriteString(ev.Content)
	case EventReasoning:
		a.reasoning.WriteString(ev.Reasoning)
	case EventToolCall:
		if ev.ToolCall != nil {
			a.resp.ToolCalls = append(a.resp.ToolCalls, *ev.ToolCall)
		}
	case EventCitation:
		if ev.Citation != nil {
			a.resp.Citations = append(a.resp.Citations, *ev.Citation)
		}
	case EventError:
		if ev.Error != nil && a.err == nil {
			a.err = ev.Error
		}
	}
	if ev.Usage != nil {
		a.resp.Usage = *ev.Usage
	}
}

// Response returns the response assembled so far. FinishReason is
// "tool_calls" when the model called tools and "stop" otherwise, since
// streams do not carry the provider's own finish reason.
func (a *StreamAccumulator) Response() *CompletionResponse {
	r := a.resp
	r.Content = a.content.String()
	r.Thinking = a.reasoning.String()
	if len(r.ToolCalls) > 0 {
		r.FinishReason = "tool_calls"
	} else {
		r.FinishReason = "stop"
	}
	return &r
}

// Err returns the first error event's error, or nil.
func (a *StreamAccumulator) Err() error { return a.err }

// CollectStream reads a stream to the end and returns the assembled
// response, or the stream's error.
func CollectStream(events <-chan StreamEvent) (*CompletionResponse, error) {
	var acc StreamAccumulator
	for ev := range events {
		acc.Add(ev)
	}
	if acc.Err() != nil {
		return nil, acc.Err()
	}
	return acc.Response(), nil
}
