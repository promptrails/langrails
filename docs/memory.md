# Memory (Conversation History)

The `memory` package manages conversation history with configurable limits and automatic truncation. Thread-safe for concurrent use.

## Basic Usage

```go
import "github.com/promptrails/langrails/memory"

mem := memory.New()
mem.AddUserMessage("What is Go?")
mem.AddAssistantMessage("Go is a programming language...")
mem.AddUserMessage("Tell me more about concurrency")

// Use with provider
resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: mem.Messages(),
})

// Store the response
mem.AddAssistantMessage(resp.Content)
```

## Message Limits

### By Message Count

Keep only the last N messages:

```go
mem := memory.New(memory.WithMaxMessages(20))

// After 20 messages, oldest non-system messages are removed
for i := 0; i < 50; i++ {
    mem.AddUserMessage("message")
    mem.AddAssistantMessage("response")
}
fmt.Println(mem.Len()) // 20
```

### By Token Count

Keep messages within a token budget:

```go
mem := memory.New(memory.WithMaxTokens(4000))

// Messages are trimmed to stay within ~4000 tokens
mem.AddUserMessage("long message...")
mem.AddAssistantMessage("long response...")
fmt.Println(mem.TokenCount()) // <= 4000
```

### Combined Limits

```go
mem := memory.New(
    memory.WithMaxMessages(50),
    memory.WithMaxTokens(8000),
)
```

## System Message Preservation

System messages at the start of the conversation are never trimmed:

```go
mem := memory.New(memory.WithMaxMessages(5))
mem.Add(langrails.Message{Role: "system", Content: "You are helpful."})
mem.AddUserMessage("msg1")
mem.AddAssistantMessage("resp1")
mem.AddUserMessage("msg2")
mem.AddAssistantMessage("resp2")
mem.AddUserMessage("msg3") // triggers trim

msgs := mem.Messages()
// msgs[0] is still the system message
// older user/assistant messages were removed
```

## Retrieving Messages

```go
// All messages
all := mem.Messages()

// Last N messages
recent := mem.Last(5)

// Count
fmt.Println(mem.Len())

// Estimated token count
fmt.Println(mem.TokenCount())
```

## Clear History

```go
mem.Clear()
fmt.Println(mem.Len()) // 0
```

## Thread Safety

Memory is safe for concurrent use:

```go
mem := memory.New(memory.WithMaxMessages(100))

// Safe to call from multiple goroutines
go func() { mem.AddUserMessage("from goroutine 1") }()
go func() { mem.AddUserMessage("from goroutine 2") }()
go func() { _ = mem.Messages() }()
```

## Full Chat Loop Example

```go
mem := memory.New(
    memory.WithMaxMessages(50),
    memory.WithMaxTokens(8000),
)

// Add system prompt
mem.Add(langrails.Message{
    Role:    "system",
    Content: "You are a helpful assistant.",
})

// Chat loop
for {
    userInput := getUserInput()
    mem.AddUserMessage(userInput)

    resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:    "gpt-4o",
        Messages: mem.Messages(),
    })
    if err != nil {
        log.Fatal(err)
    }

    mem.AddAssistantMessage(resp.Content)
    fmt.Println(resp.Content)
}
```
