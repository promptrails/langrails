package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/internal/mediautil"
	"github.com/promptrails/langrails/internal/sse"
	"github.com/promptrails/langrails/llm/compat"
)

// reasoningKey is the ToolCall.Metadata key that carries the response's
// encrypted reasoning items to the next request, so a reasoning model keeps
// its chain of thought across tool calls without server-side storage.
const reasoningKey = "openai_reasoning"

// responsesMedia is what the Responses input format can carry: documents
// inline or by URL; no audio.
var responsesMedia = mediautil.Support{DocumentData: true, DocumentURL: true}

// responsesClient speaks the Responses API (POST /v1/responses).
type responsesClient struct {
	url     string
	apiKey  string
	headers map[string]string
	client  *http.Client
}

func newResponsesClient(cfg compat.Config) *responsesClient {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &responsesClient{url: cfg.BaseURL, apiKey: cfg.APIKey, headers: cfg.ExtraHeaders, client: client}
}

// --- request ---

type rRequest struct {
	Model           string      `json:"model"`
	Instructions    string      `json:"instructions,omitempty"`
	Input           []rItem     `json:"input"`
	Tools           []rTool     `json:"tools,omitempty"`
	ToolChoice      any         `json:"tool_choice,omitempty"`
	Temperature     *float64    `json:"temperature,omitempty"`
	TopP            *float64    `json:"top_p,omitempty"`
	MaxOutputTokens *int        `json:"max_output_tokens,omitempty"`
	Reasoning       *rReasoning `json:"reasoning,omitempty"`
	Text            *rText      `json:"text,omitempty"`
	Include         []string    `json:"include,omitempty"`
	Store           bool        `json:"store"`
	Stream          bool        `json:"stream,omitempty"`
}

// rItem is an input item: a message, a function call or its output, or a
// reasoning item replayed from an earlier response.
type rItem struct {
	Type    string  `json:"type,omitempty"`
	Role    string  `json:"role,omitempty"`
	Content []rPart `json:"content,omitempty"`

	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`

	// raw replays a reasoning item verbatim.
	raw json.RawMessage
}

func (it rItem) MarshalJSON() ([]byte, error) {
	if it.raw != nil {
		return it.raw, nil
	}
	type plain rItem
	return json.Marshal(plain(it))
}

type rPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
	FileData string `json:"file_data,omitempty"`
	FileURL  string `json:"file_url,omitempty"`
	Filename string `json:"filename,omitempty"`
}

type rTool struct {
	Type              string          `json:"type"`
	Name              string          `json:"name,omitempty"`
	Description       string          `json:"description,omitempty"`
	Parameters        json.RawMessage `json:"parameters,omitempty"`
	Strict            *bool           `json:"strict,omitempty"`
	Filters           *rSearchFilters `json:"filters,omitempty"`
	UserLocation      *rUserLocation  `json:"user_location,omitempty"`
	SearchContextSize string          `json:"search_context_size,omitempty"`
}

type rSearchFilters struct {
	AllowedDomains []string `json:"allowed_domains,omitempty"`
}

type rUserLocation struct {
	Type    string `json:"type"`
	Country string `json:"country,omitempty"`
}

type rReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type rText struct {
	Format rFormat `json:"format"`
}

type rFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name,omitempty"`
	Schema json.RawMessage `json:"schema,omitempty"`
	Strict bool            `json:"strict,omitempty"`
}

func (c *responsesClient) buildRequest(req *langrails.CompletionRequest, stream bool) ([]byte, error) {
	if err := mediautil.CheckParts("openai", req, responsesMedia); err != nil {
		return nil, err
	}
	input, err := toInput(req.Messages)
	if err != nil {
		return nil, err
	}
	r := rRequest{
		Model:           req.Model,
		Instructions:    req.SystemPrompt,
		Input:           input,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxOutputTokens: req.MaxTokens,
		Stream:          stream,
		// Nothing is stored server-side; reasoning is carried in the
		// conversation as encrypted content instead.
		Store: false,
	}

	effort := string(req.ReasoningEffort)
	if effort == "" && req.ReasoningBudget != nil {
		effort = "medium"
		if *req.ReasoningBudget <= 1024 {
			effort = "low"
		} else if *req.ReasoningBudget >= 16384 {
			effort = "high"
		}
	}
	if effort != "" {
		r.Reasoning = &rReasoning{Effort: effort}
		if effort != string(langrails.ReasoningNone) {
			r.Reasoning.Summary = "auto"
			r.Include = []string{"reasoning.encrypted_content"}
		}
	}

	for _, t := range req.Tools {
		r.Tools = append(r.Tools, rTool{Type: "function", Name: t.Name, Description: t.Description, Parameters: t.Parameters})
	}
	for _, st := range req.ServerTools {
		if st.Type != langrails.ServerToolWebSearch {
			continue
		}
		ws := rTool{Type: "web_search"}
		if o := st.WebSearch; o != nil {
			if len(o.AllowedDomains) > 0 {
				ws.Filters = &rSearchFilters{AllowedDomains: o.AllowedDomains}
			}
			if o.UserLocation != "" {
				ws.UserLocation = &rUserLocation{Type: "approximate", Country: o.UserLocation}
			}
		}
		r.Tools = append(r.Tools, ws)
	}
	if tc := req.ToolChoice; tc != nil {
		switch tc.Mode {
		case langrails.ToolChoiceAuto, langrails.ToolChoiceNone, langrails.ToolChoiceRequired:
			r.ToolChoice = string(tc.Mode)
		case langrails.ToolChoiceTool:
			r.ToolChoice = map[string]string{"type": "function", "name": tc.Name}
		}
	}

	switch {
	case len(req.OutputSchema) > 0:
		r.Text = &rText{Format: rFormat{Type: "json_schema", Name: "response", Schema: compat.StrictSchema(req.OutputSchema), Strict: true}}
	case req.ResponseFormat == langrails.ResponseFormatJSONObject:
		r.Text = &rText{Format: rFormat{Type: "json_object"}}
	}
	return json.Marshal(r)
}

// toInput converts the conversation into Responses input items.
func toInput(msgs []langrails.Message) ([]rItem, error) {
	var items []rItem
	for _, m := range msgs {
		switch m.Role {
		case langrails.RoleTool:
			items = append(items, rItem{Type: "function_call_output", CallID: m.ToolCallID, Output: m.Content})
			continue
		case langrails.RoleAssistant:
			// Reasoning items from the response that made these calls come
			// first, as the model produced them.
			if len(m.ToolCalls) > 0 {
				if raw := m.ToolCalls[0].Metadata[reasoningKey]; raw != "" {
					var reasoning []json.RawMessage
					if err := json.Unmarshal([]byte(raw), &reasoning); err != nil {
						return nil, fmt.Errorf("openai: invalid %s metadata: %w", reasoningKey, err)
					}
					for _, ri := range reasoning {
						items = append(items, rItem{raw: ri})
					}
				}
			}
			if text := m.Content; text != "" {
				items = append(items, rItem{Type: "message", Role: "assistant", Content: []rPart{{Type: "output_text", Text: text}}})
			}
			for _, tc := range m.ToolCalls {
				items = append(items, rItem{Type: "function_call", CallID: tc.ID, Name: tc.Name, Arguments: tc.Arguments})
			}
			continue
		}

		role := string(m.Role)
		if m.Role == langrails.RoleSystem {
			role = "developer"
		}
		item := rItem{Type: "message", Role: role}
		if len(m.ContentParts) == 0 {
			item.Content = []rPart{{Type: "input_text", Text: m.Content}}
		}
		for i, p := range m.ContentParts {
			switch p.Type {
			case langrails.ContentText:
				item.Content = append(item.Content, rPart{Type: "input_text", Text: p.Text})
			case langrails.ContentImage:
				item.Content = append(item.Content, rPart{Type: "input_image", ImageURL: p.ImageURL})
			case langrails.ContentDocument:
				part := rPart{Type: "input_file", Filename: mediautil.Filename(p.Filename, i+1)}
				if p.Data != "" {
					part.FileData = mediautil.DataURI(p.MediaType, p.Data)
				} else {
					part.FileURL = p.URL
					part.Filename = ""
				}
				item.Content = append(item.Content, part)
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// --- response ---

type rResponse struct {
	Model             string `json:"model"`
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Output []json.RawMessage `json:"output"`
	Usage  *rUsage           `json:"usage"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type rOutputItem struct {
	Type      string `json:"type"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type        string        `json:"type"`
		Text        string        `json:"text"`
		Refusal     string        `json:"refusal"`
		Annotations []rAnnotation `json:"annotations"`
	} `json:"content"`
	Summary []struct {
		Text string `json:"text"`
	} `json:"summary"`
}

type rAnnotation struct {
	Type       string `json:"type"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	StartIndex int    `json:"start_index"`
	EndIndex   int    `json:"end_index"`
}

type rUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func (u *rUsage) toUsage() langrails.TokenUsage {
	if u == nil {
		return langrails.TokenUsage{}
	}
	return langrails.TokenUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.TotalTokens,
		CachedTokens:     u.InputTokensDetails.CachedTokens,
		ReasoningTokens:  u.OutputTokensDetails.ReasoningTokens,
	}
}

func toAnnotation(a rAnnotation) (langrails.Citation, bool) {
	if a.Type != "url_citation" {
		return langrails.Citation{}, false
	}
	return langrails.Citation{URL: a.URL, Title: a.Title, StartIndex: a.StartIndex, EndIndex: a.EndIndex}, true
}

// parseOutput converts a finished response into a CompletionResponse.
func parseOutput(r *rResponse) (*langrails.CompletionResponse, error) {
	out := &langrails.CompletionResponse{Model: r.Model, Usage: r.Usage.toUsage()}
	var text, thinking strings.Builder
	var reasoning []json.RawMessage
	for _, raw := range r.Output {
		var it rOutputItem
		if err := json.Unmarshal(raw, &it); err != nil {
			return nil, fmt.Errorf("openai: failed to parse output item: %w", err)
		}
		switch it.Type {
		case "reasoning":
			reasoning = append(reasoning, raw)
			for _, s := range it.Summary {
				thinking.WriteString(s.Text)
			}
		case "message":
			for _, c := range it.Content {
				text.WriteString(c.Text)
				text.WriteString(c.Refusal)
				for _, a := range c.Annotations {
					if cit, ok := toAnnotation(a); ok {
						out.Citations = append(out.Citations, cit)
					}
				}
			}
		case "function_call":
			out.ToolCalls = append(out.ToolCalls, langrails.ToolCall{ID: it.CallID, Name: it.Name, Arguments: it.Arguments})
		}
	}
	out.Content = text.String()
	out.Thinking = thinking.String()
	if err := attachReasoning(out.ToolCalls, reasoning); err != nil {
		return nil, err
	}
	out.FinishReason = finishReason(r, len(out.ToolCalls) > 0)
	return out, nil
}

// attachReasoning stores the response's reasoning items on its first tool
// call, which the caller sends back with the tool results.
func attachReasoning(calls []langrails.ToolCall, reasoning []json.RawMessage) error {
	if len(calls) == 0 || len(reasoning) == 0 {
		return nil
	}
	b, err := json.Marshal(reasoning)
	if err != nil {
		return err
	}
	if calls[0].Metadata == nil {
		calls[0].Metadata = map[string]string{}
	}
	calls[0].Metadata[reasoningKey] = string(b)
	return nil
}

func finishReason(r *rResponse, toolCalls bool) string {
	switch {
	case toolCalls:
		return "tool_calls"
	case r.Status == "incomplete" && r.IncompleteDetails != nil && r.IncompleteDetails.Reason == "max_output_tokens":
		return "length"
	case r.Status == "incomplete" && r.IncompleteDetails != nil && r.IncompleteDetails.Reason == "content_filter":
		return "content_filter"
	}
	return "stop"
}

// --- transport ---

func (c *responsesClient) do(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		msg := fmt.Sprintf("status %d", resp.StatusCode)
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		return nil, &langrails.APIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Provider:   "openai",
			RetryAfter: langrails.RetryAfterFromHeader(resp.Header),
		}
	}
	return resp.Body, nil
}

func (c *responsesClient) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	body, err := c.buildRequest(req, false)
	if err != nil {
		return nil, err
	}
	rc, err := c.do(ctx, body)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("openai: failed to read response: %w", err)
	}
	var r rResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("openai: failed to parse response: %w", err)
	}
	if r.Status == "failed" && r.Error != nil {
		return nil, fmt.Errorf("openai: response failed: %s", r.Error.Message)
	}
	return parseOutput(&r)
}

func (c *responsesClient) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	body, err := c.buildRequest(req, true)
	if err != nil {
		return nil, err
	}
	rc, err := c.do(ctx, body)
	if err != nil {
		return nil, err
	}
	ch := make(chan langrails.StreamEvent, 64)
	go c.readStream(ctx, rc, ch)
	return ch, nil
}

type rStreamEvent struct {
	Type        string          `json:"type"`
	Delta       string          `json:"delta"`
	OutputIndex int             `json:"output_index"`
	Item        json.RawMessage `json:"item"`
	Annotation  *rAnnotation    `json:"annotation"`
	Response    *rResponse      `json:"response"`
	Message     string          `json:"message"`
}

func (c *responsesClient) readStream(ctx context.Context, body io.ReadCloser, ch chan<- langrails.StreamEvent) {
	defer close(ch)
	defer body.Close()

	send := func(ev langrails.StreamEvent) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	// Function calls are numbered among the response's tool calls, in the
	// order their output items start.
	callIndex := map[int]int{}
	var calls []langrails.ToolCall
	var reasoning []json.RawMessage

	reader := sse.NewReader(body)
	for {
		event, ok := reader.Next()
		if !ok {
			break
		}
		var se rStreamEvent
		if err := json.Unmarshal([]byte(event.Data), &se); err != nil {
			continue
		}

		switch se.Type {
		case "response.output_text.delta", "response.refusal.delta":
			if se.Delta != "" && !send(langrails.StreamEvent{Type: langrails.EventContent, Content: se.Delta}) {
				return
			}
		case "response.reasoning_summary_text.delta":
			if se.Delta != "" && !send(langrails.StreamEvent{Type: langrails.EventReasoning, Reasoning: se.Delta}) {
				return
			}
		case "response.output_text.annotation.added":
			if se.Annotation != nil {
				if cit, ok := toAnnotation(*se.Annotation); ok && !send(langrails.StreamEvent{Type: langrails.EventCitation, Citation: &cit}) {
					return
				}
			}
		case "response.output_item.added":
			var it rOutputItem
			if json.Unmarshal(se.Item, &it) == nil && it.Type == "function_call" {
				idx := len(callIndex)
				callIndex[se.OutputIndex] = idx
				if !send(langrails.StreamEvent{Type: langrails.EventToolCallDelta, ToolCallDelta: &langrails.ToolCallDelta{
					Index: idx, ID: it.CallID, Name: it.Name, Arguments: it.Arguments,
				}}) {
					return
				}
			}
		case "response.function_call_arguments.delta":
			if idx, ok := callIndex[se.OutputIndex]; ok && se.Delta != "" {
				if !send(langrails.StreamEvent{Type: langrails.EventToolCallDelta, ToolCallDelta: &langrails.ToolCallDelta{
					Index: idx, Arguments: se.Delta,
				}}) {
					return
				}
			}
		case "response.output_item.done":
			var it rOutputItem
			if json.Unmarshal(se.Item, &it) != nil {
				continue
			}
			switch it.Type {
			case "function_call":
				calls = append(calls, langrails.ToolCall{ID: it.CallID, Name: it.Name, Arguments: it.Arguments})
			case "reasoning":
				reasoning = append(reasoning, se.Item)
			}
		case "response.completed", "response.incomplete":
			if err := attachReasoning(calls, reasoning); err != nil {
				send(langrails.StreamEvent{Type: langrails.EventError, Error: err})
				return
			}
			for i := range calls {
				if !send(langrails.StreamEvent{Type: langrails.EventToolCall, ToolCall: &calls[i]}) {
					return
				}
			}
			done := langrails.StreamEvent{Type: langrails.EventDone}
			if se.Response != nil && se.Response.Usage != nil {
				u := se.Response.Usage.toUsage()
				done.Usage = &u
			}
			send(done)
			return
		case "response.failed":
			msg := "response failed"
			if se.Response != nil && se.Response.Error != nil {
				msg = se.Response.Error.Message
			}
			send(langrails.StreamEvent{Type: langrails.EventError, Error: fmt.Errorf("openai: %s", msg)})
			return
		case "error":
			send(langrails.StreamEvent{Type: langrails.EventError, Error: fmt.Errorf("openai: %s", se.Message)})
			return
		}
	}

	if err := reader.Err(); err != nil {
		send(langrails.StreamEvent{Type: langrails.EventError, Error: fmt.Errorf("openai: stream read error: %w", err)})
		return
	}
	send(langrails.StreamEvent{Type: langrails.EventError, Error: fmt.Errorf("openai: stream ended before the response completed")})
}
