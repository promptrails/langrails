package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

func baseExec() tools.Executor {
	return tools.NewMap(map[string]tools.Func{
		"send_email": func(_ context.Context, _ string) (string, error) { return `{"sent":true}`, nil },
		"read_doc":   func(_ context.Context, _ string) (string, error) { return `{"text":"hi"}`, nil },
	})
}

func TestHITL_ApprovedRuns(t *testing.T) {
	h := NewHumanInLoop(baseExec(), func(_ context.Context, _ langrails.ToolCall) (Decision, error) {
		return Approve(), nil
	})
	out, err := h.Execute(context.Background(), "send_email", "{}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"sent":true`) {
		t.Errorf("expected tool to run, got %q", out)
	}
}

func TestHITL_RejectedReturnsReason(t *testing.T) {
	var ran bool
	exec := tools.NewMap(map[string]tools.Func{
		"send_email": func(_ context.Context, _ string) (string, error) { ran = true; return "ok", nil },
	})
	h := NewHumanInLoop(exec, func(_ context.Context, _ langrails.ToolCall) (Decision, error) {
		return Reject("not allowed"), nil
	})

	out, err := h.Execute(context.Background(), "send_email", "{}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ran {
		t.Error("rejected tool should not have executed")
	}
	if !strings.Contains(out, "not allowed") {
		t.Errorf("expected rejection reason in result, got %q", out)
	}
}

func TestHITL_InterruptOnSubset(t *testing.T) {
	var approverCalls int
	h := NewHumanInLoop(baseExec(), func(_ context.Context, _ langrails.ToolCall) (Decision, error) {
		approverCalls++
		return Approve(), nil
	}, WithInterruptOn("send_email"))

	// read_doc is not guarded → approver not consulted.
	if _, err := h.Execute(context.Background(), "read_doc", "{}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approverCalls != 0 {
		t.Errorf("expected no approval for unguarded tool, got %d", approverCalls)
	}

	// send_email is guarded → approver consulted.
	if _, err := h.Execute(context.Background(), "send_email", "{}"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approverCalls != 1 {
		t.Errorf("expected 1 approval for guarded tool, got %d", approverCalls)
	}
}

func TestHITL_ApproverErrorAborts(t *testing.T) {
	h := NewHumanInLoop(baseExec(), func(_ context.Context, _ langrails.ToolCall) (Decision, error) {
		return Decision{}, errors.New("reviewer unavailable")
	})
	if _, err := h.Execute(context.Background(), "send_email", "{}"); err == nil {
		t.Fatal("expected approver error to abort execution")
	}
}

func TestHITL_AgentIntegration(t *testing.T) {
	// Model calls a guarded tool, then answers. The reviewer rejects, so
	// the model receives a rejection result and the tool never runs.
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "send_email", Arguments: "{}"}}},
		{Content: "I won't send it then."},
	}}

	var emailSent bool
	exec := tools.NewMap(map[string]tools.Func{
		"send_email": func(_ context.Context, _ string) (string, error) { emailSent = true; return "ok", nil },
	})
	gate := NewHumanInLoop(exec, func(_ context.Context, _ langrails.ToolCall) (Decision, error) {
		return Reject("user declined"), nil
	})

	a := New(p, WithModel("test"), WithTools(nil, gate))
	result, err := a.Run(context.Background(), "email the report")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if emailSent {
		t.Error("email should not have been sent after rejection")
	}
	if result.Response.Content != "I won't send it then." {
		t.Errorf("unexpected final content: %q", result.Response.Content)
	}
	// The rejection result must have reached the model as a tool message.
	var toolMsg string
	for _, m := range result.Messages {
		if m.Role == "tool" {
			toolMsg = m.Content
		}
	}
	if !strings.Contains(toolMsg, "user declined") {
		t.Errorf("expected rejection reason in tool message, got %q", toolMsg)
	}
}
