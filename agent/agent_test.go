package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

// mockProvider returns queued responses in order.
type mockProvider struct {
	calls     int
	responses []*langrails.CompletionResponse
	lastReq   *langrails.CompletionRequest
}

func (m *mockProvider) Complete(_ context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	m.lastReq = req
	idx := m.calls
	m.calls++
	if idx < len(m.responses) {
		return m.responses[idx], nil
	}
	return &langrails.CompletionResponse{Content: "done"}, nil
}

func (m *mockProvider) Stream(_ context.Context, _ *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return nil, nil
}

func TestAgent_NoToolsSingleCall(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{Content: "Hello!", Usage: langrails.TokenUsage{TotalTokens: 10}},
	}}

	a := New(p, WithModel("test"))
	result, err := a.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Content != "Hello!" {
		t.Errorf("unexpected content: %q", result.Response.Content)
	}
	if result.Iterations != 1 {
		t.Errorf("expected 1 iteration, got %d", result.Iterations)
	}
	if result.TotalUsage.TotalTokens != 10 {
		t.Errorf("expected 10 tokens, got %d", result.TotalUsage.TotalTokens)
	}
}

func TestAgent_ToolLoop(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "echo", Arguments: `{"v":"x"}`}}},
		{Content: "final answer"},
	}}
	exec := tools.NewMap(map[string]tools.Func{
		"echo": func(_ context.Context, args string) (string, error) { return args, nil },
	})

	a := New(p, WithModel("test"), WithTools(nil, exec))
	result, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Content != "final answer" {
		t.Errorf("unexpected content: %q", result.Response.Content)
	}
	if result.Iterations != 2 {
		t.Errorf("expected 2 iterations, got %d", result.Iterations)
	}
	// user + assistant(tool call) + tool result = 3 messages.
	if len(result.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(result.Messages))
	}
}

func TestAgent_NoModel(t *testing.T) {
	a := New(&mockProvider{})
	if _, err := a.Run(context.Background(), "hi"); err == nil {
		t.Fatal("expected error when no model is set")
	}
}

func TestAgent_ToolsWithoutExecutor(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "x", Arguments: "{}"}}},
	}}
	a := New(p, WithModel("test"))
	if _, err := a.Run(context.Background(), "go"); err == nil {
		t.Fatal("expected error: tool calls but no executor")
	}
}

func TestAgent_MaxIterations(t *testing.T) {
	resp := make([]*langrails.CompletionResponse, 5)
	for i := range resp {
		resp[i] = &langrails.CompletionResponse{
			ToolCalls: []langrails.ToolCall{{ID: "c", Name: "loop", Arguments: "{}"}},
		}
	}
	p := &mockProvider{responses: resp}
	exec := tools.NewMap(map[string]tools.Func{
		"loop": func(_ context.Context, _ string) (string, error) { return "ok", nil },
	})

	a := New(p, WithModel("test"), WithTools(nil, exec), WithMaxIterations(3))
	if _, err := a.Run(context.Background(), "go"); err == nil {
		t.Fatal("expected max iterations error")
	}
}

// recorder middleware logs the order in which hooks fire.
type recorder struct {
	BaseMiddleware
	name string
	log  *[]string
}

func (r recorder) BeforeModel(_ context.Context, _ *State) error {
	*r.log = append(*r.log, "before:"+r.name)
	return nil
}

func (r recorder) AfterModel(_ context.Context, _ *State) error {
	*r.log = append(*r.log, "after:"+r.name)
	return nil
}

func (r recorder) WrapModelCall(next CallFunc) CallFunc {
	return func(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
		*r.log = append(*r.log, "enter:"+r.name)
		resp, err := next(ctx, req)
		*r.log = append(*r.log, "exit:"+r.name)
		return resp, err
	}
}

func TestAgent_MiddlewareOrder(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{{Content: "ok"}}}
	var log []string

	a := New(p, WithModel("test"),
		WithMiddleware(
			recorder{name: "m1", log: &log},
			recorder{name: "m2", log: &log},
		),
	)
	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"before:m1", "before:m2", // BeforeModel in order
		"enter:m1", "enter:m2", // WrapModelCall m1 outermost
		"exit:m2", "exit:m1",
		"after:m2", "after:m1", // AfterModel in reverse
	}
	if len(log) != len(want) {
		t.Fatalf("expected %d log entries, got %d: %v", len(want), len(log), log)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Errorf("log[%d]: expected %q, got %q", i, want[i], log[i])
		}
	}
}

// stopper ends the loop in AfterModel even though tool calls are present.
type stopper struct{ BaseMiddleware }

func (stopper) AfterModel(_ context.Context, s *State) error {
	s.Stop()
	return nil
}

func TestAgent_MiddlewareStop(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{Content: "halt", ToolCalls: []langrails.ToolCall{{ID: "c", Name: "x", Arguments: "{}"}}},
	}}
	a := New(p, WithModel("test"), WithMiddleware(stopper{}))

	result, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Iterations != 1 {
		t.Errorf("expected loop to stop after 1 iteration, got %d", result.Iterations)
	}
	if result.Response.Content != "halt" {
		t.Errorf("unexpected content: %q", result.Response.Content)
	}
}

// requestMutator rewrites the system prompt in BeforeModel.
type requestMutator struct{ BaseMiddleware }

func (requestMutator) BeforeModel(_ context.Context, s *State) error {
	s.Request.SystemPrompt = "rewritten"
	return nil
}

func TestAgent_BeforeModelMutatesRequest(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{{Content: "ok"}}}
	a := New(p, WithModel("test"), WithSystemPrompt("original"), WithMiddleware(requestMutator{}))

	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.lastReq.SystemPrompt != "rewritten" {
		t.Errorf("expected middleware to rewrite system prompt, got %q", p.lastReq.SystemPrompt)
	}
}

func TestAgent_MiddlewareErrorAborts(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{{Content: "ok"}}}
	failing := failingBefore{}
	a := New(p, WithModel("test"), WithMiddleware(failing))
	if _, err := a.Run(context.Background(), "go"); err == nil {
		t.Fatal("expected error from BeforeModel")
	}
}

type failingBefore struct{ BaseMiddleware }

func (failingBefore) BeforeModel(_ context.Context, _ *State) error {
	return errors.New("before failed")
}
