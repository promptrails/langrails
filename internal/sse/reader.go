// Package sse provides a lightweight Server-Sent Events (SSE) stream reader.
package sse

import (
	"bufio"
	"io"
	"strings"
)

// Event represents a single Server-Sent Event.
type Event struct {
	// Event is the optional event type (from "event:" field).
	Event string

	// Data is the event payload (from "data:" field).
	// Multiple "data:" lines are joined with newlines.
	Data string
}

// Reader reads SSE events from an io.Reader.
type Reader struct {
	scanner *bufio.Scanner
}

// NewReader creates a new SSE reader from the given io.Reader.
// It uses a 64KB buffer with a maximum of 1MB per line.
func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &Reader{scanner: scanner}
}

// Next reads the next SSE event from the stream. It returns the event
// and true if an event was read, or a zero Event and false if the stream
// has ended. Callers should check for scanner errors after Next returns false.
func (r *Reader) Next() (Event, bool) {
	var event Event
	var dataLines []string
	hasData := false

	for r.scanner.Scan() {
		line := r.scanner.Text()

		// Empty line signals end of an event
		if line == "" {
			if hasData {
				event.Data = strings.Join(dataLines, "\n")
				return event, true
			}
			continue
		}

		if strings.HasPrefix(line, "data:") {
			data := strings.TrimPrefix(line, "data:")
			data = strings.TrimPrefix(data, " ")
			dataLines = append(dataLines, data)
			hasData = true
		} else if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
		// Ignore "id:", "retry:", and comment lines (starting with ":")
	}

	// Handle trailing event without final empty line
	if hasData {
		event.Data = strings.Join(dataLines, "\n")
		return event, true
	}

	return Event{}, false
}

// Err returns the first non-EOF error encountered by the underlying scanner.
func (r *Reader) Err() error {
	return r.scanner.Err()
}
