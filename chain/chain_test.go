package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/promptrails/unillm"
)

type mockProvider struct {
	calls   int
	handler func(req *unillm.CompletionRequest) *unillm.CompletionResponse
}

func (m *mockProvider) Complete(_ context.Context, req *unillm.CompletionRequest) (*unillm.CompletionResponse, error) {
	m.calls++
	return m.handler(req), nil
}

func (m *mockProvider) Stream(_ context.Context, _ *unillm.CompletionRequest) (<-chan unillm.StreamEvent, error) {
	return nil, nil
}

func TestChain_TwoSteps(t *testing.T) {
	provider := &mockProvider{
		handler: func(req *unillm.CompletionRequest) *unillm.CompletionResponse {
			input := req.Messages[0].Content
			if strings.Contains(req.SystemPrompt, "Summarize") {
				return &unillm.CompletionResponse{
					Content: "Summary of: " + input,
					Usage:   unillm.TokenUsage{TotalTokens: 10},
				}
			}
			return &unillm.CompletionResponse{
				Content: "Translated: " + input,
				Usage:   unillm.TokenUsage{TotalTokens: 8},
			}
		},
	}

	c := New(provider, []Step{
		{SystemPrompt: "Summarize this."},
		{SystemPrompt: "Translate to Turkish."},
	}, WithModel("test-model"))

	result, err := c.Run(context.Background(), "Long article")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "Translated: Summary of: Long article" {
		t.Errorf("unexpected output: %q", result.Output)
	}
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(result.Steps))
	}
	if result.TotalUsage.TotalTokens != 18 {
		t.Errorf("expected 18 total tokens, got %d", result.TotalUsage.TotalTokens)
	}
	if provider.calls != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.calls)
	}
}

func TestChain_WithTransform(t *testing.T) {
	provider := &mockProvider{
		handler: func(req *unillm.CompletionRequest) *unillm.CompletionResponse {
			return &unillm.CompletionResponse{Content: req.Messages[0].Content}
		},
	}

	c := New(provider, []Step{
		{
			SystemPrompt: "Echo",
			Transform:    func(output string) string { return strings.ToUpper(output) },
		},
		{SystemPrompt: "Echo"},
	}, WithModel("test"))

	result, err := c.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "HELLO" {
		t.Errorf("expected 'HELLO', got %q", result.Output)
	}
}

func TestChain_InputTemplate(t *testing.T) {
	provider := &mockProvider{
		handler: func(req *unillm.CompletionRequest) *unillm.CompletionResponse {
			return &unillm.CompletionResponse{Content: "processed: " + req.Messages[0].Content}
		},
	}

	c := New(provider, []Step{
		{
			SystemPrompt:  "Process",
			InputTemplate: "Please analyze the following data:\n\n{input}\n\nBe concise.",
		},
	}, WithModel("test"))

	result, err := c.Run(context.Background(), "raw data")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "processed: Please analyze the following data:\n\nraw data\n\nBe concise."
	if result.Output != expected {
		t.Errorf("unexpected output: %q", result.Output)
	}
}

func TestChain_PerStepProvider(t *testing.T) {
	provider1 := &mockProvider{
		handler: func(_ *unillm.CompletionRequest) *unillm.CompletionResponse {
			return &unillm.CompletionResponse{Content: "from-provider-1"}
		},
	}
	provider2 := &mockProvider{
		handler: func(_ *unillm.CompletionRequest) *unillm.CompletionResponse {
			return &unillm.CompletionResponse{Content: "from-provider-2"}
		},
	}

	c := New(provider1, []Step{
		{SystemPrompt: "Step 1"},
		{SystemPrompt: "Step 2", Provider: provider2},
	}, WithModel("test"))

	result, err := c.Run(context.Background(), "input")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Output != "from-provider-2" {
		t.Errorf("expected output from provider2, got %q", result.Output)
	}
	if provider1.calls != 1 {
		t.Errorf("expected provider1 called once, got %d", provider1.calls)
	}
	if provider2.calls != 1 {
		t.Errorf("expected provider2 called once, got %d", provider2.calls)
	}
}

func TestChain_NoModel(t *testing.T) {
	provider := &mockProvider{
		handler: func(_ *unillm.CompletionRequest) *unillm.CompletionResponse {
			return &unillm.CompletionResponse{Content: "ok"}
		},
	}

	c := New(provider, []Step{{SystemPrompt: "Test"}})

	_, err := c.Run(context.Background(), "input")
	if err == nil {
		t.Fatal("expected error when no model specified")
	}
}
