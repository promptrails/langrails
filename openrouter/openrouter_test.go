package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/llmrails"
	"github.com/promptrails/llmrails/compat"
)

func TestNew(t *testing.T) {
	p := New("test-key")
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestNew_WithOptions(t *testing.T) {
	p := New("key",
		WithBaseURL("https://custom.url"),
		WithHTTPClient(&http.Client{}),
		WithSiteInfo("https://myapp.com", "My App"),
	)
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected auth header, got %q", r.Header.Get("Authorization"))
		}
		resp := compat.TestResponse{
			Model: "openai/gpt-4o",
			Choices: []compat.TestChoice{{
				Message:      compat.TestMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			}},
			Usage: compat.TestUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("test-key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.Content)
	}
}

func TestProvider_WithSiteInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("HTTP-Referer") != "https://myapp.com" {
			t.Errorf("expected HTTP-Referer, got %q", r.Header.Get("HTTP-Referer"))
		}
		if r.Header.Get("X-Title") != "My App" {
			t.Errorf("expected X-Title, got %q", r.Header.Get("X-Title"))
		}
		resp := compat.TestResponse{
			Choices: []compat.TestChoice{{
				Message:      compat.TestMessage{Content: "ok"},
				FinishReason: "stop",
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key",
		WithBaseURL(server.URL),
		WithSiteInfo("https://myapp.com", "My App"),
	)
	_, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "test",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	ch, err := provider.Stream(context.Background(), &llmrails.CompletionRequest{
		Model:    "test",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	for event := range ch {
		if event.Type == llmrails.EventContent {
			content += event.Content
		}
	}
	if content != "Hi" {
		t.Errorf("expected 'Hi', got %q", content)
	}
}
