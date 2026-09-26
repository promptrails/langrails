package bedrock

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertContentParts_Document(t *testing.T) {
	m := langrails.Message{Role: "user", ContentParts: []langrails.ContentPart{
		langrails.DocumentPart("JVBE", "application/pdf", "Q3 report_final.v2.pdf"),
		langrails.DocumentPart("aGk=", "text/plain", ""),
	}}
	b, _ := json.Marshal(convertContentParts(m))
	for _, want := range []string{
		`{"document":{"format":"pdf","name":"Q3 report final v2","source":{"bytes":"JVBE"}}}`,
		`{"document":{"format":"txt","name":"document-2","source":{"bytes":"aGk="}}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}

func TestBuildRequestBody_RejectsUnsupportedMedia(t *testing.T) {
	for name, part := range map[string]langrails.ContentPart{
		"audio":    langrails.AudioPart("QUFB", "audio/wav"),
		"url":      langrails.DocumentURLPart("https://x/a.pdf", "application/pdf"),
		"doc type": langrails.DocumentPart("QUFB", "application/zip", ""),
	} {
		req := &langrails.CompletionRequest{Model: "m", Messages: []langrails.Message{{Role: "user", ContentParts: []langrails.ContentPart{part}}}}
		if _, err := buildRequestBody(req); !errors.Is(err, langrails.ErrUnsupportedContent) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
