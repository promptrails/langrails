# langrails

> Unified LLM provider interface for Go. One API, 25 providers.

## What is langrails?

langrails is a lightweight Go library that provides a single interface for interacting with multiple LLM providers. Write your code once, switch providers by changing one line.

```go
import "github.com/promptrails/langrails/llm"

provider := llm.MustNew(llm.OpenAI, "sk-...")   // or Anthropic, Gemini, Bedrock, ...
resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Hello!"}},
})
```

## Features

| Feature | Description |
|---------|-------------|
| **25 Providers** | OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere, Perplexity, Ollama, Chutes AI, Z.AI, Moonshot (Kimi), Novita AI, DeepInfra, Friendli AI, Cerebras, SambaNova, Hyperbolic, Alibaba DashScope (Qwen), Hugging Face Router, Amazon Bedrock |
| **Streaming** | Channel-based, idiomatic Go |
| **Tool Calling** | Unified interface + automatic tool execution loop, with `ToolChoice` control |
| **Reasoning** | Provider-agnostic `ReasoningEffort` (minimal/low/medium/high), reasoning text + token accounting |
| **Web Search & Citations** | Provider-native search (`ServerTools`) with unified `Citations` |
| **Prompt Caching** | `CacheControl` + cached/cache-creation token reporting |
| **Chain** | Sequential multi-step prompt pipelines |
| **Graph** | LangGraph-style stateful workflows with conditional routing |
| **MCP** | Model Context Protocol client for external tools |
| **A2A** | Agent-to-Agent protocol client + server |
| **Structured Output** | JSON schema + JSON mode across all providers |
| **Vision / Multimodal** | Images + text in messages |
| **Prompt Templates** | Jinja-style `{{ variable }}` syntax |
| **Memory** | Conversation history with token limits |
| **Retry & Fallback** | Composable resilience decorators |
| **Zero Dependencies** | Only Go standard library |

## Install

```bash
go get github.com/promptrails/langrails
```

Requires Go 1.22+.

## Quick Links

- [GitHub Repository](https://github.com/promptrails/langrails)
- [Go Package Reference](https://pkg.go.dev/github.com/promptrails/langrails)
- [Getting Started](getting-started.md)
