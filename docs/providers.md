# Providers

langrails supports 24 LLM providers through a unified interface.

## Using the Registry

The simplest way to create any provider:

```go
import "github.com/promptrails/langrails/llm"

provider, err := llm.New(llm.OpenAI, "sk-...")
// or panic on error:
provider := llm.MustNew(llm.Anthropic, "sk-ant-...")
```

Available constants: `llm.OpenAI`, `llm.Anthropic`, `llm.Gemini`, `llm.DeepSeek`, `llm.Groq`, `llm.Fireworks`, `llm.XAI`, `llm.OpenRouter`, `llm.Together`, `llm.Mistral`, `llm.Cohere`, `llm.Perplexity`, `llm.Ollama`, `llm.Chutes`, `llm.ZAI`, `llm.Moonshot`, `llm.Novita`, `llm.DeepInfra`, `llm.Friendli`, `llm.Cerebras`, `llm.SambaNova`, `llm.Hyperbolic`, `llm.DashScope`, `llm.HuggingFace`

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
| Perplexity | `langrails/llm/perplexity` | `api.perplexity.ai` | Bearer token |
| Ollama | `langrails/llm/ollama` | `localhost:11434` | None (local) |
| Chutes AI | `langrails/llm/chutes` | `llm.chutes.ai` | Bearer token |
| Z.AI | `langrails/llm/zai` | `api.z.ai` | Bearer token |
| Moonshot (Kimi) | `langrails/llm/moonshot` | `api.moonshot.ai` | Bearer token |
| Novita AI | `langrails/llm/novita` | `api.novita.ai` | Bearer token |
| DeepInfra | `langrails/llm/deepinfra` | `api.deepinfra.com` | Bearer token |
| Friendli AI | `langrails/llm/friendli` | `api.friendli.ai` | Bearer token |
| Cerebras | `langrails/llm/cerebras` | `api.cerebras.ai` | Bearer token |
| SambaNova | `langrails/llm/sambanova` | `api.sambanova.ai` | Bearer token |
| Hyperbolic | `langrails/llm/hyperbolic` | `api.hyperbolic.xyz` | Bearer token |
| Alibaba DashScope (Qwen) | `langrails/llm/dashscope` | `dashscope-intl.aliyuncs.com` | Bearer token |
| Hugging Face Router | `langrails/llm/huggingface` | `router.huggingface.co` | Bearer token |

## Feature Matrix

| Feature | OpenAI | Anthropic | Gemini | Compat* |
|---------|--------|-----------|--------|---------|
| Streaming | Yes | Yes | Yes | Yes |
| Tool calling | Yes | Yes | Yes | Yes |
| Structured output | Yes (JSON schema) | Yes (tool-based) | Yes (responseSchema) | Yes (JSON schema) |
| Vision | Yes | Yes | Yes | Varies |
| System prompt | message | separate field | systemInstruction | message |
| Max tokens default | provider default | 4096 (required) | provider default | provider default |

*Compat = DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere, Perplexity, Ollama, Chutes, Z.AI, Moonshot, Novita, DeepInfra, Friendli, Cerebras, SambaNova, Hyperbolic, DashScope, Hugging Face Router

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

## Perplexity

```go
import "github.com/promptrails/langrails/llm/perplexity"

provider := perplexity.New("your-api-key")
```

**Models**: sonar-pro, sonar, sonar-deep-research, sonar-reasoning-pro, sonar-reasoning

**Notes**:
- Search-augmented LLM — responses include web search results
- Responses may include `citations` in metadata
- OpenAI-compatible API

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

## Chutes AI

```go
import "github.com/promptrails/langrails/llm/chutes"

provider := chutes.New("cpk-...")
```

**Models**: deepseek-ai/DeepSeek-V3-0324, and other open models hosted on Chutes.

## Z.AI

```go
import "github.com/promptrails/langrails/llm/zai"

provider := zai.New("your-api-key")
```

**Models**: glm-5.1 and other GLM-family models from Zhipu AI.

## Moonshot (Kimi)

```go
import "github.com/promptrails/langrails/llm/moonshot"

provider := moonshot.New("your-api-key")
```

**Models**: kimi-k2.6 and other Kimi models from Moonshot AI.

**Notes**: API keys are issued on the Moonshot platform; the model family is branded as "Kimi" but the API host and platform name use "moonshot".

## Novita AI

```go
import "github.com/promptrails/langrails/llm/novita"

provider := novita.New("your-api-key")
```

**Models**: deepseek/deepseek-r1, meta-llama/llama-3.1-* and other open models served via Novita's GPU cloud.

## DeepInfra

```go
import "github.com/promptrails/langrails/llm/deepinfra"

provider := deepinfra.New("your-api-token")
```

**Models**: deepseek-ai/DeepSeek-V3, meta-llama/Meta-Llama-3.1-70B-Instruct, and other open models served by DeepInfra.

## Friendli AI

```go
import "github.com/promptrails/langrails/llm/friendli"

provider := friendli.New("your-api-key")
```

**Models**: meta-llama-3.1-8b-instruct and other models on the Friendli serverless endpoint.

## Cerebras

```go
import "github.com/promptrails/langrails/llm/cerebras"

provider := cerebras.New("your-api-key")
```

**Models**: llama3.1-8b, llama3.3-70b, qwen-3-coder-* and other models hosted on Cerebras Inference.

**Notes**: Optimized for very high-throughput inference (often the fastest provider for Llama-class models).

## SambaNova

```go
import "github.com/promptrails/langrails/llm/sambanova"

provider := sambanova.New("your-api-key")
```

**Models**: DeepSeek-V3, Meta-Llama-3.x and other models on SambaNova Cloud.

## Hyperbolic

```go
import "github.com/promptrails/langrails/llm/hyperbolic"

provider := hyperbolic.New("your-api-key")
```

**Models**: meta-llama/Meta-Llama-3.1-* and other open models served on Hyperbolic's GPU cloud.

## Alibaba DashScope (Qwen)

```go
import "github.com/promptrails/langrails/llm/dashscope"

provider := dashscope.New("your-api-key")
```

**Models**: qwen-max, qwen-plus, qwen-turbo, qwen3-* and other Qwen-family models in Alibaba Cloud Model Studio.

**Notes**:
- Default base URL targets the international (Singapore) endpoint
- Switch to `https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions` via `WithBaseURL` for the mainland China endpoint

## Hugging Face Router

```go
import "github.com/promptrails/langrails/llm/huggingface"

provider := huggingface.New("hf_...")
```

**Models**: addressed as `<owner>/<model>` (e.g. `meta-llama/Meta-Llama-3.1-70B-Instruct`, `deepseek-ai/DeepSeek-V3`). Append `:fastest`, `:cheapest`, `:preferred`, or a specific provider like `:sambanova` to control routing.

**Notes**:
- Single API key + endpoint that proxies to Cerebras, SambaNova, Together, Fireworks, Groq, Hyperbolic, Novita and other partner providers
- Useful when you want unified billing and automatic failover across providers

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
