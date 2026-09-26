# Observability

langrails exposes hooks at three levels, so you can log, measure and trace
without wrapping every call site yourself. All of them are plain Go
callbacks: there is no dependency on a tracing library, and an
OpenTelemetry (or any other) integration is a few lines on top.

| Level | API | Sees |
|-------|-----|------|
| Provider | `langrails.WithHooks(p, langrails.Hooks{...})` | Every `Complete` / `Stream` call |
| Agent | a middleware implementing `WrapModelCall` / `WrapToolCall` | Every model call and tool execution in the loop |
| Graph | `graph.WithHooks[S](graph.Hooks[S]{...})` | Every node execution, including fan-out branches |

## Provider hooks

```go
provider := langrails.WithHooks(openai.New("sk-..."), langrails.Hooks{
    OnRequest: func(ctx context.Context, req *langrails.CompletionRequest) context.Context {
        log.Printf("→ %s (%d messages)", req.Model, len(req.Messages))
        return ctx
    },
    OnResponse: func(ctx context.Context, req *langrails.CompletionRequest, resp *langrails.CompletionResponse, d time.Duration) {
        log.Printf("← %s: %d tokens in %v", req.Model, resp.Usage.TotalTokens, d)
    },
    OnError: func(ctx context.Context, req *langrails.CompletionRequest, err error, d time.Duration) {
        log.Printf("✗ %s after %v: %v", req.Model, d, err)
    },
})
```

| Hook | When |
|------|------|
| `OnRequest` | Before each call. The context it returns is used for the call and passed to the other hooks. |
| `OnResponse` | After a successful call. For `Stream`, when the stream ends, with a response assembled from the events (content, reasoning, tool calls, citations, usage). |
| `OnError` | When a call fails — including a stream that ends with an error event, or whose reader cancels the context. |
| `OnStreamEvent` | For every stream event, before the caller receives it. |

Every field is optional. Hooks run synchronously, so keep them fast.

### Tracing

Because `OnRequest` returns a context, a span can be started there and ended
in `OnResponse` / `OnError`. With OpenTelemetry:

```go
tracer := otel.Tracer("langrails")

provider := langrails.WithHooks(p, langrails.Hooks{
    OnRequest: func(ctx context.Context, req *langrails.CompletionRequest) context.Context {
        ctx, _ = tracer.Start(ctx, "llm "+req.Model)
        return ctx
    },
    OnResponse: func(ctx context.Context, _ *langrails.CompletionRequest, resp *langrails.CompletionResponse, _ time.Duration) {
        span := trace.SpanFromContext(ctx)
        span.SetAttributes(
            attribute.Int("llm.usage.prompt_tokens", resp.Usage.PromptTokens),
            attribute.Int("llm.usage.completion_tokens", resp.Usage.CompletionTokens),
        )
        span.End()
    },
    OnError: func(ctx context.Context, _ *langrails.CompletionRequest, err error, _ time.Duration) {
        span := trace.SpanFromContext(ctx)
        span.RecordError(err)
        span.End()
    },
})
```

### Where to put it

`WithHooks` composes with the other decorators. Put it **inside** a retry to
see every attempt, or **outside** to see one call per logical request:

```go
// One span per attempt:
p := langrails.WithRetry(langrails.WithHooks(base, hooks), 3)

// One span per request, retries included:
p := langrails.WithHooks(langrails.WithRetry(base, 3), hooks)
```

## Agent hooks

Model calls inside an agent already go through the provider, so provider
hooks cover them. Tool executions are visible to middleware that implements
the optional `agent.ToolWrapper` interface:

```go
type ToolTimer struct{ agent.BaseMiddleware }

func (ToolTimer) WrapToolCall(next agent.ToolFunc) agent.ToolFunc {
    return func(ctx context.Context, call langrails.ToolCall) (string, error) {
        start := time.Now()
        out, err := next(ctx, call)
        log.Printf("tool %s took %v (err=%v)", call.Name, time.Since(start), err)
        return out, err
    }
}

a := agent.New(provider, agent.WithModel("gpt-4o"), agent.WithMiddleware(ToolTimer{}))
```

See [Agents](agents.md#middleware) for the full middleware model.

## Graph hooks

```go
result, err := g.Run(ctx, initial, graph.WithHooks[State](graph.Hooks[State]{
    OnNodeStart: func(ctx context.Context, node string, s State) context.Context {
        return ctx
    },
    OnNodeEnd: func(ctx context.Context, node string, s State, err error, d time.Duration) {
        log.Printf("node %s: %v (err=%v)", node, d, err)
    },
}))
```

- `OnNodeStart`'s returned context is passed to the node, so spans nest: a
  provider span started inside a node becomes a child of the node span.
- Fan-out branches run concurrently, so graph hooks must be safe to call from
  several goroutines at once.
- Hooks are a per-run option like the checkpointer. A subgraph embedded with
  `AsNode` runs with its own options, so it appears as a single node to the
  parent's hooks.
- `Stream` still works alongside hooks: it reports each node's *output*,
  hooks additionally see timing, errors and the state going *in*.
