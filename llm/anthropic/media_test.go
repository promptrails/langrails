package anthropic

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertContentParts_Documents(t *testing.T) {
	m := langrails.Message{Role: "user", ContentParts: []langrails.ContentPart{
		langrails.DocumentPart("JVBE", "application/pdf", "report.pdf"),
		langrails.DocumentPart("aGVsbG8=", "text/plain", ""),
		langrails.DocumentURLPart("https://x/a.pdf", ""),
	}}
	b, _ := json.Marshal(convertContentParts(m))
	for _, want := range []string{
		`{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBE"},"title":"report.pdf"}`,
		`{"type":"document","source":{"type":"text","media_type":"text/plain","data":"hello"}}`,
		`{"type":"document","source":{"type":"url","url":"https://x/a.pdf"}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}

func TestBuildRequestBody_RejectsAudio(t *testing.T) {
	req := &langrails.CompletionRequest{Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{
		langrails.AudioPart("QUFB", "audio/wav"),
	}}}}
	if _, err := New("k").buildRequestBody(req, false); !errors.Is(err, langrails.ErrUnsupportedContent) {
		t.Errorf("err = %v", err)
	}
}
