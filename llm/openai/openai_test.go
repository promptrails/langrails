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
