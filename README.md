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

## Install

```bash
go get github.com/promptrails/unillm
```

## Features

- **11 providers** — OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere
- **Streaming** — Channel-based, idiomatic Go
- **Tool calling** — Unified interface + automatic tool execution loop
- **Chain** — Sequential multi-step prompt pipelines
- **Graph** — LangGraph-style stateful workflow engine
- **MCP** — Model Context Protocol client
- **Structured output** — JSON schema across all providers
- **Retry & Fallback** — Composable resilience decorators
- **Zero dependencies** — Only Go standard library

## Documentation

| | |
|---|---|
| [Getting Started](docs/getting-started.md) | Installation, first request, error handling |
| [Providers](docs/providers.md) | All 11 providers, feature matrix, config |
| [Streaming](docs/streaming.md) | Real-time token streaming |
| [Tool Calling](docs/tool-calling.md) | Function calling + automatic tool loop |
| [Chain](docs/chain.md) | Sequential prompt pipelines |
| [Graph](docs/graph.md) | Stateful workflows, conditional routing |
| [MCP](docs/mcp.md) | Model Context Protocol integration |
| [Structured Output](docs/structured-output.md) | JSON schema constrained output |
| [Retry & Fallback](docs/retry-fallback.md) | Resilience patterns |

Full docs with search: [promptrails.github.io/unillm](https://promptrails.github.io/unillm)

## License

MIT — [PromptRails](https://promptrails.com)
