# Reasoning

langrails exposes a provider-agnostic way to control and read model reasoning
(a.k.a. extended thinking / chain-of-thought).

## Enabling reasoning

Set `ReasoningEffort` on the request to one of `minimal`, `low`, `medium`, `high`:

```go
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:           "o3", // or claude-*, gemini-2.5-*, a Bedrock Claude model, ...
    Messages:        []langrails.Message{{Role: "user", Content: "Prove that √2 is irrational."}},
    ReasoningEffort: langrails.ReasoningHigh,
})

fmt.Println(resp.Thinking) // the model's reasoning, when the provider returns it
fmt.Println(resp.Content)  // the final answer
fmt.Println(resp.Usage.ReasoningTokens)
```

`ReasoningEffort` takes precedence over the legacy `Thinking bool` /
`ThinkingBudget *int` fields, which remain supported for explicit Anthropic /
Gemini token budgets.

## How effort maps per provider

| Provider | Mapping |
|----------|---------|
| OpenAI / compat | `reasoning.effort` = the effort string directly |
| Anthropic | extended thinking with a token budget derived from effort (minimal=1024, low=4096, medium=8192, high=16384); `ThinkingBudget` overrides |
| Gemini | `generationConfig.thinkingConfig` (`includeThoughts` + budget) |
| Bedrock | `additionalModelRequestFields.reasoning_config` (Claude models) |

Providers that take a token budget derive it from the effort via
`ReasoningEffort.BudgetTokens()`; pass `ThinkingBudget` to set an exact budget.

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
