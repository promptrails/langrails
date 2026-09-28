package langrails

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/promptrails/langrails/internal/jsonschema"
)

// ParseError reports that a structured-output response could not be decoded
// into the requested type.
type ParseError struct {
	// Content is the raw response text that failed to parse.
	Content string

	// Err is the underlying decode error.
	Err error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("langrails: parse structured output: %v", e.Err)
}

func (e *ParseError) Unwrap() error { return e.Err }

// GenerateOption configures [Generate].
type GenerateOption func(*generateConfig)

type generateConfig struct {
	retries int
}

// WithParseRetries re-asks the model up to n times when its response does
// not decode into the target type, feeding the decode error back to it.
// The default is 0 (no retries).
func WithParseRetries(n int) GenerateOption {
	return func(c *generateConfig) { c.retries = n }
}

// Generate requests structured output shaped like T and decodes it.
//
// The JSON Schema is derived from T with the same struct tags typed tools
// use (json, description, jsonschema), and set as the request's
// OutputSchema. T may be any schema-able type: a struct or map is sent as
// the top-level object, anything else (a slice, a string, a number) is
// wrapped in an object with a single "value" property, since providers
// require an object at the top level, and unwrapped again after decoding.
//
//	type Sentiment struct {
//	    Label      string  `json:"label" jsonschema:"enum=positive|negative|neutral"`
//	    Confidence float64 `json:"confidence" jsonschema:"minimum=0,maximum=1"`
//	}
//	s, resp, err := langrails.Generate[Sentiment](ctx, provider, req)
//
// req is not modified. The returned response is the last one received;
// Usage covers only that call. A response that does not decode yields a
// *ParseError (after any retries).
func Generate[T any](ctx context.Context, p Provider, req *CompletionRequest, opts ...GenerateOption) (T, *CompletionResponse, error) {
	var zero T
	cfg := generateConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	s, err := jsonschema.Reflect(reflect.TypeFor[T]())
	if err != nil {
		return zero, nil, fmt.Errorf("langrails: schema for %T: %w", zero, err)
	}
	wrapped := s.Type != "object" || s.AdditionalProperties != nil
	if wrapped {
		s = &jsonschema.Schema{
			Type:       "object",
			Properties: map[string]*jsonschema.Schema{"value": s},
			Required:   []string{"value"},
		}
	}
	schema, err := json.Marshal(s)
	if err != nil {
		return zero, nil, err
	}

	r := *req
	r.OutputSchema = schema
	r.Messages = append([]Message(nil), req.Messages...)

	for attempt := 0; ; attempt++ {
		resp, err := p.Complete(ctx, &r)
		if err != nil {
			return zero, nil, err
		}

		out, perr := decodeStructured[T](resp.Content, wrapped)
		if perr == nil {
			return out, resp, nil
		}
		if attempt >= cfg.retries {
			return zero, resp, perr
		}

		r.Messages = append(r.Messages,
			Message{Role: "assistant", Content: resp.Content},
			Message{Role: "user", Content: fmt.Sprintf(
				"Your previous response could not be parsed: %v. Respond again with only a JSON value matching the schema.", perr.Err)},
		)
	}
}

func decodeStructured[T any](content string, wrapped bool) (T, *ParseError) {
	var out T
	text := stripCodeFence(content)
	if wrapped {
		var w struct {
			Value *T `json:"value"`
		}
		if err := json.Unmarshal([]byte(text), &w); err != nil {
			return out, &ParseError{Content: content, Err: err}
		}
		if w.Value == nil {
			return out, &ParseError{Content: content, Err: fmt.Errorf(`missing "value"`)}
		}
		return *w.Value, nil
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return out, &ParseError{Content: content, Err: err}
	}
	return out, nil
}

// stripCodeFence removes a surrounding ```json … ``` fence, which some
// models add even in JSON mode.
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}
