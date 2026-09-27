# langrails

> Unified LLM provider interface for Go. One API, 24 providers.

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
| **Tool Calling** | Typed tools from Go functions, automatic tool execution loop, `ToolChoice` control |
| **Reasoning** | Provider-agnostic `ReasoningEffort` (minimal/low/medium/high), reasoning text + token accounting |
| **Web Search & Citations** | Provider-native search (`ServerTools`) with unified `Citations` |
| **Caching** | Provider prompt caching (`CacheControl` + cached-token reporting) and a client-side response cache |
| **Agents** | Middleware-driven tool-calling loop with token streaming; built-in summarization, context editing, tool selection, call limits, model fallback, tool retry, PII redaction, human-in-the-loop |
| **Chain** | Sequential multi-step prompt pipelines |
| **Graph** | LangGraph-style stateful workflows: parallel fan-out, subgraphs, streaming, durable execution, human-in-the-loop interrupts |
| **MCP** | Model Context Protocol client: HTTP and stdio transports, tools, resources, prompts |
| **A2A** | Agent-to-Agent protocol client + server |
| **Structured Output** | `Generate[T]` decodes into Go types; JSON schema + JSON mode across all providers |
| **Vision / Multimodal** | Images, audio and documents (PDF) in messages |
| **Prompt Templates** | Jinja-style `{{ variable }}` syntax |
| **Memory** | Conversation history with token limits |
| **Observability** | Provider, agent (tool-call) and graph (node) hooks; tracing-ready via context |
| **Retry, Fallback & Rate Limiting** | Composable resilience decorators; jittered backoff, `Retry-After`, token-bucket limiter |
| **Zero Dependencies** | Only Go standard library |

## Install

```bash
go get github.com/promptrails/langrails
```

Requires Go 1.26.3+ (see `go.mod`).

## Documentation

| | |
|---|---|
| [Getting Started](getting-started.md) | Installation, first request, error handling |
| [Providers](providers.md) | All providers, config examples |
| [Parameters](parameters.md) | All parameters, provider support matrix |
| [Streaming](streaming.md) | Real-time token streaming |
| [Reasoning](reasoning.md) | Reasoning effort, thinking output, tokens |
| [Web Search & Citations](web-search.md) | Provider-native search + citations |
| [Caching](caching.md) | Prompt caching, cached-token reporting, response cache |
| [Vision / Multimodal](vision.md) | Images, audio and documents in messages |
| [Structured Output](structured-output.md) | JSON schema + JSON mode |
| [Prompt Templates](prompt-templates.md) | Jinja-style variable substitution |
| [Memory](memory.md) | Conversation history management |
| [Tool Calling](tool-calling.md) | Function calling + automatic tool loop |
| [Agents (Middleware)](agents.md) | Middleware agent loop + built-in middleware |
| [Chain](chain.md) | Sequential prompt pipelines |
| [Graph](graph.md) | Stateful workflows, fan-out, subgraphs, streaming |
| [Durable Execution](durable-execution.md) | Checkpointing, resume, interrupts, time travel |
| [MCP](mcp.md) | MCP client: HTTP/stdio, tools, resources, prompts |
| [A2A](a2a.md) | Agent-to-Agent protocol client + server |
| [Retry, Fallback & Rate Limiting](retry-fallback.md) | Retries, fallbacks, rate limiting |
| [Observability](observability.md) | Provider, agent and graph hooks; tracing |

## Quick Links

- [GitHub Repository](https://github.com/promptrails/langrails)
- [Go Package Reference](https://pkg.go.dev/github.com/promptrails/langrails)
