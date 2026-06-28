package agent

import (
	"context"
	"regexp"

	"github.com/promptrails/langrails"
)

// Built-in PII patterns. Credit-card matching runs before phone matching so
// long digit groups are not partially consumed as phone numbers.
var (
	reEmail = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	reCard  = regexp.MustCompile(`\b\d{4}[ -]?\d{4}[ -]?\d{4}[ -]?\d{1,4}\b`)
	rePhone = regexp.MustCompile(`\+?\d{1,3}[ .\-]?\(?\d{2,4}\)?[ .\-]?\d{3}[ .\-]?\d{3,4}\b`)
)

type redactPattern struct {
	re          *regexp.Regexp
	replacement string
}

// PIIRedactionMiddleware masks personally identifiable information using
// regular expressions. By default it redacts the messages sent to the
// model (BeforeModel); enable output redaction to also mask the model's
// responses (AfterModel). It is a zero-dependency, pattern-matching
// equivalent of LangChain's PII redaction middleware.
//
// Redaction covers message Content and the text of multimodal content
// parts. It does not rewrite tool-call arguments.
type PIIRedactionMiddleware struct {
	BaseMiddleware
	patterns     []redactPattern
	redactInput  bool
	redactOutput bool
}

// PIIOption configures a PIIRedactionMiddleware.
type PIIOption func(*PIIRedactionMiddleware)

// WithRedactInput controls whether outgoing messages are redacted before
// the model call. Default is true.
func WithRedactInput(on bool) PIIOption {
	return func(m *PIIRedactionMiddleware) { m.redactInput = on }
}

// WithRedactOutput controls whether the model's response content is
// redacted after the model call. Default is false.
func WithRedactOutput(on bool) PIIOption {
	return func(m *PIIRedactionMiddleware) { m.redactOutput = on }
}

// WithCustomPattern adds a custom redaction pattern. Matches of re are
// replaced with replacement. Custom patterns run after the built-in ones.
func WithCustomPattern(re *regexp.Regexp, replacement string) PIIOption {
	return func(m *PIIRedactionMiddleware) {
		m.patterns = append(m.patterns, redactPattern{re: re, replacement: replacement})
	}
}

// NewPIIRedaction creates a PIIRedactionMiddleware with built-in patterns
// for email addresses, credit-card numbers, and phone numbers.
func NewPIIRedaction(opts ...PIIOption) *PIIRedactionMiddleware {
	m := &PIIRedactionMiddleware{
		patterns: []redactPattern{
			{re: reEmail, replacement: "[REDACTED_EMAIL]"},
			{re: reCard, replacement: "[REDACTED_CARD]"},
			{re: rePhone, replacement: "[REDACTED_PHONE]"},
		},
		redactInput: true,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// BeforeModel redacts outgoing messages when input redaction is enabled.
func (m *PIIRedactionMiddleware) BeforeModel(_ context.Context, state *State) error {
	if !m.redactInput {
		return nil
	}
	for i := range state.Request.Messages {
		m.redactMessage(&state.Request.Messages[i])
	}
	return nil
}

// AfterModel redacts the response content when output redaction is enabled.
func (m *PIIRedactionMiddleware) AfterModel(_ context.Context, state *State) error {
	if !m.redactOutput || state.Response == nil {
		return nil
	}
	state.Response.Content = m.redact(state.Response.Content)
	return nil
}

func (m *PIIRedactionMiddleware) redactMessage(msg *langrails.Message) {
	msg.Content = m.redact(msg.Content)
	for j := range msg.ContentParts {
		if msg.ContentParts[j].Type == "text" {
			msg.ContentParts[j].Text = m.redact(msg.ContentParts[j].Text)
		}
	}
}

func (m *PIIRedactionMiddleware) redact(s string) string {
	for _, p := range m.patterns {
		s = p.re.ReplaceAllString(s, p.replacement)
	}
	return s
}
