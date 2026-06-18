package langrails

import (
	"context"
	"testing"
)

// stubProvider is a minimal Provider implementation used to lock the interface
// signature and exercise the request/response contract without a network call.
type stubProvider struct {
	lastReq *CompletionRequest
	resp    *CompletionResponse
}

func (s *stubProvider) Complete(_ context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	s.lastReq = req
	return s.resp, nil
}

func (s *stubProvider) Stream(_ context.Context, req *CompletionRequest) (<-chan StreamEvent, error) {
	s.lastReq = req
	ch := make(chan StreamEvent, 2)
	ch <- StreamEvent{Type: EventContent, Content: s.resp.Content}
	ch <- StreamEvent{Type: EventDone, Usage: &s.resp.Usage}
	close(ch)
	return ch, nil
}

// Compile-time assertion that stubProvider satisfies the Provider interface.
// This guards against accidental signature changes to Complete/Stream.
var _ Provider = (*stubProvider)(nil)

func TestProvider_CompleteContract(t *testing.T) {
	want := &CompletionResponse{
		Content:      "hi",
		Usage:        TokenUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4},
		FinishReason: "stop",
		Model:        "stub-1",
	}
	var p Provider = &stubProvider{resp: want}

	req := &CompletionRequest{
		Model:    "stub-1",
		Messages: []Message{{Role: "user", Content: "hello"}},
	}
	got, err := p.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "hi" {
		t.Errorf("Content = %q, want %q", got.Content, "hi")
	}
	if got.Usage.TotalTokens != 4 {
		t.Errorf("TotalTokens = %d, want 4", got.Usage.TotalTokens)
	}
}

func TestProvider_StreamContract(t *testing.T) {
	var p Provider = &stubProvider{resp: &CompletionResponse{Content: "chunk"}}

	ch, err := p.Stream(context.Background(), &CompletionRequest{Model: "stub-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	var sawDone bool
	for ev := range ch {
		switch ev.Type {
		case EventContent:
			content += ev.Content
		case EventDone:
			sawDone = true
		}
	}
	if content != "chunk" {
		t.Errorf("streamed content = %q, want %q", content, "chunk")
	}
	if !sawDone {
		t.Error("expected an EventDone before the channel closed")
	}
}

// Optional CompletionRequest fields are pointers so that "unset" (nil) is
// distinguishable from a zero value the caller explicitly chose.
func TestCompletionRequest_OptionalFieldsDefaultNil(t *testing.T) {
	req := CompletionRequest{
		Model:    "stub-1",
		Messages: []Message{{Role: "user", Content: "hi"}},
	}
	if req.Temperature != nil {
		t.Error("Temperature should default to nil (provider default)")
	}
	if req.MaxTokens != nil {
		t.Error("MaxTokens should default to nil (provider default)")
	}
	if req.TopP != nil || req.TopK != nil {
		t.Error("TopP/TopK should default to nil (provider default)")
	}
	if req.ToolChoice != nil {
		t.Error("ToolChoice should default to nil (provider default)")
	}
}
