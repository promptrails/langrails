package eventstream

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

// encodeFrame builds a single event-stream frame with the given string headers
// and payload. CRC fields are left zero (the decoder does not validate them).
func encodeFrame(headers map[string]string, payload []byte) []byte {
	var hb bytes.Buffer
	for name, value := range headers {
		hb.WriteByte(byte(len(name)))
		hb.WriteString(name)
		hb.WriteByte(headerStrType)
		var vlen [2]byte
		binary.BigEndian.PutUint16(vlen[:], uint16(len(value)))
		hb.Write(vlen[:])
		hb.WriteString(value)
	}
	headerBytes := hb.Bytes()

	total := preludeLen + len(headerBytes) + len(payload) + messageCRCLen
	buf := make([]byte, 0, total)
	var u32 [4]byte

	binary.BigEndian.PutUint32(u32[:], uint32(total))
	buf = append(buf, u32[:]...)
	binary.BigEndian.PutUint32(u32[:], uint32(len(headerBytes)))
	buf = append(buf, u32[:]...)
	buf = append(buf, 0, 0, 0, 0) // prelude CRC (ignored)
	buf = append(buf, headerBytes...)
	buf = append(buf, payload...)
	buf = append(buf, 0, 0, 0, 0) // message CRC (ignored)
	return buf
}

func TestDecoder_SingleFrame(t *testing.T) {
	frame := encodeFrame(
		map[string]string{":message-type": "event", ":event-type": "contentBlockDelta"},
		[]byte(`{"delta":{"text":"hi"}}`),
	)
	dec := NewDecoder(bytes.NewReader(frame))

	msg, err := dec.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if msg.EventType() != "contentBlockDelta" {
		t.Errorf("event-type = %q", msg.EventType())
	}
	if msg.MessageType() != "event" {
		t.Errorf("message-type = %q", msg.MessageType())
	}
	if string(msg.Payload) != `{"delta":{"text":"hi"}}` {
		t.Errorf("payload = %q", msg.Payload)
	}

	if _, err := dec.Next(); err != io.EOF {
		t.Errorf("expected EOF after last frame, got %v", err)
	}
}

func TestDecoder_MultipleFrames(t *testing.T) {
	var stream bytes.Buffer
	stream.Write(encodeFrame(map[string]string{":event-type": "messageStart"}, []byte(`{"role":"assistant"}`)))
	stream.Write(encodeFrame(map[string]string{":event-type": "contentBlockDelta"}, []byte(`{"delta":{"text":"a"}}`)))
	stream.Write(encodeFrame(map[string]string{":event-type": "messageStop"}, []byte(`{"stopReason":"end_turn"}`)))

	dec := NewDecoder(&stream)
	var got []string
	for {
		msg, err := dec.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got = append(got, msg.EventType())
	}

	want := []string{"messageStart", "contentBlockDelta", "messageStop"}
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("frame %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDecoder_SkipsNonStringHeaders(t *testing.T) {
	// Hand-build a frame with a boolean-true header (type 0, no value) followed
	// by a string header, to exercise skipHeaderValue.
	var hb bytes.Buffer
	hb.WriteByte(byte(len(":flag")))
	hb.WriteString(":flag")
	hb.WriteByte(0) // bool true, no value bytes
	hb.WriteByte(byte(len(":event-type")))
	hb.WriteString(":event-type")
	hb.WriteByte(headerStrType)
	var vlen [2]byte
	binary.BigEndian.PutUint16(vlen[:], uint16(len("metadata")))
	hb.Write(vlen[:])
	hb.WriteString("metadata")
	headerBytes := hb.Bytes()

	payload := []byte(`{"usage":{"totalTokens":3}}`)
	total := preludeLen + len(headerBytes) + len(payload) + messageCRCLen
	var buf bytes.Buffer
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(total))
	buf.Write(u32[:])
	binary.BigEndian.PutUint32(u32[:], uint32(len(headerBytes)))
	buf.Write(u32[:])
	buf.Write([]byte{0, 0, 0, 0})
	buf.Write(headerBytes)
	buf.Write(payload)
	buf.Write([]byte{0, 0, 0, 0})

	dec := NewDecoder(&buf)
	msg, err := dec.Next()
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if msg.EventType() != "metadata" {
		t.Errorf("event-type = %q, want metadata", msg.EventType())
	}
}

func TestDecoder_InvalidLength(t *testing.T) {
	bad := []byte{0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0} // total len = 1, too small
	dec := NewDecoder(bytes.NewReader(bad))
	if _, err := dec.Next(); err == nil {
		t.Error("expected error for invalid frame length")
	}
}
