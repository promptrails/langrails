# Agents (Middleware)

The `agent` package runs a tool-calling loop — call the model, execute any
requested tools, feed the results back, repeat — with **middleware** hooks
around each model call. This is the extension model popularized by LangChain's
`create_agent`: instead of rewriting the loop, you attach middleware that
intercepts it.

## Basic agent

```go
import (
    "github.com/promptrails/langrails/agent"
    "github.com/promptrails/langrails/tools"
)

exec := tools.NewMap(map[string]tools.Func{
    "get_weather": func(ctx context.Context, args string) (string, error) {
        return `{"temp": 22, "condition": "sunny"}`, nil
    },
})

a := agent.New(provider,
    agent.WithModel("claude-sonnet-4-6"),
    agent.WithSystemPrompt("You are a helpful assistant."),
    agent.WithTools(toolDefs, exec),
)

result, err := a.Run(ctx, "What's the weather in Istanbul?")
fmt.Println(result.Response.Content)
fmt.Println(result.Iterations, result.TotalUsage.TotalTokens)
```

Use `RunMessages` instead of `Run` to pass a full conversation history.

## Middleware

A middleware implements three hooks. Embed `agent.BaseMiddleware` and override
only the ones you need:

```go
type Middleware interface {
    // Runs before each model call, in registration order.
    // Mutate state.Request in place to change what the model sees.
    BeforeModel(ctx context.Context, state *agent.State) error

    // Runs after each model call, in reverse registration order.
    // Inspect or rewrite state.Response, or call state.Stop() to end the loop.
    AfterModel(ctx context.Context, state *agent.State) error

    // Composes around the model call (first middleware outermost).
    WrapModelCall(next agent.CallFunc) agent.CallFunc
}
```

Ordering, for middleware registered as `[m1, m2]`:

| Hook | Order |
|------|-------|
| `BeforeModel` | `m1`, then `m2` |
| `WrapModelCall` | `m1` outermost, `m2` inner |
| `AfterModel` | `m2`, then `m1` |

### Example: a logging middleware

```go
type Logging struct{ agent.BaseMiddleware }

func (Logging) AfterModel(_ context.Context, s *agent.State) error {
    log.Printf("iteration %d: %d tool calls, %d tokens",
        s.Iteration, len(s.Response.ToolCalls), s.Response.Usage.TotalTokens)
    return nil
}

a := agent.New(provider,
    agent.WithModel("claude-sonnet-4-6"),
    agent.WithMiddleware(Logging{}),
)
```

### Stopping the loop early

Call `state.Stop()` in `AfterModel` to end the loop after the current
iteration, even if the model requested tools. The current response is returned.

## Built-in middleware

| Built-in | Hook | Purpose |
|----------|------|---------|
| `SummarizationMiddleware` | BeforeModel | Compress long histories to avoid context overflow |
| `PIIRedactionMiddleware` | Before/After | Mask emails, phone numbers, card numbers |
| `HumanInLoop` | executor gate | Approve or reject tool calls before they run |

See the sections below for each.

### Summarization

`SummarizationMiddleware` keeps long conversations within the context window.
Before each model call it estimates the token count of the message history; if
it exceeds a threshold, the older messages are replaced with a single
LLM-generated summary while the most recent messages are kept verbatim.

```go
// A cheaper model is a common choice for the summarization call.
summarizer := agent.NewSummarization(provider, "claude-haiku-4-5-20251001",
    agent.WithSummaryThreshold(3000), // trigger above ~3000 estimated tokens
    agent.WithKeepRecent(4),          // keep the last 4 messages verbatim
)

a := agent.New(provider,
    agent.WithModel("claude-sonnet-4-6"),
    agent.WithMiddleware(summarizer),
)
```

The token count is a zero-dependency estimate (~4 characters per token), so it
can underestimate for multilingual or code-heavy content — set
`WithSummaryThreshold` conservatively if exact budgets matter.

The summarizer never splits a tool call from its result: if the kept tail would
begin with an orphaned `tool` message, that message is pulled into the summary
instead. The provider and model passed to `NewSummarization` may differ from the
agent's main model. Override the summarization instruction with
`WithSummaryPrompt`.

### PII redaction

`PIIRedactionMiddleware` masks personally identifiable information with regular
expressions. It ships with patterns for email addresses, credit-card numbers,
and phone numbers, and is zero-dependency (stdlib `regexp` only). By default it
redacts the messages sent to the model; opt in to also redact responses.

```go
redactor := agent.NewPIIRedaction(
    agent.WithRedactOutput(true), // also mask the model's responses
    agent.WithCustomPattern(regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`), "[REDACTED_SSN]"),
)

a := agent.New(provider,
    agent.WithModel("claude-sonnet-4-6"),
    agent.WithMiddleware(redactor),
)
```

| Option | Default | Effect |
|--------|---------|--------|
| `WithRedactInput(bool)` | `true` | Redact outgoing messages (BeforeModel) |
| `WithRedactOutput(bool)` | `false` | Redact response content (AfterModel) |
| `WithCustomPattern(re, repl)` | — | Add a custom pattern, applied after the built-ins |

Redaction covers message content and the text of multimodal content parts; it
does not rewrite tool-call arguments. The built-in patterns favor
over-redaction — the card pattern matches any 13–16 digit group, so
domain-specific numeric identifiers may also be masked.

### Human-in-the-loop

`HumanInLoop` wraps a `tools.Executor` with an approval gate. Before a guarded
tool runs, an `Approver` is consulted: approved calls execute normally, while
rejected calls return a rejection result to the model so it can adapt. This is
the interrupt pattern — a human reviews sensitive actions before they take
effect.

```go
gate := agent.NewHumanInLoop(realExecutor,
    func(ctx context.Context, call langrails.ToolCall) (agent.Decision, error) {
        // Block on a human decision (channel, HTTP callback, CLI prompt, ...).
        if userApproves(call) {
            return agent.Approve(), nil
        }
        return agent.Reject("user declined"), nil
    },
    agent.WithInterruptOn("send_email", "delete_file"), // only guard risky tools
)

a := agent.New(provider,
    agent.WithModel("claude-sonnet-4-6"),
    agent.WithTools(toolDefs, gate), // use the gate in place of the raw executor
)
```

By default every tool requires approval; `WithInterruptOn` narrows that to a
named subset. The approver may **block** while waiting for a human and may
return an **error** to abort the run.

**Durable pause and resume.** To pause across a process restart — e.g. a
multi-day approval — run the agent inside a graph node and enable
[durable execution](durable-execution.md). Have the approver return an error to
stop the run; the graph checkpoint captures the state, and `Resume` continues
once the human responds.
