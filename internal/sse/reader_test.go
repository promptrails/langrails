package sse

import (
	"strings"
	"testing"
)

func TestReader_SingleEvent(t *testing.T) {
	input := "data: hello world\n\n"
	r := NewReader(strings.NewReader(input))

	event, ok := r.Next()
	if !ok {
		t.Fatal("expected event, got none")
	}
	if event.Data != "hello world" {
		t.Errorf("expected data %q, got %q", "hello world", event.Data)
	}

	_, ok = r.Next()
	if ok {
		t.Fatal("expected no more events")
	}
}

func TestReader_MultipleEvents(t *testing.T) {
	input := "data: first\n\ndata: second\n\n"
	r := NewReader(strings.NewReader(input))

	event1, ok := r.Next()
	if !ok {
		t.Fatal("expected first event")
	}
	if event1.Data != "first" {
		t.Errorf("expected %q, got %q", "first", event1.Data)
	}

	event2, ok := r.Next()
	if !ok {
		t.Fatal("expected second event")
	}
	if event2.Data != "second" {
		t.Errorf("expected %q, got %q", "second", event2.Data)
	}
}

func TestReader_NamedEvent(t *testing.T) {
	input := "event: status\ndata: {\"status\":\"running\"}\n\n"
	r := NewReader(strings.NewReader(input))

	event, ok := r.Next()
	if !ok {
		t.Fatal("expected event")
	}
	if event.Event != "status" {
		t.Errorf("expected event type %q, got %q", "status", event.Event)
	}
	if event.Data != `{"status":"running"}` {
		t.Errorf("unexpected data: %s", event.Data)
	}
}

func TestReader_MultiLineData(t *testing.T) {
	input := "data: line1\ndata: line2\ndata: line3\n\n"
	r := NewReader(strings.NewReader(input))

	event, ok := r.Next()
	if !ok {
		t.Fatal("expected event")
	}
	if event.Data != "line1\nline2\nline3" {
		t.Errorf("expected multiline data, got %q", event.Data)
	}
}

func TestReader_DoneSignal(t *testing.T) {
	input := "data: {\"content\":\"hi\"}\n\ndata: [DONE]\n\n"
	r := NewReader(strings.NewReader(input))

	event1, ok := r.Next()
	if !ok {
		t.Fatal("expected first event")
	}
	if event1.Data != `{"content":"hi"}` {
		t.Errorf("unexpected data: %s", event1.Data)
	}

	event2, ok := r.Next()
	if !ok {
		t.Fatal("expected DONE event")
	}
	if event2.Data != "[DONE]" {
		t.Errorf("expected [DONE], got %q", event2.Data)
	}
}

func TestReader_IgnoresComments(t *testing.T) {
	input := ": this is a comment\ndata: hello\n\n"
	r := NewReader(strings.NewReader(input))

	event, ok := r.Next()
	if !ok {
		t.Fatal("expected event")
	}
	if event.Data != "hello" {
		t.Errorf("expected %q, got %q", "hello", event.Data)
	}
}

func TestReader_EmptyStream(t *testing.T) {
	r := NewReader(strings.NewReader(""))
	_, ok := r.Next()
	if ok {
		t.Fatal("expected no events from empty stream")
	}
}

func TestReader_NoTrailingNewline(t *testing.T) {
	input := "data: trailing"
	r := NewReader(strings.NewReader(input))

	event, ok := r.Next()
	if !ok {
		t.Fatal("expected event even without trailing newline")
	}
	if event.Data != "trailing" {
		t.Errorf("expected %q, got %q", "trailing", event.Data)
	}
}

func TestReader_DataWithoutSpace(t *testing.T) {
	input := "data:nospace\n\n"
	r := NewReader(strings.NewReader(input))

	event, ok := r.Next()
	if !ok {
		t.Fatal("expected event")
	}
	if event.Data != "nospace" {
		t.Errorf("expected %q, got %q", "nospace", event.Data)
	}
}

func BenchmarkReader(b *testing.B) {
	// Simulate a typical streaming response with 100 chunks
	var sb strings.Builder
	for i := 0; i < 100; i++ {
		sb.WriteString("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
	}
	sb.WriteString("data: [DONE]\n\n")
	input := sb.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := NewReader(strings.NewReader(input))
		for {
			_, ok := r.Next()
			if !ok {
				break
			}
		}
	}
}
