# MCP (Model Context Protocol)

The `mcp` package provides a client for the [Model Context Protocol](https://modelcontextprotocol.io/), enabling your LLM applications to connect to MCP servers and use their tools, resources and prompts — over HTTP or stdio.

## What is MCP?

MCP is an open protocol that standardizes how LLMs interact with external tools and data sources. An MCP server exposes tools via a JSON-RPC 2.0 API. The langrails MCP client connects to these servers, discovers available tools, and executes them.

## Connecting to an MCP Server

```go
import "github.com/promptrails/langrails/mcp"

client, err := mcp.NewClient("http://localhost:8080/mcp",
    mcp.WithBearerToken("your-token"),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()
```

The client automatically initializes the MCP session and discovers available tools on creation (following pagination). If the server assigns a streamable-HTTP session (`Mcp-Session-Id`), it is sent on every later request.

## Local Servers (stdio)

Most published MCP servers run as a local process that speaks JSON-RPC over
stdin/stdout. `NewStdioClient` starts the process and connects to it:

```go
client, err := mcp.NewStdioClient("npx",
    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
    mcp.WithEnv("DEBUG=1"),      // added to the current environment
    mcp.WithDir("/srv/project"), // working directory
    mcp.WithStderr(os.Stderr),   // server logs (discarded by default)
)
if err != nil {
    log.Fatal(err)
}
defer client.Close() // closes stdin, waits briefly, then kills the process
```

Everything else — `ToolDefinitions`, `Execute`, resources, prompts — works the
same as with an HTTP client. Calls may be made concurrently; cancelling a call's
context sends `notifications/cancelled` to the server.

This pairs with [api2mcp](https://github.com/promptrails/api2mcp), which turns
an existing HTTP API into an MCP server: serve it over stdio and point
`NewStdioClient` at the binary.

```go
client, err := mcp.NewStdioClient("api2mcp", []string{"serve", "--config", "api2mcp.yaml"})
```

`WithSetupTimeout` bounds the initial handshake (default 30s) so a server that
never answers does not hang.

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
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "Search for Go tutorials"}},
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
    "github.com/promptrails/langrails/mcp"
    "github.com/promptrails/langrails/tools"
)

client, _ := mcp.NewClient("http://localhost:8080/mcp",
    mcp.WithBearerToken("token"),
)
defer client.Close()

result, err := tools.RunLoop(ctx, provider, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: []langrails.Message{{Role: "user", Content: "What's the weather?"}},
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
allTools := append(mcpClient.ToolDefinitions(), langrails.ToolDefinition{
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

result, err := tools.RunLoop(ctx, provider, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: messages,
    Tools:    allTools,
}, &combinedExecutor{mcp: mcpClient, local: tools.NewMap(localFuncs)})
```

## Resources

Servers can expose context — files, rows, API responses — as resources:

```go
resources, err := client.ListResources(ctx)
for _, r := range resources {
    fmt.Println(r.URI, r.Name, r.MIMEType)
}

contents, err := client.ReadResource(ctx, "file:///project/README.md")
fmt.Println(contents[0].Text) // or contents[0].Blob (base64) for binary
```

## Prompts

Server prompt templates render straight into langrails messages:

```go
prompts, _ := client.ListPrompts(ctx)

msgs, err := client.GetPrompt(ctx, "code_review", map[string]string{"code": src})
resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
    Model:    "gpt-4o",
    Messages: msgs,
})
```

Text content becomes `Content`, image content an image part, and embedded text
resources are inlined as text.

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
    // Tool execution failed: network error, RPC error, or a result the
    // server marked isError (the error carries the server's message).
}
```

`Execute` returns all text blocks of the result joined with newlines.

## Scope

langrails is an MCP **client**. To expose an HTTP API as an MCP server, see
[api2mcp](https://github.com/promptrails/api2mcp).
```
