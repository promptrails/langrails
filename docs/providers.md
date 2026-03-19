# Providers

unillm supports 11 LLM providers through a unified interface. Each provider is a separate Go package that you import only when needed.

## Provider List

| Provider | Package | Base URL | Auth |
|----------|---------|----------|------|
| OpenAI | `unillm/openai` | `api.openai.com` | Bearer token |
| Anthropic | `unillm/anthropic` | `api.anthropic.com` | x-api-key header |
| Google Gemini | `unillm/gemini` | `generativelanguage.googleapis.com` | URL parameter |
| DeepSeek | `unillm/deepseek` | `api.deepseek.com` | Bearer token |
| Groq | `unillm/groq` | `api.groq.com` | Bearer token |
| Fireworks | `unillm/fireworks` | `api.fireworks.ai` | Bearer token |
| xAI | `unillm/xai` | `api.x.ai` | Bearer token |
| OpenRouter | `unillm/openrouter` | `openrouter.ai` | Bearer token |
| Together | `unillm/together` | `api.together.xyz` | Bearer token |
| Mistral | `unillm/mistral` | `api.mistral.ai` | Bearer token |
| Cohere | `unillm/cohere` | `api.cohere.com` | Bearer token |

## Feature Matrix

| Feature | OpenAI | Anthropic | Gemini | Compat* |
|---------|--------|-----------|--------|---------|
| Streaming | Yes | Yes | Yes | Yes |
| Tool calling | Yes | Yes | Yes | Yes |
| Structured output | Yes (JSON schema) | Yes (tool-based) | Yes (responseSchema) | Yes (JSON schema) |
| Vision | Yes | Yes | Yes | Varies |
| System prompt | message | separate field | systemInstruction | message |
| Max tokens default | provider default | 4096 (required) | provider default | provider default |

*Compat = DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere

## OpenAI

```go
import "github.com/promptrails/unillm/openai"

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
import "github.com/promptrails/unillm/anthropic"

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
import "github.com/promptrails/unillm/gemini"

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
import "github.com/promptrails/unillm/deepseek"

provider := deepseek.New("your-api-key")
```

**Models**: deepseek-chat, deepseek-coder, deepseek-reasoner

## Groq

```go
import "github.com/promptrails/unillm/groq"

provider := groq.New("your-api-key")
```

**Models**: llama-3.1-70b-versatile, llama-3.1-8b-instant, mixtral-8x7b-32768

## Fireworks

```go
import "github.com/promptrails/unillm/fireworks"

provider := fireworks.New("your-api-key")
```

**Models**: accounts/fireworks/models/llama-v3p1-70b-instruct, etc.

## xAI (Grok)

```go
import "github.com/promptrails/unillm/xai"

provider := xai.New("your-api-key")
```

**Models**: grok-2, grok-2-mini

## OpenRouter

```go
import "github.com/promptrails/unillm/openrouter"

provider := openrouter.New("your-api-key",
    openrouter.WithSiteInfo("https://myapp.com", "My App"),
)
```

**Models**: openai/gpt-4o, anthropic/claude-3.5-sonnet, meta-llama/llama-3.1-70b, and 100+ more

**Notes**: `WithSiteInfo` sets HTTP-Referer and X-Title headers for OpenRouter's provider ranking.

## Together

```go
import "github.com/promptrails/unillm/together"

provider := together.New("your-api-key")
```

**Models**: meta-llama/Meta-Llama-3.1-70B-Instruct-Turbo, etc.

## Mistral

```go
import "github.com/promptrails/unillm/mistral"

provider := mistral.New("your-api-key")
```

**Models**: mistral-large-latest, mistral-medium-latest, open-mistral-7b

## Cohere

```go
import "github.com/promptrails/unillm/cohere"

provider := cohere.New("your-api-key")
```

**Models**: command-r-plus, command-r, command-light

## Custom / Self-Hosted

Any OpenAI-compatible API can be used with the `compat` package directly:

```go
import "github.com/promptrails/unillm/compat"

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
