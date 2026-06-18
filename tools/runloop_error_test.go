package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

// When a tool returns an error, RunLoop must not abort: it feeds the error
// back to the model as a tool result and continues the loop, while still
// surfacing the error to the tool-call hook.
func TestRunLoop_ToolError(t *testing.T) {
	provider := &mockProvider{
		responses: []*langrails.CompletionResponse{
			{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "flaky", Arguments: `{}`}}},
			{Content: "recovered"},
		},
	}

	executor := NewMap(map[string]Func{
		"flaky": func(_ context.Context, _ string) (string, error) {
			return "", errors.New("upstream down")
		},
	})

	// RunLoop appends tool-call and tool-result messages to req.Messages in
	// place, so we inspect the request afterwards.
	req := &langrails.CompletionRequest{
		Model:    "test",
		Messages: []langrails.Message{{Role: "user", Content: "go"}},
	}

	var hookErr error
	result, err := RunLoop(context.Background(), provider, req, executor,
		WithToolCallHook(func(_ langrails.ToolCall, _ string, e error) {
			hookErr = e
		}))

	if err != nil {
		t.Fatalf("RunLoop should recover from a tool error, got: %v", err)
	}
	if result.Response.Content != "recovered" {
		t.Errorf("Content = %q, want %q", result.Response.Content, "recovered")
	}
	if result.Iterations != 2 {
		t.Errorf("Iterations = %d, want 2", result.Iterations)
	}
	if hookErr == nil {
		t.Error("hook should have received the tool error")
	}

	// The tool message fed back to the model must carry the error payload.
	var toolMsg *langrails.Message
	for i := range req.Messages {
		if req.Messages[i].Role == "tool" {
			toolMsg = &req.Messages[i]
		}
	}
	if toolMsg == nil {
		t.Fatal("expected a tool result message appended to the request")
	}
	if !strings.Contains(toolMsg.Content, "upstream down") {
		t.Errorf("tool message = %q, want it to contain the error", toolMsg.Content)
	}
	if toolMsg.ToolCallID != "c1" {
		t.Errorf("ToolCallID = %q, want %q", toolMsg.ToolCallID, "c1")
	}
}
