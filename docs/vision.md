# Vision / Multimodal

LangRails supports sending images alongside text in messages. This enables vision capabilities like image analysis, OCR, chart reading, and visual Q&A.

## Sending Images

Use `ContentParts` on a message to mix text and images:

```go
resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model: "gpt-4o",
    Messages: []langrails.Message{{
        Role: "user",
        ContentParts: []langrails.ContentPart{
            langrails.TextPart("What's in this image?"),
            langrails.ImageURLPart("https://example.com/photo.jpg"),
        },
    }},
})
```

## Image from URL

```go
langrails.ImageURLPart("https://example.com/image.png")
```

## Image from Base64

```go
// From base64-encoded data
imageData := base64.StdEncoding.EncodeToString(imageBytes)
langrails.ImageBase64Part(imageData, "image/png")
```

## Multiple Images

```go
resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model: "gpt-4o",
    Messages: []langrails.Message{{
        Role: "user",
        ContentParts: []langrails.ContentPart{
            langrails.TextPart("Compare these two images:"),
            langrails.ImageURLPart("https://example.com/image1.jpg"),
            langrails.ImageURLPart("https://example.com/image2.jpg"),
        },
    }},
})
```

## Text-Only vs Multimodal

For text-only messages, use `Content` as before:

```go
// Text only (simple)
langrails.Message{Role: "user", Content: "Hello!"}

// Multimodal (text + images)
langrails.Message{
    Role: "user",
    ContentParts: []langrails.ContentPart{
        langrails.TextPart("Describe this:"),
        langrails.ImageURLPart(url),
    },
}
```

When `ContentParts` is set, it takes precedence over `Content`.

## Provider Support

| Provider | Vision Support |
|----------|---------------|
| OpenAI | Yes (GPT-4o, GPT-4 Turbo) |
| Anthropic | Yes (Claude 3+) |
| Gemini | Yes (all models) |
| All compat | Varies by model |
| Ollama | Yes (llava, bakllava) |
