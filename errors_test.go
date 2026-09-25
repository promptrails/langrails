package langrails

import (
	"net/http"
	"testing"
	"time"
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

func TestRetryAfterFromHeader(t *testing.T) {
	cases := []struct {
		name string
		h    http.Header
		want time.Duration
	}{
		{"none", http.Header{}, 0},
		{"seconds", http.Header{"Retry-After": {"7"}}, 7 * time.Second},
		{"fractional", http.Header{"Retry-After": {"1.5"}}, 1500 * time.Millisecond},
		{"ms wins", http.Header{"Retry-After": {"7"}, "Retry-After-Ms": {"250"}}, 250 * time.Millisecond},
		{"negative", http.Header{"Retry-After": {"-3"}}, 0},
		{"garbage", http.Header{"Retry-After": {"soon"}}, 0},
		{"past date", http.Header{"Retry-After": {"Wed, 21 Oct 2015 07:28:00 GMT"}}, 0},
	}
	for _, c := range cases {
		if got := RetryAfterFromHeader(c.h); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}

	future := http.Header{"Retry-After": {time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)}}
	if got := RetryAfterFromHeader(future); got < 25*time.Second || got > 31*time.Second {
		t.Errorf("http date: got %v", got)
	}
}
