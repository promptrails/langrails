# Retry, Fallback & Rate Limiting

langrails provides composable decorators for building resilient LLM applications.

## Retry

Automatically retry on transient errors (rate limits, server errors):

```go
provider := langrails.WithRetry(openai.New("sk-..."), 3)
// 3 retries with exponential backoff: ~1s, ~2s, ~4s
```

### Custom Backoff

```go
provider := langrails.WithRetry(openai.New("sk-..."), 5,
    langrails.WithBaseDelay(500 * time.Millisecond),
    langrails.WithMaxDelay(10 * time.Second), // cap one wait (default: 1 minute)
)
// ~500ms, ~1s, ~2s, ~4s, ~8s
```

Each delay is **jittered** — randomized between half and all of its nominal
value — so many clients failing at the same moment don't retry in lockstep.
Use `langrails.WithoutJitter()` for exact delays.

### Retry-After

When a provider says how long to wait (`Retry-After` or `retry-after-ms`
header on a 429/5xx), that wait replaces the backoff. It is exposed as
`APIError.RetryAfter`. If the provider asks for longer than the max delay,
the error is returned right away instead of retrying early into another
rejection.

Writing your own provider? Fill it with
`langrails.RetryAfterFromHeader(resp.Header)`.

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

## Rate Limiting

Cap how fast requests leave the client — useful for staying under a
provider's per-minute quota instead of hitting 429s:

```go
// At most 5 requests per second, allowing bursts of up to 10.
provider := langrails.WithRateLimit(openai.New("sk-..."), 5, 10)

// Slower than one per second works too: 30 requests per minute.
provider := langrails.WithRateLimit(openai.New("sk-..."), 0.5, 1)
```

It is a token bucket: callers over the limit block until a slot frees up or
their context ends (the slot is then given back). Share one wrapped provider
across goroutines to limit them together. `Wait(ctx)` is exported if you need
to reserve a slot for something other than a completion.

Put the limiter **inside** the retry so that retries are limited too:

```go
provider := langrails.WithRetry(langrails.WithRateLimit(p, 5, 10), 3)
```

## Fallback

Automatically switch to a backup provider on failure:

```go
provider := langrails.WithFallback(
    openai.New("sk-..."),       // Primary
    anthropic.New("sk-ant-..."), // Fallback
)
```

Any error from the primary triggers the fallback — not just retryable errors.

## Composing Retry + Fallback

```go
// Each provider retries independently, then falls back
provider := langrails.WithFallback(
    langrails.WithRetry(openai.New("sk-..."), 3),
    langrails.WithRetry(anthropic.New("sk-ant-..."), 3),
)
```

This gives you: OpenAI (try 4 times) → Anthropic (try 4 times).

## Chaining Multiple Fallbacks

```go
provider := langrails.WithFallback(
    openai.New("sk-..."),
    langrails.WithFallback(
        anthropic.New("sk-ant-..."),
        groq.New("gsk-..."),
    ),
)
```

OpenAI → Anthropic → Groq priority chain.

## Interface Compliance

`RetryProvider`, `FallbackProvider` and `RateLimitProvider` implement `langrails.Provider`, so they work everywhere a provider is expected — including chains, graphs, and tool loops.
