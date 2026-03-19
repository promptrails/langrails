# Retry & Fallback

unillm provides composable decorators for building resilient LLM applications.

## Retry

Automatically retry on transient errors (rate limits, server errors):

```go
provider := unillm.WithRetry(openai.New("sk-..."), 3)
// 3 retries with exponential backoff: 1s, 2s, 4s
```

### Custom Backoff

```go
provider := unillm.WithRetry(openai.New("sk-..."), 5,
    unillm.WithBaseDelay(500 * time.Millisecond),
)
// 500ms, 1s, 2s, 4s, 8s
```

### What Gets Retried

Only retryable errors trigger retries:
- **429 (Rate Limit)** — retried
- **5xx (Server Error)** — retried
- **401/403 (Auth Error)** — NOT retried
- **400 (Bad Request)** — NOT retried
- **Network errors** — NOT retried (no APIError)

### Context Cancellation

Retries respect context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// Will stop retrying if context expires
resp, err := provider.Complete(ctx, req)
```

### Streaming

For streaming, only the initial connection is retried. Mid-stream failures are not retried (the stream would need to restart from the beginning).

## Fallback

Automatically switch to a backup provider on failure:

```go
provider := unillm.WithFallback(
    openai.New("sk-..."),       // Primary
    anthropic.New("sk-ant-..."), // Fallback
)
```

Any error from the primary triggers the fallback — not just retryable errors.

## Composing Retry + Fallback

```go
// Each provider retries independently, then falls back
provider := unillm.WithFallback(
    unillm.WithRetry(openai.New("sk-..."), 3),
    unillm.WithRetry(anthropic.New("sk-ant-..."), 3),
)
```

This gives you: OpenAI (try 4 times) → Anthropic (try 4 times).

## Chaining Multiple Fallbacks

```go
provider := unillm.WithFallback(
    openai.New("sk-..."),
    unillm.WithFallback(
        anthropic.New("sk-ant-..."),
        groq.New("gsk-..."),
    ),
)
```

OpenAI → Anthropic → Groq priority chain.

## Interface Compliance

Both `RetryProvider` and `FallbackProvider` implement `unillm.Provider`, so they work everywhere a provider is expected — including chains, graphs, and tool loops.
