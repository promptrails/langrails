package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/promptrails/llmrails"
)

// Client connects to an MCP server and provides tool discovery and execution.
// It implements tools.Executor so it can be used directly with tools.RunLoop.
type Client struct {
	baseURL string
	headers map[string]string
	client  *http.Client

	mu    sync.RWMutex
	tools []mcpTool
}

// Option configures the MCP client.
type Option func(*Client)

// WithBearerToken sets the Authorization header with a Bearer token.
func WithBearerToken(token string) Option {
	return func(c *Client) {
		c.headers["Authorization"] = "Bearer " + token
	}
}

// WithAPIKey sets the X-API-Key header.
func WithAPIKey(key string) Option {
	return func(c *Client) {
		c.headers["X-API-Key"] = key
	}
}

// WithHeader adds a custom header to all requests.
func WithHeader(key, value string) Option {
	return func(c *Client) {
		c.headers[key] = value
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		c.client = client
	}
}

// NewClient creates a new MCP client and discovers available tools
// from the server. The baseURL should be the MCP endpoint
// (e.g., "http://localhost:8080/mcp").
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	c := &Client{
		baseURL: baseURL,
		headers: map[string]string{
			"Content-Type": "application/json",
		},
		client: &http.Client{Timeout: 30 * 1e9}, // 30 seconds
	}
	for _, opt := range opts {
		opt(c)
	}

	// Initialize session
	if err := c.initialize(); err != nil {
		return nil, fmt.Errorf("mcp: failed to initialize: %w", err)
	}

	// Discover tools
	if err := c.discoverTools(); err != nil {
		return nil, fmt.Errorf("mcp: failed to discover tools: %w", err)
	}

	return c, nil
}

// ToolDefinitions returns the available tools as llmrails ToolDefinitions,
// ready to be passed to a CompletionRequest.
func (c *Client) ToolDefinitions() []llmrails.ToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()

	defs := make([]llmrails.ToolDefinition, len(c.tools))
	for i, tool := range c.tools {
		params, _ := json.Marshal(tool.InputSchema)
		defs[i] = llmrails.ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  params,
		}
	}
	return defs
}

// Execute calls a tool on the MCP server. This implements tools.Executor,
// so the client can be passed directly to tools.RunLoop.
func (c *Client) Execute(ctx context.Context, name string, arguments string) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		args = map[string]interface{}{"input": arguments}
	}

	resp, err := c.call(ctx, "tools/call", map[string]interface{}{
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

	// Extract text content from result
	for _, content := range result.Content {
		if content.Type == "text" {
			return content.Text, nil
		}
	}

	return string(resp), nil
}

// RefreshTools re-discovers tools from the server.
func (c *Client) RefreshTools() error {
	return c.discoverTools()
}

// Close releases resources. Currently a no-op but reserved for
// future connection management.
func (c *Client) Close() error {
	return nil
}

func (c *Client) initialize() error {
	_, err := c.call(context.Background(), "initialize", map[string]interface{}{
		"protocolVersion": "2025-03-26",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "llmrails",
			"version": "1.0.0",
		},
	})
	return err
}

func (c *Client) discoverTools() error {
	resp, err := c.call(context.Background(), "tools/list", nil)
	if err != nil {
		return err
	}

	var result toolListResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return fmt.Errorf("failed to parse tools list: %w", err)
	}

	c.mu.Lock()
	c.tools = result.Tools
	c.mu.Unlock()

	return nil
}

func (c *Client) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	reqBody := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
		Params:  params,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON-RPC response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}
