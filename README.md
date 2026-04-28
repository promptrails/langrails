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

- **24 providers** — OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere, Perplexity, Ollama, Chutes AI, Z.AI, Moonshot (Kimi), Novita AI, DeepInfra, Friendli AI, Cerebras, SambaNova, Hyperbolic, Alibaba DashScope (Qwen), Hugging Face Router
- **Streaming** — Channel-based, idiomatic Go
- **Tool calling** — Unified interface + automatic tool execution loop
- **Chain** — Sequential multi-step prompt pipelines
- **Graph** — LangGraph-style stateful workflow engine
- **MCP** — Model Context Protocol client
- **A2A** — Agent-to-Agent protocol client + server
- **Structured output** — JSON schema across all providers
- **Vision / Multimodal** — Images + text in messages
- **Prompt templates** — Jinja-style `{{ variable }}` syntax
- **Memory** — Conversation history with token limits
- **Retry & Fallback** — Composable resilience decorators
- **Zero dependencies** — Only Go standard library

## Documentation

| | |
|---|---|
| [Getting Started](docs/getting-started.md) | Installation, first request, error handling |
| [Providers](docs/providers.md) | All providers, config examples |
| [Parameters](docs/parameters.md) | All parameters, provider support matrix |
| [Streaming](docs/streaming.md) | Real-time token streaming |
| [Vision / Multimodal](docs/vision.md) | Images + text in messages |
| [Structured Output](docs/structured-output.md) | JSON schema constrained output |
| [Prompt Templates](docs/prompt-templates.md) | Jinja-style variable substitution |
| [Memory](docs/memory.md) | Conversation history management |
| [Tool Calling](docs/tool-calling.md) | Function calling + automatic tool loop |
| [Chain](docs/chain.md) | Sequential prompt pipelines |
| [Graph](docs/graph.md) | Stateful workflows, conditional routing |
| [MCP](docs/mcp.md) | Model Context Protocol integration |
| [A2A](docs/a2a.md) | Agent-to-Agent protocol client + server |
| [Retry & Fallback](docs/retry-fallback.md) | Resilience patterns |

Full docs with search: [promptrails.github.io/langrails](https://promptrails.github.io/langrails)

## License

MIT — [PromptRails](https://promptrails.ai)

## Part of the PromptRails AI Toolkit

- **LangRails** — Unified LLM provider
- [GuardRails](https://github.com/promptrails/guardrails) — Content safety scanning
- [MemoryRails](https://github.com/promptrails/memoryrails) — Agent memory
- [MediaRails](https://github.com/promptrails/mediarails) — AI media generation
- [Go AI Toolkit](https://github.com/promptrails/go-ai-toolkit) — Demo app
