package bedrock

import (
	"net/http"
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertToolChoice(t *testing.T) {
	if convertToolChoice(nil) != nil {
		t.Error("nil ToolChoice should convert to nil")
	}

	// Converse only models "any" and named tool; auto/none have no wire form.
	if got := convertToolChoice(langrails.AutoToolChoice()); got != nil {
		t.Errorf("auto → %+v, want nil", got)
	}
	if got := convertToolChoice(langrails.NoToolChoice()); got != nil {
		t.Errorf("none → %+v, want nil", got)
	}

	required := convertToolChoice(langrails.RequiredToolChoice())
	if required == nil || required.Any == nil {
		t.Errorf("required → %+v, want Any set", required)
	}

	forced := convertToolChoice(langrails.ForceTool("get_weather"))
	if forced == nil || forced.Tool == nil || forced.Tool.Name != "get_weather" {
		t.Errorf("forced → %+v, want Tool.Name get_weather", forced)
	}
}

func TestConvertMessages(t *testing.T) {
	req := &langrails.CompletionRequest{
		SystemPrompt: "ignored here", // handled by buildSystem, not convertMessages
		Messages: []langrails.Message{
			{Role: "system", Content: "also skipped"},
			{
				Role: "user",
				ContentParts: []langrails.ContentPart{
					langrails.TextPart("look at this"),
					langrails.ImageBase64Part("QUJD", "image/png"),
				},
			},
			{
				Role:    "assistant",
				Content: "calling a tool",
				ToolCalls: []langrails.ToolCall{
					{ID: "t1", Name: "get_weather", Arguments: `{"city":"Rome"}`},
				},
			},
			{Role: "tool", ToolCallID: "t1", Content: `{"temp":25}`},
		},
	}

	msgs := convertMessages(req)

	// system messages are skipped entirely
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages (user, assistant, tool→user), got %d", len(msgs))
	}

	// user multimodal → text block + image block
	user := msgs[0]
	if user.Role != "user" {
		t.Errorf("msgs[0].Role = %q, want user", user.Role)
	}
	var hasText, hasImage bool
	for _, b := range user.Content {
		if b.Text == "look at this" {
			hasText = true
		}
		if b.Image != nil {
			hasImage = true
			if b.Image.Format != "png" {
				t.Errorf("image format = %q, want png", b.Image.Format)
			}
			if b.Image.Source.Bytes != "QUJD" {
				t.Errorf("image bytes = %q, want QUJD", b.Image.Source.Bytes)
			}
		}
	}
	if !hasText || !hasImage {
		t.Errorf("user content missing text/image: %+v", user.Content)
	}

	// assistant → text + toolUse blocks
	asst := msgs[1]
	if asst.Role != "assistant" {
		t.Errorf("msgs[1].Role = %q, want assistant", asst.Role)
	}
	var toolUseSeen bool
	for _, b := range asst.Content {
		if b.ToolUse != nil {
			toolUseSeen = true
			if b.ToolUse.ToolUseID != "t1" || b.ToolUse.Name != "get_weather" {
				t.Errorf("toolUse = %+v", b.ToolUse)
			}
		}
	}
	if !toolUseSeen {
		t.Error("expected a toolUse block on the assistant message")
	}

	// tool result → user message with a toolResult block
	tool := msgs[2]
	if tool.Role != "user" {
		t.Errorf("tool result role = %q, want user", tool.Role)
	}
	if len(tool.Content) != 1 || tool.Content[0].ToolResult == nil {
		t.Fatalf("expected a toolResult block, got %+v", tool.Content)
	}
	tr := tool.Content[0].ToolResult
	if tr.ToolUseID != "t1" || tr.Status != "success" {
		t.Errorf("toolResult = %+v", tr)
	}
	if len(tr.Content) != 1 || tr.Content[0].Text != `{"temp":25}` {
		t.Errorf("toolResult content = %+v", tr.Content)
	}
}

// Empty tool-call arguments must default to "{}" so Converse accepts the input.
func TestConvertMessages_EmptyToolArgs(t *testing.T) {
	req := &langrails.CompletionRequest{
		Messages: []langrails.Message{
			{Role: "assistant", ToolCalls: []langrails.ToolCall{{ID: "t1", Name: "ping"}}},
		},
	}
	msgs := convertMessages(req)
	if len(msgs) != 1 || len(msgs[0].Content) != 1 {
		t.Fatalf("unexpected messages: %+v", msgs)
	}
	tu := msgs[0].Content[0].ToolUse
	if tu == nil || string(tu.Input) != "{}" {
		t.Errorf("empty args input = %s, want {}", tu.Input)
	}
}

func TestWithHTTPClient(t *testing.T) {
	hc := &http.Client{}
	p := New(WithHTTPClient(hc))
	if p.client != hc {
		t.Error("WithHTTPClient should set the provider's HTTP client")
	}
}
