package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/internal/sse"
)

// Config holds the configuration for an OpenAI-compatible provider.
type Config struct {
	// Name is the provider identifier (e.g., "openai", "deepseek").
	Name string

	// BaseURL is the full URL for the chat completions endpoint.
	BaseURL string

	// APIKey is the authentication key.
	APIKey string

	// ExtraHeaders are additional HTTP headers sent with every request.
	ExtraHeaders map[string]string

	// HTTPClient is an optional custom HTTP client. If nil, a default
	// client with a 5-minute timeout is used.
	HTTPClient *http.Client
}

// Provider implements langrails.Provider for OpenAI-compatible APIs.
type Provider struct {
	config Config
	client *http.Client
}

// New creates a new OpenAI-compatible provider with the given configuration.
func New(cfg Config) *Provider {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * 60 * 1e9} // 5 minutes
	}
	return &Provider{config: cfg, client: client}
}

// Complete sends a non-streaming completion request.
func (p *Provider) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	body, err := p.buildRequestBody(req, false)
	if err != nil {
		return nil, err
	}

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	raw, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read response: %w", p.config.Name, err)
	}

	var oaiResp response
	if err := json.Unmarshal(raw, &oaiResp); err != nil {
		return nil, fmt.Errorf("%s: failed to parse response: %w", p.config.Name, err)
	}

	return p.parseResponse(&oaiResp), nil
}

// Stream sends a streaming completion request and returns a channel of events.
func (p *Provider) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, err
	}

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan langrails.StreamEvent, 64)
	go p.readStream(ctx, respBody, ch)
	return ch, nil
}

func (p *Provider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create request: %w", p.config.Name, err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	for k, v := range p.config.ExtraHeaders {
		httpReq.Header.Set(k, v)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", p.config.Name, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)

		msg := fmt.Sprintf("status %d", resp.StatusCode)
		var errResp errorResponse
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}

		return nil, &langrails.APIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Provider:   p.config.Name,
		}
	}

	return resp.Body, nil
}

func (p *Provider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- langrails.StreamEvent) {
	defer close(ch)
	defer body.Close()

	reader := sse.NewReader(body)
	var pendingToolCalls []langrails.ToolCall

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		event, ok := reader.Next()
		if !ok {
			break
		}

		if event.Data == "[DONE]" {
			// Send accumulated tool calls if any
			for i := range pendingToolCalls {
				ch <- langrails.StreamEvent{
					Type:     langrails.EventToolCall,
					ToolCall: &pendingToolCalls[i],
				}
			}
			ch <- langrails.StreamEvent{Type: langrails.EventDone}
			return
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
			ch <- langrails.StreamEvent{
				Type:  langrails.EventError,
				Error: fmt.Errorf("%s: failed to parse stream chunk: %w", p.config.Name, err),
			}
			return
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta

		// Reasoning (emitted before content, matching provider order)
		if r := firstNonEmpty(delta.ReasoningContent, delta.Reasoning); r != "" {
			ch <- langrails.StreamEvent{
				Type:      langrails.EventReasoning,
				Reasoning: r,
			}
		}

		// Content
		if delta.Content != "" {
			ch <- langrails.StreamEvent{
				Type:    langrails.EventContent,
				Content: delta.Content,
			}
		}

		// Tool calls (accumulate across chunks)
		for _, tc := range delta.ToolCalls {
			for len(pendingToolCalls) <= tc.Index {
				pendingToolCalls = append(pendingToolCalls, langrails.ToolCall{})
			}
			if tc.ID != "" {
				pendingToolCalls[tc.Index].ID = tc.ID
			}
			if tc.Function.Name != "" {
				pendingToolCalls[tc.Index].Name = tc.Function.Name
			}
			pendingToolCalls[tc.Index].Arguments += tc.Function.Arguments
		}

		// Check finish reason
		if chunk.Choices[0].FinishReason == "stop" || chunk.Choices[0].FinishReason == "tool_calls" {
			// Usage may come in the final chunk
			if chunk.Usage != nil {
				u := toUsage(*chunk.Usage)
				ch <- langrails.StreamEvent{Usage: &u}
			}
		}
	}

	if err := reader.Err(); err != nil {
		ch <- langrails.StreamEvent{
			Type:  langrails.EventError,
			Error: fmt.Errorf("%s: stream read error: %w", p.config.Name, err),
		}
		return
	}

	// Stream ended without [DONE], send any pending tool calls
	for i := range pendingToolCalls {
		ch <- langrails.StreamEvent{
			Type:     langrails.EventToolCall,
			ToolCall: &pendingToolCalls[i],
		}
	}
	ch <- langrails.StreamEvent{Type: langrails.EventDone}
}

func (p *Provider) buildRequestBody(req *langrails.CompletionRequest, stream bool) ([]byte, error) {
	oaiReq := request{
		Model:    req.Model,
		Messages: convertMessages(req),
		Stream:   stream,
	}

	if req.Temperature != nil {
		oaiReq.Temperature = req.Temperature
	}
	if req.MaxTokens != nil {
		oaiReq.MaxTokens = req.MaxTokens
	}
	if req.TopP != nil {
		oaiReq.TopP = req.TopP
	}
	if req.FrequencyPenalty != nil {
		oaiReq.FrequencyPenalty = req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		oaiReq.PresencePenalty = req.PresencePenalty
	}
	if len(req.Stop) > 0 {
		oaiReq.Stop = req.Stop
	}
	if req.Seed != nil {
		oaiReq.Seed = req.Seed
	}

	// Reasoning effort. An explicit ReasoningEffort wins; otherwise fall back to
	// the legacy Thinking + ThinkingBudget heuristic for backward compatibility.
	if req.ReasoningEffort != "" {
		oaiReq.Reasoning = &reasoningParam{Effort: string(req.ReasoningEffort)}
	} else if req.Thinking {
		effort := "medium"
		if req.ThinkingBudget != nil {
			if *req.ThinkingBudget <= 1024 {
				effort = "low"
			} else if *req.ThinkingBudget >= 16384 {
				effort = "high"
			}
		}
		oaiReq.Reasoning = &reasoningParam{Effort: effort}
	}

	if len(req.Tools) > 0 {
		oaiReq.Tools = convertTools(req.Tools)
	}
	if tc := convertToolChoice(req.ToolChoice); tc != nil {
		oaiReq.ToolChoice = tc
	}

	// Built-in web search (OpenAI web_search_options). Other compat providers
	// (e.g. Perplexity sonar) search implicitly by model and ignore this field
	// but still return citations, which are parsed in parseResponse.
	//
	// Note: The unified WebSearchOptions fields (MaxUses, AllowedDomains,
	// BlockedDomains, UserLocation) are Anthropic/Gemini-specific and have no
	// equivalent in the OpenAI-compatible web_search_options endpoint. They
	// are silently ignored here; only the presence of a web search server tool
	// is honoured (enabling the provider's built-in search).
	if hasWebSearch(req.ServerTools) {
		oaiReq.WebSearchOptions = &webSearchOptions{}
	}

	switch {
	case req.OutputSchema != nil:
		schema := enforceStrictSchema(*req.OutputSchema)
		oaiReq.ResponseFormat = &responseFormat{
			Type: "json_schema",
			JSONSchema: &jsonSchemaParam{
				Name:   "response",
				Schema: schema,
				Strict: true,
			},
		}
	case req.ResponseFormat == langrails.ResponseFormatJSONObject:
		oaiReq.ResponseFormat = &responseFormat{Type: "json_object"}
	}

	return json.Marshal(oaiReq)
}

// convertToolChoice maps the unified ToolChoice to the OpenAI tool_choice value
// (a string for auto/none/required, or an object for a specific tool). Returns
// nil when no tool choice is set.
func convertToolChoice(tc *langrails.ToolChoice) interface{} {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case langrails.ToolChoiceAuto:
		return "auto"
	case langrails.ToolChoiceNone:
		return "none"
	case langrails.ToolChoiceRequired:
		return "required"
	case langrails.ToolChoiceTool:
		var fn toolChoiceFunction
		fn.Type = "function"
		fn.Function.Name = tc.Name
		return fn
	default:
		return nil
	}
}

// firstNonEmpty returns the first non-empty string from values.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// toUsage maps the OpenAI usage object (including the optional cached/reasoning
// token details) to the unified TokenUsage.
func toUsage(u usage) langrails.TokenUsage {
	tu := langrails.TokenUsage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
	if u.PromptTokensDetails != nil {
		tu.CachedTokens = u.PromptTokensDetails.CachedTokens
	}
	if u.CompletionTokensDetails != nil {
		tu.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
	}
	return tu
}

func (p *Provider) parseResponse(resp *response) *langrails.CompletionResponse {
	result := &langrails.CompletionResponse{
		Model: resp.Model,
		Usage: toUsage(resp.Usage),
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.Thinking = firstNonEmpty(choice.Message.ReasoningContent, choice.Message.Reasoning)
		result.FinishReason = choice.FinishReason

		for _, tc := range choice.Message.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, langrails.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		// OpenAI-style inline url_citation annotations.
		for _, a := range choice.Message.Annotations {
			if a.Type == "url_citation" && a.URLCitation != nil {
				result.Citations = append(result.Citations, langrails.Citation{
					URL:        a.URLCitation.URL,
					Title:      a.URLCitation.Title,
					StartIndex: a.URLCitation.StartIndex,
					EndIndex:   a.URLCitation.EndIndex,
				})
			}
		}
	}

	// Perplexity-style top-level citation URLs.
	for _, u := range resp.Citations {
		result.Citations = append(result.Citations, langrails.Citation{URL: u})
	}

	return result
}

// hasWebSearch reports whether the server tools include web search.
func hasWebSearch(tools []langrails.ServerTool) bool {
	for _, st := range tools {
		if st.Type == langrails.ServerToolWebSearch {
			return true
		}
	}
	return false
}

func convertMessages(req *langrails.CompletionRequest) []message {
	var msgs []message

	if req.SystemPrompt != "" {
		msgs = append(msgs, message{
			Role:    "system",
			Content: req.SystemPrompt,
		})
	}

	for _, m := range req.Messages {
		msg := message{
			Role: m.Role,
		}

		// Multimodal content parts
		if len(m.ContentParts) > 0 {
			parts := make([]contentPart, 0, len(m.ContentParts))
			for _, p := range m.ContentParts {
				switch p.Type {
				case "text":
					parts = append(parts, contentPart{Type: "text", Text: p.Text})
				case "image":
					parts = append(parts, contentPart{
						Type:     "image_url",
						ImageURL: &imageURL{URL: p.ImageURL},
					})
				}
			}
			msg.Content = parts
		} else {
			msg.Content = m.Content
		}

		if m.ToolCallID != "" {
			msg.ToolCallID = m.ToolCallID
		}

		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, toolCall{
					ID:   tc.ID,
					Type: "function",
					Function: functionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}

		msgs = append(msgs, msg)
	}

	return msgs
}

func convertTools(tools []langrails.ToolDefinition) []tool {
	result := make([]tool, len(tools))
	for i, t := range tools {
		result[i] = tool{
			Type: "function",
			Function: functionDef{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return result
}

// enforceStrictSchema ensures the JSON schema has additionalProperties: false
// at the top level, which is required by OpenAI's strict mode.
func enforceStrictSchema(schema []byte) json.RawMessage {
	var s map[string]interface{}
	if err := json.Unmarshal(schema, &s); err != nil {
		return schema
	}
	s["additionalProperties"] = false
	out, err := json.Marshal(s)
	if err != nil {
		return schema
	}
	return out
}
