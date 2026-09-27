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
	return a.run(ctx, messages, nil)
}

// run is the agent loop shared by RunMessages and StreamMessages. When emit
// is nil the model is called with Complete; otherwise with Stream, and
// progress is reported through emit as it happens.
func (a *Agent) run(ctx context.Context, messages []langrails.Message, emit func(Event)) (*Result, error) {
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
	runTool := a.toolChain()

	for i := 0; i < a.maxIterations; i++ {
		iteration := i + 1
		state := &State{Request: req, Iteration: iteration}

		for _, m := range a.middlewares {
			if err := m.BeforeModel(ctx, state); err != nil {
				return nil, fmt.Errorf("agent: before_model (iteration %d): %w", iteration, err)
			}
		}

		streamed := false
		call := a.baseCall()
		if emit != nil {
			call = a.streamCall(iteration, emit, &streamed)
		}
		for j := len(a.middlewares) - 1; j >= 0; j-- {
			call = a.middlewares[j].WrapModelCall(call)
		}

		resp, err := call(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("agent: iteration %d: %w", iteration, err)
		}
		state.Response = resp

		for j := len(a.middlewares) - 1; j >= 0; j-- {
			if err := a.middlewares[j].AfterModel(ctx, state); err != nil {
				return nil, fmt.Errorf("agent: after_model (iteration %d): %w", iteration, err)
			}
		}

		// A middleware answered without reaching the provider (a cache, a
		// canned reply): deliver its content in one piece.
		if emit != nil && !streamed && resp.Content != "" {
			emit(Event{Type: EventContent, Iteration: iteration, Content: resp.Content})
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
			if emit != nil {
				call := tc
				emit(Event{Type: EventToolCall, Iteration: iteration, ToolCall: &call})
			}
			out, execErr := runTool(ctx, tc)
			if emit != nil {
				call := tc
				emit(Event{Type: EventToolResult, Iteration: iteration, ToolCall: &call, Content: out, ToolError: execErr})
			}
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

// streamCall is the innermost CallFunc for streaming runs: it streams from
// the provider, forwards text and reasoning chunks through emit as they
// arrive, and returns the assembled response to the middleware chain.
func (a *Agent) streamCall(iteration int, emit func(Event), streamed *bool) CallFunc {
	return func(ctx context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
		events, err := a.provider.Stream(ctx, req)
		if err != nil {
			return nil, err
		}
		var acc langrails.StreamAccumulator
		for ev := range events {
			acc.Add(ev)
			switch ev.Type {
			case langrails.EventContent:
				*streamed = true
				emit(Event{Type: EventContent, Iteration: iteration, Content: ev.Content})
			case langrails.EventReasoning:
				*streamed = true
				emit(Event{Type: EventReasoning, Iteration: iteration, Reasoning: ev.Reasoning})
			}
		}
		if err := acc.Err(); err != nil {
			return nil, err
		}
		return acc.Response(), nil
	}
}

// toolChain composes the executor with every middleware that implements
// ToolWrapper, first registered outermost.
func (a *Agent) toolChain() ToolFunc {
	run := ToolFunc(func(ctx context.Context, call langrails.ToolCall) (string, error) {
		return a.executor.Execute(ctx, call.Name, call.Arguments)
	})
	for j := len(a.middlewares) - 1; j >= 0; j-- {
		if w, ok := a.middlewares[j].(ToolWrapper); ok {
			run = w.WrapToolCall(run)
		}
	}
	return run
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
