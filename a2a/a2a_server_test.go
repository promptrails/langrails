package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// configurableHandler is a TaskHandler whose behavior is fully injectable,
// so each test can drive a specific server code path.
type configurableHandler struct {
	send   func(ctx context.Context, req SendMessageRequest) (*Task, error)
	stream func(ctx context.Context, req SendMessageRequest, events chan<- StreamEvent)
	get    func(ctx context.Context, taskID string) (*Task, error)
	cancel func(ctx context.Context, taskID string) (*Task, error)
}

func (h *configurableHandler) HandleMessage(ctx context.Context, req SendMessageRequest) (*Task, error) {
	return h.send(ctx, req)
}

func (h *configurableHandler) HandleMessageStream(ctx context.Context, req SendMessageRequest, events chan<- StreamEvent) {
	h.stream(ctx, req, events)
}

func (h *configurableHandler) GetTask(ctx context.Context, taskID string) (*Task, error) {
	return h.get(ctx, taskID)
}

func (h *configurableHandler) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	return h.cancel(ctx, taskID)
}

func newConfigurableServer(t *testing.T, h *configurableHandler) *httptest.Server {
	t.Helper()
	card := AgentCard{Name: "Cfg Agent", Version: ProtocolVersion}
	a2aHandler := NewHandler(card, h)
	mux := http.NewServeMux()
	mux.Handle("/a2a/", a2aHandler)
	mux.Handle("/a2a", a2aHandler)
	return httptest.NewServer(mux)
}

func postRPC(t *testing.T, url, body string) JSONRPCResponse {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}
	defer resp.Body.Close()
	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	return rpcResp
}

func TestServer_MethodNotAllowed(t *testing.T) {
	server := newConfigurableServer(t, &configurableHandler{})
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPut, server.URL+"/a2a", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", resp.StatusCode)
	}
}

func TestServer_SendMessage_InvalidParams(t *testing.T) {
	server := newConfigurableServer(t, &configurableHandler{})
	defer server.Close()

	// params is a string where an object is expected → ErrCodeInvalidParams.
	body := `{"jsonrpc":"2.0","method":"message/send","params":"not-an-object","id":1}`
	rpcResp := postRPC(t, server.URL+"/a2a", body)
	if rpcResp.Error == nil || rpcResp.Error.Code != ErrCodeInvalidParams {
		t.Errorf("expected invalid params error, got %+v", rpcResp.Error)
	}
}

// A handler returning a plain (non-*Error) error must surface as a generic
// internal error, not panic or leak the typed path.
func TestServer_HandlerError_Generic(t *testing.T) {
	server := newConfigurableServer(t, &configurableHandler{
		send: func(context.Context, SendMessageRequest) (*Task, error) {
			return nil, errors.New("boom")
		},
	})
	defer server.Close()

	body := `{"jsonrpc":"2.0","method":"message/send","params":{"message":{"role":"user","parts":[]}},"id":1}`
	rpcResp := postRPC(t, server.URL+"/a2a", body)
	if rpcResp.Error == nil || rpcResp.Error.Code != ErrCodeInternalError {
		t.Errorf("expected internal error, got %+v", rpcResp.Error)
	}
	if rpcResp.Error != nil && rpcResp.Error.Message != "boom" {
		t.Errorf("expected message 'boom', got %q", rpcResp.Error.Message)
	}
}

// A handler returning a typed *Error must preserve its code through the wire.
func TestServer_CancelTask_NotCancelable(t *testing.T) {
	server := newConfigurableServer(t, &configurableHandler{
		cancel: func(context.Context, string) (*Task, error) {
			return nil, ErrTaskNotCancelable
		},
	})
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	_, err := client.CancelTask(context.Background(), "task-1")
	var a2aErr *Error
	if !errors.As(err, &a2aErr) {
		t.Fatalf("expected *Error, got %T (%v)", err, err)
	}
	if a2aErr.Code != ErrCodeTaskNotCancelable {
		t.Errorf("expected not-cancelable code, got %d", a2aErr.Code)
	}
}

// The server should forward an error event from a streaming handler as a
// JSON-RPC error frame, which the client decodes into a StreamEvent error.
func TestServer_Stream_ErrorEvent(t *testing.T) {
	server := newConfigurableServer(t, &configurableHandler{
		stream: func(_ context.Context, _ SendMessageRequest, events chan<- StreamEvent) {
			defer close(events)
			events <- StreamEvent{Type: "error", Error: errors.New("stream failed")}
		},
	})
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	ch, err := client.StreamMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: RoleUser, Parts: []Part{NewTextPart("hi")}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var sawError bool
	for ev := range ch {
		if ev.Type == "error" {
			sawError = true
			if ev.Error == nil {
				t.Error("error event has nil Error")
			}
		}
	}
	if !sawError {
		t.Error("expected an error stream event")
	}
}

// The server should forward a full task result (status + messages) as a
// "task" stream event.
func TestServer_Stream_TaskEvent(t *testing.T) {
	server := newConfigurableServer(t, &configurableHandler{
		stream: func(_ context.Context, _ SendMessageRequest, events chan<- StreamEvent) {
			defer close(events)
			events <- StreamEvent{Type: "task", Task: &Task{
				ID:       "task-9",
				Status:   TaskStatus{State: TaskStateCompleted},
				Messages: []Message{{Role: RoleAgent, Parts: []Part{NewTextPart("done")}}},
			}}
		},
	})
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	ch, err := client.StreamMessage(context.Background(), SendMessageRequest{
		Message: Message{Role: RoleUser, Parts: []Part{NewTextPart("hi")}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var task *Task
	for ev := range ch {
		if ev.Type == "task" {
			task = ev.Task
		}
	}
	if task == nil || task.ID != "task-9" {
		t.Fatalf("expected task-9, got %+v", task)
	}
}

func TestClient_GetAgentCard_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL)
	if _, err := client.GetAgentCard(context.Background()); err == nil {
		t.Fatal("expected error on non-200 agent card response")
	}
}

func TestIsAgentCardPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/agent-card.json", true},
		{"/a2a/agent-card.json", true},
		{"/deep/nested/agent-card.json", true},
		{"/", false},
		{"/agent-card.jsonx", false},
		{"/something-else", false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			if got := isAgentCardPath(tc.path); got != tc.want {
				t.Errorf("isAgentCardPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
