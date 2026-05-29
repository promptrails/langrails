# Streaming

All providers support streaming responses via Go channels. This is useful for real-time UIs, CLI tools, or any scenario where you want to display tokens as they arrive.

## Basic Streaming

```go
events, err := provider.Stream(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Write a poem about Go"}},
})
if err != nil {
    log.Fatal(err)
}

for event := range events {
    switch event.Type {
    case langrails.EventReasoning:
        fmt.Print(event.Reasoning) // reasoning/thinking chunk (before content)

    case langrails.EventContent:
        fmt.Print(event.Content) // Print each chunk as it arrives

    case langrails.EventCitation:
        fmt.Printf("\n[source] %s\n", event.Citation.URL)

    case langrails.EventToolCall:
        fmt.Printf("\nTool call: %s(%s)\n", event.ToolCall.Name, event.ToolCall.Arguments)

    case langrails.EventDone:
        fmt.Println("\n--- stream complete ---")

    case langrails.EventError:
        log.Printf("stream error: %v", event.Error)
    }

    // Token usage (may come with the final event)
    if event.Usage != nil {
        fmt.Printf("Tokens: %d\n", event.Usage.TotalTokens)
    }
}
```

## Event Types

| Type | Description | Fields |
|------|-------------|--------|
| `EventReasoning` | Reasoning/thinking text chunk (emitted before content) | `Reasoning` |
| `EventContent` | Text content chunk | `Content` |
| `EventCitation` | A source/citation was emitted | `Citation` |
| `EventToolCall` | Tool/function call | `ToolCall` (ID, Name, Arguments) |
| `EventDone` | Stream completed | — |
| `EventError` | Error occurred | `Error` |

Any event may also carry `Usage` (token counts, typically with the final event).

The `Usage` field may be present on any event type (typically the last content or done event).

## Collecting Full Response

If you want the full response but still want to process chunks:

```go
var fullContent strings.Builder

events, _ := provider.Stream(ctx, req)
for event := range events {
    if event.Type == langrails.EventContent {
        fullContent.WriteString(event.Content)
        // Also display to user...
    }
}

fmt.Println("Full response:", fullContent.String())
```

## Cancellation

Streaming respects context cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

events, err := provider.Stream(ctx, req)
// Channel will be closed when context is cancelled
```

You can also cancel mid-stream:

```go
ctx, cancel := context.WithCancel(context.Background())

events, _ := provider.Stream(ctx, req)
for event := range events {
    if event.Type == langrails.EventContent {
        fmt.Print(event.Content)
        if strings.Contains(event.Content, "stop word") {
            cancel() // Stop streaming
        }
    }
}
```

## Streaming with Tool Calls

When a model decides to call tools during streaming, tool call events are accumulated and emitted before the `EventDone` event:

```go
var toolCalls []langrails.ToolCall

events, _ := provider.Stream(ctx, req)
for event := range events {
    switch event.Type {
    case langrails.EventContent:
        fmt.Print(event.Content)
    case langrails.EventToolCall:
        toolCalls = append(toolCalls, *event.ToolCall)
    case langrails.EventDone:
        if len(toolCalls) > 0 {
            // Process tool calls...
        }
    }
}
```

## Provider Notes

| Provider | Streaming Protocol | Notes |
|----------|--------------------|-------|
| OpenAI | SSE with `data: [DONE]` | Standard OpenAI format |
| Anthropic | SSE with event types | Uses `content_block_delta`, `message_stop` |
| Gemini | SSE with `?alt=sse` | API key in URL parameter |
| All compat providers | Same as OpenAI | DeepSeek, Groq, etc. |

The channel is always closed when the stream ends, so `range` over the channel is safe and will not block forever.
