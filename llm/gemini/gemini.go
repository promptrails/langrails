package gemini

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

const (
	defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"
)

// Provider implements langrails.Provider for Google's Gemini API.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// Option configures the Gemini provider.
type Option func(*Provider)

// WithBaseURL sets a custom base URL.
func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// New creates a new Gemini provider with the given API key and options.
func New(apiKey string, opts ...Option) *Provider {
	p := &Provider{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		client:  &http.Client{Timeout: 5 * 60 * 1e9},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Complete sends a non-streaming completion request.
func (p *Provider) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", p.baseURL, req.Model, p.apiKey)

	respBody, err := p.doRequest(ctx, url, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	raw, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to read response: %w", err)
	}

	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("gemini: failed to parse response: %w", err)
	}

	return p.parseResponse(&resp), nil
}

// Stream sends a streaming completion request and returns a channel of events.
func (p *Provider) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	body, err := p.buildRequestBody(req)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse&key=%s", p.baseURL, req.Model, p.apiKey)

	respBody, err := p.doRequest(ctx, url, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan langrails.StreamEvent, 64)
	go p.readStream(respBody, ch)
	return ch, nil
}

func (p *Provider) doRequest(ctx context.Context, url string, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
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
			Provider:   "gemini",
		}
	}

	return resp.Body, nil
}

func (p *Provider) readStream(body io.ReadCloser, ch chan<- langrails.StreamEvent) {
	defer close(ch)
	defer body.Close()

	reader := sse.NewReader(body)

	for {
		event, ok := reader.Next()
		if !ok {
			break
		}

		var resp response
		if err := json.Unmarshal([]byte(event.Data), &resp); err != nil {
			ch <- langrails.StreamEvent{
				Type:  langrails.EventError,
				Error: fmt.Errorf("gemini: failed to parse stream chunk: %w", err),
			}
			return
		}

		if len(resp.Candidates) == 0 {
			continue
		}

		candidate := resp.Candidates[0]
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				ch <- langrails.StreamEvent{
					Type:    langrails.EventContent,
					Content: part.Text,
				}
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				tc := langrails.ToolCall{
					ID:        part.FunctionCall.Name, // Gemini doesn't provide IDs
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				}
				if part.FunctionCall.ThoughtSignature != "" {
					tc.Metadata = map[string]string{"thoughtSignature": part.FunctionCall.ThoughtSignature}
				}
				ch <- langrails.StreamEvent{
					Type:     langrails.EventToolCall,
					ToolCall: &tc,
				}
			}
		}

		if candidate.FinishReason != "" && candidate.FinishReason != "STOP" {
			continue
		}
		if candidate.FinishReason == "STOP" {
			if resp.UsageMetadata != nil {
				ch <- langrails.StreamEvent{
					Usage: &langrails.TokenUsage{
						PromptTokens:     resp.UsageMetadata.PromptTokenCount,
						CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
						TotalTokens:      resp.UsageMetadata.TotalTokenCount,
					},
				}
			}
		}
	}

	if err := reader.Err(); err != nil {
		ch <- langrails.StreamEvent{
			Type:  langrails.EventError,
			Error: fmt.Errorf("gemini: stream read error: %w", err),
		}
		return
	}

	ch <- langrails.StreamEvent{Type: langrails.EventDone}
}

func (p *Provider) buildRequestBody(req *langrails.CompletionRequest) ([]byte, error) {
	r := request{
		Contents: convertMessages(req),
	}

	if req.SystemPrompt != "" {
		r.SystemInstruction = &content{
			Parts: []part{{Text: req.SystemPrompt}},
		}
	}

	needsConfig := req.Temperature != nil || req.MaxTokens != nil || req.TopP != nil ||
		req.TopK != nil || len(req.Stop) > 0 || req.OutputSchema != nil
	if needsConfig {
		r.GenerationConfig = &generationConfig{
			Temperature:   req.Temperature,
			MaxTokens:     req.MaxTokens,
			TopP:          req.TopP,
			TopK:          req.TopK,
			StopSequences: req.Stop,
		}
	}

	// Structured output via responseSchema
	if req.OutputSchema != nil {
		if r.GenerationConfig == nil {
			r.GenerationConfig = &generationConfig{}
		}
		schema := json.RawMessage(*req.OutputSchema)
		r.GenerationConfig.ResponseMIMEType = "application/json"
		r.GenerationConfig.ResponseSchema = &schema
	}

	if len(req.Tools) > 0 {
		r.Tools = convertTools(req.Tools)
	}

	return json.Marshal(r)
}

func (p *Provider) parseResponse(resp *response) *langrails.CompletionResponse {
	result := &langrails.CompletionResponse{}

	if resp.UsageMetadata != nil {
		result.Usage = langrails.TokenUsage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		result.FinishReason = candidate.FinishReason

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				result.Content += part.Text
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				tc := langrails.ToolCall{
					ID:        part.FunctionCall.Name,
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				}
				if part.FunctionCall.ThoughtSignature != "" {
					tc.Metadata = map[string]string{"thoughtSignature": part.FunctionCall.ThoughtSignature}
				}
				result.ToolCalls = append(result.ToolCalls, tc)
			}
		}
	}

	return result
}

func convertMessages(req *langrails.CompletionRequest) []content {
	var contents []content

	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		c := content{Role: role}

		switch {
		case m.Role == "tool":
			// Tool results in Gemini are user messages with functionResponse
			var respData map[string]interface{}
			_ = json.Unmarshal([]byte(m.Content), &respData)
			if respData == nil {
				respData = map[string]interface{}{"result": m.Content}
			}
			c.Role = "user"
			c.Parts = []part{{
				FunctionResponse: &functionResponse{
					Name:     m.ToolCallID,
					Response: respData,
				},
			}}
		case len(m.ToolCalls) > 0:
			for _, tc := range m.ToolCalls {
				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				fc := &functionCall{
					Name: tc.Name,
					Args: args,
				}
				if tc.Metadata != nil {
					if sig, ok := tc.Metadata["thoughtSignature"]; ok {
						fc.ThoughtSignature = sig
					}
				}
				c.Parts = append(c.Parts, part{
					FunctionCall: fc,
				})
			}
		default:
			c.Parts = []part{{Text: m.Content}}
		}

		contents = append(contents, c)
	}

	return contents
}

func convertTools(tools []langrails.ToolDefinition) []toolDeclaration {
	decls := make([]functionDecl, len(tools))
	for i, t := range tools {
		decls[i] = functionDecl{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		}
	}
	return []toolDeclaration{{FunctionDeclarations: decls}}
}
