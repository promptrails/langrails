package compat

import (
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertMessages(t *testing.T) {
	req := &langrails.CompletionRequest{
		SystemPrompt: "be brief",
		Messages: []langrails.Message{
			{
				Role: "user",
				ContentParts: []langrails.ContentPart{
					langrails.TextPart("what is this?"),
					langrails.ImageURLPart("https://example.com/cat.png"),
				},
			},
			{
				Role:    "assistant",
				Content: "let me check",
				ToolCalls: []langrails.ToolCall{
					{ID: "c1", Name: "get_weather", Arguments: `{"city":"Paris"}`},
				},
			},
			{Role: "tool", ToolCallID: "c1", Content: `{"temp":20}`},
		},
	}

	msgs := convertMessages(req)

	// system prompt becomes the first message
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (system + 3), got %d", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[0].Content != "be brief" {
		t.Errorf("msgs[0] = %+v, want system 'be brief'", msgs[0])
	}

	// multimodal user message → []contentPart with text + image_url
	parts, ok := msgs[1].Content.([]contentPart)
	if !ok {
		t.Fatalf("multimodal Content type = %T, want []contentPart", msgs[1].Content)
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "what is this?" {
		t.Errorf("parts[0] = %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil || parts[1].ImageURL.URL != "https://example.com/cat.png" {
		t.Errorf("parts[1] = %+v", parts[1])
	}

	// assistant tool call
	if len(msgs[2].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(msgs[2].ToolCalls))
	}
	tc := msgs[2].ToolCalls[0]
	if tc.ID != "c1" || tc.Type != "function" || tc.Function.Name != "get_weather" || tc.Function.Arguments != `{"city":"Paris"}` {
		t.Errorf("tool call = %+v", tc)
	}

	// tool result message carries the tool_call_id
	if msgs[3].ToolCallID != "c1" || msgs[3].Content != `{"temp":20}` {
		t.Errorf("tool result = %+v", msgs[3])
	}
}

func TestConvertMessages_NoSystemPrompt(t *testing.T) {
	req := &langrails.CompletionRequest{
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	}
	msgs := convertMessages(req)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" {
		t.Errorf("Content = %v, want %q", msgs[0].Content, "hi")
	}
}

func TestConvertToolChoice(t *testing.T) {
	if convertToolChoice(nil) != nil {
		t.Error("nil ToolChoice should convert to nil")
	}
	tests := []struct {
		mode langrails.ToolChoiceMode
		want string
	}{
		{langrails.ToolChoiceAuto, "auto"},
		{langrails.ToolChoiceNone, "none"},
		{langrails.ToolChoiceRequired, "required"},
	}
	for _, tc := range tests {
		got := convertToolChoice(&langrails.ToolChoice{Mode: tc.mode})
		if got != tc.want {
			t.Errorf("mode %q → %v, want %q", tc.mode, got, tc.want)
		}
	}

	forced := convertToolChoice(langrails.ForceTool("get_weather"))
	fn, ok := forced.(toolChoiceFunction)
	if !ok {
		t.Fatalf("forced tool choice type = %T, want toolChoiceFunction", forced)
	}
	if fn.Function.Name != "get_weather" {
		t.Errorf("forced tool name = %q, want %q", fn.Function.Name, "get_weather")
	}

	if convertToolChoice(&langrails.ToolChoice{Mode: "bogus"}) != nil {
		t.Error("unknown mode should convert to nil")
	}
}
