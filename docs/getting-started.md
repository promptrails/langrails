# Getting Started

## Installation

```bash
go get github.com/promptrails/langrails
```

Requires Go 1.22 or later. No external dependencies — only Go standard library.

## Quick Start (Registry)

The easiest way to create a provider is using the `llm` registry:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/promptrails/langrails"
    "github.com/promptrails/langrails/llm"
)

func main() {
    // Create a provider using the registry
    provider, err := llm.New(llm.OpenAI, "sk-your-api-key")
    if err != nil {
        log.Fatal(err)
    }

    // Send a completion request
    resp, err := provider.Complete(context.Background(), &langrails.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "You are a helpful assistant.",
        Messages: []langrails.Message{
            {Role: "user", Content: "What is Go?"},
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(resp.Content)
    fmt.Printf("Tokens used: %d\n", resp.Usage.TotalTokens)
}
```

## Switching Providers

With the registry, switching providers is a single constant change:

```go
// OpenAI
provider, _ := llm.New(llm.OpenAI, "sk-...")

// Anthropic
provider, _ := llm.New(llm.Anthropic, "sk-ant-...")

// Gemini
provider, _ := llm.New(llm.Gemini, "your-key")

// Local (Ollama — no key needed)
provider, _ := llm.New(llm.Ollama, "")
```

Your `CompletionRequest` stays the same. langrails handles the API differences internally.

## Direct Provider Import

You can also import providers directly for provider-specific options:

```go
import "github.com/promptrails/langrails/llm/openai"

provider := openai.New("sk-...",
    openai.WithBaseURL("https://my-proxy.com/v1/chat/completions"),
    openai.WithHTTPClient(&http.Client{Timeout: 2 * time.Minute}),
)
```

## Request Parameters

```go
temp := 0.7
maxTokens := 1000

req := &langrails.CompletionRequest{
    Model:        "gpt-4o",
    SystemPrompt: "You are a helpful assistant.",
    Messages: []langrails.Message{
        {Role: "user", Content: "Explain quantum computing"},
    },
    Temperature: &temp,      // Optional: 0-2 range
    MaxTokens:   &maxTokens, // Optional: max output tokens
}
```

## Response Structure

```go
resp, err := provider.Complete(ctx, req)

resp.Content       // Generated text
resp.ToolCalls     // Tool/function calls (if any)
resp.Usage         // Token usage stats
resp.FinishReason  // "stop", "tool_calls", "length", etc.
resp.Model         // Actual model used
```

## Error Handling

All providers return `*langrails.APIError` for HTTP errors:

```go
resp, err := provider.Complete(ctx, req)
if err != nil {
    var apiErr *langrails.APIError
    if errors.As(err, &apiErr) {
        fmt.Printf("Provider: %s\n", apiErr.Provider)
        fmt.Printf("Status: %d\n", apiErr.StatusCode)
        fmt.Printf("Message: %s\n", apiErr.Message)

        if apiErr.IsAuthError() {
            // Invalid API key (401/403)
        }
        if apiErr.IsRateLimitError() {
            // Rate limited (429)
        }
        if apiErr.IsRetryable() {
            // Safe to retry (429, 5xx)
        }
    }
}
```

## Next Steps

- [Streaming](streaming.md) — Real-time token streaming
- [Tool Calling](tool-calling.md) — Function/tool calling
- [Providers](providers.md) — All 12 supported providers
- [Chain](chain.md) — Sequential prompt pipelines
- [Graph](graph.md) — Stateful workflow engine
- [MCP](mcp.md) — Model Context Protocol integration
