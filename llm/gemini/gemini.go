package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/internal/mediautil"
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

	// Vertex AI mode (opt-in via WithVertex). When set, requests go to the
	// region's aiplatform endpoint under vertexProject and authenticate with a
	// Bearer token from tokenSource instead of the generativelanguage endpoint
	// with an API key. The request/response wire format is identical, so only
	// the URL and the auth differ.
	vertexProject string
	vertexRegion  string
	tokenSource   TokenSource
}

// TokenSource yields an OAuth2 bearer token for Vertex AI requests. It must be
// safe for concurrent use and is expected to cache/refresh internally.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// Option configures the Gemini provider.
type Option func(*Provider)

// WithBaseURL sets a custom base URL. In Vertex mode it overrides the derived
// `https://{region}-aiplatform.googleapis.com/v1` prefix (used by tests).
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

// WithVertex switches the provider to Google Cloud Vertex AI. Requests go to
// `{region}-aiplatform.googleapis.com` under projectID and authenticate with a
// Bearer token from ts (see NewServiceAccountTokenSource) instead of an API key.
// Vertex is region-based, so — unlike the generativelanguage.googleapis.com
// API-key endpoint — it is not subject to the caller-IP geo restriction
// ("User location is not supported for the API use"). The wire format is
// identical to the Gemini API, so everything below Complete/Stream is shared.
// Call New with an empty apiKey when using this.
func WithVertex(projectID, region string, ts TokenSource) Option {
	return func(p *Provider) {
		p.vertexProject = projectID
		p.vertexRegion = region
		p.tokenSource = ts
	}
}

func (p *Provider) isVertex() bool { return p.vertexProject != "" && p.tokenSource != nil }

func (p *Provider) providerName() string {
	if p.isVertex() {
		return "vertex"
	}
	return "gemini"
}

// buildURL constructs the endpoint for a model + method ("generateContent" or
// "streamGenerateContent"), branching on Vertex vs the API-key endpoint.
func (p *Provider) buildURL(model, method string, stream bool) string {
	if p.isVertex() {
		base := p.baseURL
		if base == "" || base == defaultBaseURL {
			base = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1", p.vertexRegion)
		}
		u := fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s:%s",
			base, p.vertexProject, p.vertexRegion, model, method)
		if stream {
			u += "?alt=sse"
		}
		return u
	}
	if stream {
		return fmt.Sprintf("%s/%s:%s?alt=sse&key=%s", p.baseURL, model, method, p.apiKey)
	}
	return fmt.Sprintf("%s/%s:%s?key=%s", p.baseURL, model, method, p.apiKey)
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

	url := p.buildURL(req.Model, "generateContent", false)

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

	url := p.buildURL(req.Model, "streamGenerateContent", true)

	respBody, err := p.doRequest(ctx, url, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan langrails.StreamEvent, 64)
	go p.readStream(ctx, respBody, ch)
	return ch, nil
}

func (p *Provider) doRequest(ctx context.Context, url string, body []byte) (io.ReadCloser, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Vertex authenticates with an OAuth2 bearer token; the API-key path carries
	// its key in the URL and needs no header.
	if p.isVertex() {
		tok, err := p.tokenSource.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("vertex: obtain access token: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", p.providerName(), err)
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
			Provider:   p.providerName(),
		}
	}

	return resp.Body, nil
}

func (p *Provider) readStream(ctx context.Context, body io.ReadCloser, ch chan<- langrails.StreamEvent) {
	defer close(ch)
	defer body.Close()

	reader := sse.NewReader(body)

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
				if part.Thought {
					ch <- langrails.StreamEvent{
						Type:      langrails.EventReasoning,
						Reasoning: part.Text,
					}
				} else {
					ch <- langrails.StreamEvent{
						Type:    langrails.EventContent,
						Content: part.Text,
					}
				}
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				tc := langrails.ToolCall{
					ID:        part.FunctionCall.Name, // Gemini doesn't provide IDs
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				}
				// Gemini returns the thoughtSignature at the PART level, not
				// inside functionCall — read it from there or it's lost.
				if part.ThoughtSignature != "" {
					tc.Metadata = map[string]string{"thoughtSignature": part.ThoughtSignature}
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
						ReasoningTokens:  resp.UsageMetadata.ThoughtsTokenCount,
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

	// Structured output via responseSchema, or schema-less JSON mode.
	if req.OutputSchema != nil {
		if r.GenerationConfig == nil {
			r.GenerationConfig = &generationConfig{}
		}
		schema := json.RawMessage(*req.OutputSchema)
		r.GenerationConfig.ResponseMIMEType = "application/json"
		r.GenerationConfig.ResponseJSONSchema = &schema
	} else if req.ResponseFormat == langrails.ResponseFormatJSONObject {
		if r.GenerationConfig == nil {
			r.GenerationConfig = &generationConfig{}
		}
		r.GenerationConfig.ResponseMIMEType = "application/json"
	}

	// Reasoning / thinking. Gemini 3 uses named thinking levels; Gemini 2.5
	// uses token budgets. Keep the legacy Thinking/ThinkingBudget behavior for
	// callers that request it without a provider-agnostic ReasoningEffort.
	if req.Thinking || req.ReasoningEffort != "" {
		if r.GenerationConfig == nil {
			r.GenerationConfig = &generationConfig{}
		}
		tc := &thinkingConfig{IncludeThoughts: true}
		if req.ReasoningEffort != "" && isGemini3(req.Model) {
			level := string(req.ReasoningEffort)
			tc.ThinkingLevel = &level
		} else {
			budget := 0
			if req.ThinkingBudget != nil {
				budget = *req.ThinkingBudget
			} else {
				budget = req.ReasoningEffort.BudgetTokens()
			}
			if budget > 0 {
				tc.ThinkingBudget = &budget
			}
		}
		r.GenerationConfig.ThinkingConfig = tc
	} else if len(req.Tools) > 0 && isGemini25Flash(req.Model) {
		// Gemini 2.5 Flash defaults to DYNAMIC thinking. Combined with function
		// calling under a detailed agentic system prompt it intermittently
		// returns an EMPTY candidate (finish=STOP, 0 output tokens) instead of a
		// tool call — stalling tool-use loops. Flash (and flash-lite) support
		// disabling thinking (budget 0), so do so for tool-use requests that did
		// not explicitly ask for thinking. Structured-output / plain calls keep
		// dynamic thinking, and callers can force it back on via Thinking.
		if r.GenerationConfig == nil {
			r.GenerationConfig = &generationConfig{}
		}
		zero := 0
		r.GenerationConfig.ThinkingConfig = &thinkingConfig{ThinkingBudget: &zero}
	}

	if len(req.Tools) > 0 {
		r.Tools = convertTools(req.Tools)
	}
	for _, st := range req.ServerTools {
		if st.Type == langrails.ServerToolWebSearch {
			r.Tools = append(r.Tools, toolDeclaration{GoogleSearch: &struct{}{}})
		}
	}
	if tc := convertToolChoice(req.ToolChoice); tc != nil {
		r.ToolConfig = tc
	}

	return json.Marshal(r)
}

// convertToolChoice maps the unified ToolChoice to Gemini's functionCallingConfig.
// Returns nil when no choice is set.
func convertToolChoice(tc *langrails.ToolChoice) *toolConfig {
	if tc == nil {
		return nil
	}
	switch tc.Mode {
	case langrails.ToolChoiceAuto:
		return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "AUTO"}}
	case langrails.ToolChoiceNone:
		return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "NONE"}}
	case langrails.ToolChoiceRequired:
		return &toolConfig{FunctionCallingConfig: functionCallingConfig{Mode: "ANY"}}
	case langrails.ToolChoiceTool:
		return &toolConfig{FunctionCallingConfig: functionCallingConfig{
			Mode:                 "ANY",
			AllowedFunctionNames: []string{tc.Name},
		}}
	default:
		return nil
	}
}

func (p *Provider) parseResponse(resp *response) *langrails.CompletionResponse {
	result := &langrails.CompletionResponse{}

	if resp.UsageMetadata != nil {
		result.Usage = langrails.TokenUsage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
			ReasoningTokens:  resp.UsageMetadata.ThoughtsTokenCount,
			CachedTokens:     resp.UsageMetadata.CachedContentTokenCount,
		}
	}

	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]
		result.FinishReason = candidate.FinishReason
		result.Citations = append(result.Citations, groundingCitations(candidate.GroundingMetadata)...)

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				if part.Thought {
					result.Thinking += part.Text
				} else {
					result.Content += part.Text
				}
			}
			if part.FunctionCall != nil {
				args, _ := json.Marshal(part.FunctionCall.Args)
				tc := langrails.ToolCall{
					ID:        part.FunctionCall.Name,
					Name:      part.FunctionCall.Name,
					Arguments: string(args),
				}
				// Gemini returns the thoughtSignature at the PART level, not
				// inside functionCall — read it from there or it's lost.
				if part.ThoughtSignature != "" {
					tc.Metadata = map[string]string{"thoughtSignature": part.ThoughtSignature}
				}
				result.ToolCalls = append(result.ToolCalls, tc)
			}
		}
	}

	return result
}

func convertMessages(req *langrails.CompletionRequest) []content {
	// Gemini 2.5+ REQUIRES a thoughtSignature on every functionCall part it is
	// sent; replaying a functionCall without one fails the whole request with
	// "Function call is missing a thought_signature". Only Gemini (with thinking)
	// produces these signatures — so tool calls that originated from a DIFFERENT
	// model in the same conversation (a fallback model, or a mid-conversation
	// model switch) carry none, and feeding that history back to Gemini hard-400s.
	//
	// When an assistant turn's tool calls are ALL unsigned (i.e. it did not come
	// from Gemini), render its calls — and their results — as plain TEXT instead
	// of functionCall/functionResponse parts. Gemini then accepts the history and
	// the model still sees what was called and what came back. A genuine Gemini
	// parallel turn is untouched: its first call carries the signature, so
	// turnIsSigned is true and we keep the functionCall parts.
	textified := map[string]bool{}
	for _, m := range req.Messages {
		if len(m.ToolCalls) > 0 && !turnIsSigned(m.ToolCalls) {
			for _, tc := range m.ToolCalls {
				textified[toolCallKey(tc)] = true
			}
		}
	}

	var contents []content

	for _, m := range req.Messages {
		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		c := content{Role: role}

		switch {
		case m.Role == "tool":
			// Result of an unsigned (non-Gemini) call → text, so it pairs with the
			// textified call above instead of a dangling functionResponse.
			if textified[m.ToolCallID] {
				c.Role = "user"
				c.Parts = []part{{Text: fmt.Sprintf("Result of tool `%s`:\n%s", m.ToolCallID, m.Content)}}
				break
			}
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
		case len(m.ToolCalls) > 0 && !turnIsSigned(m.ToolCalls):
			// Unsigned tool calls (non-Gemini origin) → text, so Gemini doesn't
			// reject the missing thoughtSignature.
			var b strings.Builder
			if m.Content != "" {
				b.WriteString(m.Content)
				b.WriteString("\n")
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "Called tool `%s` with arguments: %s\n", tc.Name, tc.Arguments)
			}
			if txt := strings.TrimSpace(b.String()); txt != "" {
				c.Parts = []part{{Text: txt}}
			}
		case len(m.ToolCalls) > 0:
			for _, tc := range m.ToolCalls {
				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				p := part{FunctionCall: &functionCall{
					Name: tc.Name,
					Args: args,
				}}
				// Echo the thoughtSignature at the PART level (where Gemini
				// expects it on replay), not inside functionCall.
				if tc.Metadata != nil {
					if sig, ok := tc.Metadata["thoughtSignature"]; ok {
						p.ThoughtSignature = sig
					}
				}
				c.Parts = append(c.Parts, p)
			}
		default:
			c.Parts = convertContentParts(m)
		}

		// A part with no initialized data field — e.g. an empty-content turn
		// yielding part{Text:""}, which marshals to {} because of omitempty —
		// makes Gemini 400 the WHOLE request ("contents[N].parts[0].data:
		// required oneof field 'data' must have one initialized field"). Drop any
		// content that produced no renderable parts instead of emitting an empty
		// one; an empty assistant turn carries no information to replay anyway.
		if len(c.Parts) == 0 {
			continue
		}
		contents = append(contents, c)
	}

	return contents
}

// isGemini25Flash reports whether the model is a Gemini 2.5 Flash variant
// (flash or flash-lite) — the models that both default to dynamic thinking AND
// support disabling it (thinkingBudget 0). Pro cannot disable thinking; 2.0 has
// none.
func isGemini25Flash(model string) bool {
	m := strings.ToLower(model)
	return strings.Contains(m, "2.5") && strings.Contains(m, "flash")
}

// isGemini3 reports whether model belongs to the Gemini 3 family. Gemini 3
// accepts named thinkingLevel values and recommends them over the legacy
// numeric thinkingBudget used by Gemini 2.5.
func isGemini3(model string) bool {
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "gemini-3")
}

// turnIsSigned reports whether any tool call in an assistant turn carries a
// Gemini thoughtSignature. For a genuine Gemini parallel turn only the first
// call is signed, so "any" (not "all") correctly identifies a Gemini-origin turn.
func turnIsSigned(tcs []langrails.ToolCall) bool {
	for _, tc := range tcs {
		if tc.Metadata != nil && tc.Metadata["thoughtSignature"] != "" {
			return true
		}
	}
	return false
}

// toolCallKey is the identifier used to pair a tool call with its result. Gemini
// has no call IDs (langrails sets ID = function name), other providers do; either
// way the caller pairs the result's ToolCallID to this key.
func toolCallKey(tc langrails.ToolCall) string {
	if tc.ID != "" {
		return tc.ID
	}
	return tc.Name
}

// convertContentParts builds Gemini parts from a message, handling multimodal
// image parts. Base64 data URIs become inlineData; other URLs become fileData
// (which Gemini resolves only for File API / Cloud Storage URIs).
func convertContentParts(m langrails.Message) []part {
	if len(m.ContentParts) == 0 {
		// Empty content would become part{Text:""} → marshals to {} (omitempty) →
		// Gemini rejects the request. Emit no part; the caller drops the content.
		if m.Content == "" {
			return nil
		}
		return []part{{Text: m.Content}}
	}
	var parts []part
	for _, cp := range m.ContentParts {
		switch cp.Type {
		case "image":
			mt, data, url, isB64 := mediautil.ParseImageURL(cp.ImageURL)
			if isB64 {
				parts = append(parts, part{InlineData: &inlineData{MIMEType: mt, Data: data}})
			} else {
				parts = append(parts, part{FileData: &fileData{FileURI: url}})
			}
		default:
			if cp.Text == "" {
				continue
			}
			parts = append(parts, part{Text: cp.Text})
		}
	}
	return parts
}

// groundingCitations extracts citations from Gemini grounding metadata.
func groundingCitations(gm *groundingMetadata) []langrails.Citation {
	if gm == nil {
		return nil
	}
	var citations []langrails.Citation
	for _, chunk := range gm.GroundingChunks {
		if chunk.Web != nil {
			citations = append(citations, langrails.Citation{
				URL:   chunk.Web.URI,
				Title: chunk.Web.Title,
			})
		}
	}
	return citations
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
