package langrails

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError represents an error response from an LLM provider's API.
type APIError struct {
	// StatusCode is the HTTP status code returned by the provider.
	StatusCode int

	// Message is the human-readable error message.
	Message string

	// Provider is the name of the provider that returned the error.
	Provider string

	// RetryAfter is how long the provider asked the caller to wait before
	// retrying, from the Retry-After (or retry-after-ms) response header.
	// 0 when the provider did not say. RetryProvider honors it.
	RetryAfter time.Duration
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

// RetryAfterFromHeader reads how long a response asks the client to wait:
// the non-standard retry-after-ms header (OpenAI, Azure) when present, else
// Retry-After as delta-seconds or an HTTP date. Returns 0 when neither is
// set or parseable. Custom Provider implementations can use it to fill
// APIError.RetryAfter.
func RetryAfterFromHeader(h http.Header) time.Duration {
	if ms := strings.TrimSpace(h.Get("retry-after-ms")); ms != "" {
		if n, err := strconv.ParseFloat(ms, 64); err == nil && n > 0 {
			return time.Duration(n * float64(time.Millisecond))
		}
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil {
		if n <= 0 {
			return 0
		}
		return time.Duration(n * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
