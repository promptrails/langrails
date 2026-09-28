# Reasoning

langrails exposes a provider-agnostic way to control and read model reasoning
(a.k.a. extended thinking / chain-of-thought).

## Enabling reasoning

Set `ReasoningEffort` on the request to one of `minimal`, `low`, `medium`, `high`:

```go
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:           "o3", // or claude-*, gemini-*, a Bedrock Claude model, ...
    Messages:        []langrails.Message{{Role: "user", Content: "Prove that √2 is irrational."}},
    ReasoningEffort: langrails.ReasoningHigh,
})

fmt.Println(resp.Thinking) // the model's reasoning, when the provider returns it
fmt.Println(resp.Content)  // the final answer
fmt.Println(resp.Usage.ReasoningTokens)
```

For an exact token budget set `ReasoningBudget *int`. It turns reasoning on by
itself, and takes precedence over the budget an effort level implies.

## How effort maps per provider

| Provider | Mapping |
|----------|---------|
| OpenAI / compat | `reasoning.effort` = the effort string directly |
| Anthropic | extended thinking with a token budget derived from effort (minimal=1024, low=4096, medium=8192, high=16384); `ReasoningBudget` overrides |
| Gemini 3 | `generationConfig.thinkingConfig.thinkingLevel` = the effort string directly |
| Gemini 2.5 | `generationConfig.thinkingConfig` (`includeThoughts` + budget derived from effort) |
| Bedrock | `additionalModelRequestFields.reasoning_config` (Claude models) |

Providers that take a token budget derive it from the effort via
`ReasoningEffort.BudgetTokens()`; pass `ReasoningBudget` to set an exact budget.
For Gemini 3, prefer `ReasoningEffort` (sent as `thinkingLevel`); a
`ReasoningBudget` is sent as a numeric budget instead. OpenAI and compat
providers take only an effort level, so a budget is mapped to the nearest one.

## Streaming reasoning

Reasoning is streamed as `EventReasoning` events, emitted before the
`EventContent` answer chunks:

```go
ch, _ := provider.Stream(ctx, req)
for ev := range ch {
    switch ev.Type {
    case langrails.EventReasoning:
        fmt.Print("\033[90m" + ev.Reasoning + "\033[0m") // dim
    case langrails.EventContent:
        fmt.Print(ev.Content)
    }
}
```

## Notes

- `resp.Usage.ReasoningTokens` is populated when the provider reports it
  (OpenAI `completion_tokens_details.reasoning_tokens`, Gemini
  `thoughtsTokenCount`). Most providers count these within `CompletionTokens`.
- Not every model supports reasoning; sending reasoning options to a
  non-reasoning model is ignored or rejected by the provider.
- Bedrock reasoning is model-family specific (the `reasoning_config` form is for
  Anthropic Claude models on Bedrock).
