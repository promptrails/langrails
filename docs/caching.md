# Prompt Caching

Prompt caching lets a provider reuse the compute for a repeated prompt prefix
(long system prompts, documents, few-shot examples), cutting cost and latency on
follow-up requests.

## Enabling caching

Set `CacheControl: true`. langrails places a cache breakpoint at the end of the
prompt prefix so everything before it can be cached:

```go
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:        "claude-sonnet-4-20250514",
    SystemPrompt: longSharedInstructions,
    Messages:     conversation,
    CacheControl: true,
})

fmt.Println(resp.Usage.CachedTokens)        // prompt tokens served from cache
fmt.Println(resp.Usage.CacheCreationTokens) // tokens written to the cache
```

## Per-provider behavior

| Provider | Mechanism | Cached-token reporting |
|----------|-----------|------------------------|
| Anthropic | `cache_control: ephemeral` breakpoint on the last prompt block | `cache_read_input_tokens` → `CachedTokens`, `cache_creation_input_tokens` → `CacheCreationTokens` |
| Bedrock | `cachePoint` block at the prompt prefix | `cacheReadInputTokens` → `CachedTokens`, `cacheWriteInputTokens` → `CacheCreationTokens` |
| OpenAI / compat | automatic (no request marker) | `prompt_tokens_details.cached_tokens` → `CachedTokens` |
| Gemini | implicit caching (no request marker) | `cachedContentTokenCount` → `CachedTokens` |

For OpenAI and Gemini, `CacheControl` is a no-op on the request — caching is
automatic — but `CachedTokens` is still reported in `Usage`.

## Usage fields

```go
type TokenUsage struct {
    PromptTokens, CompletionTokens, TotalTokens int
    CachedTokens        int // prompt tokens served from cache (read hits)
    CacheCreationTokens int // tokens written to cache this request
    ReasoningTokens     int // reasoning/thinking tokens
}
```

## Notes

- Caching has provider-specific minimums (e.g. Anthropic requires a minimum
  cacheable prompt length) and TTLs; below the threshold nothing is cached.
- langrails caches the whole prompt prefix (up to the last message). Gemini's
  explicit context-cache API (separately created cached content) is out of scope.
