package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/promptrails/langrails/internal/sse"
)

// sessionHeader is the streamable-HTTP session ID a server may assign on
// initialize; the client must echo it on every later request.
const sessionHeader = "Mcp-Session-Id"

// errNoResponse is returned when an SSE response stream ends without the
// response to the request that opened it.
var errNoResponse = errors.New("stream ended without a response")

type httpTransport struct {
	url     string
	headers map[string]string
	client  *http.Client
	handle  requestHandler

	nextID atomic.Int64

	mu        sync.Mutex
	sessionID string
}

func (t *httpTransport) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	body, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := t.post(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if sid := resp.Header.Get(sessionHeader); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp jsonRPCResponse
	if isEventStream(resp.Header.Get("Content-Type")) {
		rpcResp, err = t.readStream(ctx, resp.Body, id)
		if err != nil {
			return nil, err
		}
	} else {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		if err := json.Unmarshal(respBody, &rpcResp); err != nil {
			return nil, fmt.Errorf("failed to parse JSON-RPC response: %w", err)
		}
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// readStream reads an SSE response until the response to request id
// arrives. Requests the server makes in the meantime (an elicitation, a
// ping) are answered with a POST of their own; notifications are skipped.
func (t *httpTransport) readStream(ctx context.Context, body io.Reader, id int64) (jsonRPCResponse, error) {
	r := sse.NewReader(body)
	for {
		ev, ok := r.Next()
		if !ok {
			if err := r.Err(); err != nil {
				return jsonRPCResponse{}, fmt.Errorf("failed to read event stream: %w", err)
			}
			return jsonRPCResponse{}, errNoResponse
		}
		var msg incoming
		if err := json.Unmarshal([]byte(ev.Data), &msg); err != nil {
			continue
		}
		switch {
		case msg.Method != "" && len(msg.ID) > 0:
			result, rpcErr := t.handle(ctx, msg.Method, msg.Params)
			if err := t.send(ctx, reply(msg.ID, result, rpcErr)); err != nil {
				return jsonRPCResponse{}, fmt.Errorf("failed to answer %s: %w", msg.Method, err)
			}
		case msg.Method != "":
			// Notification (progress, logging); nothing to do.
		default:
			var got int64
			if json.Unmarshal(msg.ID, &got) == nil && got == id {
				return jsonRPCResponse{ID: got, Result: msg.Result, Error: msg.Error}, nil
			}
		}
	}
}

func (t *httpTransport) notify(ctx context.Context, method string, params interface{}) error {
	return t.send(ctx, jsonRPCNotification{JSONRPC: "2.0", Method: method, Params: params})
}

// send posts a message that has no response of its own: a notification or
// the client's answer to a server request.
func (t *httpTransport) send(ctx context.Context, msg any) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	resp, err := t.post(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}
	return nil
}

func (t *httpTransport) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// Streamable HTTP servers answer with either JSON or an SSE stream, and
	// the spec requires the client to accept both.
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set(sessionHeader, t.sessionID)
	}
	t.mu.Unlock()

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	return resp, nil
}

func (t *httpTransport) close() error { return nil }

func isEventStream(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == "text/event-stream"
}
