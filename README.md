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
- **Tool loop** — Automatic LLM → tool → LLM execution cycle
- **Chain** — Sequential multi-step prompt pipelines
- **Graph** — LangGraph-style stateful workflow engine with conditional routing
- **MCP** — Model Context Protocol client for external tool integration
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

## Tool Loop

Automatic LLM → tool → LLM cycle:

```go
import "github.com/promptrails/unillm/tools"

executor := tools.NewMap(map[string]tools.Func{
    "get_weather": func(ctx context.Context, args string) (string, error) {
        // Parse args, call API, return result
        return `{"temp": 22, "condition": "sunny"}`, nil
    },
})

result, err := tools.RunLoop(ctx, provider, &unillm.CompletionRequest{
    Model: "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Weather in Istanbul?"}},
    Tools: toolDefs,
}, executor)

fmt.Println(result.Response.Content)  // "It's 22°C and sunny in Istanbul."
fmt.Println(result.Iterations)        // 2 (initial + after tool result)
```

## Chain

Sequential prompt chain — output of each step feeds into the next:

```go
import "github.com/promptrails/unillm/chain"

c := chain.New(provider, []chain.Step{
    {SystemPrompt: "Summarize the following text in 2 sentences."},
    {SystemPrompt: "Translate the following to Turkish."},
}, chain.WithModel("gpt-4o"))

result, err := c.Run(ctx, "Long article text here...")
fmt.Println(result.Output) // Turkish summary
```

Steps support per-step providers, models, transforms, and input templates:

```go
chain.Step{
    SystemPrompt:  "Analyze sentiment",
    Provider:      anthropic.New("sk-ant-..."),  // different provider for this step
    Model:         "claude-sonnet-4-20250514",
    InputTemplate: "Analyze the sentiment of:\n\n{input}",
    Transform:     func(s string) string { return strings.TrimSpace(s) },
}
```

## Graph (LangGraph-style)

Stateful workflow with conditional routing:

```go
import "github.com/promptrails/unillm/graph"

type State struct {
    Input     string
    Sentiment string
    Output    string
}

g := graph.New[State]()

g.AddNode("classify", func(ctx context.Context, s State) (State, error) {
    // Call LLM to classify sentiment
    resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{...})
    s.Sentiment = resp.Content
    return s, nil
})

g.AddNode("positive", func(ctx context.Context, s State) (State, error) {
    s.Output = "Thank you for the positive feedback!"
    return s, nil
})

g.AddNode("negative", func(ctx context.Context, s State) (State, error) {
    s.Output = "We're sorry to hear that. How can we help?"
    return s, nil
})

g.SetEntryPoint("classify")
g.AddConditionalEdge("classify", func(s State) string {
    if s.Sentiment == "positive" { return "positive" }
    return "negative"
})
g.AddEdge("positive", graph.END)
g.AddEdge("negative", graph.END)

result, _ := g.Run(ctx, State{Input: "I love this product!"})
fmt.Println(result.State.Output)
```

## MCP (Model Context Protocol)

Connect to MCP servers and use their tools:

```go
import (
    "github.com/promptrails/unillm/mcp"
    "github.com/promptrails/unillm/tools"
)

// Connect to MCP server
client, _ := mcp.NewClient("http://localhost:8080/mcp",
    mcp.WithBearerToken("token"),
)
defer client.Close()

// Get tool definitions for the LLM
toolDefs := client.ToolDefinitions()

// Use MCP tools in the tool loop (client implements tools.Executor)
result, _ := tools.RunLoop(ctx, provider, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Search for Go tutorials"}},
    Tools:    toolDefs,
}, client)
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
├── tools/          # Tool loop execution
├── chain/          # Sequential prompt chains
├── graph/          # LangGraph-style workflow engine
├── mcp/            # MCP client
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
