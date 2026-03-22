package compat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/langrails"
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
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{
		Name:    "test",
		BaseURL: server.URL,
		APIKey:  "test-key",
	})

	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:        "gpt-4o",
		SystemPrompt: "You are helpful.",
		Messages:     []langrails.Message{{Role: "user", Content: "Hi"}},
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
		_ = json.NewDecoder(r.Body).Decode(&req)

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
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})

	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "What's the weather in Istanbul?"}},
		Tools: []langrails.ToolDefinition{{
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
		_ = json.NewEncoder(w).Encode(errorResponse{
			Error: struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			}{Message: "Invalid API key"},
		})
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "bad-key"})

	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}

	apiErr, ok := err.(*langrails.APIError)
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
			_, _ = w.Write([]byte("data: " + chunk + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})

	ch, err := provider.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var content string
	var gotDone bool
	for event := range ch {
		switch event.Type {
		case langrails.EventContent:
			content += event.Content
		case langrails.EventDone:
			gotDone = true
		case langrails.EventError:
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
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{
		Name:         "test",
		BaseURL:      server.URL,
		APIKey:       "key",
		ExtraHeaders: map[string]string{"X-Custom": "value"},
	})

	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "test",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Complete_StructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Error("expected json_schema response format")
		}
		if req.ResponseFormat.JSONSchema == nil || !req.ResponseFormat.JSONSchema.Strict {
			t.Error("expected strict mode")
		}
		resp := response{Choices: []choice{{Message: choiceMessage{Content: `{"a":1}`}, FinishReason: "stop"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	schema := []byte(`{"type":"object","properties":{"a":{"type":"integer"}}}`)
	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:        "test",
		Messages:     []langrails.Message{{Role: "user", Content: "Hi"}},
		OutputSchema: &schema,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"a":1}` {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestProvider_Complete_WithAllParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.FrequencyPenalty == nil || *req.FrequencyPenalty != 0.5 {
			t.Error("expected frequency_penalty 0.5")
		}
		if req.PresencePenalty == nil || *req.PresencePenalty != 0.3 {
			t.Error("expected presence_penalty 0.3")
		}
		if len(req.Stop) != 1 || req.Stop[0] != "END" {
			t.Error("expected stop sequence")
		}
		if req.Seed == nil || *req.Seed != 42 {
			t.Error("expected seed 42")
		}
		resp := response{Choices: []choice{{Message: choiceMessage{Content: "ok"}, FinishReason: "stop"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	fp := 0.5
	pp := 0.3
	seed := 42
	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:            "test",
		Messages:         []langrails.Message{{Role: "user", Content: "Hi"}},
		FrequencyPenalty: &fp,
		PresencePenalty:  &pp,
		Stop:             []string{"END"},
		Seed:             &seed,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Complete_Reasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Reasoning == nil || req.Reasoning.Effort != "medium" {
			t.Error("expected reasoning effort medium")
		}
		resp := response{Choices: []choice{{Message: choiceMessage{Content: "ok"}, FinishReason: "stop"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "o1",
		Messages: []langrails.Message{{Role: "user", Content: "Think"}},
		Thinking: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Stream_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":""}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Istanbul\"}"}}]}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		}
		for _, c := range chunks {
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
			flusher.Flush()
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	ch, err := provider.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "Weather?"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var toolCalls []langrails.ToolCall
	for event := range ch {
		if event.Type == langrails.EventToolCall && event.ToolCall != nil {
			toolCalls = append(toolCalls, *event.ToolCall)
		}
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].Name != "get_weather" {
		t.Errorf("expected 'get_weather', got %q", toolCalls[0].Name)
	}
	if toolCalls[0].Arguments != `{"city":"Istanbul"}` {
		t.Errorf("unexpected args: %s", toolCalls[0].Arguments)
	}
}
