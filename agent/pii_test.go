package agent

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func TestPII_RedactsInput(t *testing.T) {
	mw := NewPIIRedaction()
	state := &State{Request: &langrails.CompletionRequest{
		Messages: []langrails.Message{
			{Role: "user", Content: "email me at jane.doe@example.com or call +1 415 555 1234"},
			{Role: "user", Content: "card 4111 1111 1111 1111"},
		},
	}}

	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got0 := state.Request.Messages[0].Content
	if strings.Contains(got0, "jane.doe@example.com") {
		t.Errorf("email not redacted: %q", got0)
	}
	if !strings.Contains(got0, "[REDACTED_EMAIL]") {
		t.Errorf("expected email placeholder: %q", got0)
	}
	if !strings.Contains(got0, "[REDACTED_PHONE]") {
		t.Errorf("expected phone placeholder: %q", got0)
	}

	got1 := state.Request.Messages[1].Content
	if strings.Contains(got1, "4111") || !strings.Contains(got1, "[REDACTED_CARD]") {
		t.Errorf("card not redacted: %q", got1)
	}
}

func TestPII_OutputOptIn(t *testing.T) {
	// Output redaction is off by default.
	off := NewPIIRedaction()
	state := &State{Response: &langrails.CompletionResponse{Content: "reach me: a@b.com"}}
	if err := off.AfterModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(state.Response.Content, "a@b.com") {
		t.Error("output should not be redacted by default")
	}

	// With output redaction enabled.
	on := NewPIIRedaction(WithRedactOutput(true))
	state2 := &State{Response: &langrails.CompletionResponse{Content: "reach me: a@b.com"}}
	if err := on.AfterModel(context.Background(), state2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(state2.Response.Content, "a@b.com") {
		t.Errorf("output email not redacted: %q", state2.Response.Content)
	}
}

func TestPII_InputOptOut(t *testing.T) {
	mw := NewPIIRedaction(WithRedactInput(false))
	state := &State{Request: &langrails.CompletionRequest{
		Messages: []langrails.Message{{Role: "user", Content: "a@b.com"}},
	}}
	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Request.Messages[0].Content != "a@b.com" {
		t.Errorf("input should be untouched when redaction disabled, got %q", state.Request.Messages[0].Content)
	}
}

func TestPII_CustomPattern(t *testing.T) {
	ssn := regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	mw := NewPIIRedaction(WithCustomPattern(ssn, "[REDACTED_SSN]"))
	state := &State{Request: &langrails.CompletionRequest{
		Messages: []langrails.Message{{Role: "user", Content: "ssn 123-45-6789"}},
	}}
	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(state.Request.Messages[0].Content, "[REDACTED_SSN]") {
		t.Errorf("custom pattern not applied: %q", state.Request.Messages[0].Content)
	}
}

func TestPII_RedactsContentParts(t *testing.T) {
	mw := NewPIIRedaction()
	state := &State{Request: &langrails.CompletionRequest{
		Messages: []langrails.Message{{
			Role: "user",
			ContentParts: []langrails.ContentPart{
				langrails.TextPart("write to a@b.com"),
				langrails.ImageURLPart("https://example.com/x.png"),
			},
		}},
	}}
	if err := mw.BeforeModel(context.Background(), state); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(state.Request.Messages[0].ContentParts[0].Text, "a@b.com") {
		t.Error("email in content part not redacted")
	}
}
