package compat

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertMessages_AudioAndDocument(t *testing.T) {
	req := &langrails.CompletionRequest{Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{
		langrails.TextPart("summarize"),
		langrails.AudioPart("QUFB", "audio/mpeg"),
		langrails.DocumentPart("JVBE", "application/pdf", "report.pdf"),
	}}}}
	b, _ := json.Marshal(convertMessages(req))
	for _, want := range []string{
		`{"type":"input_audio","input_audio":{"data":"QUFB","format":"mp3"}}`,
		`{"type":"file","file":{"filename":"report.pdf","file_data":"data:application/pdf;base64,JVBE"}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}

func TestBuildRequestBody_RejectsUnsupportedMedia(t *testing.T) {
	p := New(Config{Name: "test"})
	cases := map[string]langrails.ContentPart{
		"audio format": langrails.AudioPart("QUFB", "audio/ogg"),
		"document url": langrails.DocumentURLPart("https://x/a.pdf", "application/pdf"),
	}
	for name, part := range cases {
		req := &langrails.CompletionRequest{Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{part}}}}
		if _, err := p.buildRequestBody(req, false); !errors.Is(err, langrails.ErrUnsupportedContent) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	bad := &langrails.CompletionRequest{Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{
		{Type: langrails.ContentDocument},
	}}}}
	if _, err := p.buildRequestBody(bad, false); err == nil {
		t.Error("empty document part should fail")
	}
}
