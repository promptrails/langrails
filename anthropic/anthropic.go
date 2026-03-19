package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/promptrails/llmrails"
	"github.com/promptrails/llmrails/internal/sse"
)

const (
	defaultBaseURL   = "https://api.anthropic.com/v1/messages"
	defaultMaxTokens = 4096
	apiVersion       = "2023-06-01"
)

// Provider implements llmrails.Provider for Anthropic's Messages API.
type Provider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

// Option configures the Anthropic provider.
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

// New creates a new Anthropic provider with the given API key and options.
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
func (p *Provider) Complete(ctx context.Context, req *llmrails.CompletionRequest) (*llmrails.CompletionResponse, error) {
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
		return nil, fmt.Errorf("anthropic: failed to read response: %w", err)
	}

	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("anthropic: failed to parse response: %w", err)
	}

	return p.parseResponse(&resp), nil
}

// Stream sends a streaming completion request and returns a channel of events.
func (p *Provider) Stream(ctx context.Context, req *llmrails.CompletionRequest) (<-chan llmrails.StreamEvent, error) {
	body, err := p.buildRequestBody(req, true)
	if err != nil {
		return nil, err
	}

	respBody, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan llmrails.StreamEvent, 64)
	go p.readStream(respBody, ch)
	return ch, nil
}

func (p *Provider) doRequest(ctx context.Context, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)

		msg := fmt.Sprintf("status %d", resp.StatusCode)
		var errResp errorResponse
		if json.Unmarshal(raw, &errResp) == nil && errResp.Error.Message != "" {
			msg = errResp.Error.Message
		}

		return nil, &llmrails.APIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Provider:   "anthropic",
		}
	}

	return resp.Body, nil
}

func (p *Provider) readStream(body io.ReadCloser, ch chan<- llmrails.StreamEvent) {
	defer close(ch)
	defer body.Close()

	reader := sse.NewReader(body)
	var pendingToolCalls []llmrails.ToolCall
	var currentToolIndex int = -1

	for {
		event, ok := reader.Next()
		if !ok {
			break
		}

		var se streamEvent
		if err := json.Unmarshal([]byte(event.Data), &se); err != nil {
			continue // Skip unparseable events
		}

		switch se.Type {
		case "content_block_start":
			if se.ContentBlock != nil && se.ContentBlock.Type == "tool_use" {
				currentToolIndex++
				pendingToolCalls = append(pendingToolCalls, llmrails.ToolCall{
					ID:   se.ContentBlock.ID,
					Name: se.ContentBlock.Name,
				})
			}

		case "content_block_delta":
			if se.Delta == nil {
				continue
			}
			switch se.Delta.Type {
			case "text_delta":
				if se.Delta.Text != "" {
					ch <- llmrails.StreamEvent{
						Type:    llmrails.EventContent,
						Content: se.Delta.Text,
					}
				}
			case "input_json_delta":
				if currentToolIndex >= 0 && currentToolIndex < len(pendingToolCalls) {
					pendingToolCalls[currentToolIndex].Arguments += se.Delta.PartialJSON
				}
			}

		case "message_delta":
			if se.Usage != nil {
				ch <- llmrails.StreamEvent{
					Usage: &llmrails.TokenUsage{
						PromptTokens:     se.Usage.InputTokens,
						CompletionTokens: se.Usage.OutputTokens,
						TotalTokens:      se.Usage.InputTokens + se.Usage.OutputTokens,
					},
				}
			}

		case "message_stop":
			for i := range pendingToolCalls {
				ch <- llmrails.StreamEvent{
					Type:     llmrails.EventToolCall,
					ToolCall: &pendingToolCalls[i],
				}
			}
			ch <- llmrails.StreamEvent{Type: llmrails.EventDone}
			return
		}
	}

	if err := reader.Err(); err != nil {
		ch <- llmrails.StreamEvent{
			Type:  llmrails.EventError,
			Error: fmt.Errorf("anthropic: stream read error: %w", err),
		}
		return
	}

	ch <- llmrails.StreamEvent{Type: llmrails.EventDone}
}

func (p *Provider) buildRequestBody(req *llmrails.CompletionRequest, stream bool) ([]byte, error) {
	maxTokens := defaultMaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}

	r := request{
		Model:     req.Model,
		Messages:  convertMessages(req),
		MaxTokens: maxTokens,
		Stream:    stream,
	}

	if req.SystemPrompt != "" {
		r.System = req.SystemPrompt
	}
	if req.Temperature != nil {
		r.Temperature = req.Temperature
	}
	if req.TopP != nil {
		r.TopP = req.TopP
	}
	if req.TopK != nil {
		r.TopK = req.TopK
	}
	if len(req.Stop) > 0 {
		r.Stop = req.Stop
	}

	// Extended thinking
	if req.Thinking {
		budget := 10000 // default
		if req.ThinkingBudget != nil {
			budget = *req.ThinkingBudget
		}
		r.Thinking = &thinking{Type: "enabled", BudgetTokens: budget}
	}

	if len(req.Tools) > 0 {
		r.Tools = convertTools(req.Tools)
	}

	// Structured output: define schema as a tool and force the model to use it
	if req.OutputSchema != nil {
		r.Tools = append(r.Tools, tool{
			Name:        "structured_output",
			Description: "Return the response in the specified JSON schema.",
			InputSchema: json.RawMessage(*req.OutputSchema),
		})
		r.ToolChoice = &toolChoice{Type: "tool", Name: "structured_output"}
	}

	return json.Marshal(r)
}

func (p *Provider) parseResponse(resp *response) *llmrails.CompletionResponse {
	result := &llmrails.CompletionResponse{
		Model:        resp.Model,
		FinishReason: resp.StopReason,
		Usage: llmrails.TokenUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "thinking":
			result.Thinking += block.Text
		case "text":
			result.Content += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			// If this is our structured_output tool, return as content
			if block.Name == "structured_output" {
				result.Content = string(args)
				continue
			}
			result.ToolCalls = append(result.ToolCalls, llmrails.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}

	return result
}

func convertMessages(req *llmrails.CompletionRequest) []message {
	var msgs []message

	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			// Tool results in Anthropic are user messages with tool_result blocks
			msgs = append(msgs, message{
				Role: "user",
				Content: []contentBlock{
					{
						Type:      "tool_result",
						ToolUseID: m.ToolCallID,
						Content:   m.Content,
					},
				},
			})

		case "assistant":
			var blocks []contentBlock
			if m.Content != "" {
				blocks = append(blocks, contentBlock{
					Type: "text",
					Text: m.Content,
				})
			}
			for _, tc := range m.ToolCalls {
				var input json.RawMessage
				_ = json.Unmarshal([]byte(tc.Arguments), &input)
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			msgs = append(msgs, message{
				Role:    "assistant",
				Content: blocks,
			})

		default:
			msgs = append(msgs, message{
				Role: m.Role,
				Content: []contentBlock{
					{Type: "text", Text: m.Content},
				},
			})
		}
	}

	return msgs
}

func convertTools(tools []llmrails.ToolDefinition) []tool {
	result := make([]tool, len(tools))
	for i, t := range tools {
		result[i] = tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		}
	}
	return result
}
