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

| Middleware | Hook | Purpose |
|------------|------|---------|
| `SummarizationMiddleware` | BeforeModel | Compress long histories to avoid context overflow |
| `PIIRedactionMiddleware` | Before/After | Mask emails, phone numbers, card numbers |
| `HumanInLoopMiddleware` | AfterModel | Pause for approval before executing tool calls |

See the sections below for each.
