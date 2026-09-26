package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/promptrails/langrails"
)

// protocolVersion is the MCP revision the client announces.
const protocolVersion = "2025-03-26"

// defaultSetupTimeout bounds the initialize handshake and the first tool
// discovery, so a server that never answers does not hang NewClient.
const defaultSetupTimeout = 30 * time.Second

// transport carries JSON-RPC messages to an MCP server.
type transport interface {
	// call sends a request and returns its result.
	call(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
	// notify sends a notification, which has no response.
	notify(ctx context.Context, method string, params interface{}) error
	close() error
}

// Client connects to an MCP server and provides tool discovery and execution.
// It implements tools.Executor so it can be used directly with tools.RunLoop.
//
// Create one with NewClient (streamable HTTP) or NewStdioClient (a local
// server process speaking over stdin/stdout).
type Client struct {
	t transport

	// HTTP settings.
	headers map[string]string
	client  *http.Client

	// Stdio settings.
	env    []string
	dir    string
	stderr io.Writer

	setupTimeout time.Duration

	mu    sync.RWMutex
	tools []mcpTool
}

// Option configures the MCP client. Options that only apply to one
// transport (headers for HTTP, environment for stdio) are ignored by the
// other.
type Option func(*Client)

// WithBearerToken sets the Authorization header with a Bearer token (HTTP).
func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.headers["Authorization"] = "Bearer " + token
	}
}

// WithAPIKey sets the X-API-Key header (HTTP).
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.headers["X-API-Key"] = key
	}
}

// WithHeader adds a custom header to all requests (HTTP).
func WithHeader(key, value string) Option {
	return func(c *Client) {
		c.headers[key] = value
	}
}

// WithHTTPClient sets a custom HTTP client (HTTP).
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.client = client
	}
}

// WithEnv adds "KEY=value" entries to the server process environment,
// on top of the current process's environment (stdio).
func WithEnv(env ...string) Option {
	return func(c *Client) {
		c.env = append(c.env, env...)
	}
}

// WithDir sets the server process working directory (stdio).
func WithDir(dir string) Option {
	return func(c *Client) {
		c.dir = dir
	}
}

// WithStderr receives the server process's stderr, where servers write
// their logs (stdio). By default it is discarded.
func WithStderr(w io.Writer) Option {
	return func(c *Client) {
		c.stderr = w
	}
}

// WithSetupTimeout bounds the initialize handshake and initial tool
// discovery. Default is 30 seconds.
func WithSetupTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.setupTimeout = d
	}
}

func newClient(opts []Option) *Client {
	c := &Client{
		headers: map[string]string{
			"Content-Type": "application/json",
		},
		client:       &http.Client{Timeout: 30 * time.Second},
		setupTimeout: defaultSetupTimeout,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewClient creates a new MCP client and discovers available tools
// from the server. The baseURL should be the MCP endpoint
// (e.g., "http://localhost:8080/mcp").
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	c := newClient(opts)
	c.t = &httpTransport{url: baseURL, headers: c.headers, client: c.client}
	if err := c.setup(); err != nil {
		return nil, err
	}
	return c, nil
}

// NewStdioClient starts an MCP server as a child process and talks to it
// over stdin/stdout, the transport most published MCP servers use:
//
//	client, err := mcp.NewStdioClient("npx", []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"})
//
// Close stops the process. If the handshake fails the process is stopped
// before the error is returned.
func NewStdioClient(command string, args []string, opts ...Option) (*Client, error) {
	c := newClient(opts)
	t, err := startStdio(command, args, c.env, c.dir, c.stderr)
	if err != nil {
		return nil, fmt.Errorf("mcp: %w", err)
	}
	c.t = t
	if err := c.setup(); err != nil {
		_ = t.close()
		return nil, err
	}
	return c, nil
}

func (c *Client) setup() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.setupTimeout)
	defer cancel()

	if err := c.initialize(ctx); err != nil {
		return fmt.Errorf("mcp: failed to initialize: %w", err)
	}
	if err := c.discoverTools(ctx); err != nil {
		return fmt.Errorf("mcp: failed to discover tools: %w", err)
	}
	return nil
}

// ToolDefinitions returns the available tools as langrails ToolDefinitions,
// ready to be passed to a CompletionRequest.
func (c *Client) ToolDefinitions() []langrails.ToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()

	defs := make([]langrails.ToolDefinition, len(c.tools))
	for i, tool := range c.tools {
		params, _ := json.Marshal(tool.InputSchema)
		defs[i] = langrails.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		}
	}
	return defs
}

// Execute calls a tool on the MCP server. This implements tools.Executor,
// so the client can be passed directly to tools.RunLoop.
//
// The text content of the result is returned (multiple text blocks are
// joined with newlines). A result the server marks as an error (isError)
// is returned as an error carrying that text.
func (c *Client) Execute(ctx context.Context, name string, arguments string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		args = map[string]interface{}{"input": arguments}
	}

	resp, err := c.t.call(ctx, "tools/call", map[string]interface{}{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return "", fmt.Errorf("mcp: tool call %q failed: %w", name, err)
	}

	var result toolCallResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return string(resp), nil
	}

	var texts []string
	for _, content := range result.Content {
		if content.Type == "text" {
			texts = append(texts, content.Text)
		}
	}
	text := strings.Join(texts, "\n")
	if len(texts) == 0 {
		text = string(resp)
	}
	if result.IsError {
		return "", fmt.Errorf("mcp: tool %q returned an error: %s", name, text)
	}
	return text, nil
}

// RefreshTools re-discovers tools from the server.
func (c *Client) RefreshTools() error {
	return c.discoverTools(context.Background())
}

// Close releases the connection. For a stdio client it stops the server
// process.
func (c *Client) Close() error {
	return c.t.close()
}

func (c *Client) initialize(ctx context.Context) error {
	_, err := c.t.call(ctx, "initialize", map[string]interface{}{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "langrails",
			"version": "1.0.0",
		},
	})
	if err != nil {
		return err
	}
	// Required by the spec, but best-effort: simple servers that reject the
	// notification still serve tools correctly.
	_ = c.t.notify(ctx, "notifications/initialized", nil)
	return nil
}

func (c *Client) discoverTools(ctx context.Context) error {
	var all []mcpTool
	err := paginate(ctx, c.t, "tools/list", func(raw json.RawMessage) (string, error) {
		var result toolListResult
		if err := json.Unmarshal(raw, &result); err != nil {
			return "", fmt.Errorf("failed to parse tools list: %w", err)
		}
		all = append(all, result.Tools...)
		return result.NextCursor, nil
	})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.tools = all
	c.mu.Unlock()
	return nil
}

// paginate calls a list method until the server stops returning a cursor.
func paginate(ctx context.Context, t transport, method string, page func(json.RawMessage) (string, error)) error {
	cursor := ""
	for {
		var params interface{}
		if cursor != "" {
			params = map[string]interface{}{"cursor": cursor}
		}
		raw, err := t.call(ctx, method, params)
		if err != nil {
			return err
		}
		next, err := page(raw)
		if err != nil {
			return err
		}
		if next == "" || next == cursor {
			return nil
		}
		cursor = next
	}
}
