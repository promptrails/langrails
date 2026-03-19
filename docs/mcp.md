# MCP (Model Context Protocol)

The `mcp` package provides a client for the [Model Context Protocol](https://modelcontextprotocol.io/), enabling your LLM applications to connect to MCP servers and use their tools.

## What is MCP?

MCP is an open protocol that standardizes how LLMs interact with external tools and data sources. An MCP server exposes tools via a JSON-RPC 2.0 API. The unillm MCP client connects to these servers, discovers available tools, and executes them.

## Connecting to an MCP Server

```go
import "github.com/promptrails/unillm/mcp"

client, err := mcp.NewClient("http://localhost:8080/mcp",
    mcp.WithBearerToken("your-token"),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

The client automatically initializes the MCP session and discovers available tools on creation.

## Authentication

```go
// Bearer token
mcp.NewClient(url, mcp.WithBearerToken("token"))

// API key
mcp.NewClient(url, mcp.WithAPIKey("key"))

// Custom header
mcp.NewClient(url, mcp.WithHeader("X-Custom-Auth", "value"))
```

## Using MCP Tools with LLM

```go
// 1. Get tool definitions for the LLM
toolDefs := client.ToolDefinitions()

// 2. Send to provider with tools
resp, err := provider.Complete(ctx, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "Search for Go tutorials"}},
    Tools:    toolDefs,
})

// 3. If model wants to call a tool, execute it via MCP
if len(resp.ToolCalls) > 0 {
    tc := resp.ToolCalls[0]
    result, err := client.Execute(ctx, tc.Name, tc.Arguments)
    // Send result back to model...
}
```

## MCP + Tool Loop

The MCP client implements `tools.Executor`, so it works directly with `tools.RunLoop`:

```go
import (
    "github.com/promptrails/unillm/mcp"
    "github.com/promptrails/unillm/tools"
)

client, _ := mcp.NewClient("http://localhost:8080/mcp",
    mcp.WithBearerToken("token"),
)
defer client.Close()

result, err := tools.RunLoop(ctx, provider, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []unillm.Message{{Role: "user", Content: "What's the weather?"}},
    Tools:    client.ToolDefinitions(),
}, client) // client implements tools.Executor

fmt.Println(result.Response.Content)
```

## Combining MCP Tools with Local Tools

```go
// MCP tools from server
mcpClient, _ := mcp.NewClient("http://localhost:8080/mcp")

// Local tools
localFuncs := map[string]tools.Func{
    "calculate": func(ctx context.Context, args string) (string, error) {
        // Local calculation...
        return "42", nil
    },
}

// Combine tool definitions
allTools := append(mcpClient.ToolDefinitions(), unillm.ToolDefinition{
    Name:        "calculate",
    Description: "Perform calculations",
    Parameters:  json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`),
})

// Combined executor that routes to MCP or local
type combinedExecutor struct {
    mcp   *mcp.Client
    local *tools.MapExecutor
}

func (c *combinedExecutor) Execute(ctx context.Context, name string, args string) (string, error) {
    // Try local first
    if result, err := c.local.Execute(ctx, name, args); err == nil {
        return result, nil
    }
    // Fall back to MCP
    return c.mcp.Execute(ctx, name, args)
}

result, err := tools.RunLoop(ctx, provider, &unillm.CompletionRequest{
    Model:    "gpt-4o",
    Messages: messages,
    Tools:    allTools,
}, &combinedExecutor{mcp: mcpClient, local: tools.NewMap(localFuncs)})
```

## Refreshing Tools

If the MCP server's tool list changes:

```go
err := client.RefreshTools()
```

## Custom HTTP Client

```go
client, _ := mcp.NewClient(url,
    mcp.WithHTTPClient(&http.Client{
        Timeout: time.Minute,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{...},
        },
    }),
)
```

## Error Handling

```go
client, err := mcp.NewClient(url)
if err != nil {
    // Connection or initialization failed
}

result, err := client.Execute(ctx, "tool_name", args)
if err != nil {
    // Tool execution failed (network error, RPC error, etc.)
}
```
