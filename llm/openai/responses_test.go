package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func TestWithResponsesAPI_SelectsEndpoint(t *testing.T) {
	tests := []struct {
		opts      []Option
		responses bool
	}{
		{nil, false},
		{[]Option{WithResponsesAPI()}, true},
		{[]Option{WithBaseURL("https://proxy/v1/chat/completions"), WithResponsesAPI()}, true},
		{[]Option{WithBaseURL("https://proxy/v1/responses")}, true},
	}
	for i, tt := range tests {
		p := New("k", tt.opts...)
		if (p.responses != nil) != tt.responses {
			t.Errorf("case %d: responses = %v", i, p.responses != nil)
		}
	}
	if p := New("k", WithResponsesAPI()); p.responses.url != "https://api.openai.com/v1/responses" {
		t.Errorf("url = %q", p.responses.url)
	}
}

func TestResponses_Request(t *testing.T) {
	reasoning := `[{"type":"reasoning","id":"rs_1","encrypted_content":"enc","summary":[]}]`
	temp := 0.2
	req := &langrails.CompletionRequest{
		Model:        "gpt-6",
		SystemPrompt: "be brief",
		Temperature:  &temp,
		Messages: []langrails.Message{
			{Role: langrails.RoleSystem, Content: "policy"},
			{Role: langrails.RoleUser, ContentParts: []langrails.ContentPart{
				langrails.TextPart("look"),
				langrails.ImageURLPart("https://x/y.png"),
				langrails.DocumentPart("UERG", "application/pdf", "a.pdf"),
			}},
			{Role: langrails.RoleAssistant, Content: "checking", ToolCalls: []langrails.ToolCall{
				{ID: "call_1", Name: "lookup", Arguments: `{"q":1}`, Metadata: map[string]string{reasoningKey: reasoning}},
			}},
			{Role: langrails.RoleTool, ToolCallID: "call_1", Content: "42"},
		},
		Tools:           []langrails.ToolDefinition{{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}},
		ServerTools:     []langrails.ServerTool{langrails.WebSearch(&langrails.WebSearchOptions{AllowedDomains: []string{"go.dev"}, UserLocation: "TR"})},
		ToolChoice:      &langrails.ToolChoice{Mode: langrails.ToolChoiceTool, Name: "lookup"},
		ReasoningEffort: langrails.ReasoningHigh,
		OutputSchema:    json.RawMessage(`{"type":"object","properties":{"a":{"type":"string"}}}`),
	}
	body, err := (&responsesClient{}).buildRequest(req, true)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}

	if got["instructions"] != "be brief" || got["store"] != false || got["stream"] != true || got["temperature"] != 0.2 {
		t.Errorf("top level = %v", got)
	}
	r := got["reasoning"].(map[string]any)
	if r["effort"] != "high" || r["summary"] != "auto" || got["include"].([]any)[0] != "reasoning.encrypted_content" {
		t.Errorf("reasoning = %v include = %v", r, got["include"])
	}

	input := got["input"].([]any)
	types := []string{}
	for _, it := range input {
		m := it.(map[string]any)
		types = append(types, m["type"].(string))
	}
	if strings.Join(types, ",") != "message,message,reasoning,message,function_call,function_call_output" {
		t.Fatalf("input types = %v", types)
	}
	if input[0].(map[string]any)["role"] != "developer" {
		t.Errorf("system message role = %v", input[0])
	}
	parts := input[1].(map[string]any)["content"].([]any)
	if parts[1].(map[string]any)["type"] != "input_image" || parts[2].(map[string]any)["file_data"] != "data:application/pdf;base64,UERG" {
		t.Errorf("parts = %v", parts)
	}
	if input[2].(map[string]any)["encrypted_content"] != "enc" {
		t.Errorf("reasoning not replayed: %v", input[2])
	}
	if fc := input[4].(map[string]any); fc["call_id"] != "call_1" || fc["arguments"] != `{"q":1}` {
		t.Errorf("function_call = %v", fc)
	}
	if out := input[5].(map[string]any); out["call_id"] != "call_1" || out["output"] != "42" {
		t.Errorf("function_call_output = %v", out)
	}

	tools := got["tools"].([]any)
	if tools[0].(map[string]any)["name"] != "lookup" || tools[1].(map[string]any)["type"] != "web_search" {
		t.Errorf("tools = %v", tools)
	}
	if loc := tools[1].(map[string]any)["user_location"].(map[string]any); loc["country"] != "TR" {
		t.Errorf("web search = %v", tools[1])
	}
	if tc := got["tool_choice"].(map[string]any); tc["name"] != "lookup" {
		t.Errorf("tool_choice = %v", tc)
	}
	format := got["text"].(map[string]any)["format"].(map[string]any)
	if format["type"] != "json_schema" || format["strict"] != true || !strings.Contains(string(body), `"additionalProperties":false`) {
		t.Errorf("format = %v", format)
	}
}

func TestResponses_RequestReasoningNone(t *testing.T) {
	body, err := (&responsesClient{}).buildRequest(&langrails.CompletionRequest{
		Model: "m", Messages: []langrails.Message{{Role: "user", Content: "hi"}}, ReasoningEffort: langrails.ReasoningNone,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"reasoning":{"effort":"none"}`) || strings.Contains(string(body), "include") {
		t.Errorf("body = %s", body)
	}
}

func TestResponses_RejectsAudio(t *testing.T) {
	_, err := (&responsesClient{}).buildRequest(&langrails.CompletionRequest{Messages: []langrails.Message{
		{Role: "user", ContentParts: []langrails.ContentPart{langrails.AudioPart("x", "audio/wav")}},
	}}, false)
	if !errors.Is(err, langrails.ErrUnsupportedContent) {
		t.Fatalf("err = %v", err)
	}
}

func responsesServer(t *testing.T, handler func(w http.ResponseWriter, body map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("path = %s auth = %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		handler(w, body)
	}))
}

func TestResponses_Complete(t *testing.T) {
	srv := responsesServer(t, func(w http.ResponseWriter, _ map[string]any) {
		_, _ = w.Write([]byte(`{
			"model": "gpt-6-2026",
			"status": "completed",
			"output": [
				{"type": "reasoning", "id": "rs_1", "encrypted_content": "enc", "summary": [{"type": "summary_text", "text": "thinking..."}]},
				{"type": "web_search_call", "id": "ws_1", "status": "completed"},
				{"type": "message", "role": "assistant", "content": [{"type": "output_text", "text": "Go 1.27 is out.",
					"annotations": [{"type": "url_citation", "url": "https://go.dev", "title": "Go", "start_index": 0, "end_index": 6}]}]},
				{"type": "function_call", "call_id": "call_1", "name": "lookup", "arguments": "{}"}
			],
			"usage": {"input_tokens": 10, "output_tokens": 20, "total_tokens": 30,
				"input_tokens_details": {"cached_tokens": 4}, "output_tokens_details": {"reasoning_tokens": 12}}
		}`))
	})
	defer srv.Close()

	p := New("k", WithBaseURL(srv.URL+"/v1/chat/completions"), WithResponsesAPI())
	resp, err := p.Complete(context.Background(), &langrails.CompletionRequest{Model: "gpt-6", Messages: []langrails.Message{{Role: "user", Content: "news?"}}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content != "Go 1.27 is out." || resp.Thinking != "thinking..." || resp.Model != "gpt-6-2026" || resp.FinishReason != "tool_calls" {
		t.Errorf("resp = %+v", resp)
	}
	if len(resp.Citations) != 1 || resp.Citations[0].URL != "https://go.dev" || resp.Citations[0].EndIndex != 6 {
		t.Errorf("citations = %+v", resp.Citations)
	}
	if u := resp.Usage; u.TotalTokens != 30 || u.CachedTokens != 4 || u.ReasoningTokens != 12 {
		t.Errorf("usage = %+v", u)
	}
	if len(resp.ToolCalls) != 1 || !strings.Contains(resp.ToolCalls[0].Metadata[reasoningKey], `"enc"`) {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
}

func TestResponses_FinishReasons(t *testing.T) {
	if got := finishReason(&rResponse{Status: "incomplete", IncompleteDetails: &struct {
		Reason string `json:"reason"`
	}{"max_output_tokens"}}, false); got != "length" {
		t.Errorf("got %q", got)
	}
	if got := finishReason(&rResponse{Status: "completed"}, false); got != "stop" {
		t.Errorf("got %q", got)
	}
}

func TestResponses_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": {"message": "bad input"}}`))
	}))
	defer srv.Close()
	_, err := New("k", WithBaseURL(srv.URL+"/responses")).Complete(context.Background(), &langrails.CompletionRequest{Model: "m"})
	var apiErr *langrails.APIError
	if !errors.As(err, &apiErr) || apiErr.Message != "bad input" || apiErr.StatusCode != 400 {
		t.Fatalf("err = %v", err)
	}
}

func TestResponses_Stream(t *testing.T) {
	events := []string{
		`{"type":"response.created","response":{}}`,
		`{"type":"response.reasoning_summary_text.delta","delta":"hmm"}`,
		`{"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","id":"rs_1","encrypted_content":"enc"}}`,
		`{"type":"response.output_text.delta","delta":"Hel"}`,
		`{"type":"response.output_text.delta","delta":"lo"}`,
		`{"type":"response.output_text.annotation.added","annotation":{"type":"url_citation","url":"https://a"}}`,
		`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","call_id":"call_1","name":"lookup","arguments":""}}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"{\"q\":"}`,
		`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"1}"}`,
		`{"type":"response.output_item.done","output_index":2,"item":{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{\"q\":1}"}}`,
		`{"type":"response.completed","response":{"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}}`,
	}
	srv := responsesServer(t, func(w http.ResponseWriter, body map[string]any) {
		if body["stream"] != true {
			t.Error("stream not requested")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			_, _ = w.Write([]byte("event: x\ndata: " + e + "\n\n"))
		}
	})
	defer srv.Close()

	ch, err := New("k", WithBaseURL(srv.URL+"/v1/responses")).Stream(context.Background(), &langrails.CompletionRequest{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var deltas []langrails.ToolCallDelta
	var done bool
	var acc langrails.StreamAccumulator
	for ev := range ch {
		acc.Add(ev)
		if ev.Type == langrails.EventToolCallDelta {
			deltas = append(deltas, *ev.ToolCallDelta)
		}
		if ev.Type == langrails.EventDone {
			done = true
		}
	}
	resp := acc.Response()
	if acc.Err() != nil || !done || resp.Content != "Hello" || resp.Thinking != "hmm" || resp.Usage.TotalTokens != 7 || len(resp.Citations) != 1 {
		t.Fatalf("resp = %+v, err = %v", resp, acc.Err())
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Arguments != `{"q":1}` || !strings.Contains(resp.ToolCalls[0].Metadata[reasoningKey], "enc") {
		t.Errorf("tool calls = %+v", resp.ToolCalls)
	}
	if len(deltas) != 3 || deltas[0].Name != "lookup" || deltas[0].Index != 0 || deltas[1].Arguments+deltas[2].Arguments != `{"q":1}` {
		t.Errorf("deltas = %+v", deltas)
	}
}

func TestResponses_StreamFailure(t *testing.T) {
	for _, tt := range []struct{ events, want string }{
		{`data: {"type":"response.failed","response":{"error":{"message":"overloaded"}}}` + "\n\n", "overloaded"},
		{`data: {"type":"error","message":"boom"}` + "\n\n", "boom"},
		{`data: {"type":"response.output_text.delta","delta":"x"}` + "\n\n", "before the response completed"},
	} {
		srv := responsesServer(t, func(w http.ResponseWriter, _ map[string]any) { _, _ = w.Write([]byte(tt.events)) })
		ch, err := New("k", WithBaseURL(srv.URL+"/v1/responses")).Stream(context.Background(), &langrails.CompletionRequest{Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = langrails.CollectStream(ch)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("err = %v, want %q", err, tt.want)
		}
		srv.Close()
	}
}
