package a2a

import "fmt"

// Standard JSON-RPC 2.0 error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// A2A-specific error codes.
const (
	ErrCodeTaskNotFound         = -32001
	ErrCodeTaskNotCancelable    = -32002
	ErrCodeUnsupportedOperation = -32003
	ErrCodePushNotSupported     = -32006
)

// Error represents an A2A protocol error.
type Error struct {
	Code    int
	Message string
}

// Error implements the error interface.
func (e *Error) Error() string {
	return fmt.Sprintf("a2a error %d: %s", e.Code, e.Message)
}

// ToJSONRPC converts the error to a JSONRPCError.
func (e *Error) ToJSONRPC() *JSONRPCError {
	return &JSONRPCError{Code: e.Code, Message: e.Message}
}

// Common errors.
var (
	ErrTaskNotFound      = &Error{Code: ErrCodeTaskNotFound, Message: "task not found"}
	ErrTaskNotCancelable = &Error{Code: ErrCodeTaskNotCancelable, Message: "task cannot be canceled"}
	ErrMethodNotFound    = &Error{Code: ErrCodeMethodNotFound, Message: "method not found"}
)
