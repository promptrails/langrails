# LangRails

Unified LLM provider interface for Go. One API, 11 providers.

[![Go Reference](https://pkg.go.dev/badge/github.com/promptrails/langrails.svg)](https://pkg.go.dev/github.com/promptrails/langrails)
[![CI](https://github.com/promptrails/langrails/actions/workflows/ci.yml/badge.svg)](https://github.com/promptrails/langrails/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/promptrails/langrails)](https://goreportcard.com/report/github.com/promptrails/langrails)

```go
provider := openai.New("sk-...")
resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Hello!"}},
})
fmt.Println(resp.Content)
```

## Install

```bash
go get github.com/promptrails/langrails
```

## Features

- **11 providers** — OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere
- **Streaming** — Channel-based, idiomatic Go
- **Tool calling** — Unified interface + automatic tool execution loop
- **Chain** — Sequential multi-step prompt pipelines
- **Graph** — LangGraph-style stateful workflow engine
- **MCP** — Model Context Protocol client
- **A2A** — Agent-to-Agent protocol client + server
- **Structured output** — JSON schema across all providers
- **Retry & Fallback** — Composable resilience decorators
- **Zero dependencies** — Only Go standard library

## Documentation

| | |
|---|---|
| [Getting Started](docs/getting-started.md) | Installation, first request, error handling |
| [Providers](docs/providers.md) | All 11 providers, config examples |
| [Parameters & Feature Matrix](docs/parameters.md) | All parameters, provider support matrix |
| [Streaming](docs/streaming.md) | Real-time token streaming |
| [Tool Calling](docs/tool-calling.md) | Function calling + automatic tool loop |
| [Chain](docs/chain.md) | Sequential prompt pipelines |
| [Graph](docs/graph.md) | Stateful workflows, conditional routing |
| [MCP](docs/mcp.md) | Model Context Protocol integration |
| [A2A](docs/a2a.md) | Agent-to-Agent protocol client + server |
| [Structured Output](docs/structured-output.md) | JSON schema constrained output |
| [Retry & Fallback](docs/retry-fallback.md) | Resilience patterns |

Full docs with search: [promptrails.github.io/langrails](https://promptrails.github.io/langrails)

## License

MIT — [PromptRails](https://promptrails.com)
