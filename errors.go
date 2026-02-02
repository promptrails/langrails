package unillm

import "fmt"

// APIError represents an error response from an LLM provider's API.
type APIError struct {
	// StatusCode is the HTTP status code returned by the provider.
	StatusCode int

	// Message is the human-readable error message.
	Message string

	// Provider is the name of the provider that returned the error.
	Provider string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("%s: api error (status %d): %s", e.Provider, e.StatusCode, e.Message)
}

// IsAuthError returns true if the error is an authentication/authorization error (401/403).
func (e *APIError) IsAuthError() bool {
	return e.StatusCode == 401 || e.StatusCode == 403
}

// IsRateLimitError returns true if the error is a rate limit error (429).
func (e *APIError) IsRateLimitError() bool {
	return e.StatusCode == 429
}

// IsServerError returns true if the error is a server-side error (5xx).
func (e *APIError) IsServerError() bool {
	return e.StatusCode >= 500 && e.StatusCode < 600
}

// IsRetryable returns true if the request can be retried.
// Rate limit errors and server errors are considered retryable.
func (e *APIError) IsRetryable() bool {
	return e.IsRateLimitError() || e.IsServerError()
}
