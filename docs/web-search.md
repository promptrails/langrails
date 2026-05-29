# Web Search & Citations

langrails can enable a provider's **built-in** web search (executed server-side
by the provider, not by your code) and returns any sources as unified
`Citations` on the response.

## Enabling web search

Add a web-search server tool via `ServerTools`:

```go
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o-search-preview", // or claude-*, gemini-2.x, sonar, ...
    Messages: []langrails.Message{{Role: "user", Content: "What shipped in Go 1.26?"}},
    ServerTools: []langrails.ServerTool{
        langrails.WebSearch(&langrails.WebSearchOptions{MaxUses: 3}),
    },
})

for _, c := range resp.Citations {
    fmt.Printf("- %s (%s)\n", c.Title, c.URL)
}
```

`WebSearchOptions` fields (`MaxUses`, `AllowedDomains`, `BlockedDomains`,
`UserLocation`) are best-effort: each provider honors the subset it supports and
ignores the rest. Pass `langrails.WebSearch(nil)` for provider defaults.

On Anthropic, `AllowedDomains`/`BlockedDomains` map to the web search tool's
`allowed_domains`/`blocked_domains` (don't set both), and `UserLocation` maps to
an approximate `user_location` country.

`ServerTools` is distinct from `Tools`: `Tools` are functions **you** execute and
return results for; `ServerTools` run **inside the provider**.

## Per-provider behavior

| Provider | How it's enabled | Citations source |
|----------|------------------|------------------|
| OpenAI / compat | `web_search_options` (search-capable models) | inline `url_citation` annotations |
| Anthropic | `web_search_20250305` server tool (`max_uses`) | text-block `citations` |
| Gemini | `googleSearch` grounding tool | `groundingMetadata.groundingChunks` |
| Perplexity | implicit (sonar models) — no request flag needed | top-level `citations` |
| OpenRouter | `:online` model suffix / web plugin | top-level `citations` |
| Bedrock | not supported (Converse has no built-in search) | — |

For Perplexity you don't need to set `ServerTools` at all — its sonar models
search implicitly, and langrails still parses the returned citations.

## Citations during streaming

When a provider streams citations, they arrive as `EventCitation` events
(`ev.Citation`). Otherwise read `resp.Citations` from the final response.

## The Citation type

```go
type Citation struct {
    URL, Title, Snippet  string
    StartIndex, EndIndex int // span into Content, when the provider supplies offsets
}
```

## Notes

- Web search support and the exact request mechanism differ sharply across
  providers and even across models of the same provider — pick a search-capable
  model.
- Bedrock Converse has no first-class web search; the option is ignored there.
