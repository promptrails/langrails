package langrails

import (
	"errors"
	"testing"
)

// EventType constants must hold their documented wire values; providers and
// callers compare against these strings.
func TestEventType_Values(t *testing.T) {
	tests := []struct {
		got  EventType
		want string
	}{
		{EventContent, "content"},
		{EventReasoning, "reasoning"},
		{EventCitation, "citation"},
		{EventToolCall, "tool_call"},
		{EventDone, "done"},
		{EventError, "error"},
	}
	for _, tc := range tests {
		if string(tc.got) != tc.want {
			t.Errorf("EventType = %q, want %q", string(tc.got), tc.want)
		}
	}
}

// A typical stream carries content/reasoning chunks plus typed payloads
// (citation, tool call, usage, error) on the matching event.
func TestStreamEvent_PayloadsByType(t *testing.T) {
	content := StreamEvent{Type: EventContent, Content: "hi"}
	if content.Content != "hi" {
		t.Errorf("Content = %q, want %q", content.Content, "hi")
	}

	reasoning := StreamEvent{Type: EventReasoning, Reasoning: "thinking"}
	if reasoning.Reasoning != "thinking" {
		t.Errorf("Reasoning = %q, want %q", reasoning.Reasoning, "thinking")
	}

	cite := StreamEvent{Type: EventCitation, Citation: &Citation{URL: "https://a.com"}}
	if cite.Citation == nil || cite.Citation.URL != "https://a.com" {
		t.Errorf("Citation = %+v", cite.Citation)
	}

	call := StreamEvent{Type: EventToolCall, ToolCall: &ToolCall{ID: "1", Name: "get_weather"}}
	if call.ToolCall == nil || call.ToolCall.Name != "get_weather" {
		t.Errorf("ToolCall = %+v", call.ToolCall)
	}

	done := StreamEvent{Type: EventDone, Usage: &TokenUsage{TotalTokens: 7}}
	if done.Usage == nil || done.Usage.TotalTokens != 7 {
		t.Errorf("Usage = %+v", done.Usage)
	}

	streamErr := errors.New("boom")
	errEvent := StreamEvent{Type: EventError, Error: streamErr}
	if !errors.Is(errEvent.Error, streamErr) {
		t.Errorf("Error = %v, want %v", errEvent.Error, streamErr)
	}
}
