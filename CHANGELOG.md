# Changelog

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
