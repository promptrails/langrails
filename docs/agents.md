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

## Streaming

`Stream` runs the same loop but reports progress as it happens — ideal for
chat UIs that show tokens and tool activity live:

```go
for ev := range a.Stream(ctx, "What's the weather in Istanbul?") {
    switch ev.Type {
    case agent.EventReasoning:
        fmt.Print(ev.Reasoning)
    case agent.EventContent:
        fmt.Print(ev.Content)
    case agent.EventToolCall:
        fmt.Printf("\n[calling %s(%s)]\n", ev.ToolCall.Name, ev.ToolCall.Arguments)
    case agent.EventToolResult:
        fmt.Printf("[result: %s]\n", ev.Content)
    case agent.EventDone:
        fmt.Println("\ntokens:", ev.Result.TotalUsage.TotalTokens)
    case agent.EventError:
        log.Fatal(ev.Error)
    }
}
```

| Event | Fields |
|-------|--------|
| `EventContent` | `Content` — a text chunk |
| `EventReasoning` | `Reasoning` — a reasoning chunk |
| `EventToolCall` | `ToolCall` — about to execute |
| `EventToolResult` | `ToolCall`, `Content` (result), `ToolError` |
| `EventDone` | `Result` — same as `Run` returns; always the last event |
| `EventError` | `Error` — always the last event |

Every event carries its loop `Iteration`. `StreamMessages` takes a full
history, like `RunMessages`.

Model calls use the provider's `Stream` method and still pass through all
middleware, which sees the assembled response. Two consequences:

- Tokens reach you **before** `AfterModel` runs, so output-rewriting middleware
  (e.g. `WithRedactOutput`) cannot retract text already streamed; the final
  `Result` carries the rewritten response.
- If a middleware answers without calling the provider (a cache, a canned
  reply), its content arrives as a single `EventContent`.

Cancel the context to stop early; the channel is always closed.

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

### Wrapping tool calls

A middleware may also implement the optional `agent.ToolWrapper` interface to
wrap every tool execution — the tool-side counterpart of `WrapModelCall`, with
the same ordering (first registered outermost):

```go
type ToolWrapper interface {
    WrapToolCall(next agent.ToolFunc) agent.ToolFunc
}
```

Use it to log, time, retry or block tool calls. An error returned from the
chain is not fatal: it is sent to the model as the tool result, like an
executor error. See [Observability](observability.md) for a timing example.

### Stopping the loop early

Call `state.Stop()` in `AfterModel` to end the loop after the current
iteration, even if the model requested tools. The current response is returned.

## Built-in middleware

| Built-in | Hook | Purpose |
|----------|------|---------|
| `SummarizationMiddleware` | BeforeModel | Compress long histories to avoid context overflow |
| `ContextEditing` | BeforeModel | Clear old tool results past a token threshold (no model call) |
| `ToolSelector` | BeforeModel | Let a cheap model pick the relevant tools from a large set |
| `PIIRedactionMiddleware` | Before/After | Mask emails, phone numbers, card numbers |
| `ModelCallLimit` | AfterModel | Cap model calls per run; end gracefully or fail |
| `ModelFallback` | WrapModelCall | Retry a failed call with other models |
| `ToolCallLimit` | WrapToolCall | Cap tool executions per run (all or per tool) |
| `ToolRetry` | WrapToolCall | Retry failing tools with backoff |
| `HumanInLoop` | executor gate | Approve or reject tool calls before they run |

They compose freely:

```go
a := agent.New(provider,
    agent.WithModel("gpt-4o"),
    agent.WithTools(set.Definitions(), set),
    agent.WithMiddleware(
        agent.NewToolSelector(provider, "gpt-4o-mini", agent.WithMaxTools(5)),
        agent.NewContextEditing(),
        agent.NewModelFallback("gpt-4o-mini"),
        agent.NewModelCallLimit(10),
        agent.NewToolCallLimit(3, "web_search"),
        agent.NewToolRetry(2),
    ),
)
```

See the sections below for each.

### Call limits

`ModelCallLimit` caps model calls per run. When the model still wants tools
after the last allowed call, the run **ends gracefully** with that response
(its tool calls unexecuted), or fails with `agent.ErrCallLimit` if you ask:

```go
agent.NewModelCallLimit(5)               // end with the 5th response
agent.NewModelCallLimit(5).FailOnLimit() // error instead
```

This differs from `WithMaxIterations`, which always fails.

`ToolCallLimit` caps tool executions per run — all tools together, or each
named tool separately. A call over the limit is **not executed**; the model
gets an error result saying the limit was reached, so it can answer with what
it already has:

```go
agent.NewToolCallLimit(10)                // at most 10 tool runs in total
agent.NewToolCallLimit(2, "web_search")   // at most 2 searches; others unlimited
```

Counts are per run, so one agent (and one middleware value) can serve many
runs concurrently.

### Tool retry

```go
agent.NewToolRetry(3,
    agent.WithToolRetryDelay(200*time.Millisecond), // doubles each retry
    agent.WithToolRetryIf(isTransient),             // default: retry any error
    agent.WithToolRetryOn("fetch_url"),             // default: every tool
)
```

Only the last error reaches the model.

### Model fallback

`ModelFallback` retries a failed model call with other models **on the same
provider**, in order:

```go
agent.NewModelFallback("gpt-4o-mini", "gpt-3.5-turbo")
```

To fall back to a different provider, wrap the provider instead:
`langrails.WithFallback(openaiProvider, anthropicProvider)`.

### Context editing

For tool-heavy runs, old tool results are usually the bulk of the context and
the least useful part of it. `ContextEditing` replaces the content of all but
the most recent tool results with a placeholder once the history passes a
token threshold. Unlike summarization it needs no model call, and the model
still sees that each call happened.

```go
agent.NewContextEditing(
    agent.WithClearThreshold(50_000), // default 100,000 estimated tokens
    agent.WithKeepToolResults(3),     // default 3
    agent.WithClearedPlaceholder("[cleared]"),
)
```

### Tool selection

With dozens of tools, sending all of them on every call costs tokens and
raises the chance of a wrong pick. `ToolSelector` asks a separate, cheaper
model to choose the relevant ones for the user's request (via structured
output, constrained to your tool names), once per run:

```go
agent.NewToolSelector(provider, "gpt-4o-mini",
    agent.WithMaxTools(5),               // default 5; skipped when you have ≤ 5
    agent.WithAlwaysInclude("handoff"),  // kept regardless, not counted
)
```

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

**Durable pause and resume.** `HumanInLoop` blocks while it waits. To pause
across a process restart — e.g. a multi-day approval — put the decision in a
graph node with `graph.Await` and enable a checkpointer: the run stops, is saved,
and continues when you `Resume` it with the answer. See
[Interrupts](durable-execution.md#interrupts-human-in-the-loop).
