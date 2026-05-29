package bedrock

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func testProvider(serverURL string) *Provider {
	return New(
		WithRegion("us-east-1"),
		WithStaticCredentials("AKIDEXAMPLE", "secret", ""),
		WithBaseURL(serverURL),
	)
}

func TestProvider_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/model/") || !strings.HasSuffix(r.URL.Path, "/converse") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("request was not signed")
		}

		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(req.System) != 1 || req.System[0].Text != "Be helpful" {
			t.Errorf("system = %+v", req.System)
		}
		if len(req.Messages) != 1 || req.Messages[0].Content[0].Text != "Hello" {
			t.Errorf("messages = %+v", req.Messages)
		}
		if req.InferenceConfig == nil || req.InferenceConfig.MaxTokens == nil || *req.InferenceConfig.MaxTokens != 4096 {
			t.Errorf("inferenceConfig = %+v", req.InferenceConfig)
		}

		resp := response{
			StopReason: "end_turn",
			Usage:      usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		}
		resp.Output.Message.Role = "assistant"
		resp.Output.Message.Content = []responseContentBlock{{Text: "Hi there!"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	resp, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:        "anthropic.claude-3-5-sonnet-20241022-v2:0",
		SystemPrompt: "Be helpful",
		Messages:     []langrails.Message{{Role: "user", Content: "Hello"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Content != "Hi there!" {
		t.Errorf("content = %q", resp.Content)
	}
	if resp.FinishReason != "end_turn" {
		t.Errorf("finish reason = %q", resp.FinishReason)
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("total tokens = %d", resp.Usage.TotalTokens)
	}
}

func TestProvider_Complete_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ToolConfig == nil || len(req.ToolConfig.Tools) != 1 {
			t.Errorf("expected one tool, got %+v", req.ToolConfig)
		}

		resp := response{StopReason: "tool_use"}
		resp.Output.Message.Content = []responseContentBlock{{
			ToolUse: &responseToolUse{ToolUseID: "tool-1", Name: "get_weather", Input: json.RawMessage(`{"city":"NYC"}`)},
		}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	resp, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "anthropic.claude",
		Messages: []langrails.Message{{Role: "user", Content: "weather?"}},
		Tools: []langrails.ToolDefinition{{
			Name:        "get_weather",
			Description: "get weather",
			Parameters:  json.RawMessage(`{"type":"object"}`),
		}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	tc := resp.ToolCalls[0]
	if tc.ID != "tool-1" || tc.Name != "get_weather" || tc.Arguments != `{"city":"NYC"}` {
		t.Errorf("tool call = %+v", tc)
	}
}

func TestProvider_Complete_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"not authorized"}`))
	}))
	defer server.Close()

	p := testProvider(server.URL)
	_, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:    "anthropic.claude",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	apiErr, ok := err.(*langrails.APIError)
	if !ok {
		t.Fatalf("expected *langrails.APIError, got %T (%v)", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "not authorized" {
		t.Errorf("api error = %+v", apiErr)
	}
	if !apiErr.IsAuthError() {
		t.Error("expected auth error")
	}
}

func TestProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.amazon.eventstream")
		var stream bytes.Buffer
		stream.Write(frame(map[string]string{":event-type": "messageStart"}, []byte(`{"role":"assistant"}`)))
		stream.Write(frame(map[string]string{":event-type": "contentBlockDelta"}, []byte(`{"contentBlockIndex":0,"delta":{"text":"Hello"}}`)))
		stream.Write(frame(map[string]string{":event-type": "contentBlockDelta"}, []byte(`{"contentBlockIndex":0,"delta":{"text":" world"}}`)))
		stream.Write(frame(map[string]string{":event-type": "messageStop"}, []byte(`{"stopReason":"end_turn"}`)))
		stream.Write(frame(map[string]string{":event-type": "metadata"}, []byte(`{"usage":{"inputTokens":3,"outputTokens":2,"totalTokens":5}}`)))
		_, _ = w.Write(stream.Bytes())
	}))
	defer server.Close()

	p := testProvider(server.URL)
	ch, err := p.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "anthropic.claude",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var content strings.Builder
	var usage *langrails.TokenUsage
	var sawDone bool
	for ev := range ch {
		switch {
		case ev.Type == langrails.EventContent:
			content.WriteString(ev.Content)
		case ev.Type == langrails.EventDone:
			sawDone = true
		case ev.Type == langrails.EventError:
			t.Fatalf("unexpected error event: %v", ev.Error)
		}
		if ev.Usage != nil {
			usage = ev.Usage
		}
	}

	if content.String() != "Hello world" {
		t.Errorf("content = %q", content.String())
	}
	if !sawDone {
		t.Error("missing done event")
	}
	if usage == nil || usage.TotalTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestProvider_Stream_ToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var stream bytes.Buffer
		stream.Write(frame(map[string]string{":event-type": "contentBlockStart"},
			[]byte(`{"contentBlockIndex":0,"start":{"toolUse":{"toolUseId":"t1","name":"lookup"}}}`)))
		stream.Write(frame(map[string]string{":event-type": "contentBlockDelta"},
			[]byte(`{"contentBlockIndex":0,"delta":{"toolUse":{"input":"{\"q\":"}}}`)))
		stream.Write(frame(map[string]string{":event-type": "contentBlockDelta"},
			[]byte(`{"contentBlockIndex":0,"delta":{"toolUse":{"input":"42}"}}}`)))
		stream.Write(frame(map[string]string{":event-type": "messageStop"}, []byte(`{"stopReason":"tool_use"}`)))
		_, _ = w.Write(stream.Bytes())
	}))
	defer server.Close()

	p := testProvider(server.URL)
	ch, err := p.Stream(context.Background(), &langrails.CompletionRequest{
		Model:    "anthropic.claude",
		Messages: []langrails.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var tc *langrails.ToolCall
	for ev := range ch {
		if ev.Type == langrails.EventToolCall {
			tc = ev.ToolCall
		}
	}
	if tc == nil {
		t.Fatal("no tool call event")
	}
	if tc.ID != "t1" || tc.Name != "lookup" || tc.Arguments != `{"q":42}` {
		t.Errorf("tool call = %+v", tc)
	}
}

// frame builds an event-stream frame for tests (CRCs zeroed).
func frame(headers map[string]string, payload []byte) []byte {
	var hb bytes.Buffer
	for name, value := range headers {
		hb.WriteByte(byte(len(name)))
		hb.WriteString(name)
		hb.WriteByte(7) // string type
		var vlen [2]byte
		binary.BigEndian.PutUint16(vlen[:], uint16(len(value)))
		hb.Write(vlen[:])
		hb.WriteString(value)
	}
	headerBytes := hb.Bytes()
	total := 12 + len(headerBytes) + len(payload) + 4
	var buf bytes.Buffer
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(total))
	buf.Write(u32[:])
	binary.BigEndian.PutUint32(u32[:], uint32(len(headerBytes)))
	buf.Write(u32[:])
	buf.Write([]byte{0, 0, 0, 0})
	buf.Write(headerBytes)
	buf.Write(payload)
	buf.Write([]byte{0, 0, 0, 0})
	return buf.Bytes()
}

func TestProvider_ToolChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ToolConfig == nil || req.ToolConfig.ToolChoice == nil ||
			req.ToolConfig.ToolChoice.Tool == nil || req.ToolConfig.ToolChoice.Tool.Name != "lookup" {
			t.Errorf("toolChoice = %+v", req.ToolConfig)
		}
		resp := response{StopReason: "tool_use"}
		resp.Output.Message.Content = []responseContentBlock{{Text: "ok"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	_, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:      "anthropic.claude",
		Messages:   []langrails.Message{{Role: "user", Content: "hi"}},
		Tools:      []langrails.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: langrails.ForceTool("lookup"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_CachePointAndUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1]
		if last.Content[len(last.Content)-1].CachePoint == nil {
			t.Errorf("expected trailing cachePoint, got %+v", last.Content)
		}
		resp := response{
			StopReason: "end_turn",
			Usage:      usage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15, CacheReadInputTokens: 90, CacheWriteInputTokens: 40},
		}
		resp.Output.Message.Content = []responseContentBlock{{Text: "ok"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	resp, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:        "anthropic.claude",
		Messages:     []langrails.Message{{Role: "user", Content: "big"}},
		CacheControl: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Usage.CachedTokens != 90 || resp.Usage.CacheCreationTokens != 40 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestProvider_Vision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		blocks := req.Messages[0].Content
		if len(blocks) != 2 || blocks[1].Image == nil {
			t.Fatalf("expected image block, got %+v", blocks)
		}
		if blocks[1].Image.Format != "png" || blocks[1].Image.Source.Bytes != "AAAB" {
			t.Errorf("image = %+v", blocks[1].Image)
		}
		resp := response{StopReason: "end_turn"}
		resp.Output.Message.Content = []responseContentBlock{{Text: "ok"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	_, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model: "anthropic.claude",
		Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{
			langrails.TextPart("what is this"),
			langrails.ImageBase64Part("AAAB", "image/png"),
		}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvider_Reasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if len(req.AdditionalModelRequestFields) == 0 {
			t.Fatal("expected additionalModelRequestFields for reasoning")
		}
		if !strings.Contains(string(req.AdditionalModelRequestFields), `"thinking"`) {
			t.Errorf("additionalModelRequestFields = %s", req.AdditionalModelRequestFields)
		}
		if !strings.Contains(string(req.AdditionalModelRequestFields), "16384") {
			t.Errorf("expected high budget 16384, got %s", req.AdditionalModelRequestFields)
		}
		resp := response{StopReason: "end_turn"}
		resp.Output.Message.Content = []responseContentBlock{
			{ReasoningContent: &reasoningContentBlock{ReasoningText: &struct {
				Text      string `json:"text"`
				Signature string `json:"signature,omitempty"`
			}{Text: "thinking through it"}}},
			{Text: "the answer"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	resp, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:           "anthropic.claude-3-7-sonnet-20250219-v1:0",
		Messages:        []langrails.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: langrails.ReasoningHigh,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Thinking != "thinking through it" {
		t.Errorf("Thinking = %q", resp.Thinking)
	}
	if resp.Content != "the answer" {
		t.Errorf("Content = %q", resp.Content)
	}
}

func TestProvider_Stream_Reasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		var stream bytes.Buffer
		stream.Write(frame(map[string]string{":event-type": "contentBlockDelta"},
			[]byte(`{"contentBlockIndex":0,"delta":{"reasoningContent":{"text":"hmm"}}}`)))
		stream.Write(frame(map[string]string{":event-type": "contentBlockDelta"},
			[]byte(`{"contentBlockIndex":1,"delta":{"text":"answer"}}`)))
		stream.Write(frame(map[string]string{":event-type": "messageStop"}, []byte(`{"stopReason":"end_turn"}`)))
		_, _ = w.Write(stream.Bytes())
	}))
	defer server.Close()

	p := testProvider(server.URL)
	ch, err := p.Stream(context.Background(), &langrails.CompletionRequest{
		Model:           "anthropic.claude",
		Messages:        []langrails.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: langrails.ReasoningLow,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var reasoning, content string
	for ev := range ch {
		switch ev.Type {
		case langrails.EventReasoning:
			reasoning += ev.Reasoning
		case langrails.EventContent:
			content += ev.Content
		}
	}
	if reasoning != "hmm" {
		t.Errorf("reasoning = %q", reasoning)
	}
	if content != "answer" {
		t.Errorf("content = %q", content)
	}
}

func TestProvider_ToolChoiceNoneOmitsToolConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ToolConfig != nil {
			t.Errorf("expected no toolConfig for ToolChoiceNone, got %+v", req.ToolConfig)
		}
		resp := response{StopReason: "end_turn"}
		resp.Output.Message.Content = []responseContentBlock{{Text: "ok"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := testProvider(server.URL)
	_, err := p.Complete(context.Background(), &langrails.CompletionRequest{
		Model:      "anthropic.claude",
		Messages:   []langrails.Message{{Role: "user", Content: "hi"}},
		Tools:      []langrails.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ToolChoice: langrails.NoToolChoice(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
