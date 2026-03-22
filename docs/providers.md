# Providers

langrails supports 12 LLM providers through a unified interface.

## Using the Registry

The simplest way to create any provider:

```go
import "github.com/promptrails/langrails/llm"

provider, err := llm.New(llm.OpenAI, "sk-...")
// or panic on error:
provider := llm.MustNew(llm.Anthropic, "sk-ant-...")
```

Available constants: `llm.OpenAI`, `llm.Anthropic`, `llm.Gemini`, `llm.DeepSeek`, `llm.Groq`, `llm.Fireworks`, `llm.XAI`, `llm.OpenRouter`, `llm.Together`, `llm.Mistral`, `llm.Cohere`, `llm.Ollama`

For provider-specific options (custom base URL, HTTP client), use the direct import instead.

## Provider List

| Provider | Package | Base URL | Auth |
|----------|---------|----------|------|
| OpenAI | `langrails/llm/openai` | `api.openai.com` | Bearer token |
| Anthropic | `langrails/llm/anthropic` | `api.anthropic.com` | x-api-key header |
| Google Gemini | `langrails/llm/gemini` | `generativelanguage.googleapis.com` | URL parameter |
| DeepSeek | `langrails/llm/deepseek` | `api.deepseek.com` | Bearer token |
| Groq | `langrails/llm/groq` | `api.groq.com` | Bearer token |
| Fireworks | `langrails/llm/fireworks` | `api.fireworks.ai` | Bearer token |
| xAI | `langrails/llm/xai` | `api.x.ai` | Bearer token |
| OpenRouter | `langrails/llm/openrouter` | `openrouter.ai` | Bearer token |
| Together | `langrails/llm/together` | `api.together.xyz` | Bearer token |
| Mistral | `langrails/llm/mistral` | `api.mistral.ai` | Bearer token |
| Cohere | `langrails/llm/cohere` | `api.cohere.com` | Bearer token |
| Ollama | `langrails/llm/ollama` | `localhost:11434` | None (local) |

## Feature Matrix

| Feature | OpenAI | Anthropic | Gemini | Compat* |
|---------|--------|-----------|--------|---------|
| Streaming | Yes | Yes | Yes | Yes |
| Tool calling | Yes | Yes | Yes | Yes |
| Structured output | Yes (JSON schema) | Yes (tool-based) | Yes (responseSchema) | Yes (JSON schema) |
| Vision | Yes | Yes | Yes | Varies |
| System prompt | message | separate field | systemInstruction | message |
| Max tokens default | provider default | 4096 (required) | provider default | provider default |

*Compat = DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere, Ollama

## OpenAI

```go
import "github.com/promptrails/langrails/llm/openai"

provider := openai.New("sk-...")

// With options
provider := openai.New("sk-...",
    openai.WithBaseURL("https://my-proxy.com/v1/chat/completions"),
    openai.WithHTTPClient(&http.Client{Timeout: 2 * time.Minute}),
)
```

**Models**: gpt-4o, gpt-4o-mini, gpt-4-turbo, gpt-3.5-turbo, o1, o1-mini

**Azure OpenAI**: Use `WithBaseURL` to point to your Azure endpoint.

## Anthropic

```go
import "github.com/promptrails/langrails/llm/anthropic"

provider := anthropic.New("sk-ant-...")
```

**Models**: claude-sonnet-4-20250514, claude-opus-4-20250514, claude-haiku-4-5-20251001

**Notes**:
- System prompts are sent as a separate `system` field (not as a message)
- `max_tokens` is required and defaults to 4096 if not set
- Tool results are sent as user messages with `tool_result` content blocks
- Structured output uses a forced tool call internally

## Google Gemini

```go
import "github.com/promptrails/langrails/llm/gemini"

provider := gemini.New("your-api-key")
```

**Models**: gemini-2.0-flash, gemini-1.5-pro, gemini-1.5-flash

**Notes**:
- API key is passed as a URL query parameter (not a header)
- Uses "model" role instead of "assistant"
- System prompts use `systemInstruction` field
- Streaming uses `?alt=sse` parameter
- Structured output uses `responseMimeType` + `responseSchema`

## DeepSeek

```go
import "github.com/promptrails/langrails/llm/deepseek"

provider := deepseek.New("your-api-key")
```

**Models**: deepseek-chat, deepseek-coder, deepseek-reasoner

## Groq

```go
import "github.com/promptrails/langrails/llm/groq"

provider := groq.New("your-api-key")
```

**Models**: llama-3.1-70b-versatile, llama-3.1-8b-instant, mixtral-8x7b-32768

## Fireworks

```go
import "github.com/promptrails/langrails/llm/fireworks"

provider := fireworks.New("your-api-key")
```

**Models**: accounts/fireworks/models/llama-v3p1-70b-instruct, etc.

## xAI (Grok)

```go
import "github.com/promptrails/langrails/llm/xai"

provider := xai.New("your-api-key")
```

**Models**: grok-2, grok-2-mini

## OpenRouter

```go
import "github.com/promptrails/langrails/llm/openrouter"

provider := openrouter.New("your-api-key",
    openrouter.WithSiteInfo("https://myapp.com", "My App"),
)
```

**Models**: openai/gpt-4o, anthropic/claude-3.5-sonnet, meta-llama/llama-3.1-70b, and 100+ more

**Notes**: `WithSiteInfo` sets HTTP-Referer and X-Title headers for OpenRouter's provider ranking.

## Together

```go
import "github.com/promptrails/langrails/llm/together"

provider := together.New("your-api-key")
```

**Models**: meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo, etc.

## Mistral

```go
import "github.com/promptrails/langrails/llm/mistral"

provider := mistral.New("your-api-key")
```

**Models**: mistral-large-latest, mistral-medium-latest, open-mistral-7b

## Cohere

```go
import "github.com/promptrails/langrails/llm/cohere"

provider := cohere.New("your-api-key")
```

**Models**: command-r-plus, command-r, command-light

## Ollama

```go
import "github.com/promptrails/langrails/llm/ollama"

provider := ollama.New()
```

**Models**: llama3.2, mistral, codellama, llava (vision), and any model you pull

**Notes**:
- No API key required for local instances
- Default URL: `http://localhost:11434/v1/chat/completions`
- Custom URL: `ollama.New(ollama.WithBaseURL("http://remote:11434/v1/chat/completions"))`
- Vision support with llava/bakllava models

## Custom / Self-Hosted

Any OpenAI-compatible API can be used with the `compat` package directly:

```go
import "github.com/promptrails/langrails/llm/compat"

provider := compat.New(compat.Config{
    Name:    "my-server",
    BaseURL: "http://localhost:11434/v1/chat/completions",
    APIKey:  "optional-key",
})
```

This works with Ollama, vLLM, LiteLLM proxy, or any server implementing the OpenAI chat completions API.

## Common Options

All providers support these options:

```go
// Custom base URL
provider := openai.New("key", openai.WithBaseURL("https://custom-url"))

// Custom HTTP client
provider := openai.New("key", openai.WithHTTPClient(&http.Client{
    Timeout: 2 * time.Minute,
    Transport: &http.Transport{
        MaxIdleConns: 10,
    },
}))
```
