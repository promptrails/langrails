# Tool Calling

Tool calling (also called function calling) lets the LLM request execution of external functions. langrails provides a unified tool calling interface across all providers and an automatic tool execution loop.

## Typed Tools

The easiest way to define a tool is from a typed Go function. The parameter
schema is generated from the input struct, arguments are decoded before your
function runs, and the result is encoded as JSON (a `string` result is sent
as is):

```go
import "github.com/promptrails/langrails/tools"

type WeatherArgs struct {
    City string `json:"city" description:"City name"`
    Unit string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit"`
    Days int    `json:"days,omitempty" jsonschema:"minimum=1,maximum=14"`
}

type Weather struct {
    Temp      int    `json:"temp"`
    Condition string `json:"condition"`
}

weather := tools.MustNew("get_weather", "Get current weather for a city",
    func(ctx context.Context, in WeatherArgs) (Weather, error) {
        return Weather{Temp: 22, Condition: "sunny"}, nil
    })

set := tools.NewSet(weather /*, more tools... */)

result, err := tools.RunLoop(ctx, provider, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Weather in Istanbul?"}},
    Tools:    set.Definitions(),
}, set)
```

`tools.New` returns an error instead of panicking; use it when the input type
is not known to be valid at compile time.

### Schema tags

Schemas follow `encoding/json`: the `json` tag names a property, `-` skips
it, and embedded structs are flattened. A property is **required** unless its
json tag has `omitempty`/`omitzero` or the field is a pointer.

| Tag | Example | Effect |
|-----|---------|--------|
| `description` | `description:"City name"` | Property description |
| `jsonschema:"enum=…"` | `enum=celsius\|fahrenheit` | Allowed values (typed by the field kind) |
| `jsonschema:"minimum=…,maximum=…"` | `minimum=1,maximum=14` | Numeric bounds |
| `jsonschema:"minLength=…,maxLength=…"` | `maxLength=64` | String length |
| `jsonschema:"minItems=…,maxItems=…"` | `minItems=1` | Array length |
| `jsonschema:"format=…"` | `format=email` | String format |
| `jsonschema:"required"` / `"optional"` | | Override the default |

`time.Time` becomes a `date-time` string, `[]byte` a base64 string, and
`any`/`json.RawMessage` an unconstrained value. Recursive types are rejected,
because several providers do not accept `$ref`.

## Defining Tools Manually

Tools can also be defined using `langrails.ToolDefinition` with a hand-written JSON schema for parameters:

```go
tools := []langrails.ToolDefinition{
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
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Weather in Istanbul?"}},
    Tools:    tools,
})

if len(resp.ToolCalls) > 0 {
    // Model wants to call a tool
    tc := resp.ToolCalls[0]
    fmt.Printf("Tool: %s\nArgs: %s\n", tc.Name, tc.Arguments)

    // Execute the tool (your implementation)
    result := executeMyTool(tc.Name, tc.Arguments)

    // Send result back to the model
    resp, err = provider.Complete(ctx, &langrails.CompletionRequest{
        Model: "gpt-4o",
        Messages: []langrails.Message{
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
import "github.com/promptrails/langrails/tools"

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
result, err := tools.RunLoop(ctx, provider, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Weather in Istanbul?"}},
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
    tools.WithToolCallHook(func(call langrails.ToolCall, result string, err error) {
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

## Controlling tool choice

Use `ToolChoice` to control whether and which tool the model calls:

```go
req.ToolChoice = langrails.AutoToolChoice()     // model decides (default)
req.ToolChoice = langrails.NoToolChoice()       // forbid tool calls
req.ToolChoice = langrails.RequiredToolChoice() // must call some tool
req.ToolChoice = langrails.ForceTool("get_weather") // must call this tool
```

Supported across OpenAI/compat, Anthropic, Gemini and Bedrock. Notes:

- When `OutputSchema` (structured output) is set, it forces its own tool and
  takes precedence over `ToolChoice`.
- Bedrock (Converse) has no "none" mode; `NoToolChoice()` omits tools entirely.
