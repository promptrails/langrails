# LangRails

Unified LLM provider interface for Go. One API, many providers.

[![Go Reference](https://pkg.go.dev/badge/github.com/promptrails/langrails.svg)](https://pkg.go.dev/github.com/promptrails/langrails)
[![CI](https://github.com/promptrails/langrails/actions/workflows/ci.yml/badge.svg)](https://github.com/promptrails/langrails/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/promptrails/langrails)](https://goreportcard.com/report/github.com/promptrails/langrails)

```go
import "github.com/promptrails/langrails/llm"

provider := llm.MustNew(llm.OpenAI, "sk-...")
resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Hello!"}},
})
fmt.Println(resp.Content)
```

Switch providers by changing one constant:

```go
provider := llm.MustNew(llm.Anthropic, "sk-ant-...")  // or Gemini, Ollama, ...
```

## Install

```bash
go get github.com/promptrails/langrails
```

## Features

- **24 providers** — OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere, Ollama, Chutes AI, Z.AI, Moonshot (Kimi), Novita AI, DeepInfra, Friendli AI, Cerebras, SambaNova, Hyperbolic, Alibaba DashScope (Qwen), Hugging Face Router, Amazon Bedrock (Converse API)
- **Streaming** — Channel-based, idiomatic Go
- **Tool calling** — Typed tools from Go functions (schema generated from structs), automatic tool execution loop, `ToolChoice` control
- **Reasoning** — Provider-agnostic `ReasoningEffort` (minimal/low/medium/high), reasoning text + token accounting
- **Web search & citations** — Provider-native search (`ServerTools`) with unified `Citations` in the response
- **Caching** — Provider prompt caching (`CacheControl` + cached-token reporting) and a client-side response cache
- **Agents** — Middleware-driven tool-calling loop (summarization, PII redaction, human-in-the-loop)
- **Chain** — Sequential multi-step prompt pipelines
- **Graph** — LangGraph-style stateful workflows: parallel fan-out, subgraphs, streaming, durable execution
- **MCP** — Model Context Protocol client
- **A2A** — Agent-to-Agent protocol client + server
- **Structured output** — `Generate[T]` decodes into Go types; JSON schema + JSON mode across all providers
- **Vision / Multimodal** — Images, audio and documents (PDF) in messages
- **Prompt templates** — Jinja-style `{{ variable }}` syntax
- **Memory** — Conversation history with token limits
- **Observability** — Provider, agent (tool-call) and graph (node) hooks; tracing-ready via context
- **Retry, Fallback & Rate Limiting** — Composable resilience decorators; backoff with jitter, honors `Retry-After`, token-bucket rate limiter
- **Zero dependencies** — Only Go standard library

## Documentation

| | |
|---|---|
| [Getting Started](docs/getting-started.md) | Installation, first request, error handling |
| [Providers](docs/providers.md) | All providers, config examples |
| [Parameters](docs/parameters.md) | All parameters, provider support matrix |
| [Streaming](docs/streaming.md) | Real-time token streaming |
| [Reasoning](docs/reasoning.md) | Reasoning effort, thinking output, tokens |
| [Web Search & Citations](docs/web-search.md) | Provider-native search + citations |
| [Caching](docs/caching.md) | Prompt caching, cached-token reporting, response cache |
| [Vision / Multimodal](docs/vision.md) | Images, audio and documents in messages |
| [Structured Output](docs/structured-output.md) | Typed output, JSON schema + JSON mode |
| [Prompt Templates](docs/prompt-templates.md) | Jinja-style variable substitution |
| [Memory](docs/memory.md) | Conversation history management |
| [Tool Calling](docs/tool-calling.md) | Typed tools, function calling + automatic tool loop |
| [Agents (Middleware)](docs/agents.md) | Middleware agent loop + built-in middleware |
| [Chain](docs/chain.md) | Sequential prompt pipelines |
| [Graph](docs/graph.md) | Stateful workflows, fan-out, subgraphs, streaming |
| [Durable Execution](docs/durable-execution.md) | Checkpointing, resume, time travel |
| [MCP](docs/mcp.md) | Model Context Protocol integration |
| [A2A](docs/a2a.md) | Agent-to-Agent protocol client + server |
| [Retry, Fallback & Rate Limiting](docs/retry-fallback.md) | Retries, fallbacks, rate limiting |
| [Observability](docs/observability.md) | Provider, agent and graph hooks; tracing |

Full docs with search: [promptrails.github.io/langrails](https://promptrails.github.io/langrails)

## License

MIT — [PromptRails](https://promptrails.ai)

## Part of the PromptRails AI Toolkit

- **LangRails** — Unified LLM provider
- [GuardRails](https://github.com/promptrails/guardrails) — Content safety scanning
- [MemoryRails](https://github.com/promptrails/memoryrails) — Agent memory
- [MediaRails](https://github.com/promptrails/mediarails) — AI media generation
- [Go AI Toolkit](https://github.com/promptrails/go-ai-toolkit) — Demo app
