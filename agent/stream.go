package agent

import (
	"context"

	"github.com/promptrails/langrails"
)

// EventType identifies an agent stream event.
type EventType string

const (
	// EventContent is a chunk of the model's text, as it streams.
	EventContent EventType = "content"

	// EventReasoning is a chunk of the model's reasoning, as it streams.
	EventReasoning EventType = "reasoning"

	// EventToolCall reports a tool call the agent is about to execute.
	EventToolCall EventType = "tool_call"

	// EventToolResult reports a finished tool call: Content holds the
	// result, ToolError the error if it failed.
	EventToolResult EventType = "tool_result"

	// EventDone is the last event of a successful run; Result is set.
	EventDone EventType = "done"

	// EventError is the last event of a failed run; Error is set.
	EventError EventType = "error"
)

// Event is one step of a streaming agent run.
type Event struct {
	Type EventType

	// Iteration is the 1-indexed loop iteration the event belongs to.
	Iteration int

	// Content is the text chunk (EventContent) or the tool's result
	// (EventToolResult).
	Content string

	// Reasoning is the reasoning chunk (EventReasoning).
	Reasoning string

	// ToolCall is the call being made (EventToolCall, EventToolResult).
	ToolCall *langrails.ToolCall

	// ToolError is the tool's error (EventToolResult), sent to the model as
	// its result.
	ToolError error

	// Result is the run's outcome (EventDone).
	Result *Result

	// Error is why the run failed (EventError).
	Error error
}

// Stream runs the agent like Run, reporting progress as it happens: model
// text and reasoning token by token, each tool call and its result, and
// finally EventDone with the Result (or EventError). The channel is closed
// after the final event.
//
//	for ev := range a.Stream(ctx, "What's the weather in Istanbul?") {
//	    switch ev.Type {
//	    case agent.EventContent:
//	        fmt.Print(ev.Content)
//	    case agent.EventToolCall:
//	        fmt.Printf("\n[calling %s]\n", ev.ToolCall.Name)
//	    case agent.EventError:
//	        log.Fatal(ev.Error)
//	    }
//	}
//
// Model calls go through the provider's Stream method and still pass through
// every middleware, which sees the assembled response. Tokens reach the
// caller before AfterModel runs, so a middleware that rewrites output (such
// as PII redaction of responses) cannot retract text already streamed; the
// Result carries the rewritten response. If a middleware answers without
// calling the provider, its content arrives as a single EventContent.
//
// Stop reading early by cancelling ctx; the run then ends at its next
// model or tool call.
func (a *Agent) Stream(ctx context.Context, input string) <-chan Event {
	return a.StreamMessages(ctx, []langrails.Message{{Role: "user", Content: input}})
}

// StreamMessages is Stream with a full message history, like RunMessages.
func (a *Agent) StreamMessages(ctx context.Context, messages []langrails.Message) <-chan Event {
	out := make(chan Event)
	go func() {
		defer close(out)
		emit := func(ev Event) {
			select {
			case out <- ev:
			case <-ctx.Done():
			}
		}
		res, err := a.run(ctx, messages, emit)
		if err != nil {
			emit(Event{Type: EventError, Error: err})
			return
		}
		emit(Event{Type: EventDone, Iteration: res.Iterations, Result: res})
	}()
	return out
}
