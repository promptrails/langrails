# Getting Started

## Installation

```bash
go get github.com/promptrails/unillm
```

Requires Go 1.22 or later. No external dependencies — only Go standard library.

## Your First Request

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/promptrails/unillm"
    "github.com/promptrails/unillm/openai"
)

func main() {
    // Create a provider
    provider := openai.New("sk-your-api-key")

    // Send a completion request
    resp, err := provider.Complete(context.Background(), &unillm.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "You are a helpful assistant.",
        Messages: []unillm.Message{
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

Every provider implements the same `unillm.Provider` interface. To switch from OpenAI to Anthropic, just change the import and constructor:

```go
// Before
import "github.com/promptrails/unillm/openai"
provider := openai.New("sk-...")

// After
import "github.com/promptrails/unillm/anthropic"
provider := anthropic.New("sk-ant-...")
```

Your `CompletionRequest` stays the same. unillm handles the API differences internally.

## Request Parameters

```go
temp := 0.7
maxTokens := 1000

req := &unillm.CompletionRequest{
    Model:        "gpt-4o",
    SystemPrompt: "You are a helpful assistant.",
    Messages: []unillm.Message{
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

All providers return `*unillm.APIError` for HTTP errors:

```go
resp, err := provider.Complete(ctx, req)
if err != nil {
    var apiErr *unillm.APIError
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
- [Providers](providers.md) — All 11 supported providers
- [Chain](chain.md) — Sequential prompt pipelines
- [Graph](graph.md) — Stateful workflow engine
- [MCP](mcp.md) — Model Context Protocol integration
