# unillm

Unified LLM provider interface for Go. One API, 11 providers.

[![Go Reference](https://pkg.go.dev/badge/github.com/promptrails/unillm.svg)](https://pkg.go.dev/github.com/promptrails/unillm)
[![CI](https://github.com/promptrails/unillm/actions/workflows/ci.yml/badge.svg)](https://github.com/promptrails/unillm/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/promptrails/unillm)](https://goreportcard.com/report/github.com/promptrails/unillm)

## Install

```bash
go get github.com/promptrails/unillm
```

## Quick Start

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
- **Structured output** — JSON schema support across all providers
- **Retry & Fallback** — Composable resilience decorators
- **Zero dependencies** — Only Go standard library

## Documentation

| Topic | Description |
|-------|-------------|
| [Getting Started](docs/getting-started.md) | Installation, first request, error handling |
| [Providers](docs/providers.md) | All 11 providers with config and model examples |
| [Streaming](docs/streaming.md) | Real-time token streaming with channels |
| [Tool Calling](docs/tool-calling.md) | Function calling and automatic tool loop |
| [Chain](docs/chain.md) | Sequential multi-step prompt pipelines |
| [Graph](docs/graph.md) | LangGraph-style stateful workflows |
| [MCP](docs/mcp.md) | Model Context Protocol integration |
| [Structured Output](docs/structured-output.md) | JSON schema constrained output |
| [Retry & Fallback](docs/retry-fallback.md) | Resilience and failover patterns |

## Architecture

```
unillm/
├── provider.go     # Provider interface
├── message.go      # Message, ToolCall, ToolDefinition
├── stream.go       # StreamEvent types
├── errors.go       # APIError with helpers
├── retry.go        # RetryProvider decorator
├── fallback.go     # FallbackProvider decorator
├── compat/         # Shared OpenAI-compatible base
├── tools/          # Tool loop execution
├── chain/          # Sequential prompt chains
├── graph/          # Stateful workflow engine
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

## License

MIT — [PromptRails](https://promptrails.com)
