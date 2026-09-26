package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func TestConvertContentParts_AudioAndDocuments(t *testing.T) {
	m := langrails.Message{Role: "user", ContentParts: []langrails.ContentPart{
		langrails.AudioPart("QUFB", "audio/wav"),
		langrails.DocumentPart("JVBE", "application/pdf", ""),
		langrails.DocumentURLPart("gs://bucket/a.pdf", "application/pdf"),
	}}
	b, _ := json.Marshal(convertContentParts(m))
	for _, want := range []string{
		`{"inlineData":{"mimeType":"audio/wav","data":"QUFB"}}`,
		`{"inlineData":{"mimeType":"application/pdf","data":"JVBE"}}`,
		`{"fileData":{"mimeType":"application/pdf","fileUri":"gs://bucket/a.pdf"}}`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %s in %s", want, b)
		}
	}
}
