// Package eventstream decodes the binary vnd.amazon.eventstream framing used by
// AWS streaming APIs (such as Bedrock's converse-stream), using only the
// standard library.
//
// Each frame is laid out as:
//
//	[4 bytes total length][4 bytes headers length][4 bytes prelude CRC]
//	[headers][payload][4 bytes message CRC]
//
// Headers are a sequence of: [1 byte name len][name][1 byte value type][value].
// We only need the string-typed headers (":event-type", ":message-type",
// ":exception-type"), so other value types are skipped. CRCs are not validated.
//
// See https://docs.aws.amazon.com/transcribe/latest/dg/streaming-setting-up.html
package eventstream

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Message is a single decoded event-stream frame.
type Message struct {
	// Headers maps header names (e.g. ":event-type") to their string values.
	// Only string-typed headers are captured.
	Headers map[string]string

	// Payload is the raw frame payload (typically JSON).
	Payload []byte
}

// EventType returns the value of the ":event-type" header (e.g. "contentBlockDelta").
func (m Message) EventType() string { return m.Headers[":event-type"] }

// MessageType returns the value of the ":message-type" header
// ("event" for normal events, "exception" for errors).
func (m Message) MessageType() string { return m.Headers[":message-type"] }

const (
	preludeLen     = 12 // total len (4) + headers len (4) + prelude CRC (4)
	messageCRCLen  = 4
	headerStrType  = 7
	maxMessageSize = 24 * 1024 * 1024 // 24MB safety cap
)

// Decoder reads event-stream frames sequentially from an io.Reader.
type Decoder struct {
	r io.Reader
}

// NewDecoder creates a Decoder that reads frames from r.
func NewDecoder(r io.Reader) *Decoder { return &Decoder{r: r} }

// Next reads and decodes the next frame. It returns io.EOF when the stream ends
// cleanly between frames.
func (d *Decoder) Next() (Message, error) {
	prelude := make([]byte, preludeLen)
	if _, err := io.ReadFull(d.r, prelude); err != nil {
		// io.EOF here means a clean end between frames; surface it as-is.
		return Message{}, err
	}

	totalLen := binary.BigEndian.Uint32(prelude[0:4])
	headersLen := binary.BigEndian.Uint32(prelude[4:8])

	if totalLen < preludeLen+messageCRCLen || totalLen > maxMessageSize {
		return Message{}, fmt.Errorf("eventstream: invalid frame length %d", totalLen)
	}
	if headersLen > totalLen-preludeLen-messageCRCLen {
		return Message{}, fmt.Errorf("eventstream: invalid headers length %d", headersLen)
	}

	// Read the remainder of the frame (headers + payload + message CRC).
	rest := make([]byte, totalLen-preludeLen)
	if _, err := io.ReadFull(d.r, rest); err != nil {
		return Message{}, fmt.Errorf("eventstream: short frame: %w", err)
	}

	headerBytes := rest[:headersLen]
	payload := rest[headersLen : len(rest)-messageCRCLen]

	headers, err := parseHeaders(headerBytes)
	if err != nil {
		return Message{}, err
	}

	// Copy the payload so callers can retain it after the next read.
	out := make([]byte, len(payload))
	copy(out, payload)

	return Message{Headers: headers, Payload: out}, nil
}

func parseHeaders(b []byte) (map[string]string, error) {
	headers := make(map[string]string)
	for i := 0; i < len(b); {
		if i+1 > len(b) {
			return nil, fmt.Errorf("eventstream: truncated header name length")
		}
		nameLen := int(b[i])
		i++
		if i+nameLen > len(b) {
			return nil, fmt.Errorf("eventstream: truncated header name")
		}
		name := string(b[i : i+nameLen])
		i += nameLen

		if i+1 > len(b) {
			return nil, fmt.Errorf("eventstream: truncated header type")
		}
		valueType := b[i]
		i++

		switch valueType {
		case headerStrType:
			if i+2 > len(b) {
				return nil, fmt.Errorf("eventstream: truncated string header length")
			}
			vlen := int(binary.BigEndian.Uint16(b[i : i+2]))
			i += 2
			if i+vlen > len(b) {
				return nil, fmt.Errorf("eventstream: truncated string header value")
			}
			headers[name] = string(b[i : i+vlen])
			i += vlen
		default:
			// Skip non-string header values; their widths are fixed or
			// length-prefixed. We only consume what we can determine safely.
			n, err := skipHeaderValue(valueType, b[i:])
			if err != nil {
				return nil, err
			}
			i += n
		}
	}
	return headers, nil
}

// skipHeaderValue advances past a non-string header value, returning the number
// of bytes consumed.
func skipHeaderValue(valueType byte, b []byte) (int, error) {
	switch valueType {
	case 0, 1: // bool true / false — no value bytes
		return 0, nil
	case 2: // byte
		return fixed(b, 1)
	case 3: // short
		return fixed(b, 2)
	case 4: // integer
		return fixed(b, 4)
	case 5: // long
		return fixed(b, 8)
	case 6: // byte array — 2-byte length prefix
		if len(b) < 2 {
			return 0, fmt.Errorf("eventstream: truncated bytearray header")
		}
		n := int(binary.BigEndian.Uint16(b[:2]))
		return fixed(b, 2+n)
	case 8: // timestamp (8 bytes)
		return fixed(b, 8)
	case 9: // uuid (16 bytes)
		return fixed(b, 16)
	default:
		return 0, fmt.Errorf("eventstream: unknown header value type %d", valueType)
	}
}

func fixed(b []byte, n int) (int, error) {
	if len(b) < n {
		return 0, fmt.Errorf("eventstream: truncated header value")
	}
	return n, nil
}
