# Prompt Templates

The `prompt` package provides reusable prompt templates with Jinja-style `{{ variable }}` syntax and built-in functions.

## Basic Usage

```go
import "github.com/promptrails/langrails/prompt"

t := prompt.MustNew("greeting", "Hello {{ name }}, you are a {{ role }}.")
result, err := t.Execute(map[string]any{
    "name": "Alice",
    "role": "admin",
})
// result: "Hello Alice, you are a admin."
```

## Built-in Functions

Pipe syntax: `{{ variable | function }}`

```go
// Uppercase
prompt.MustNew("t", "{{ name | upper }}").MustExecute(map[string]any{"name": "alice"})
// → "ALICE"

// Lowercase
prompt.MustNew("t", "{{ name | lower }}").MustExecute(map[string]any{"name": "ALICE"})
// → "alice"

// Trim whitespace
prompt.MustNew("t", "{{ text | trim }}").MustExecute(map[string]any{"text": "  hello  "})
// → "hello"
```

Available functions: `upper`, `lower`, `trim`, `join`, `contains`, `replace`, `default`.

## With LLM Provider

```go
t := prompt.MustNew("analyze", `You are a {{ role }}.
Analyze the following {{ language }} code for security issues:

{{ code }}

Focus on: {{ focus }}`)

systemPrompt := t.MustExecute(map[string]any{
    "role":     "security expert",
    "language": "Go",
    "code":     userCode,
    "focus":    "SQL injection and XSS",
})

resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:        "gpt-4o",
    SystemPrompt: systemPrompt,
    Messages:     []langrails.Message{{Role: "user", Content: "Review this code"}},
})
```

## Advanced Syntax

Templates support Go's `text/template` features for conditionals and loops:

```go
// Conditionals
t := prompt.MustNew("t", `{{if .premium}}You are a premium user.{{else}}Free tier.{{end}}`)

// Loops
t := prompt.MustNew("t", `Rules:{{range .rules}}
- {{.}}{{end}}`)

// Mixed: simple vars + advanced syntax
t := prompt.MustNew("t", `Hello {{ name }}. {{if .vip}}Welcome back!{{end}}`)
```

## Prompt Builder

Build complex prompts from sections:

```go
b := prompt.NewBuilder()
b.AddLine("You are a {{ role }}.")
b.AddSection("Context", "The user is asking about {{ topic }}.")
b.AddSection("Rules", "- Be concise\n- Be accurate\n- Cite sources")
b.AddTemplate("Respond in {{ language }}.")

result, _ := b.Build(map[string]any{
    "role":     "helpful assistant",
    "topic":    "quantum computing",
    "language": "Turkish",
})
```

## Reusable Templates

```go
// Define once
var (
    analyzeTemplate = prompt.MustNew("analyze",
        "Analyze this {{ type }}: {{ content }}")

    summarizeTemplate = prompt.MustNew("summarize",
        "Summarize in {{ count }} sentences: {{ text }}")
)

// Use many times
result1, _ := analyzeTemplate.Execute(map[string]any{"type": "code", "content": code})
result2, _ := summarizeTemplate.Execute(map[string]any{"count": "3", "text": article})
```
