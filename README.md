# unillm

Unified LLM provider interface for Go. One API, 11 providers.

[![Go Reference](https://pkg.go.dev/badge/github.com/promptrails/unillm.svg)](https://pkg.go.dev/github.com/promptrails/unillm)
[![CI](https://github.com/promptrails/unillm/actions/workflows/ci.yml/badge.svg)](https://github.com/promptrails/unillm/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/promptrails/unillm)](https://goreportcard.com/report/github.com/promptrails/unillm)

```go
provider := openai.New("sk-...")
resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Hello!"}},
})
fmt.Println(resp.Content)
```

Switch providers by changing one line:

```go
provider := anthropic.New("sk-ant-...")  // or gemini, deepseek, groq, ...
```

## Features

- **11 providers** — OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere
- **Streaming** — Channel-based, idiomatic Go
- **Tool calling** — Unified interface across all providers
- **Structured output** — JSON schema support (OpenAI-compatible providers)
- **Retry** — Exponential backoff for rate limits and server errors
- **Fallback** — Automatic failover to backup providers
- **Zero dependencies** — Only Go standard library

## Install

```bash
go get github.com/promptrails/unillm
```

## Quick Start

### Basic Completion

```go
package main

import (
    "context"
    "fmt"

    "github.com/promptrails/unillm"
    "github.com/promptrails/unillm/openai"
)

func main() {
    provider := openai.New("sk-...")

    resp, err := provider.Complete(context.Background(), &unillm.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "You are a helpful assistant.",
        Messages: []unillm.Message{
            {Role: "user", Content: "What is Go?"},
        },
    })
    if err != nil {
        panic(err)
    }

    fmt.Println(resp.Content)
    fmt.Printf("Tokens: %d prompt, %d completion\n",
        resp.Usage.PromptTokens, resp.Usage.CompletionTokens)
}
```

### Streaming

```go
events, err := provider.Stream(ctx, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Write a poem"}},
})
if err != nil {
    panic(err)
}

for event := range events {
    switch event.Type {
    case unillm.EventContent:
        fmt.Print(event.Content)
    case unillm.EventDone:
        fmt.Println("\n--- done ---")
    case unillm.EventError:
        fmt.Fprintf(os.Stderr, "error: %v\n", event.Error)
    }
}
```

### Tool Calling

```go
resp, err := provider.Complete(ctx, &unillm.CompletionRequest{
    Model: "gpt-4o",
    Messages: []unillm.Message{
        {Role: "user", Content: "What's the weather in Istanbul?"},
    },
    Tools: []unillm.ToolDefinition{{
        Name:        "get_weather",
        Description: "Get current weather for a city",
        Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`),
    }},
})

if len(resp.ToolCalls) > 0 {
    tc := resp.ToolCalls[0]
    fmt.Printf("Tool: %s, Args: %s\n", tc.Name, tc.Arguments)

    // Send tool result back
    resp, _ = provider.Complete(ctx, &unillm.CompletionRequest{
        Model: "gpt-4o",
        Messages: []unillm.Message{
            {Role: "user", Content: "What's the weather in Istanbul?"},
            {Role: "assistant", ToolCalls: resp.ToolCalls},
            {Role: "tool", ToolCallID: tc.ID, Content: `{"temp": 22, "condition": "sunny"}`},
        },
        Tools: tools, // same tools as before
    })
}
```

### Retry + Fallback

```go
// Retry with exponential backoff (1s, 2s, 4s)
provider := unillm.WithRetry(openai.New("sk-..."), 3)

// Fallback to another provider
provider = unillm.WithFallback(
    openai.New("sk-..."),
    anthropic.New("sk-ant-..."),
)

// Compose both
provider = unillm.WithFallback(
    unillm.WithRetry(openai.New("sk-..."), 3),
    unillm.WithRetry(anthropic.New("sk-ant-..."), 3),
)
```

## Providers

| Provider | Package | Models |
|----------|---------|--------|
| OpenAI | `unillm/openai` | GPT-4o, GPT-4, GPT-3.5 |
| Anthropic | `unillm/anthropic` | Claude Sonnet, Opus, Haiku |
| Google Gemini | `unillm/gemini` | Gemini 2.0 Flash, 1.5 Pro |
| DeepSeek | `unillm/deepseek` | DeepSeek Chat, Coder |
| Groq | `unillm/groq` | Llama, Mixtral (fast inference) |
| Fireworks | `unillm/fireworks` | Llama, Mixtral |
| xAI | `unillm/xai` | Grok |
| OpenRouter | `unillm/openrouter` | 100+ models via routing |
| Together | `unillm/together` | Llama, Mixtral, Qwen |
| Mistral | `unillm/mistral` | Mistral Large, Medium |
| Cohere | `unillm/cohere` | Command R+ |

### Provider-Specific Options

```go
// Custom base URL (Azure OpenAI, proxies)
openai.New("key", openai.WithBaseURL("https://my-proxy.com/v1/chat/completions"))

// Custom HTTP client
openai.New("key", openai.WithHTTPClient(&http.Client{Timeout: 10 * time.Second}))

// OpenRouter site info for ranking
openrouter.New("key", openrouter.WithSiteInfo("https://myapp.com", "My App"))
```

## Architecture

```
unillm/
├── provider.go     # Provider interface
├── message.go      # Message, ToolCall, ToolDefinition
├── stream.go       # StreamEvent types
├── errors.go       # APIError with helpers
├── retry.go        # RetryProvider decorator
├── fallback.go     # FallbackProvider decorator
├── compat/         # Shared OpenAI-compatible logic
├── openai/         # OpenAI
├── anthropic/      # Anthropic (Claude)
├── gemini/         # Google Gemini
├── deepseek/       # DeepSeek
├── groq/           # Groq
├── fireworks/      # Fireworks AI
├── xai/            # xAI (Grok)
├── openrouter/     # OpenRouter
├── together/       # Together AI
├── mistral/        # Mistral AI
└── cohere/         # Cohere
```

The `compat` package implements the full OpenAI-compatible protocol (request building, response parsing, SSE streaming, tool call accumulation). Providers like DeepSeek, Groq, etc. are thin wrappers that only specify their base URL and name.

OpenAI, Anthropic, and Gemini each have their own implementations due to API differences.

## License

MIT — [PromptRails](https://promptrails.com)
