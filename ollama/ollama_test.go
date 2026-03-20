package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/compat"
)

func TestNew(t *testing.T) {
	p := New()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNew_WithOptions(t *testing.T) {
	p := New(
		WithBaseURL("http://custom:11434/v1/chat/completions"),
		WithHTTPClient(&http.Client{}),
	)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := compat.TestResponse{
			Model: "llama3.2",
			Choices: []compat.TestChoice{{
				Message:      compat.TestMessage{Role: "assistant", Content: "Hello from Ollama!"},
				FinishReason: "stop",
			}},
			Usage: compat.TestUsage{PromptTokens: 10, CompletionTokens: 8, TotalTokens: 18},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "llama3.2",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello from Ollama!" {
		t.Errorf("expected 'Hello from Ollama!', got %q", resp.Content)
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

	provider := New(WithBaseURL(server.URL))
	ch, err := provider.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "llama3.2",
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
