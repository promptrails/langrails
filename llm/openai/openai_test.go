package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/llm/compat"
)

func oaiResponse() compat.TestResponse {
	return compat.TestResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4o",
		Choices: []compat.TestChoice{{
			Message:      compat.TestMessage{Role: "assistant", Content: "Hello!"},
			FinishReason: "stop",
		}},
		Usage: compat.TestUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}
}

func TestNew(t *testing.T) {
	p := New("sk-test")
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNew_WithOptions(t *testing.T) {
	p := New("sk-test",
		WithBaseURL("https://custom.url/v1/chat/completions"),
		WithHTTPClient(&http.Client{}),
	)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(oaiResponse())
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.Content)
	}
}

func TestProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	ch, err := provider.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	for event := range ch {
		if event.Type == langrails.EventContent {
			content += event.Content
		}
	}
	if content != "Hi" {
		t.Errorf("expected 'Hi', got %q", content)
	}
}

// TestOpenAISendsReasoningEffortNotTheObject drives New(), not compat.Config.
//
// The wiring is the whole fix: compat defaults to the {"reasoning":{...}}
// object, which OpenAI does not read, so for as long as this package took the
// default the reasoning-effort setting was a silent no-op against OpenAI. A
// test that built a compat.Config itself would pass with that wiring missing.
func TestOpenAISendsReasoningEffortNotTheObject(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(oaiResponse())
	}))
	defer server.Close()

	_, err := New("key", WithBaseURL(server.URL)).Complete(context.Background(), &langrails.CompletionRequest{
		Model:           "gpt-5.6-terra",
		Messages:        []langrails.Message{{Role: "user", Content: "Hi"}},
		ReasoningEffort: langrails.ReasoningNone,
		Tools:           []langrails.ToolDefinition{{Name: "search", Parameters: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got := body["reasoning_effort"]; got != "none" {
		t.Fatalf(`body["reasoning_effort"] = %v, want "none"`, got)
	}
	if _, ok := body["reasoning"]; ok {
		t.Fatal("OpenAI must not be sent the OpenRouter object form")
	}
}
