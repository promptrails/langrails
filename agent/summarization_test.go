package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

// summarizerProvider records whether it was asked to summarize and returns
// a fixed summary.
type summarizerProvider struct {
	called   bool
	gotInput string
}

func (s *summarizerProvider) Complete(_ context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	s.called = true
	if len(req.Messages) > 0 {
		s.gotInput = req.Messages[0].Content
	}
	return &langrails.CompletionResponse{Content: "SUMMARY"}, nil
}

func (s *summarizerProvider) Stream(_ context.Context, _ *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return nil, nil
}

func longMessages(n int) []langrails.Message {
	msgs := make([]langrails.Message, n)
	body := strings.Repeat("x", 400) // ~100 tokens each
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = langrails.Message{Role: role, Content: body}
	}
	return msgs
}

func TestSummarization_BelowThresholdNoOp(t *testing.T) {
	sp := &summarizerProvider{}
	mw := NewSummarization(sp, "small", WithSummaryThreshold(100000))

	state := &State{Request: &langrails.CompletionRequest{Messages: longMessages(6)}}
	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sp.called {
		t.Error("summarizer should not be called below threshold")
	}
	if len(state.Request.Messages) != 6 {
		t.Errorf("expected messages untouched, got %d", len(state.Request.Messages))
	}
}

func TestSummarization_CompressesOldMessages(t *testing.T) {
	sp := &summarizerProvider{}
	mw := NewSummarization(sp, "small", WithSummaryThreshold(200), WithKeepRecent(2))

	state := &State{Request: &langrails.CompletionRequest{Messages: longMessages(8)}}
	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sp.called {
		t.Fatal("expected summarizer to be called")
	}
	// 1 summary + 2 recent = 3 messages.
	if len(state.Request.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(state.Request.Messages))
	}
	if !strings.HasPrefix(state.Request.Messages[0].Content, summaryPrefix) {
		t.Errorf("expected first message to be the summary, got %q", state.Request.Messages[0].Content)
	}
}

func TestSummarization_DoesNotOrphanToolResult(t *testing.T) {
	sp := &summarizerProvider{}
	mw := NewSummarization(sp, "small", WithSummaryThreshold(1), WithKeepRecent(1))

	// Tail would otherwise start with a "tool" message, orphaning it.
	msgs := []langrails.Message{
		{Role: "user", Content: strings.Repeat("a", 400)},
		{Role: "assistant", ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "x"}}},
		{Role: "tool", Content: "result", ToolCallID: "c1"},
	}
	state := &State{Request: &langrails.CompletionRequest{Messages: msgs}}
	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The kept tail must not begin with a tool message.
	for i, m := range state.Request.Messages {
		if i == 0 {
			continue // summary
		}
		if m.Role == "tool" && i == 1 {
			t.Error("kept tail begins with an orphaned tool result")
		}
	}
}

func TestSummarization_AgentIntegration(t *testing.T) {
	// Main provider returns a final answer; summarizer compresses history.
	main := &mockProvider{responses: []*langrails.CompletionResponse{{Content: "answer"}}}
	sp := &summarizerProvider{}

	a := New(main, WithModel("test"),
		WithMiddleware(NewSummarization(sp, "small", WithSummaryThreshold(200), WithKeepRecent(2))),
	)
	result, err := a.RunMessages(context.Background(), longMessages(8))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Content != "answer" {
		t.Errorf("unexpected content: %q", result.Response.Content)
	}
	if !sp.called {
		t.Error("expected summarization to run during the agent loop")
	}
	// Main provider should have seen the compressed history (3 messages).
	if len(main.lastReq.Messages) != 3 {
		t.Errorf("expected main provider to see 3 messages, got %d", len(main.lastReq.Messages))
	}
}
