package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/promptrails/llmrails/internal/sse"
)

// Client communicates with a remote A2A agent.
type Client struct {
	baseURL string
	headers map[string]string
	client  *http.Client
}

// ClientOption configures the A2A client.
type ClientOption func(*Client)

// WithBearerToken sets a Bearer token for authentication.
func WithBearerToken(token string) ClientOption {
	return func(c *Client) {
		c.headers["Authorization"] = "Bearer " + token
	}
}

// WithAPIKey sets an X-API-Key header.
func WithAPIKey(key string) ClientOption {
	return func(c *Client) {
		c.headers["X-API-Key"] = key
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.client = client
	}
}

// NewClient creates a new A2A client. The baseURL should be the
// agent's A2A endpoint (e.g., "https://agent.example.com/a2a").
func NewClient(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		headers: map[string]string{"Content-Type": "application/json"},
		client:  &http.Client{Timeout: 60 * 1e9},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetAgentCard fetches the agent's discovery card.
func (c *Client) GetAgentCard(ctx context.Context) (*AgentCard, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/agent-card.json", nil)
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to create request: %w", err)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("a2a: agent card request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a2a: agent card returned status %d", resp.StatusCode)
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("a2a: failed to parse agent card: %w", err)
	}
	return &card, nil
}

// SendMessage sends a message to the agent and returns the task.
// This is the message/send JSON-RPC method.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (*Task, error) {
	result, err := c.call(ctx, "message/send", req)
	if err != nil {
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(result, &task); err != nil {
		return nil, fmt.Errorf("a2a: failed to parse task: %w", err)
	}
	return &task, nil
}

// StreamMessage sends a message and returns a channel of streaming events.
// This is the message/stream JSON-RPC method.
func (c *Client) StreamMessage(ctx context.Context, req SendMessageRequest) (<-chan StreamEvent, error) {
	body, err := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "message/stream",
		Params:  mustMarshal(req),
		ID:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to create request: %w", err)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("a2a: stream request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("a2a: stream returned status %d", resp.StatusCode)
	}

	ch := make(chan StreamEvent, 64)
	go c.readStream(resp.Body, ch)
	return ch, nil
}

// GetTask retrieves a task by ID.
func (c *Client) GetTask(ctx context.Context, taskID string) (*Task, error) {
	result, err := c.call(ctx, "tasks/get", GetTaskRequest{ID: taskID})
	if err != nil {
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(result, &task); err != nil {
		return nil, fmt.Errorf("a2a: failed to parse task: %w", err)
	}
	return &task, nil
}

// CancelTask cancels a running task.
func (c *Client) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	result, err := c.call(ctx, "tasks/cancel", CancelTaskRequest{ID: taskID})
	if err != nil {
		return nil, err
	}

	var task Task
	if err := json.Unmarshal(result, &task); err != nil {
		return nil, fmt.Errorf("a2a: failed to parse task: %w", err)
	}
	return &task, nil
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  mustMarshal(params),
		ID:      1,
	})
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to create request: %w", err)
	}
	for k, v := range c.headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("a2a: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a2a: server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp JSONRPCResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("a2a: failed to parse response: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, &Error{Code: rpcResp.Error.Code, Message: rpcResp.Error.Message}
	}

	result, err := json.Marshal(rpcResp.Result)
	if err != nil {
		return nil, fmt.Errorf("a2a: failed to re-marshal result: %w", err)
	}
	return result, nil
}

// StreamEvent represents a streaming event from message/stream.
type StreamEvent struct {
	// Type is "status", "artifact", "task", or "error".
	Type string

	// StatusUpdate is set when Type is "status".
	StatusUpdate *TaskStatusUpdateEvent

	// ArtifactUpdate is set when Type is "artifact".
	ArtifactUpdate *TaskArtifactUpdateEvent

	// Task is set when Type is "task" (final result).
	Task *Task

	// Error is set when Type is "error".
	Error error
}

func (c *Client) readStream(body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	reader := sse.NewReader(body)
	for {
		event, ok := reader.Next()
		if !ok {
			break
		}

		if event.Data == "[DONE]" {
			return
		}

		var rpcResp JSONRPCResponse
		if err := json.Unmarshal([]byte(event.Data), &rpcResp); err != nil {
			ch <- StreamEvent{Type: "error", Error: fmt.Errorf("a2a: failed to parse stream event: %w", err)}
			return
		}

		if rpcResp.Error != nil {
			ch <- StreamEvent{Type: "error", Error: &Error{Code: rpcResp.Error.Code, Message: rpcResp.Error.Message}}
			return
		}

		resultBytes, _ := json.Marshal(rpcResp.Result)

		// Try to detect event type from result fields
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(resultBytes, &probe); err != nil {
			continue
		}

		if _, ok := probe["artifact"]; ok {
			var evt TaskArtifactUpdateEvent
			if json.Unmarshal(resultBytes, &evt) == nil {
				ch <- StreamEvent{Type: "artifact", ArtifactUpdate: &evt}
			}
		} else if _, ok := probe["status"]; ok {
			if _, hasMessages := probe["messages"]; hasMessages {
				// Full task result
				var task Task
				if json.Unmarshal(resultBytes, &task) == nil {
					ch <- StreamEvent{Type: "task", Task: &task}
				}
			} else {
				var evt TaskStatusUpdateEvent
				if json.Unmarshal(resultBytes, &evt) == nil {
					ch <- StreamEvent{Type: "status", StatusUpdate: &evt}
				}
			}
		}
	}
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
