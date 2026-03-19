package a2a

import "encoding/json"

// ProtocolVersion is the A2A protocol version implemented.
const ProtocolVersion = "0.3"

// TaskState represents the lifecycle state of an A2A task.
type TaskState string

const (
	TaskStateSubmitted     TaskState = "submitted"
	TaskStateWorking       TaskState = "working"
	TaskStateInputRequired TaskState = "input_required"
	TaskStateCompleted     TaskState = "completed"
	TaskStateFailed        TaskState = "failed"
	TaskStateCanceled      TaskState = "canceled"
	TaskStateRejected      TaskState = "rejected"
	TaskStateAuthRequired  TaskState = "auth_required"
)

// IsTerminal returns true if the task state is a terminal state.
func (s TaskState) IsTerminal() bool {
	return s == TaskStateCompleted || s == TaskStateFailed ||
		s == TaskStateCanceled || s == TaskStateRejected
}

// Role represents the sender of a message.
type Role string

const (
	RoleUser  Role = "user"
	RoleAgent Role = "agent"
)

// Message represents a conversational message in the A2A protocol.
type Message struct {
	Role             Role     `json:"role"`
	Parts            []Part   `json:"parts"`
	CreatedAt        string   `json:"created_at,omitempty"`
	ReferenceTaskIDs []string `json:"reference_task_ids,omitempty"`
}

// Part represents a content block within a message. It is polymorphic,
// discriminated by the Type field.
type Part struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	MimeType string         `json:"mime_type,omitempty"`
	File     *FileContent   `json:"file,omitempty"`
}

// FileContent represents a file attachment.
type FileContent struct {
	Name      string `json:"name,omitempty"`
	URI       string `json:"uri,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Bytes     string `json:"bytes,omitempty"`
}

// NewTextPart creates a text content part.
func NewTextPart(text string) Part {
	return Part{Type: "text", Text: text}
}

// NewDataPart creates a structured data content part.
func NewDataPart(data map[string]any) Part {
	return Part{Type: "data", Data: data}
}

// Task represents an A2A task with its full state.
type Task struct {
	ID        string         `json:"id"`
	ContextID string         `json:"context_id,omitempty"`
	Status    TaskStatus     `json:"status"`
	Messages  []Message      `json:"messages,omitempty"`
	Artifacts []Artifact     `json:"artifacts,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
	UpdatedAt string         `json:"updated_at,omitempty"`
}

// TaskStatus represents the current status of a task.
type TaskStatus struct {
	State     TaskState `json:"state"`
	Message   string    `json:"message,omitempty"`
	Timestamp string    `json:"timestamp,omitempty"`
}

// Artifact represents a generated output from an agent.
type Artifact struct {
	ID        string         `json:"id,omitempty"`
	Parts     []Part         `json:"parts"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at,omitempty"`
}

// AgentCard describes an A2A agent's capabilities and endpoints.
type AgentCard struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	URL                string            `json:"url"`
	Provider           *AgentProvider    `json:"provider,omitempty"`
	Version            string            `json:"version"`
	Capabilities       AgentCapabilities `json:"capabilities"`
	Skills             []AgentSkill      `json:"skills,omitempty"`
	SecuritySchemes    map[string]any    `json:"securitySchemes,omitempty"`
	Security           []map[string]any  `json:"security,omitempty"`
	DefaultInputModes  []string          `json:"defaultInputModes,omitempty"`
	DefaultOutputModes []string          `json:"defaultOutputModes,omitempty"`
}

// AgentProvider describes the organization providing the agent.
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// AgentCapabilities describes what an agent supports.
type AgentCapabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
}

// AgentSkill describes a specific capability of an agent.
type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

// SendMessageRequest is the params for message/send and message/stream.
type SendMessageRequest struct {
	Message       Message                   `json:"message"`
	Configuration *SendMessageConfiguration `json:"configuration,omitempty"`
	Metadata      map[string]any            `json:"metadata,omitempty"`
}

// SendMessageConfiguration configures how a message is processed.
type SendMessageConfiguration struct {
	AcceptedOutputModes []string `json:"acceptedOutputModes,omitempty"`
	HistoryLength       *int     `json:"historyLength,omitempty"`
	Blocking            *bool    `json:"blocking,omitempty"`
}

// GetTaskRequest is the params for tasks/get.
type GetTaskRequest struct {
	ID            string `json:"id"`
	HistoryLength *int   `json:"historyLength,omitempty"`
}

// CancelTaskRequest is the params for tasks/cancel.
type CancelTaskRequest struct {
	ID string `json:"id"`
}

// TaskStatusUpdateEvent is emitted during streaming for status changes.
type TaskStatusUpdateEvent struct {
	TaskID    string     `json:"task_id"`
	ContextID string     `json:"context_id,omitempty"`
	Status    TaskStatus `json:"status"`
}

// TaskArtifactUpdateEvent is emitted during streaming for new artifacts.
type TaskArtifactUpdateEvent struct {
	TaskID   string   `json:"task_id"`
	Artifact Artifact `json:"artifact"`
}

// JSON-RPC types

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      any             `json:"id"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
	ID      any           `json:"id"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}
