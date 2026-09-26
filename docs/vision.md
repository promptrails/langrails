# Vision / Multimodal

LangRails supports sending images, audio and documents (PDF and more) alongside
text in messages. This enables image analysis, OCR, chart reading, transcription,
audio Q&A and document understanding.

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

## Audio

```go
audio := base64.StdEncoding.EncodeToString(wavBytes)

resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model: "gpt-4o-audio-preview",
    Messages: []langrails.Message{{
        Role: "user",
        ContentParts: []langrails.ContentPart{
            langrails.TextPart("Transcribe and summarize this call."),
            langrails.AudioPart(audio, "audio/wav"),
        },
    }},
})
```

OpenAI-compatible providers accept WAV (`audio/wav`) and MP3 (`audio/mpeg`);
Gemini accepts any audio type it supports (`audio/ogg`, `audio/flac`, ...).

## Documents (PDF)

```go
pdf := base64.StdEncoding.EncodeToString(pdfBytes)

langrails.DocumentPart(pdf, "application/pdf", "q3-report.pdf") // inline
langrails.DocumentURLPart("https://example.com/q3.pdf", "application/pdf") // by URL
```

The filename is optional; providers that require one (Bedrock) get a
generated name when it is empty. On Anthropic a `text/plain` document is sent
as a text document; on Bedrock the media type picks the Converse format (PDF,
CSV, DOC/DOCX, XLS/XLSX, HTML, TXT, Markdown).

## Unsupported content

A provider that cannot carry a part — audio to Anthropic, a document URL to
OpenAI — returns an error wrapping `langrails.ErrUnsupportedContent` **before
sending anything**, rather than silently dropping the part:

```go
_, err := provider.Complete(ctx, req)
if errors.Is(err, langrails.ErrUnsupportedContent) {
    // fall back: extract text locally, pick another provider, ...
}
```

The check is about the wire format. Whether a particular *model* accepts audio
or PDFs is still up to the provider.

## Provider Support

| Provider | Audio | Documents (inline) | Documents (URL) |
|----------|-------|--------------------|-----------------|
| OpenAI / compat | WAV, MP3 (`input_audio`) | Yes (`file`) | No |
| Anthropic | No | PDF, plain text | Yes |
| Gemini | Yes (`inlineData`) | Yes (`inlineData`) | File API / GCS URIs |
| Bedrock | No | PDF, CSV, DOC(X), XLS(X), HTML, TXT, MD | No |

| Provider | Vision Support |
|----------|---------------|
| OpenAI | Yes (GPT-4o, GPT-4 Turbo) |
| Anthropic | Yes (Claude 3+) — base64 or URL |
| Gemini | Yes (all models) — base64 inline; URLs via File API/GCS |
| Bedrock | Yes (Claude/Nova) — base64 only (Converse can't fetch URLs) |
| All compat | Varies by model |
| Ollama | Yes (llava, bakllava) |

Use `langrails.ImageBase64Part(data, "image/png")` for base64 (most portable) or
`langrails.ImageURLPart(url)` for a URL. Bedrock supports base64 only; Gemini
resolves URLs only for File API / Cloud Storage URIs.
