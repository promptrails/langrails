// Package httpjson posts JSON to an HTTP API and decodes the JSON reply,
// turning non-2xx responses into *langrails.APIError.
package httpjson

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/promptrails/langrails"
)

// Client posts JSON requests on behalf of one provider.
type Client struct {
	// Provider names the provider in errors.
	Provider string

	// HTTP is the client used for requests.
	HTTP *http.Client

	// Headers are sent with every request.
	Headers map[string]string

	// ErrorMessage extracts a human-readable message from an error body.
	// When nil or when it returns "", the message is "status <code>".
	ErrorMessage func(body []byte) string
}

// Post marshals in, posts it to url and decodes the response into out.
// extra headers are added for this request only.
func (c *Client) Post(ctx context.Context, url string, in, out any, extra map[string]string) error {
	body, err := json.Marshal(in)
	if err != nil {
		return fmt.Errorf("%s: failed to encode request: %w", c.Provider, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s: failed to create request: %w", c.Provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request failed: %w", c.Provider, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: failed to read response: %w", c.Provider, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := ""
		if c.ErrorMessage != nil {
			msg = c.ErrorMessage(raw)
		}
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		return &langrails.APIError{
			StatusCode: resp.StatusCode,
			Message:    msg,
			Provider:   c.Provider,
			RetryAfter: langrails.RetryAfterFromHeader(resp.Header),
		}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: failed to parse response: %w", c.Provider, err)
	}
	return nil
}

// OpenAIError reads the message from an OpenAI-style error body,
// {"error": {"message": "..."}}.
func OpenAIError(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	return e.Error.Message
}

// DetailError reads the message from a FastAPI-style error body,
// {"detail": "..."} or {"detail": [{"msg": "..."}]}, falling back to an
// OpenAI-style body.
func DetailError(body []byte) string {
	var e struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(body, &e) != nil || len(e.Detail) == 0 {
		return OpenAIError(body)
	}
	var s string
	if json.Unmarshal(e.Detail, &s) == nil {
		return s
	}
	var items []struct {
		Msg string `json:"msg"`
	}
	if json.Unmarshal(e.Detail, &items) == nil && len(items) > 0 {
		msgs := make([]string, 0, len(items))
		for _, it := range items {
			if it.Msg != "" {
				msgs = append(msgs, it.Msg)
			}
		}
		return strings.Join(msgs, "; ")
	}
	return string(e.Detail)
}
