package a2a

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Mock task handler for server tests
type mockTaskHandler struct {
	handleMessageFunc func(ctx context.Context, req SendMessageRequest) (*Task, error)
	getTaskFunc       func(ctx context.Context, taskID string) (*Task, error)
	cancelTaskFunc    func(ctx context.Context, taskID string) (*Task, error)
}

func (m *mockTaskHandler) HandleMessage(ctx context.Context, req SendMessageRequest) (*Task, error) {
	return m.handleMessageFunc(ctx, req)
}

func (m *mockTaskHandler) HandleMessageStream(_ context.Context, req SendMessageRequest, events chan<- StreamEvent) {
	defer close(events)
	events <- StreamEvent{
		Type:         "status",
		StatusUpdate: &TaskStatusUpdateEvent{TaskID: "task-1", Status: TaskStatus{State: TaskStateWorking}},
	}
	events <- StreamEvent{
		Type:           "artifact",
		ArtifactUpdate: &TaskArtifactUpdateEvent{TaskID: "task-1", Artifact: Artifact{Parts: []Part{NewTextPart("Hello!")}}},
	}
	events <- StreamEvent{
		Type:         "status",
		StatusUpdate: &TaskStatusUpdateEvent{TaskID: "task-1", Status: TaskStatus{State: TaskStateCompleted}},
	}
}

func (m *mockTaskHandler) GetTask(ctx context.Context, taskID string) (*Task, error) {
	return m.getTaskFunc(ctx, taskID)
}

func (m *mockTaskHandler) CancelTask(ctx context.Context, taskID string) (*Task, error) {
	return m.cancelTaskFunc(ctx, taskID)
}

func newTestA2AServer(t *testing.T) (*httptest.Server, *mockTaskHandler) {
	t.Helper()

	handler := &mockTaskHandler{
		handleMessageFunc: func(_ context.Context, req SendMessageRequest) (*Task, error) {
			text := ""
			for _, p := range req.Message.Parts {
				if p.Type == "text" {
					text += p.Text
				}
			}
			return &Task{
				ID:     "task-1",
				Status: TaskStatus{State: TaskStateCompleted},
				Messages: []Message{
					req.Message,
					{Role: RoleAgent, Parts: []Part{NewTextPart("Reply to: " + text)}},
				},
				Artifacts: []Artifact{{Parts: []Part{NewTextPart("Reply to: " + text)}}},
			}, nil
		},
		getTaskFunc: func(_ context.Context, taskID string) (*Task, error) {
			if taskID == "not-found" {
				return nil, ErrTaskNotFound
			}
			return &Task{
				ID:     taskID,
				Status: TaskStatus{State: TaskStateCompleted},
			}, nil
		},
		cancelTaskFunc: func(_ context.Context, taskID string) (*Task, error) {
			return &Task{
				ID:     taskID,
				Status: TaskStatus{State: TaskStateCanceled},
			}, nil
		},
	}

	card := AgentCard{
		Name:        "Test Agent",
		Description: "A test agent",
		Version:     ProtocolVersion,
		Capabilities: AgentCapabilities{
			Streaming: true,
		},
		Skills: []AgentSkill{
			{ID: "echo", Name: "Echo", Description: "Echoes input"},
		},
	}

	a2aHandler := NewHandler(card, handler)

	mux := http.NewServeMux()
	mux.Handle("/a2a/", a2aHandler)
	mux.Handle("/a2a", a2aHandler)

	return httptest.NewServer(mux), handler
}

func TestClient_GetAgentCard(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	card, err := client.GetAgentCard(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if card.Name != "Test Agent" {
		t.Errorf("expected 'Test Agent', got %q", card.Name)
	}
	if card.Version != ProtocolVersion {
		t.Errorf("expected version %q, got %q", ProtocolVersion, card.Version)
	}
	if len(card.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(card.Skills))
	}
}

func TestClient_SendMessage(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	task, err := client.SendMessage(context.Background(), SendMessageRequest{
		Message: Message{
			Role:  RoleUser,
			Parts: []Part{NewTextPart("Hello!")},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != "task-1" {
		t.Errorf("expected task ID 'task-1', got %q", task.ID)
	}
	if task.Status.State != TaskStateCompleted {
		t.Errorf("expected completed, got %q", task.Status.State)
	}
	if len(task.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(task.Messages))
	}
}

func TestClient_GetTask(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	task, err := client.GetTask(context.Background(), "task-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID != "task-123" {
		t.Errorf("expected task ID 'task-123', got %q", task.ID)
	}
}

func TestClient_GetTask_NotFound(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	_, err := client.GetTask(context.Background(), "not-found")
	if err == nil {
		t.Fatal("expected error")
	}
	a2aErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected Error, got %T", err)
	}
	if a2aErr.Code != ErrCodeTaskNotFound {
		t.Errorf("expected task not found error code, got %d", a2aErr.Code)
	}
}

func TestClient_CancelTask(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	task, err := client.CancelTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status.State != TaskStateCanceled {
		t.Errorf("expected canceled, got %q", task.Status.State)
	}
}

func TestClient_StreamMessage(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	ch, err := client.StreamMessage(context.Background(), SendMessageRequest{
		Message: Message{
			Role:  RoleUser,
			Parts: []Part{NewTextPart("Hello!")},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}

	hasStatus := false
	hasArtifact := false
	for _, e := range events {
		if e.Type == "status" {
			hasStatus = true
		}
		if e.Type == "artifact" {
			hasArtifact = true
		}
	}
	if !hasStatus {
		t.Error("expected status event")
	}
	if !hasArtifact {
		t.Error("expected artifact event")
	}
}

func TestClient_WithOptions(t *testing.T) {
	c := NewClient("http://example.com",
		WithBearerToken("token"),
		WithAPIKey("key"),
		WithHTTPClient(&http.Client{}),
	)
	if c.headers["Authorization"] != "Bearer token" {
		t.Error("expected bearer token header")
	}
	if c.headers["X-API-Key"] != "key" {
		t.Error("expected API key header")
	}
}

func TestServer_AgentCard(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	resp, err := http.Get(server.URL + "/a2a/agent-card.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("failed to parse agent card: %v", err)
	}
	if card.Name != "Test Agent" {
		t.Errorf("expected 'Test Agent', got %q", card.Name)
	}
}

func TestServer_MethodNotFound(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	client := NewClient(server.URL + "/a2a")
	_, err := client.call(context.Background(), "nonexistent/method", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTaskState_IsTerminal(t *testing.T) {
	terminals := []TaskState{TaskStateCompleted, TaskStateFailed, TaskStateCanceled, TaskStateRejected}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Errorf("expected %q to be terminal", s)
		}
	}

	nonTerminals := []TaskState{TaskStateSubmitted, TaskStateWorking, TaskStateInputRequired, TaskStateAuthRequired}
	for _, s := range nonTerminals {
		if s.IsTerminal() {
			t.Errorf("expected %q to be non-terminal", s)
		}
	}
}

func TestNewTextPart(t *testing.T) {
	p := NewTextPart("hello")
	if p.Type != "text" || p.Text != "hello" {
		t.Errorf("unexpected part: %+v", p)
	}
}

func TestNewDataPart(t *testing.T) {
	p := NewDataPart(map[string]any{"key": "value"})
	if p.Type != "data" || p.Data["key"] != "value" {
		t.Errorf("unexpected part: %+v", p)
	}
}

func TestError(t *testing.T) {
	err := &Error{Code: -32001, Message: "task not found"}
	expected := "a2a error -32001: task not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}

	rpcErr := err.ToJSONRPC()
	if rpcErr.Code != -32001 {
		t.Errorf("expected code -32001, got %d", rpcErr.Code)
	}
}

func TestServer_InvalidJSON(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	resp, err := http.Post(server.URL+"/a2a", "application/json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var rpcResp JSONRPCResponse
	_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
	if rpcResp.Error == nil {
		t.Fatal("expected error response")
	}
}

func TestServer_InvalidVersion(t *testing.T) {
	server, _ := newTestA2AServer(t)
	defer server.Close()

	body := `{"jsonrpc":"1.0","method":"tasks/get","params":{"id":"test"},"id":1}`
	resp, err := http.Post(server.URL+"/a2a", "application/json",
		strings.NewReader(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	var rpcResp JSONRPCResponse
	_ = json.NewDecoder(resp.Body).Decode(&rpcResp)
	if rpcResp.Error == nil || rpcResp.Error.Code != ErrCodeInvalidRequest {
		t.Error("expected invalid request error")
	}
}
