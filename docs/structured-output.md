# Structured Output

Structured output constrains the LLM to respond in a specific JSON format defined by a JSON schema. This is useful for extracting structured data, building type-safe APIs, and ensuring consistent output.

## Basic Usage

```go
schema := []byte(`{
    "type": "object",
    "properties": {
        "sentiment": {
            "type": "string",
            "enum": ["positive", "negative", "neutral"]
        },
        "confidence": {
            "type": "number",
            "minimum": 0,
            "maximum": 1
        },
        "summary": {
            "type": "string"
        }
    },
    "required": ["sentiment", "confidence", "summary"]
}`)

resp, err := provider.Complete(ctx, &unillm.CompletionRequest{
    Model:        "gpt-4o",
    SystemPrompt: "Analyze the sentiment of the given text.",
    Messages:     []unillm.Message{{Role: "user", Content: "I love this product!"}},
    OutputSchema: &schema,
})

// resp.Content is guaranteed to be valid JSON matching the schema
fmt.Println(resp.Content)
// {"sentiment": "positive", "confidence": 0.95, "summary": "Enthusiastic positive review"}
```

## Parsing Responses

```go
type SentimentResult struct {
    Sentiment  string  `json:"sentiment"`
    Confidence float64 `json:"confidence"`
    Summary    string  `json:"summary"`
}

var result SentimentResult
if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
    log.Fatal(err)
}

fmt.Printf("Sentiment: %s (%.0f%%)\n", result.Sentiment, result.Confidence*100)
```

## Provider Implementation

Each provider handles structured output differently:

### OpenAI & Compatible Providers
Uses `response_format` with `json_schema` type and strict mode:
```json
{
    "response_format": {
        "type": "json_schema",
        "json_schema": {
            "name": "response",
            "schema": { ... },
            "strict": true
        }
    }
}
```

### Anthropic
Uses a forced tool call pattern: the schema is defined as a tool named `structured_output` with `tool_choice: {"type": "tool", "name": "structured_output"}`. The model's tool call arguments become the response content.

### Gemini
Uses native `responseMimeType` and `responseSchema` in the generation config:
```json
{
    "generationConfig": {
        "responseMimeType": "application/json",
        "responseSchema": { ... }
    }
}
```

## Complex Schemas

```go
schema := []byte(`{
    "type": "object",
    "properties": {
        "entities": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "type": {"type": "string", "enum": ["person", "org", "location"]},
                    "mentions": {"type": "integer"}
                },
                "required": ["name", "type", "mentions"]
            }
        },
        "language": {"type": "string"},
        "word_count": {"type": "integer"}
    },
    "required": ["entities", "language", "word_count"]
}`)
```

## Provider Support

| Provider | Method | Strict Enforcement |
|----------|--------|--------------------|
| OpenAI | JSON schema mode | Yes (strict: true) |
| Anthropic | Forced tool call | Yes (via tool schema) |
| Gemini | responseSchema | Yes |
| DeepSeek | JSON schema mode | Yes |
| Groq | JSON schema mode | Yes |
| Fireworks | JSON schema mode | Varies |
| Together | JSON schema mode | Varies |
| Mistral | JSON schema mode | Varies |
| Cohere | JSON schema mode | Varies |
| xAI | JSON schema mode | Varies |
| OpenRouter | JSON schema mode | Depends on underlying model |
