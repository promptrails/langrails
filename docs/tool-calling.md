# Tool Calling

Tool calling (also called function calling) lets the LLM request execution of external functions. unillm provides a unified tool calling interface across all providers and an automatic tool execution loop.

## Defining Tools

Tools are defined using `unillm.ToolDefinition` with a JSON schema for parameters:

```go
tools := []unillm.ToolDefinition{
    {
        Name:        "get_weather",
        Description: "Get current weather for a city",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name"},
                "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
            },
            "required": ["city"]
        }`),
    },
    {
        Name:        "search_web",
        Description: "Search the web for information",
        Parameters: json.RawMessage(`{
            "type": "object",
            "properties": {
                "query": {"type": "string"}
            },
            "required": ["query"]
        }`),
    },
}
```

## Manual Tool Calling

Handle tool calls yourself for full control:

```go
resp, err := provider.Complete(ctx, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Weather in Istanbul?"}},
    Tools:    tools,
})

if len(resp.ToolCalls) > 0 {
    // Model wants to call a tool
    tc := resp.ToolCalls[0]
    fmt.Printf("Tool: %s\nArgs: %s\n", tc.Name, tc.Arguments)

    // Execute the tool (your implementation)
    result := executeMyTool(tc.Name, tc.Arguments)

    // Send result back to the model
    resp, err = provider.Complete(ctx, &unillm.CompletionRequest{
        Model: "gpt-4o",
        Messages: []unillm.Message{
            {Role: "user", Content: "Weather in Istanbul?"},
            {Role: "assistant", ToolCalls: resp.ToolCalls},
            {Role: "tool", ToolCallID: tc.ID, Content: result},
        },
        Tools: tools,
    })
    // resp.Content now has the final answer
}
```

## Automatic Tool Loop

The `tools` package automates the entire cycle:

```go
import "github.com/promptrails/unillm/tools"

// Define tool implementations
executor := tools.NewMap(map[string]tools.Func{
    "get_weather": func(ctx context.Context, args string) (string, error) {
        var params struct {
            City string `json:"city"`
        }
        json.Unmarshal([]byte(args), &params)

        // Call weather API...
        return `{"temp": 22, "condition": "sunny"}`, nil
    },
    "search_web": func(ctx context.Context, args string) (string, error) {
        // Search API...
        return `{"results": [...]}`, nil
    },
})

// RunLoop handles the entire LLM ↔ tool cycle
result, err := tools.RunLoop(ctx, provider, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Weather in Istanbul?"}},
    Tools:    toolDefs,
}, executor)

fmt.Println(result.Response.Content)  // Final text answer
fmt.Println(result.Iterations)        // Number of LLM calls
fmt.Println(result.TotalUsage)        // Accumulated token usage
```

## Tool Loop Options

```go
// Limit iterations (default: 20)
result, err := tools.RunLoop(ctx, provider, req, executor,
    tools.WithMaxIterations(5),
)

// Hook for observability
result, err := tools.RunLoop(ctx, provider, req, executor,
    tools.WithToolCallHook(func(call unillm.ToolCall, result string, err error) {
        log.Printf("Tool: %s, Result: %s, Error: %v", call.Name, result, err)
    }),
)
```

## Custom Executor

Implement `tools.Executor` for complex routing:

```go
type MyExecutor struct {
    db     *sql.DB
    cache  *redis.Client
}

func (e *MyExecutor) Execute(ctx context.Context, name string, arguments string) (string, error) {
    switch name {
    case "query_database":
        return e.queryDB(ctx, arguments)
    case "get_cache":
        return e.getCache(ctx, arguments)
    default:
        return "", fmt.Errorf("unknown tool: %s", name)
    }
}

// Use with RunLoop
result, err := tools.RunLoop(ctx, provider, req, &MyExecutor{db: db, cache: cache})
```

## Provider Support

| Provider | Tool Calling | Parallel Tool Calls | Tool Choice |
|----------|-------------|--------------------| ------------|
| OpenAI | Yes | Yes | auto |
| Anthropic | Yes (tool_use blocks) | Yes | auto, tool |
| Gemini | Yes (functionCall) | Yes | auto |
| All compat providers | Yes | Yes | auto |

## Error Handling

When a tool execution fails, the error is sent back to the model as a JSON error object. The model can then decide to retry, use a different tool, or respond with an error message:

```go
executor := tools.NewMap(map[string]tools.Func{
    "risky_tool": func(ctx context.Context, args string) (string, error) {
        return "", errors.New("service unavailable")
    },
})

// RunLoop sends {"error": "service unavailable"} back to the model
// The model will typically acknowledge the error in its response
```
