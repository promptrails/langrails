package llmrails

import (
	"testing"
)

func TestAPIError_Error(t *testing.T) {
	err := &APIError{StatusCode: 401, Message: "invalid key", Provider: "openai"}
	expected := "openai: api error (status 401): invalid key"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestAPIError_IsAuthError(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{401, true},
		{403, true},
		{400, false},
		{429, false},
		{500, false},
	}
	for _, tt := range tests {
		err := &APIError{StatusCode: tt.status}
		if got := err.IsAuthError(); got != tt.want {
			t.Errorf("status %d: IsAuthError() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestAPIError_IsRateLimitError(t *testing.T) {
	if !(&APIError{StatusCode: 429}).IsRateLimitError() {
		t.Error("429 should be rate limit error")
	}
	if (&APIError{StatusCode: 500}).IsRateLimitError() {
		t.Error("500 should not be rate limit error")
	}
}

func TestAPIError_IsServerError(t *testing.T) {
	tests := []struct {
		status int
		want   bool
	}{
		{500, true},
		{502, true},
		{503, true},
		{599, true},
		{400, false},
		{429, false},
	}
	for _, tt := range tests {
		err := &APIError{StatusCode: tt.status}
		if got := err.IsServerError(); got != tt.want {
			t.Errorf("status %d: IsServerError() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestAPIError_IsRetryable(t *testing.T) {
	if !(&APIError{StatusCode: 429}).IsRetryable() {
		t.Error("429 should be retryable")
	}
	if !(&APIError{StatusCode: 500}).IsRetryable() {
		t.Error("500 should be retryable")
	}
	if (&APIError{StatusCode: 401}).IsRetryable() {
		t.Error("401 should not be retryable")
	}
}
