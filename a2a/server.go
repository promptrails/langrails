package a2a

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// TaskHandler is the interface that users implement to handle A2A tasks.
// The server handles protocol details (JSON-RPC, SSE) and delegates
// business logic to this handler.
type TaskHandler interface {
	// HandleMessage processes a message and returns the resulting task.
	// This is called for message/send requests.
	HandleMessage(ctx context.Context, req SendMessageRequest) (*Task, error)

	// HandleMessageStream processes a message with streaming.
	// Implementations should send events to the channel and close it when done.
	// This is called for message/stream requests.
	HandleMessageStream(ctx context.Context, req SendMessageRequest, events chan<- StreamEvent)

	// GetTask retrieves a task by ID.
	GetTask(ctx context.Context, taskID string) (*Task, error)

	// CancelTask cancels a running task.
	CancelTask(ctx context.Context, taskID string) (*Task, error)
}

// Handler is an http.Handler that serves the A2A protocol.
// It handles agent card discovery and JSON-RPC dispatch.
type Handler struct {
	card        AgentCard
	taskHandler TaskHandler
}

// NewHandler creates a new A2A HTTP handler.
//
//	handler := a2a.NewHandler(card, &myTaskHandler{})
//	http.Handle("/a2a/", handler)
//
// The handler serves:
//   - GET  /agent-card.json  → Agent card discovery
//   - POST /                 → JSON-RPC 2.0 dispatch
func NewHandler(card AgentCard, handler TaskHandler) *Handler {
	return &Handler{card: card, taskHandler: handler}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Agent card discovery
	if r.Method == http.MethodGet && isAgentCardPath(r.URL.Path) {
		h.serveAgentCard(w)
		return
	}

	// JSON-RPC dispatch
	if r.Method == http.MethodPost {
		h.dispatch(w, r)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (h *Handler) serveAgentCard(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.card)
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONRPCError(w, nil, ErrCodeParseError, "failed to read request")
		return
	}

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSONRPCError(w, nil, ErrCodeParseError, "invalid JSON")
		return
	}

	if req.JSONRPC != "2.0" {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidRequest, "jsonrpc must be 2.0")
		return
	}

	switch req.Method {
	case "message/send":
		h.handleSendMessage(w, r, req)
	case "message/stream":
		h.handleStreamMessage(w, r, req)
	case "tasks/get":
		h.handleGetTask(w, req)
	case "tasks/cancel":
		h.handleCancelTask(w, req)
	default:
		writeJSONRPCError(w, req.ID, ErrCodeMethodNotFound, fmt.Sprintf("unknown method: %s", req.Method))
	}
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request, req JSONRPCRequest) {
	var params SendMessageRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params")
		return
	}

	task, err := h.taskHandler.HandleMessage(r.Context(), params)
	if err != nil {
		writeError(w, req.ID, err)
		return
	}

	writeJSONRPCResult(w, req.ID, task)
}

func (h *Handler) handleStreamMessage(w http.ResponseWriter, r *http.Request, req JSONRPCRequest) {
	var params SendMessageRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONRPCError(w, req.ID, ErrCodeInternalError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	events := make(chan StreamEvent, 64)
	go h.taskHandler.HandleMessageStream(r.Context(), params, events)

	for event := range events {
		var result any
		switch event.Type {
		case "status":
			result = event.StatusUpdate
		case "artifact":
			result = event.ArtifactUpdate
		case "task":
			result = event.Task
		case "error":
			errResp := JSONRPCResponse{
				JSONRPC: "2.0",
				Error:   &JSONRPCError{Code: ErrCodeInternalError, Message: event.Error.Error()},
				ID:      req.ID,
			}
			data, _ := json.Marshal(errResp)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			continue
		}

		rpcResp := JSONRPCResponse{JSONRPC: "2.0", Result: result, ID: req.ID}
		data, _ := json.Marshal(rpcResp)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (h *Handler) handleGetTask(w http.ResponseWriter, req JSONRPCRequest) {
	var params GetTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params")
		return
	}

	task, err := h.taskHandler.GetTask(context.Background(), params.ID)
	if err != nil {
		writeError(w, req.ID, err)
		return
	}

	writeJSONRPCResult(w, req.ID, task)
}

func (h *Handler) handleCancelTask(w http.ResponseWriter, req JSONRPCRequest) {
	var params CancelTaskRequest
	if err := json.Unmarshal(req.Params, &params); err != nil {
		writeJSONRPCError(w, req.ID, ErrCodeInvalidParams, "invalid params")
		return
	}

	task, err := h.taskHandler.CancelTask(context.Background(), params.ID)
	if err != nil {
		writeError(w, req.ID, err)
		return
	}

	writeJSONRPCResult(w, req.ID, task)
}

func writeJSONRPCResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	})
}

func writeJSONRPCError(w http.ResponseWriter, id any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		Error:   &JSONRPCError{Code: code, Message: message},
		ID:      id,
	})
}

func writeError(w http.ResponseWriter, id any, err error) {
	if a2aErr, ok := err.(*Error); ok {
		writeJSONRPCError(w, id, a2aErr.Code, a2aErr.Message)
		return
	}
	writeJSONRPCError(w, id, ErrCodeInternalError, err.Error())
}

func isAgentCardPath(path string) bool {
	return path == "/agent-card.json" ||
		len(path) > 16 && path[len(path)-16:] == "/agent-card.json"
}
