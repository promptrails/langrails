package mediautil

import (
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func reqWith(parts ...langrails.ContentPart) *langrails.CompletionRequest {
	return &langrails.CompletionRequest{Messages: []langrails.Message{
		{Role: "user", Content: "plain"},
		{Role: "user", ContentParts: parts},
	}}
}

func TestCheckParts(t *testing.T) {
	all := Support{Audio: true, DocumentData: true, DocumentURL: true}
	wav := Support{Audio: true, AudioFormats: []string{"audio/wav"}}

	cases := []struct {
		name        string
		part        langrails.ContentPart
		support     Support
		unsupported bool // wraps ErrUnsupportedContent
		malformed   bool // fails without wrapping it
	}{
		{"text and image always pass", langrails.ImageURLPart("https://x/a.png"), Support{}, false, false},
		{"audio allowed", langrails.AudioPart("QUFB", "audio/wav"), wav, false, false},
		{"audio not supported", langrails.AudioPart("QUFB", "audio/wav"), Support{}, true, false},
		{"audio format not allowed", langrails.AudioPart("QUFB", "audio/ogg"), wav, true, false},
		{"audio missing media type", langrails.ContentPart{Type: langrails.ContentAudio, Data: "QUFB"}, all, false, true},
		{"document data allowed", langrails.DocumentPart("JVBE", "application/pdf", ""), all, false, false},
		{"document data not supported", langrails.DocumentPart("JVBE", "application/pdf", ""), Support{DocumentURL: true}, true, false},
		{"document missing media type", langrails.DocumentPart("JVBE", "", ""), all, false, true},
		{"document url allowed", langrails.DocumentURLPart("https://x/a.pdf", ""), all, false, false},
		{"document url not supported", langrails.DocumentURLPart("https://x/a.pdf", ""), Support{DocumentData: true}, true, false},
		{"document without data or url", langrails.ContentPart{Type: langrails.ContentDocument}, all, false, true},
	}
	for _, c := range cases {
		err := CheckParts("p", reqWith(langrails.TextPart("hi"), c.part), c.support)
		switch {
		case c.unsupported:
			if !errors.Is(err, langrails.ErrUnsupportedContent) {
				t.Errorf("%s: err = %v, want ErrUnsupportedContent", c.name, err)
			}
		case c.malformed:
			if err == nil || errors.Is(err, langrails.ErrUnsupportedContent) {
				t.Errorf("%s: err = %v, want a malformed-part error", c.name, err)
			}
		default:
			if err != nil {
				t.Errorf("%s: unexpected error %v", c.name, err)
			}
		}
		if err != nil && !strings.HasPrefix(err.Error(), "p: message 1 part 1") {
			t.Errorf("%s: error does not locate the part: %v", c.name, err)
		}
	}
}

func TestHelpers(t *testing.T) {
	if got := DataURI("application/pdf", "JVBE"); got != "data:application/pdf;base64,JVBE" {
		t.Errorf("DataURI = %q", got)
	}
	if got := DecodeText("aGVsbG8="); got != "hello" {
		t.Errorf("DecodeText = %q", got)
	}
	if got := DecodeText("not base64!"); got != "not base64!" {
		t.Errorf("DecodeText fallback = %q", got)
	}
	if Filename("a.pdf", 3) != "a.pdf" || Filename("", 3) != "document-3" {
		t.Error("Filename")
	}
}
