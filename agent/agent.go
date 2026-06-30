package agent

import (
	"context"
	"fmt"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

// MaxIterations is the default maximum number of agent loop iterations.
const MaxIterations = 20

// Agent runs a tool-calling loop with middleware hooks around each model
// call. It builds on the same loop as tools.RunLoop but exposes
// BeforeModel/AfterModel/WrapModelCall interception points, which is the
// extension model used for summarization, redaction, human-in-the-loop,
// and other cross-cutting behavior.
type Agent struct {
	provider      langrails.Provider
	model         string
	systemPrompt  string
	tools         []langrails.ToolDefinition
	executor      tools.Executor
	middlewares   []Middleware
	maxIterations int
}

// Option configures an Agent.
type Option func(*Agent)

// WithModel sets the model the agent uses for every call. Required.
func WithModel(model string) Option {
	return func(a *Agent) { a.model = model }
}

// WithSystemPrompt sets the system instruction sent on every call.
func WithSystemPrompt(prompt string) Option {
	return func(a *Agent) { a.systemPrompt = prompt }
}

// WithTools registers the tool definitions advertised to the model and the
// executor that runs them when the model calls a tool.
func WithTools(defs []langrails.ToolDefinition, executor tools.Executor) Option {
	return func(a *Agent) {
		a.tools = defs
		a.executor = executor
	}
}

// WithMiddleware appends middleware to the agent. BeforeModel hooks run in
// the order middleware is added; AfterModel hooks run in reverse order.
func WithMiddleware(mw ...Middleware) Option {
	return func(a *Agent) { a.middlewares = append(a.middlewares, mw...) }
}

// WithMaxIterations sets the maximum number of loop iterations. Default is
// MaxIterations.
func WithMaxIterations(n int) Option {
	return func(a *Agent) { a.maxIterations = n }
}

// New creates an agent. WithModel is required; without tools the agent
// performs a single model call wrapped by any middleware.
func New(provider langrails.Provider, opts ...Option) *Agent {
	a := &Agent{provider: provider, maxIterations: MaxIterations}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Result holds the outcome of an agent run.
type Result struct {
	// Response is the final model response (no outstanding tool calls).
	Response *langrails.CompletionResponse

	// Messages is the full conversation including tool calls and results.
	Messages []langrails.Message

	// TotalUsage is the accumulated token usage across all iterations.
	TotalUsage langrails.TokenUsage

	// Iterations is the number of model calls made.
	Iterations int
}

// Run executes the agent with a single user message as input.
func (a *Agent) Run(ctx context.Context, input string) (*Result, error) {
	return a.RunMessages(ctx, []langrails.Message{{Role: "user", Content: input}})
}

// RunMessages executes the agent with a full message history, giving the
// caller control over prior turns. The messages are deep-copied before use
// (including content parts and tool calls), so middleware such as PII
// redaction cannot mutate the caller's original history.
func (a *Agent) RunMessages(ctx context.Context, messages []langrails.Message) (*Result, error) {
	if a.model == "" {
		return nil, fmt.Errorf("agent: no model set (use WithModel)")
	}

	req := &langrails.CompletionRequest{
		Model:        a.model,
		SystemPrompt: a.systemPrompt,
		Messages:     cloneMessages(messages),
		Tools:        a.tools,
	}

	result := &Result{}

	for i := 0; i < a.maxIterations; i++ {
		state := &State{Request: req, Iteration: i + 1}

		for _, m := range a.middlewares {
			if err := m.BeforeModel(ctx, state); err != nil {
				return nil, fmt.Errorf("agent: before_model (iteration %d): %w", i+1, err)
			}
		}

		call := a.baseCall()
		for j := len(a.middlewares) - 1; j >= 0; j-- {
			call = a.middlewares[j].WrapModelCall(call)
		}

		resp, err := call(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: iteration %d: %w", i+1, err)
		}
		state.Response = resp

		for j := len(a.middlewares) - 1; j >= 0; j-- {
			if err := a.middlewares[j].AfterModel(ctx, state); err != nil {
				return nil, fmt.Errorf("agent: after_model (iteration %d): %w", i+1, err)
			}
		}

		result.Iterations++
		addUsage(&result.TotalUsage, resp.Usage)

		// Loop ends when middleware stops it or the model has no tool calls.
		if state.Stopped() || len(resp.ToolCalls) == 0 {
			result.Response = resp
			result.Messages = req.Messages
			return result, nil
		}

		if a.executor == nil {
			return nil, fmt.Errorf("agent: model requested tools but no executor configured")
		}

		req.Messages = append(req.Messages, langrails.Message{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})
		for _, tc := range resp.ToolCalls {
			out, execErr := a.executor.Execute(ctx, tc.Name, tc.Arguments)
			if execErr != nil {
				out = fmt.Sprintf(`{"error": %q}`, execErr.Error())
			}
			req.Messages = append(req.Messages, langrails.Message{
				Role:       "tool",
				Content:    out,
				ToolCallID: tc.ID,
			})
		}
	}

	return nil, fmt.Errorf("agent: exceeded maximum iterations (%d)", a.maxIterations)
}

// baseCall is the innermost CallFunc that invokes the provider.
func (a *Agent) baseCall() CallFunc {
	return func(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
		return a.provider.Complete(ctx, req)
	}
}

// cloneMessages deep-copies a message slice so middleware (for example PII
// redaction) can mutate message content, content parts, and tool calls
// without touching the caller's original history.
func cloneMessages(msgs []langrails.Message) []langrails.Message {
	if msgs == nil {
		return nil
	}
	out := make([]langrails.Message, len(msgs))
	for i, m := range msgs {
		c := m // copies scalar fields and slice headers
		if m.ContentParts != nil {
			c.ContentParts = append([]langrails.ContentPart(nil), m.ContentParts...)
		}
		if m.ToolCalls != nil {
			c.ToolCalls = make([]langrails.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				c.ToolCalls[j] = tc
				if tc.Metadata != nil {
					md := make(map[string]string, len(tc.Metadata))
					for k, v := range tc.Metadata {
						md[k] = v
					}
					c.ToolCalls[j].Metadata = md
				}
			}
		}
		out[i] = c
	}
	return out
}

func addUsage(total *langrails.TokenUsage, u langrails.TokenUsage) {
	total.PromptTokens += u.PromptTokens
	total.CompletionTokens += u.CompletionTokens
	total.TotalTokens += u.TotalTokens
	total.CachedTokens += u.CachedTokens
	total.CacheCreationTokens += u.CacheCreationTokens
	total.ReasoningTokens += u.ReasoningTokens
}
