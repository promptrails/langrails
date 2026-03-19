# A2A (Agent-to-Agent Protocol)

The `a2a` package implements the [Agent-to-Agent (A2A) protocol](https://google.github.io/A2A/) — an open standard by Google for inter-agent communication. It provides both a client and server implementation.

## Concepts

- **Agent Card**: Describes an agent's capabilities, skills, and endpoints
- **Task**: A unit of work with messages, artifacts, and lifecycle state
- **Message**: Conversation messages with polymorphic content parts (text, data, files)
- **Artifact**: Generated outputs from an agent
- **JSON-RPC 2.0**: Transport protocol over HTTP

## Client

### Discover Agent

```go
import "github.com/promptrails/llmrails/a2a"

client := a2a.NewClient("https://agent.example.com/a2a")

// Fetch agent capabilities
card, err := client.GetAgentCard(ctx)
fmt.Println(card.Name)           // "My Agent"
fmt.Println(card.Skills)         // Available skills
fmt.Println(card.Capabilities)   // Streaming, push notifications
```

### Send Message

```go
task, err := client.SendMessage(ctx, a2a.SendMessageRequest{
    Message: a2a.Message{
        Role:  a2a.RoleUser,
        Parts: []a2a.Part{a2a.NewTextPart("Summarize this article")},
    },
})

fmt.Println(task.Status.State)  // "completed"
for _, msg := range task.Messages {
    for _, part := range msg.Parts {
        if part.Type == "text" {
            fmt.Println(part.Text)
        }
    }
}
```

### Stream Message

```go
events, err := client.StreamMessage(ctx, a2a.SendMessageRequest{
    Message: a2a.Message{
        Role:  a2a.RoleUser,
        Parts: []a2a.Part{a2a.NewTextPart("Write a long report")},
    },
})

for event := range events {
    switch event.Type {
    case "status":
        fmt.Printf("Status: %s\n", event.StatusUpdate.Status.State)
    case "artifact":
        for _, part := range event.ArtifactUpdate.Artifact.Parts {
            fmt.Print(part.Text)
        }
    case "task":
        fmt.Printf("Final task: %s\n", event.Task.Status.State)
    case "error":
        fmt.Printf("Error: %v\n", event.Error)
    }
}
```

### Manage Tasks

```go
// Get task status
task, err := client.GetTask(ctx, "task-id")

// Cancel a running task
task, err := client.CancelTask(ctx, "task-id")
```

### Authentication

```go
// Bearer token
client := a2a.NewClient(url, a2a.WithBearerToken("token"))

// API key
client := a2a.NewClient(url, a2a.WithAPIKey("key"))
```

## Server

Implement the `TaskHandler` interface to create an A2A server:

```go
type TaskHandler interface {
    HandleMessage(ctx context.Context, req SendMessageRequest) (*Task, error)
    HandleMessageStream(ctx context.Context, req SendMessageRequest, events chan<- StreamEvent)
    GetTask(ctx context.Context, taskID string) (*Task, error)
    CancelTask(ctx context.Context, taskID string) (*Task, error)
}
```

### Example Server

```go
type MyAgent struct {
    provider llmrails.Provider
    tasks    map[string]*a2a.Task
}

func (a *MyAgent) HandleMessage(ctx context.Context, req a2a.SendMessageRequest) (*a2a.Task, error) {
    // Extract text from message parts
    var input string
    for _, part := range req.Message.Parts {
        if part.Type == "text" {
            input += part.Text
        }
    }

    // Call LLM
    resp, err := a.provider.Complete(ctx, &llmrails.CompletionRequest{
        Model:    "gpt-4o",
        Messages: []llmrails.Message{{Role: "user", Content: input}},
    })
    if err != nil {
        return nil, err
    }

    // Build task
    task := &a2a.Task{
        ID:     generateID(),
        Status: a2a.TaskStatus{State: a2a.TaskStateCompleted},
        Messages: []a2a.Message{
            req.Message,
            {Role: a2a.RoleAgent, Parts: []a2a.Part{a2a.NewTextPart(resp.Content)}},
        },
        Artifacts: []a2a.Artifact{
            {Parts: []a2a.Part{a2a.NewTextPart(resp.Content)}},
        },
    }
    a.tasks[task.ID] = task
    return task, nil
}

func (a *MyAgent) HandleMessageStream(ctx context.Context, req a2a.SendMessageRequest, events chan<- a2a.StreamEvent) {
    defer close(events)

    events <- a2a.StreamEvent{
        Type: "status",
        StatusUpdate: &a2a.TaskStatusUpdateEvent{
            TaskID: "task-1",
            Status: a2a.TaskStatus{State: a2a.TaskStateWorking},
        },
    }

    // Stream LLM response...
    // Send artifact events...

    events <- a2a.StreamEvent{
        Type: "status",
        StatusUpdate: &a2a.TaskStatusUpdateEvent{
            TaskID: "task-1",
            Status: a2a.TaskStatus{State: a2a.TaskStateCompleted},
        },
    }
}

func (a *MyAgent) GetTask(_ context.Context, taskID string) (*a2a.Task, error) {
    task, ok := a.tasks[taskID]
    if !ok {
        return nil, a2a.ErrTaskNotFound
    }
    return task, nil
}

func (a *MyAgent) CancelTask(_ context.Context, taskID string) (*a2a.Task, error) {
    task, ok := a.tasks[taskID]
    if !ok {
        return nil, a2a.ErrTaskNotFound
    }
    task.Status.State = a2a.TaskStateCanceled
    return task, nil
}
```

### Serve

```go
card := a2a.AgentCard{
    Name:        "My Agent",
    Description: "An AI assistant",
    URL:         "https://myapp.com/a2a",
    Version:     a2a.ProtocolVersion,
    Capabilities: a2a.AgentCapabilities{
        Streaming: true,
    },
    Skills: []a2a.AgentSkill{
        {ID: "chat", Name: "Chat", Description: "General conversation"},
    },
}

agent := &MyAgent{provider: openai.New("sk-..."), tasks: map[string]*a2a.Task{}}
handler := a2a.NewHandler(card, agent)

http.Handle("/a2a/", handler)
http.ListenAndServe(":8080", nil)
```

The handler serves:
- `GET /a2a/agent-card.json` → Agent card discovery
- `POST /a2a/` → JSON-RPC 2.0 dispatch (message/send, message/stream, tasks/get, tasks/cancel)

## Task Lifecycle

```
submitted → working → completed
                    → failed
                    → input_required → working → ...
         → canceled
         → rejected
```

Terminal states: `completed`, `failed`, `canceled`, `rejected`

## Message Parts

```go
// Text content
a2a.NewTextPart("Hello!")

// Structured data
a2a.NewDataPart(map[string]any{
    "temperature": 22,
    "city":        "Istanbul",
})

// File attachment
a2a.Part{
    Type:     "file",
    MimeType: "image/png",
    File: &a2a.FileContent{
        Name:      "chart.png",
        Bytes:     base64EncodedData,
        MediaType: "image/png",
    },
}
```

## Error Handling

```go
task, err := client.SendMessage(ctx, req)
if err != nil {
    var a2aErr *a2a.Error
    if errors.As(err, &a2aErr) {
        switch a2aErr.Code {
        case a2a.ErrCodeTaskNotFound:
            // Task doesn't exist
        case a2a.ErrCodeTaskNotCancelable:
            // Task already in terminal state
        case a2a.ErrCodeMethodNotFound:
            // Agent doesn't support this method
        }
    }
}
```
