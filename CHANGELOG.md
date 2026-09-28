# Changelog

## [Unreleased]

The v1 candidate. After v1.0.0 the API is stable: breaking changes would
need a /v2 module path.

### Breaking

The request types are settled before the freeze (see "Migrating" below):

- `CompletionRequest.OutputSchema` is `json.RawMessage` instead of `*[]byte`
- `CompletionRequest.Thinking` and `ThinkingBudget` are removed in favor of
  `ReasoningEffort` plus the new `ReasoningBudget *int`
- `Message.Role` is a typed `Role` (`RoleSystem`, `RoleUser`, `RoleAssistant`,
  `RoleTool`)

Behavior changes worth knowing:

- compat: strict-mode structured output now applies to every nested object,
  and optional properties become required-but-nullable. Nested schemas used
  to be rejected by OpenAI
- retry: backoff is jittered and capped at one minute by default, and a
  provider's `Retry-After` replaces the backoff; `WithoutJitter` restores
  exact delays
- mcp: a tool result marked `isError` is returned as an error, and multiple
  text blocks are joined instead of keeping only the first

### Added

- tools: typed tools from Go functions (`tools.New`, `tools.Set`), with the
  parameter schema generated from the input struct
- `Generate[T]`: typed structured output, with parse retries and
  `ParseError`
- multimodal: audio and document (PDF) input parts (`AudioPart`,
  `DocumentPart`, `DocumentURLPart`); providers reject parts they cannot
  carry with `ErrUnsupportedContent` before sending
- agent: `Stream` / `StreamMessages` for token-level streaming of the whole
  tool loop
- agent: built-in `ModelCallLimit`, `ToolCallLimit`, `ToolRetry`,
  `ModelFallback`, `ContextEditing` and `ToolSelector` middlewares, plus the
  `ToolWrapper` interface for wrapping tool calls
- graph: durable human-in-the-loop interrupts (`Await`, `WithResumeValue`,
  `RunResult.Interruption`)
- graph: node hooks (`WithHooks`)
- mcp: stdio transport (`NewStdioClient`), resources and prompts; the HTTP
  client echoes `Mcp-Session-Id`, sends `notifications/initialized` and
  follows pagination
- observability: `WithHooks` provider decorator (request, response, error,
  stream events; context-returning for tracing)
- resilience: `WithRateLimit` token bucket, `APIError.RetryAfter` and
  `RetryAfterFromHeader`, `WithMaxDelay`
- caching: `WithCache` response cache with `NewMemoryCache` (LRU + TTL)
- streaming: `StreamAccumulator` and `CollectStream`
- llm: `FromString("provider:model", key)` with API keys from the
  conventional environment variables, `ParseModel`, `MustFromString`

### Docs

- the docs site uses the ai-chat design and no longer reads purple
- new Observability page; stale Perplexity and provider-count references
  removed

### Migrating from v0.10

```go
// OutputSchema
req.OutputSchema = &schema        // before
req.OutputSchema = schema         // after

// Reasoning
req.Thinking = true               // before
req.ReasoningEffort = langrails.ReasoningMedium

req.Thinking, req.ThinkingBudget = true, &n // before
req.ReasoningBudget = &n                     // after (turns reasoning on)

// Role: literals are unchanged; a string variable needs a conversion
msg := langrails.Message{Role: "user"}           // still compiles
msg := langrails.Message{Role: langrails.Role(r)} // r is a string
```

## [v0.10.0] - 2026-09-15

**Breaking: the `llm/perplexity` provider is removed**, along with the
`llm.Perplexity` constant. `llm.New("perplexity", key)` now returns an
unsupported-provider error.

Perplexity retires the chat-completions API this provider spoke on 2026-09-27,
replacing it with an Agent API at `/v1/agent`. After that date the package
reaches no models at all, so it is removed rather than left to fail at run
time. Pin `v0.9.2` if you need it for the remaining days.

The Agent API needs a new provider rather than a changed base URL: it takes an
`input` field and returns a typed `output` array of `message`, `search_results`
and `fetch_url_results` items. It is not written here yet because the published
documentation does not state how a multi-turn conversation is sent, whether
streaming exists, or whether `tools` means function calling or only
Perplexity's own `web_search` — which is both methods of `Provider` plus its
core data structure. It waits for documentation or a verified integration
rather than guesses.

`compat` keeps its Perplexity-style top-level `citations` handling: that is a
wire format other providers use too, not this provider.

## [v0.9.2] - 2026-09-15

- openai: send reasoning effort as the documented top-level `reasoning_effort` string. compat serialized the OpenRouter `{"reasoning":{"effort":...}}` object for every provider, which OpenAI does not read, so the setting was a silent no-op against it
- compat: `Config.ReasoningStyle` picks the wire form; defaults to the object form, so the other 21 compat providers are unchanged
- types: `ReasoningNone` ("none") explicitly turns reasoning OFF, which is distinct from `ReasoningOff` ("" — say nothing and let the provider default). OpenAI's chat/completions refuses function tools on a model that reasons by default unless the effort is explicitly "none"
- anthropic, bedrock, gemini: treat `ReasoningNone` as off rather than as a request to think — these enable thinking on any non-empty effort, and bedrock would have read it as a 10k-token budget

## [v0.9.1] - 2026-09-12

- gemini: map provider-agnostic reasoning effort to Gemini 3 `thinkingLevel`

## [v0.9.0] - 2026-08-27

- gemini: opt-in Vertex AI backend, selected by region, so a deployment whose egress IP Gemini geo-blocks can reach the same models

## [v0.8.6] - 2026-08-23

- build: Go 1.27, golangci-lint v2

## [v0.8.5] - 2026-08-21

- gemini: stop sending a thinking config with 2.5-flash function calling, which returned empty completions

## [v0.8.4] - 2026-08-21

- compat: round-trip tool-call metadata, so a Gemini thoughtSignature survives a turn through an OpenAI-shaped provider

## [v0.8.3] - 2026-08-21

- gemini: drop empty content parts and turns, which Gemini rejects with a 400

## [v0.8.2] - 2026-08-05

- gemini: send structured outputs through `responseJsonSchema` so standard JSON Schema keywords are accepted

## [v0.8.1] - 2026-07-05

- gemini: read/write thoughtSignature at the part level, not inside functionCall

## [v0.8.0] - 2026-06-30

- agent: handle multimodal messages correctly
- graph: enforce the step budget across fan-out branches
- agent: simplify message copy, note redaction/estimate limits
- graph: keep run options per-call instead of mutating the Graph
- docs: cover agents, durable execution and the new graph features in README
- agent: human-in-the-loop approval gate
- agent: regex-based PII redaction middleware
- agent: summarization middleware
- agent: middleware-driven tool-calling loop
- graph: stream step events as nodes complete
- graph: embed a compiled graph as a node (AsNode)
- graph: durable execution via checkpointer + Resume
- graph: add parallel fan-out with a reducer (Send API)
- feat(bedrock): cache tools + system prefixes for multi-turn agents

## [v0.7.3] - 2026-06-25

- feat(anthropic): cache tools + system prefixes for multi-turn agents
- test: cover message/tool-choice conversion in core providers
- test: cover mcp client and tool-loop error paths
- test: cover a2a server paths, server tools, tool choice, streams; expand docs
- test: add perplexity, message, and provider tests; refresh docs

## [v0.7.2] - 2026-06-17

- fix(gemini): textify tool calls without a thoughtSignature

## [v0.7.1] - 2026-06-01

- fix: propagate context cancellation across streaming, fallback, and a2a

## [v0.7.0] - 2026-05-29

- feat(bedrock): add Amazon Bedrock LLM provider via Converse API
- feat(bedrock): prompt caching via cachePoint + cache token usage
- feat(bedrock): wire reasoning (additionalModelRequestFields + reasoningContent)
- feat(gemini): googleSearch grounding citations + cached token usage
- feat(gemini): wire reasoning (thinkingConfig + thought parts + thought tokens)
- feat(anthropic): web search server tool, citations, prompt caching
- feat(anthropic): reasoning via ReasoningEffort + stream thinking deltas
- feat(compat): built-in web search + citation parsing
- feat(compat): surface reasoning content (response + stream)
- feat(compat): wire ToolChoice, JSON mode, ReasoningEffort, cached/reasoning token usage
- feat(vision): wire image content parts into anthropic, gemini, bedrock
- feat(anthropic,gemini,bedrock): wire public ToolChoice
- feat(types): add reasoning, tool-choice, server-tools, citations, cache to domain model
- docs: document reasoning, web search/citations, caching, tool choice, JSON mode, vision
- fix(anthropic): pass web search domain/location filters
- fix(gemini): honor ResponseFormatJSONObject without a schema
- fix(bedrock): use thinking (not reasoning_config) for Converse reasoning
- fix(bedrock): remove unused streamMessageStop type (lint)

## [v0.6.2] - 2026-05-29

- chore: bump Go directive to 1.26.3

## [v0.6.1] - 2026-04-28

- feat(llm): add cerebras, sambanova, hyperbolic, dashscope, huggingface

## [v0.6.0] - 2026-04-28

- feat(llm): add chutes, zai, moonshot, novita, deepinfra, friendli
- docs: fix canonical homepage URL (.com → .ai)

## [v0.5.2] - 2026-04-01

- feat: add Gemini thought_signature support for tool calls

## [v0.5.1] - 2026-04-01

- chore: upgrade Go from 1.24 to 1.26
- ci: restrict test matrix to Go 1.26 (matches go.mod requirement)
- ci: fix Go 1.26 compatibility in CI workflow

## [v0.5.0] - 2026-03-23

- feat: add Perplexity provider (search-augmented LLM)
- docs: add registry usage examples to getting-started and providers pages

## [v0.4.0] - 2026-03-22

- refactor: move providers under llm/ with registry pattern
- docs: add toolkit links to README

## [v0.3.0] - 2026-03-20

- docs: add Ollama to providers page and feature matrix

## [v0.2.0] - 2026-03-20

- feat: add Ollama, vision/multimodal, prompt templates, and memory

## [v0.1.0] - 2026-03-20

- docs: capitalize LangRails in title and heading
- refactor: rename project from llmrails to langrails

## [v0.0.0] - 2026-02-02

Initial project foundation:

- feat: core types, provider interface, and stream events
- feat: OpenAI-compatible base and OpenAI provider
- feat: Anthropic (Claude) provider
- feat: Google Gemini provider
- feat: internal SSE stream reader with tests
- feat: retry and fallback provider decorators with tests
- feat: automatic tool calling loop with executor interface
- feat: sequential prompt chain execution
- feat: LangGraph-style stateful workflow graph engine
- feat: MCP client for tool discovery and execution
- feat: structured output support for Anthropic and Gemini
- feat: A2A (Agent-to-Agent) protocol client and server
- feat: add DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, Cohere providers
- test: compat provider tests with mock HTTP server
- docs: comprehensive documentation for all packages
- chore: add Makefile, golangci-lint config, gosec to CI
- Various fixes and improvements
