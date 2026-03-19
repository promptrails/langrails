package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/llmrails"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestProvider_Complete(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected x-api-key header, got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != apiVersion {
			t.Errorf("expected anthropic-version %q", r.Header.Get("anthropic-version"))
		}

		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("expected model claude-sonnet-4-20250514, got %q", req.Model)
		}
		if req.System != "Be helpful" {
			t.Errorf("expected system prompt, got %q", req.System)
		}
		if req.MaxTokens != 4096 {
			t.Errorf("expected max_tokens 4096, got %d", req.MaxTokens)
		}

		resp := response{
			ID:         "msg_123",
			Model:      "claude-sonnet-4-20250514",
			Content:    []contentBlock{{Type: "text", Text: "Hello!"}},
			StopReason: "end_turn",
			Usage:      usage{InputTokens: 10, OutputTokens: 5},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	provider := New("test-key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:        "claude-sonnet-4-20250514",
		SystemPrompt: "Be helpful",
		Messages:     []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.Content)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestProvider_Complete_ToolCalls(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		resp := response{
			Model:      "claude-sonnet-4-20250514",
			Content:    []contentBlock{{Type: "tool_use", ID: "tc_1", Name: "get_weather", Input: json.RawMessage(`{"city":"Istanbul"}`)}},
			StopReason: "tool_use",
			Usage:      usage{InputTokens: 20, OutputTokens: 10},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []llmrails.Message{{Role: "user", Content: "Weather?"}},
		Tools: []llmrails.ToolDefinition{{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCalls[0].Name != "get_weather" {
		t.Errorf("expected tool 'get_weather', got %q", resp.ToolCalls[0].Name)
	}
}

func TestProvider_Complete_StructuredOutput(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Should have structured_output tool and tool_choice
		if req.ToolChoice == nil || req.ToolChoice.Name != "structured_output" {
			t.Error("expected tool_choice for structured_output")
		}

		resp := response{
			Model:      "claude-sonnet-4-20250514",
			Content:    []contentBlock{{Type: "tool_use", ID: "tc_1", Name: "structured_output", Input: json.RawMessage(`{"sentiment":"positive"}`)}},
			StopReason: "tool_use",
			Usage:      usage{InputTokens: 15, OutputTokens: 8},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	schema := []byte(`{"type":"object","properties":{"sentiment":{"type":"string"}}}`)
	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:        "claude-sonnet-4-20250514",
		Messages:     []llmrails.Message{{Role: "user", Content: "Analyze"}},
		OutputSchema: &schema,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"sentiment":"positive"}` {
		t.Errorf("expected structured output, got %q", resp.Content)
	}
}

func TestProvider_Complete_Thinking(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Thinking == nil || req.Thinking.Type != "enabled" {
			t.Error("expected thinking enabled")
		}
		if req.Thinking.BudgetTokens != 5000 {
			t.Errorf("expected budget 5000, got %d", req.Thinking.BudgetTokens)
		}

		resp := response{
			Model: "claude-sonnet-4-20250514",
			Content: []contentBlock{
				{Type: "thinking", Text: "Let me think..."},
				{Type: "text", Text: "The answer is 42."},
			},
			StopReason: "end_turn",
			Usage:      usage{InputTokens: 10, OutputTokens: 20},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer server.Close()

	budget := 5000
	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:          "claude-sonnet-4-20250514",
		Messages:       []llmrails.Message{{Role: "user", Content: "Think hard"}},
		Thinking:       true,
		ThinkingBudget: &budget,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "Let me think..." {
		t.Errorf("expected thinking content, got %q", resp.Thinking)
	}
	if resp.Content != "The answer is 42." {
		t.Errorf("expected answer, got %q", resp.Content)
	}
}

func TestProvider_Complete_APIError(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(errorResponse{
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{Message: "invalid api key"},
		})
	})
	defer server.Close()

	provider := New("bad-key", WithBaseURL(server.URL))
	_, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*llmrails.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected 401, got %d", apiErr.StatusCode)
	}
}

func TestProvider_Stream(t *testing.T) {
	server := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" World"}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":10,"output_tokens":5}}`,
			`{"type":"message_stop"}`,
		}
		for _, e := range events {
			_, _ = w.Write([]byte("data: " + e + "\n\n"))
			flusher.Flush()
		}
	})
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	ch, err := provider.Stream(context.Background(), &llmrails.CompletionRequest{
		Model:    "claude-sonnet-4-20250514",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	var gotDone bool
	for event := range ch {
		switch event.Type {
		case llmrails.EventContent:
			content += event.Content
		case llmrails.EventDone:
			gotDone = true
		case llmrails.EventError:
			t.Fatalf("unexpected error: %v", event.Error)
		}
	}
	if content != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", content)
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestProvider_ConvertMessages_ToolResults(t *testing.T) {
	req := &llmrails.CompletionRequest{
		Messages: []llmrails.Message{
			{Role: "user", Content: "Weather?"},
			{Role: "assistant", Content: "", ToolCalls: []llmrails.ToolCall{{ID: "tc_1", Name: "get_weather", Arguments: `{"city":"Istanbul"}`}}},
			{Role: "tool", ToolCallID: "tc_1", Content: `{"temp":22}`},
		},
	}
	msgs := convertMessages(req)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// Tool result should be user message with tool_result block
	if msgs[2].Role != "user" {
		t.Errorf("expected tool result as user message, got %q", msgs[2].Role)
	}
	if msgs[2].Content[0].Type != "tool_result" {
		t.Errorf("expected tool_result block, got %q", msgs[2].Content[0].Type)
	}
}

func TestProvider_WithHTTPClient(t *testing.T) {
	p := New("key", WithHTTPClient(&http.Client{}))
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}
