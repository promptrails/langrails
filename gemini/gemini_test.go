package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/promptrails/llmrails"
)

func TestProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("expected API key in URL, got %q", r.URL.Query().Get("key"))
		}

		resp := response{
			Candidates: []candidate{{
				Content:      content{Role: "model", Parts: []part{{Text: "Hello!"}}},
				FinishReason: "STOP",
			}},
			UsageMetadata: &usageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 5, TotalTokenCount: 15},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("test-key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:        "gemini-2.0-flash",
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
		t.Errorf("expected 15 tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestProvider_Complete_ToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := response{
			Candidates: []candidate{{
				Content: content{Role: "model", Parts: []part{{
					FunctionCall: &functionCall{Name: "get_weather", Args: map[string]interface{}{"city": "Istanbul"}},
				}}},
				FinishReason: "STOP",
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
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
		t.Errorf("expected 'get_weather', got %q", resp.ToolCalls[0].Name)
	}
}

func TestProvider_Complete_StructuredOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.GenerationConfig == nil || req.GenerationConfig.ResponseMIMEType != "application/json" {
			t.Error("expected responseMimeType application/json")
		}

		resp := response{
			Candidates: []candidate{{
				Content:      content{Role: "model", Parts: []part{{Text: `{"sentiment":"positive"}`}}},
				FinishReason: "STOP",
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	schema := []byte(`{"type":"object","properties":{"sentiment":{"type":"string"}}}`)
	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:        "gemini-2.0-flash",
		Messages:     []llmrails.Message{{Role: "user", Content: "Analyze"}},
		OutputSchema: &schema,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != `{"sentiment":"positive"}` {
		t.Errorf("unexpected content: %q", resp.Content)
	}
}

func TestProvider_Complete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(errorResponse{
			Error: struct {
				Message string `json:"message"`
				Status  string `json:"status"`
				Code    int    `json:"code"`
			}{Message: "invalid model"},
		})
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	_, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "bad-model",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*llmrails.APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
}

func TestProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		chunks := []response{
			{Candidates: []candidate{{Content: content{Parts: []part{{Text: "Hello"}}}}}},
			{Candidates: []candidate{{Content: content{Parts: []part{{Text: " World"}}}, FinishReason: "STOP"}},
				UsageMetadata: &usageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 3, TotalTokenCount: 8}},
		}
		for _, chunk := range chunks {
			data, _ := json.Marshal(chunk)
			_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	ch, err := provider.Stream(context.Background(), &llmrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
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
	if content != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", content)
	}
}

func TestProvider_ConvertMessages(t *testing.T) {
	req := &llmrails.CompletionRequest{
		Messages: []llmrails.Message{
			{Role: "user", Content: "Weather?"},
			{Role: "assistant", ToolCalls: []llmrails.ToolCall{{Name: "get_weather", Arguments: `{"city":"Istanbul"}`}}},
			{Role: "tool", ToolCallID: "get_weather", Content: `{"temp":22}`},
		},
	}
	msgs := convertMessages(req)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[1].Role != "model" {
		t.Errorf("expected 'model' role for assistant, got %q", msgs[1].Role)
	}
	if msgs[2].Parts[0].FunctionResponse == nil {
		t.Error("expected functionResponse for tool message")
	}
}

func TestProvider_WithHTTPClient(t *testing.T) {
	p := New("key", WithHTTPClient(&http.Client{}))
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
}

func TestProvider_Complete_WithAllParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.GenerationConfig == nil {
			t.Fatal("expected generationConfig")
		}
		if req.GenerationConfig.TopK == nil || *req.GenerationConfig.TopK != 40 {
			t.Error("expected topK=40")
		}
		if len(req.GenerationConfig.StopSequences) != 1 || req.GenerationConfig.StopSequences[0] != "END" {
			t.Error("expected stop sequence")
		}

		resp := response{Candidates: []candidate{{Content: content{Parts: []part{{Text: "ok"}}}, FinishReason: "STOP"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	topK := 40
	provider := New("key", WithBaseURL(server.URL))
	_, err := provider.Complete(context.Background(), &llmrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
		Messages: []llmrails.Message{{Role: "user", Content: "Hi"}},
		TopK:     &topK,
		Stop:     []string{"END"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
