package bedrock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/internal/awssig"
	"github.com/promptrails/langrails/internal/eventstream"
	"github.com/promptrails/langrails/internal/mediautil"
)

const (
	defaultRegion    = "us-east-1"
	defaultMaxTokens = 4096
	service          = "bedrock"
)

// Provider implements langrails.Provider for Amazon Bedrock via the unified
// Converse API. The model identifier passed in CompletionRequest.Model is a
// Bedrock model or inference-profile ID (e.g.
// "anthropic.claude-3-5-sonnet-20241022-v2:0").
type Provider struct {
	region  string
	creds   awssig.Credentials
	baseURL string
	client  *http.Client
}

// Option configures the Bedrock provider.
type Option func(*Provider)

// WithRegion sets the AWS region. Defaults to AWS_REGION / AWS_DEFAULT_REGION,
// then "us-east-1".
func WithRegion(region string) Option {
	return func(p *Provider) { p.region = region }
}

// WithStaticCredentials sets explicit AWS credentials. sessionToken may be empty
// for long-term credentials. Defaults are read from the standard AWS_ACCESS_KEY_ID,
// AWS_SECRET_ACCESS_KEY and AWS_SESSION_TOKEN environment variables.
func WithStaticCredentials(accessKeyID, secretAccessKey, sessionToken string) Option {
	return func(p *Provider) {
		p.creds = awssig.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			SessionToken:    sessionToken,
		}
	}
}

// WithBaseURL overrides the Bedrock runtime endpoint. Mainly useful for tests
// or VPC endpoints. When unset it is derived from the region.
func WithBaseURL(rawURL string) Option {
	return func(p *Provider) { p.baseURL = strings.TrimRight(rawURL, "/") }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) { p.client = client }
}

// New creates a Bedrock provider. With no options it reads region and
// credentials from the standard AWS environment variables.
func New(opts ...Option) *Provider {
	p := &Provider{
		region: firstNonEmpty(os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION")),
		creds: awssig.Credentials{
			AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
			SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
		},
		client: &http.Client{Timeout: 5 * time.Minute},
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.region == "" {
		p.region = defaultRegion
	}
	if p.baseURL == "" {
		p.baseURL = fmt.Sprintf("https://bedrock-runtime.%s.amazonaws.com", p.region)
	}
	return p
}

// Complete sends a non-streaming Converse request.
func (p *Provider) Complete(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	body, err := buildRequestBody(req)
	if err != nil {
		return nil, err
	}

	respBody, err := p.doRequest(ctx, req.Model, "converse", body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	raw, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("bedrock: failed to read response: %w", err)
	}

	var resp response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("bedrock: failed to parse response: %w", err)
	}

	return parseResponse(&resp), nil
}

// Stream sends a streaming Converse request and returns a channel of events.
func (p *Provider) Stream(ctx context.Context, req *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	body, err := buildRequestBody(req)
	if err != nil {
		return nil, err
	}

	respBody, err := p.doRequest(ctx, req.Model, "converse-stream", body)
	if err != nil {
		return nil, err
	}

	ch := make(chan langrails.StreamEvent, 64)
	go readStream(respBody, ch)
	return ch, nil
}

func (p *Provider) doRequest(ctx context.Context, modelID, action string, body []byte) (io.ReadCloser, error) {
	if modelID == "" {
		return nil, fmt.Errorf("bedrock: model is required")
	}
	endpoint := p.baseURL + "/model/" + url.PathEscape(modelID) + "/" + action

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("bedrock: failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	signer := &awssig.Signer{Credentials: p.creds, Region: p.region, Service: service}
	signer.Sign(httpReq, body, time.Now().UTC())

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bedrock: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)

		msg := fmt.Sprintf("status %d", resp.StatusCode)
		var errResp struct {
			Message string `json:"message"`
			Msg     string `json:"Message"`
		}
		if json.Unmarshal(raw, &errResp) == nil {
			if errResp.Message != "" {
				msg = errResp.Message
			} else if errResp.Msg != "" {
				msg = errResp.Msg
			}
		}

		return nil, &langrails.APIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Provider:   "bedrock",
		}
	}

	return resp.Body, nil
}

func readStream(body io.ReadCloser, ch chan<- langrails.StreamEvent) {
	defer close(ch)
	defer body.Close()

	dec := eventstream.NewDecoder(body)

	type pendingTool struct {
		id, name string
		input    strings.Builder
	}
	pending := map[int]*pendingTool{}
	var order []int

	for {
		msg, err := dec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			ch <- langrails.StreamEvent{Type: langrails.EventError, Error: fmt.Errorf("bedrock: stream read error: %w", err)}
			return
		}

		if msg.MessageType() == "exception" {
			var ex streamException
			_ = json.Unmarshal(msg.Payload, &ex)
			detail := ex.Message
			if detail == "" {
				detail = msg.Headers[":exception-type"]
			}
			ch <- langrails.StreamEvent{Type: langrails.EventError, Error: fmt.Errorf("bedrock: stream exception: %s", detail)}
			return
		}

		switch msg.EventType() {
		case "contentBlockStart":
			var ev streamContentBlockStart
			if json.Unmarshal(msg.Payload, &ev) != nil {
				continue
			}
			if ev.Start.ToolUse != nil {
				pending[ev.ContentBlockIndex] = &pendingTool{id: ev.Start.ToolUse.ToolUseID, name: ev.Start.ToolUse.Name}
				order = append(order, ev.ContentBlockIndex)
			}

		case "contentBlockDelta":
			var ev streamContentBlockDelta
			if json.Unmarshal(msg.Payload, &ev) != nil {
				continue
			}
			if ev.Delta.ReasoningContent != nil && ev.Delta.ReasoningContent.Text != "" {
				ch <- langrails.StreamEvent{Type: langrails.EventReasoning, Reasoning: ev.Delta.ReasoningContent.Text}
			}
			if ev.Delta.Text != "" {
				ch <- langrails.StreamEvent{Type: langrails.EventContent, Content: ev.Delta.Text}
			}
			if ev.Delta.ToolUse != nil {
				if pt, ok := pending[ev.ContentBlockIndex]; ok {
					pt.input.WriteString(ev.Delta.ToolUse.Input)
				}
			}

		case "messageStop":
			for _, idx := range order {
				pt := pending[idx]
				tc := langrails.ToolCall{ID: pt.id, Name: pt.name, Arguments: pt.input.String()}
				ch <- langrails.StreamEvent{Type: langrails.EventToolCall, ToolCall: &tc}
			}
			order = nil

		case "metadata":
			var ev streamMetadata
			if json.Unmarshal(msg.Payload, &ev) != nil {
				continue
			}
			ch <- langrails.StreamEvent{Usage: &langrails.TokenUsage{
				PromptTokens:        ev.Usage.InputTokens,
				CompletionTokens:    ev.Usage.OutputTokens,
				TotalTokens:         ev.Usage.TotalTokens,
				CachedTokens:        ev.Usage.CacheReadInputTokens,
				CacheCreationTokens: ev.Usage.CacheWriteInputTokens,
			}}
		}
	}

	ch <- langrails.StreamEvent{Type: langrails.EventDone}
}

func buildRequestBody(req *langrails.CompletionRequest) ([]byte, error) {
	r := request{
		Messages: convertMessages(req),
		System:   buildSystem(req),
	}

	// Prompt caching: mark a cache breakpoint at the end of the last message.
	if req.CacheControl && len(r.Messages) > 0 {
		last := &r.Messages[len(r.Messages)-1]
		last.Content = append(last.Content, contentBlock{CachePoint: &cachePoint{Type: "default"}})
	}

	maxTokens := defaultMaxTokens
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	ic := &inferenceConfig{MaxTokens: &maxTokens}
	if req.Temperature != nil {
		ic.Temperature = req.Temperature
	}
	if req.TopP != nil {
		ic.TopP = req.TopP
	}
	if len(req.Stop) > 0 {
		ic.StopSequences = req.Stop
	}
	r.InferenceConfig = ic

	// Reasoning. Carried via additionalModelRequestFields (model-family specific;
	// the thinking form is for Anthropic Claude models on Bedrock — it mirrors
	// Anthropic's native thinking field passed through Converse).
	if req.Thinking || req.ReasoningEffort != "" {
		budget := 0
		if req.ThinkingBudget != nil {
			budget = *req.ThinkingBudget
		} else {
			budget = req.ReasoningEffort.BudgetTokens()
		}
		if budget == 0 {
			budget = 10000
		}
		r.AdditionalModelRequestFields = json.RawMessage(
			fmt.Sprintf(`{"thinking":{"type":"enabled","budget_tokens":%d}}`, budget))
	}

	var tools []toolEntry
	for _, t := range req.Tools {
		tools = append(tools, toolEntry{ToolSpec: toolSpec{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: toolInputSchema{JSON: t.Parameters},
		}})
	}

	if req.OutputSchema != nil {
		tools = append(tools, toolEntry{ToolSpec: toolSpec{
			Name:        "structured_output",
			Description: "Return the response in the specified JSON schema.",
			InputSchema: toolInputSchema{JSON: json.RawMessage(*req.OutputSchema)},
		}})
	}

	// ToolChoiceNone forbids tool calls; Converse has no "none", so we omit the
	// tool config entirely (the model can't call what it isn't given).
	noTools := req.OutputSchema == nil && req.ToolChoice != nil && req.ToolChoice.Mode == langrails.ToolChoiceNone

	if len(tools) > 0 && !noTools {
		r.ToolConfig = &toolConfig{Tools: tools}
		switch {
		case req.OutputSchema != nil:
			r.ToolConfig.ToolChoice = &toolChoice{Tool: &toolChoiceName{Name: "structured_output"}}
		default:
			r.ToolConfig.ToolChoice = convertToolChoice(req.ToolChoice)
		}
	}

	return json.Marshal(r)
}

// convertToolChoice maps the unified ToolChoice to Bedrock Converse's toolChoice.
// Converse supports auto/any/tool (no "none"); auto returns nil to use the default.
func convertToolChoice(tc *langrails.ToolChoice) *toolChoice {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case langrails.ToolChoiceRequired:
		return &toolChoice{Any: &struct{}{}}
	case langrails.ToolChoiceTool:
		return &toolChoice{Tool: &toolChoiceName{Name: tc.Name}}
	default:
		return nil
	}
}

// buildSystem collects the system prompt and any system-role messages into
// Converse system blocks (Converse keeps system separate from messages).
func buildSystem(req *langrails.CompletionRequest) []systemBlock {
	var blocks []systemBlock
	if req.SystemPrompt != "" {
		blocks = append(blocks, systemBlock{Text: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		if m.Role == "system" && m.Content != "" {
			blocks = append(blocks, systemBlock{Text: m.Content})
		}
	}
	return blocks
}

func convertMessages(req *langrails.CompletionRequest) []message {
	var msgs []message

	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			// Handled by buildSystem.
			continue

		case "tool":
			msgs = append(msgs, message{
				Role: "user",
				Content: []contentBlock{{
					ToolResult: &toolResult{
						ToolUseID: m.ToolCallID,
						Content:   []toolResultContentItem{{Text: m.Content}},
						Status:    "success",
					},
				}},
			})

		case "assistant":
			var blocks []contentBlock
			if m.Content != "" {
				blocks = append(blocks, contentBlock{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := json.RawMessage(tc.Arguments)
				if len(input) == 0 {
					input = json.RawMessage("{}")
				}
				blocks = append(blocks, contentBlock{ToolUse: &toolUse{
					ToolUseID: tc.ID,
					Name:      tc.Name,
					Input:     input,
				}})
			}
			msgs = append(msgs, message{Role: "assistant", Content: blocks})

		default:
			msgs = append(msgs, message{
				Role:    "user",
				Content: convertContentParts(m),
			})
		}
	}

	return msgs
}

// convertContentParts builds Converse content blocks from a message, handling
// multimodal image parts. Converse embeds images as bytes, so only base64 data
// URIs are supported; plain image URLs are skipped (Converse can't fetch them).
func convertContentParts(m langrails.Message) []contentBlock {
	if len(m.ContentParts) == 0 {
		return []contentBlock{{Text: m.Content}}
	}
	var blocks []contentBlock
	for _, cp := range m.ContentParts {
		switch cp.Type {
		case "image":
			mt, data, _, isB64 := mediautil.ParseImageURL(cp.ImageURL)
			if format := mediautil.ImageFormat(mt); isB64 && format != "" {
				blocks = append(blocks, contentBlock{Image: &imageBlock{
					Format: format,
					Source: imageSourceBytes{Bytes: data},
				}})
			}
		default:
			blocks = append(blocks, contentBlock{Text: cp.Text})
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, contentBlock{Text: m.Content})
	}
	return blocks
}

func parseResponse(resp *response) *langrails.CompletionResponse {
	result := &langrails.CompletionResponse{
		FinishReason: resp.StopReason,
		Usage: langrails.TokenUsage{
			PromptTokens:        resp.Usage.InputTokens,
			CompletionTokens:    resp.Usage.OutputTokens,
			TotalTokens:         resp.Usage.TotalTokens,
			CachedTokens:        resp.Usage.CacheReadInputTokens,
			CacheCreationTokens: resp.Usage.CacheWriteInputTokens,
		},
	}

	for _, block := range resp.Output.Message.Content {
		switch {
		case block.ReasoningContent != nil && block.ReasoningContent.ReasoningText != nil:
			result.Thinking += block.ReasoningContent.ReasoningText.Text
		case block.ToolUse != nil:
			args := string(block.ToolUse.Input)
			if block.ToolUse.Name == "structured_output" {
				result.Content = args
				continue
			}
			result.ToolCalls = append(result.ToolCalls, langrails.ToolCall{
				ID:        block.ToolUse.ToolUseID,
				Name:      block.ToolUse.Name,
				Arguments: args,
			})
		case block.Text != "":
			result.Content += block.Text
		}
	}

	return result
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
