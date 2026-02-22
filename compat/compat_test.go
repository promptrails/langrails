package compat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/unillm"
)

func TestProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Authorization header, got %q", r.Header.Get("Authorization"))
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "gpt-4o" {
			t.Errorf("expected model gpt-4o, got %q", req.Model)
		}
		if req.Stream {
			t.Error("expected stream=false")
		}
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages (system+user), got %d", len(req.Messages))
		}

		resp := response{
			ID:    "chatcmpl-123",
			Model: "gpt-4o",
			Choices: []choice{{
				Message:      choiceMessage{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			}},
			Usage: usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "test-key",
	})

	resp, err := provider.Complete(context.Background(), &unillm.CompletionRequest{
		Model:        "gpt-4o",
		SystemPrompt: "You are helpful.",
		Messages:     []unillm.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Content != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.Content)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestProvider_Complete_WithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		json.NewDecoder(r.Body).Decode(&req)

		if len(req.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "get_weather" {
			t.Errorf("expected tool 'get_weather', got %q", req.Tools[0].Function.Name)
		}

		resp := response{
			Model: "gpt-4o",
			Choices: []choice{{
				Message: choiceMessage{
					Role: "assistant",
					ToolCalls: []toolCall{{
						ID:   "call_123",
						Type: "function",
						Function: functionCall{
							Name:      "get_weather",
							Arguments: `{"city":"Istanbul"}`,
						},
					}},
				},
				FinishReason: "tool_calls",
			}},
			Usage: usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})

	resp, err := provider.Complete(context.Background(), &unillm.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []unillm.Message{{Role: "user", Content: "What's the weather in Istanbul?"}},
		Tools: []unillm.ToolDefinition{{
			Name:        "get_weather",
			Description: "Get current weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
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
	if resp.ToolCalls[0].Arguments != `{"city":"Istanbul"}` {
		t.Errorf("unexpected arguments: %s", resp.ToolCalls[0].Arguments)
	}
}

func TestProvider_Complete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(errorResponse{
			Error: struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			}{Message: "Invalid API key"},
		})
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "bad-key"})

	_, err := provider.Complete(context.Background(), &unillm.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []unillm.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*unillm.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
	if !apiErr.IsAuthError() {
		t.Error("expected IsAuthError() to be true")
	}
}

func TestProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		chunks := []string{
			`{"choices":[{"delta":{"content":"Hello"},"finish_reason":""}]}`,
			`{"choices":[{"delta":{"content":" World"},"finish_reason":""}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		}

		for _, chunk := range chunks {
			w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})

	ch, err := provider.Stream(context.Background(), &unillm.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []unillm.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	var gotDone bool
	for event := range ch {
		switch event.Type {
		case unillm.EventContent:
			content += event.Content
		case unillm.EventDone:
			gotDone = true
		case unillm.EventError:
			t.Fatalf("unexpected error event: %v", event.Error)
		}
	}

	if content != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", content)
	}
	if !gotDone {
		t.Error("expected done event")
	}
}

func TestProvider_ExtraHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			t.Errorf("expected X-Custom header, got %q", r.Header.Get("X-Custom"))
		}
		resp := response{Model: "test", Choices: []choice{{Message: choiceMessage{Content: "ok"}, FinishReason: "stop"}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{
		Name:         "test",
		BaseURL:      server.URL,
		APIKey:       "key",
		ExtraHeaders: map[string]string{"X-Custom": "value"},
	})

	_, err := provider.Complete(context.Background(), &unillm.CompletionRequest{
		Model:    "test",
		Messages: []unillm.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
