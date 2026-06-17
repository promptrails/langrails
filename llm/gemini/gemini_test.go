package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
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
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:        "gemini-2.0-flash",
		SystemPrompt: "Be helpful",
		Messages:     []langrails.Message{{Role: "user", Content: "Hi"}},
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
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
		Messages: []langrails.Message{{Role: "user", Content: "Weather?"}},
		Tools: []langrails.ToolDefinition{{
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
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:        "gemini-2.0-flash",
		Messages:     []langrails.Message{{Role: "user", Content: "Analyze"}},
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
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "bad-model",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*langrails.APIError)
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
	ch, err := provider.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
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
	if content != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", content)
	}
}

func TestProvider_ConvertMessages(t *testing.T) {
	// A signed (Gemini-origin) tool call keeps its functionCall / functionResponse
	// parts.
	req := &langrails.CompletionRequest{
		Messages: []langrails.Message{
			{Role: "user", Content: "Weather?"},
			{Role: "assistant", ToolCalls: []langrails.ToolCall{{
				ID:        "get_weather",
				Name:      "get_weather",
				Arguments: `{"city":"Istanbul"}`,
				Metadata:  map[string]string{"thoughtSignature": "sig-abc"},
			}}},
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
	if msgs[1].Parts[0].FunctionCall == nil || msgs[1].Parts[0].FunctionCall.ThoughtSignature != "sig-abc" {
		t.Error("expected functionCall with thoughtSignature for signed tool call")
	}
	if msgs[2].Parts[0].FunctionResponse == nil {
		t.Error("expected functionResponse for tool message")
	}
}

// Tool calls that carry NO thoughtSignature (e.g. produced by a fallback model
// or a mid-conversation model switch) must be rendered as text, not functionCall
// parts — otherwise Gemini 2.5+ rejects the whole request with "Function call is
// missing a thought_signature". Their results are textified too so nothing is a
// dangling functionResponse.
func TestProvider_ConvertMessages_UnsignedToolCallsTextified(t *testing.T) {
	req := &langrails.CompletionRequest{
		Messages: []langrails.Message{
			{Role: "user", Content: "List my data sources"},
			{Role: "assistant", ToolCalls: []langrails.ToolCall{{
				ID:        "call_1",
				Name:      "list_data_sources",
				Arguments: `{}`,
				// no Metadata → unsigned (came from a non-Gemini model)
			}}},
			{Role: "tool", ToolCallID: "call_1", Content: `{"sources":["pg"]}`},
		},
	}
	msgs := convertMessages(req)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	// Assistant turn: text, no functionCall.
	if msgs[1].Parts[0].FunctionCall != nil {
		t.Error("unsigned tool call must NOT be a functionCall part")
	}
	if !strings.Contains(msgs[1].Parts[0].Text, "list_data_sources") {
		t.Errorf("expected the call rendered as text, got %q", msgs[1].Parts[0].Text)
	}
	// Tool result: text, no functionResponse (else it dangles).
	if msgs[2].Parts[0].FunctionResponse != nil {
		t.Error("result of an unsigned call must NOT be a functionResponse part")
	}
	if !strings.Contains(msgs[2].Parts[0].Text, "pg") {
		t.Errorf("expected the result rendered as text, got %q", msgs[2].Parts[0].Text)
	}
}

// A genuine Gemini PARALLEL turn signs only its first call; the others are
// unsigned but must stay functionCall parts (turnIsSigned is true for the turn).
func TestProvider_ConvertMessages_ParallelKeepsFunctionCalls(t *testing.T) {
	req := &langrails.CompletionRequest{
		Messages: []langrails.Message{
			{Role: "assistant", ToolCalls: []langrails.ToolCall{
				{ID: "a", Name: "first", Arguments: `{}`, Metadata: map[string]string{"thoughtSignature": "sig"}},
				{ID: "b", Name: "second", Arguments: `{}`}, // unsigned secondary parallel call
			}},
		},
	}
	msgs := convertMessages(req)
	if len(msgs[0].Parts) != 2 {
		t.Fatalf("expected 2 functionCall parts, got %d", len(msgs[0].Parts))
	}
	for i, p := range msgs[0].Parts {
		if p.FunctionCall == nil {
			t.Errorf("part %d: expected functionCall (parallel turn is signed), got %+v", i, p)
		}
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
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
		Messages: []langrails.Message{{Role: "user", Content: "Hi"}},
		TopK:     &topK,
		Stop:     []string{"END"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_ToolChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		fc := req.ToolConfig
		if fc == nil || fc.FunctionCallingConfig.Mode != "ANY" {
			t.Fatalf("toolConfig = %+v", fc)
		}
		if len(fc.FunctionCallingConfig.AllowedFunctionNames) != 1 || fc.FunctionCallingConfig.AllowedFunctionNames[0] != "lookup" {
			t.Errorf("allowed names = %+v", fc.FunctionCallingConfig.AllowedFunctionNames)
		}
		resp := response{Candidates: []candidate{{Content: content{Parts: []part{{Text: "ok"}}}, FinishReason: "STOP"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:      "gemini-2.0-flash",
		Messages:   []langrails.Message{{Role: "user", Content: "hi"}},
		Tools:      []langrails.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: langrails.ForceTool("lookup"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_JSONMode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.GenerationConfig == nil || req.GenerationConfig.ResponseMIMEType != "application/json" {
			t.Errorf("expected responseMimeType application/json, got %+v", req.GenerationConfig)
		}
		if req.GenerationConfig.ResponseSchema != nil {
			t.Error("JSON mode must not carry a responseSchema")
		}
		resp := response{Candidates: []candidate{{Content: content{Parts: []part{{Text: "{}"}}}, FinishReason: "STOP"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:          "gemini-2.0-flash",
		Messages:       []langrails.Message{{Role: "user", Content: "json please"}},
		ResponseFormat: langrails.ResponseFormatJSONObject,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_GroundingCitations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		var hasSearch bool
		for _, tl := range req.Tools {
			if tl.GoogleSearch != nil {
				hasSearch = true
			}
		}
		if !hasSearch {
			t.Errorf("expected googleSearch tool, got %+v", req.Tools)
		}
		resp := response{
			Candidates: []candidate{{
				Content:      content{Parts: []part{{Text: "grounded answer"}}},
				FinishReason: "STOP",
				GroundingMetadata: &groundingMetadata{GroundingChunks: []struct {
					Web *struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					} `json:"web,omitempty"`
				}{
					{Web: &struct {
						URI   string `json:"uri"`
						Title string `json:"title"`
					}{URI: "https://src.com", Title: "Source"}},
				}},
			}},
			UsageMetadata: &usageMetadata{CachedContentTokenCount: 42},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:       "gemini-2.0-flash",
		Messages:    []langrails.Message{{Role: "user", Content: "latest?"}},
		ServerTools: []langrails.ServerTool{langrails.WebSearch(nil)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].URL != "https://src.com" {
		t.Errorf("citations = %+v", resp.Citations)
	}
	if resp.Usage.CachedTokens != 42 {
		t.Errorf("CachedTokens = %d, want 42", resp.Usage.CachedTokens)
	}
}

func TestProvider_Vision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		parts := req.Contents[0].Parts
		if len(parts) != 2 || parts[1].InlineData == nil {
			t.Fatalf("expected inlineData image part, got %+v", parts)
		}
		if parts[1].InlineData.MIMEType != "image/jpeg" || parts[1].InlineData.Data != "ZZZ" {
			t.Errorf("inlineData = %+v", parts[1].InlineData)
		}
		resp := response{Candidates: []candidate{{Content: content{Parts: []part{{Text: "ok"}}}, FinishReason: "STOP"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	_, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model: "gemini-2.0-flash",
		Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{
			langrails.TextPart("describe"),
			langrails.ImageBase64Part("ZZZ", "image/jpeg"),
		}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Stream_RespectsCanceledContext(t *testing.T) {
	p := New("key", WithBaseURL("http://127.0.0.1:1"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Stream(ctx, &langrails.CompletionRequest{
		Model:    "gemini-2.0-flash",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestProvider_Reasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.GenerationConfig == nil || req.GenerationConfig.ThinkingConfig == nil {
			t.Fatalf("expected thinkingConfig, got %+v", req.GenerationConfig)
		}
		tc := req.GenerationConfig.ThinkingConfig
		if !tc.IncludeThoughts {
			t.Error("expected includeThoughts=true")
		}
		if tc.ThinkingBudget == nil || *tc.ThinkingBudget != 8192 {
			t.Errorf("thinkingBudget = %v, want 8192 (medium)", tc.ThinkingBudget)
		}
		resp := response{
			Candidates: []candidate{{Content: content{Parts: []part{
				{Text: "reasoning here", Thought: true},
				{Text: "final answer"},
			}}, FinishReason: "STOP"}},
			UsageMetadata: &usageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 10, TotalTokenCount: 15, ThoughtsTokenCount: 7},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := New("key", WithBaseURL(server.URL))
	resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
		Model:           "gemini-2.5-pro",
		Messages:        []langrails.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: langrails.ReasoningMedium,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "reasoning here" {
		t.Errorf("Thinking = %q", resp.Thinking)
	}
	if resp.Content != "final answer" {
		t.Errorf("Content = %q", resp.Content)
	}
	if resp.Usage.ReasoningTokens != 7 {
		t.Errorf("ReasoningTokens = %d, want 7", resp.Usage.ReasoningTokens)
	}
}
