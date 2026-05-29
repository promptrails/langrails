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

func TestProvider_ReasoningEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Reasoning == nil || req.Reasoning.Effort != "high" {
			t.Errorf("expected reasoning effort high, got %+v", req.Reasoning)
		}
		_ = json.NewEncoder(w).Encode(response{Choices: []choice{{Message: choiceMessage{Content: "ok"}, FinishReason: "stop"}}})
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:           "o3",
		Messages:        []langrails.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: langrails.ReasoningHigh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_ToolChoice(t *testing.T) {
	cases := []struct {
		name string
		tc   *langrails.ToolChoice
		want interface{} // expected JSON value of tool_choice
	}{
		{"auto", langrails.AutoToolChoice(), "auto"},
		{"none", langrails.NoToolChoice(), "none"},
		{"required", langrails.RequiredToolChoice(), "required"},
		{"specific", langrails.ForceTool("get_weather"), map[string]interface{}{
			"type": "function", "function": map[string]interface{}{"name": "get_weather"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Decode into a generic map to inspect the raw tool_choice JSON.
				var raw map[string]json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&raw)
				var got interface{}
				_ = json.Unmarshal(raw["tool_choice"], &got)
				if !jsonEqual(got, tc.want) {
					t.Errorf("tool_choice = %#v, want %#v", got, tc.want)
				}
				_ = json.NewEncoder(w).Encode(response{Choices: []choice{{Message: choiceMessage{Content: "ok"}, FinishReason: "stop"}}})
			}))
			defer server.Close()

			provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
			_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
				Model:      "gpt-4o",
				Messages:   []langrails.Message{{Role: "user", Content: "hi"}},
				Tools:      []langrails.ToolDefinition{{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)}},
				ToolChoice: tc.tc,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestProvider_JSONObjectMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
			t.Errorf("expected response_format json_object, got %+v", req.ResponseFormat)
		}
		if req.ResponseFormat.JSONSchema != nil {
			t.Error("json_object mode must not carry a json_schema")
		}
		_ = json.NewEncoder(w).Encode(response{Choices: []choice{{Message: choiceMessage{Content: "{}"}, FinishReason: "stop"}}})
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:          "gpt-4o",
		Messages:       []langrails.Message{{Role: "user", Content: "json please"}},
		ResponseFormat: langrails.ResponseFormatJSONObject,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_UsageDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := response{
			Choices: []choice{{Message: choiceMessage{Content: "ok"}, FinishReason: "stop"}},
			Usage: usage{
				PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150,
				PromptTokensDetails:     &promptTokensDetails{CachedTokens: 80},
				CompletionTokensDetails: &completionTokensDetails{ReasoningTokens: 30},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.CachedTokens != 80 {
		t.Errorf("CachedTokens = %d, want 80", resp.Usage.CachedTokens)
	}
	if resp.Usage.ReasoningTokens != 30 {
		t.Errorf("ReasoningTokens = %d, want 30", resp.Usage.ReasoningTokens)
	}
}

func TestProvider_ReasoningContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := response{Choices: []choice{{
			Message:      choiceMessage{Content: "42", ReasoningContent: "first I considered..."},
			FinishReason: "stop",
		}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "deepseek-reasoner",
		Messages: []langrails.Message{{Role: "user", Content: "what is 6*7"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "first I considered..." {
		t.Errorf("Thinking = %q", resp.Thinking)
	}
	if resp.Content != "42" {
		t.Errorf("Content = %q", resp.Content)
	}
}

func TestProvider_Stream_Reasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"delta":{"reasoning_content":"thinking"}}]}`,
			`{"choices":[{"delta":{"reasoning_content":" more"}}]}`,
			`{"choices":[{"delta":{"content":"answer"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
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
		Model:    "deepseek-reasoner",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reasoning, content string
	for event := range ch {
		switch event.Type {
		case langrails.EventReasoning:
			reasoning += event.Reasoning
		case langrails.EventContent:
			content += event.Content
		}
	}
	if reasoning != "thinking more" {
		t.Errorf("reasoning = %q", reasoning)
	}
	if content != "answer" {
		t.Errorf("content = %q", content)
	}
}

func TestProvider_WebSearchAndCitations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.WebSearchOptions == nil {
			t.Error("expected web_search_options to be set")
		}
		resp := response{
			Choices: []choice{{
				Message: choiceMessage{
					Content: "Per recent news...",
					Annotations: []annotation{{
						Type: "url_citation",
						URLCitation: &struct {
							URL        string `json:"url"`
							Title      string `json:"title"`
							StartIndex int    `json:"start_index"`
							EndIndex   int    `json:"end_index"`
						}{URL: "https://a.com", Title: "A", StartIndex: 0, EndIndex: 5},
					}},
				},
				FinishReason: "stop",
			}},
			Citations: []string{"https://b.com"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New(Config{Name: "test", BaseURL: server.URL, APIKey: "key"})
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:       "gpt-4o-search-preview",
		Messages:    []langrails.Message{{Role: "user", Content: "latest news?"}},
		ServerTools: []langrails.ServerTool{langrails.WebSearch(nil)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Citations) != 2 {
		t.Fatalf("expected 2 citations, got %d (%+v)", len(resp.Citations), resp.Citations)
	}
	if resp.Citations[0].URL != "https://a.com" || resp.Citations[0].Title != "A" {
		t.Errorf("citation[0] = %+v", resp.Citations[0])
	}
	if resp.Citations[1].URL != "https://b.com" {
		t.Errorf("citation[1] = %+v", resp.Citations[1])
	}
}

// jsonEqual compares two values by their JSON encodings (order-independent for objects).
func jsonEqual(a, b interface{}) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}
