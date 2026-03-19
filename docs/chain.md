# Chain

The `chain` package enables sequential multi-step LLM pipelines where each step's output feeds into the next step's input.

## Basic Chain

```go
import "github.com/promptrails/unillm/chain"

c := chain.New(provider, []chain.Step{
    {SystemPrompt: "Summarize the following text in 2 sentences."},
    {SystemPrompt: "Translate the following to Turkish."},
}, chain.WithModel("gpt-4o"))

result, err := c.Run(ctx, "Long article text here...")

fmt.Println(result.Output)           // Turkish summary
fmt.Println(len(result.Steps))       // 2
fmt.Println(result.TotalUsage)       // Combined token usage
```

## Input Templates

Use `{input}` placeholder to wrap the previous step's output:

```go
chain.Step{
    SystemPrompt:  "You are a data analyst.",
    InputTemplate: "Analyze the following data and identify trends:\n\n{input}\n\nFocus on key metrics.",
}
```

## Transform Functions

Process output between steps:

```go
chain.Step{
    SystemPrompt: "Extract key points as a bullet list.",
    Transform: func(output string) string {
        // Clean up, filter, or transform before passing to next step
        return strings.TrimSpace(output)
    },
}
```

## Multi-Provider Chains

Each step can use a different provider and model:

```go
c := chain.New(openaiProvider, []chain.Step{
    {
        SystemPrompt: "Analyze this code for security issues.",
        Model:        "gpt-4o",
        // Uses default provider (openaiProvider)
    },
    {
        SystemPrompt: "Write a detailed report based on the analysis.",
        Provider:     anthropicProvider,  // Different provider for this step
        Model:        "claude-sonnet-4-20250514",
    },
    {
        SystemPrompt: "Summarize the report in 3 bullet points.",
        Provider:     groqProvider,       // Fast inference for summary
        Model:        "llama-3.1-70b-versatile",
    },
})
```

## Result Structure

```go
result, err := c.Run(ctx, input)

result.Output       // Final output from last step
result.Steps        // []StepResult with per-step details
result.TotalUsage   // Accumulated TokenUsage across all steps

// Per-step details
for i, step := range result.Steps {
    fmt.Printf("Step %d: model=%s, tokens=%d\n",
        i+1, step.Model, step.Usage.TotalTokens)
    fmt.Printf("Output: %s\n\n", step.Output)
}
```

## Use Cases

- **Summarize → Translate**: Multi-language content processing
- **Extract → Validate → Format**: Data processing pipelines
- **Analyze → Plan → Execute**: Multi-step reasoning
- **Draft → Review → Polish**: Content creation workflows
