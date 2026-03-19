# unillm

> Unified LLM provider interface for Go. One API, 11 providers.

## What is unillm?

unillm is a lightweight Go library that provides a single interface for interacting with multiple LLM providers. Write your code once, switch providers by changing one line.

```go
provider := openai.New("sk-...")      // or anthropic, gemini, deepseek, groq, ...
resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Hello!"}},
})
```

## Features

| Feature | Description |
|---------|-------------|
| **11 Providers** | OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere |
| **Streaming** | Channel-based, idiomatic Go |
| **Tool Calling** | Unified interface + automatic tool execution loop |
| **Chain** | Sequential multi-step prompt pipelines |
| **Graph** | LangGraph-style stateful workflows with conditional routing |
| **MCP** | Model Context Protocol client for external tools |
| **Structured Output** | JSON schema support across all providers |
| **Retry & Fallback** | Composable resilience decorators |
| **Zero Dependencies** | Only Go standard library |

## Install

```bash
go get github.com/promptrails/unillm
```

Requires Go 1.22+.

## Quick Links

- [GitHub Repository](https://github.com/promptrails/unillm)
- [Go Package Reference](https://pkg.go.dev/github.com/promptrails/unillm)
- [Getting Started](getting-started.md)
