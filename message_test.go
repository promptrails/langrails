package langrails

import (
	"encoding/json"
	"testing"
)

func TestTextPart(t *testing.T) {
	p := TextPart("hello")
	if p.Type != "text" {
		t.Errorf("Type = %q, want %q", p.Type, "text")
	}
	if p.Text != "hello" {
		t.Errorf("Text = %q, want %q", p.Text, "hello")
	}
	if p.ImageURL != "" {
		t.Errorf("ImageURL = %q, want empty", p.ImageURL)
	}
}

func TestImageURLPart(t *testing.T) {
	p := ImageURLPart("https://example.com/cat.png")
	if p.Type != "image" {
		t.Errorf("Type = %q, want %q", p.Type, "image")
	}
	if p.ImageURL != "https://example.com/cat.png" {
		t.Errorf("ImageURL = %q, want %q", p.ImageURL, "https://example.com/cat.png")
	}
	if p.Text != "" {
		t.Errorf("Text = %q, want empty", p.Text)
	}
}

func TestImageBase64Part(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		mediaType string
		want      string
	}{
		{"png", "AAAA", "image/png", "data:image/png;base64,AAAA"},
		{"jpeg", "/9j/4AAQ", "image/jpeg", "data:image/jpeg;base64,/9j/4AAQ"},
		{"empty data", "", "image/webp", "data:image/webp;base64,"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := ImageBase64Part(tc.data, tc.mediaType)
			if p.Type != "image" {
				t.Errorf("Type = %q, want %q", p.Type, "image")
			}
			if p.ImageURL != tc.want {
				t.Errorf("ImageURL = %q, want %q", p.ImageURL, tc.want)
			}
		})
	}
}

// A message with ContentParts mixes text and image parts; the helpers should
// compose into the slice in order.
func TestContentParts_Compose(t *testing.T) {
	msg := Message{
		Role: "user",
		ContentParts: []ContentPart{
			TextPart("describe this"),
			ImageURLPart("https://example.com/cat.png"),
			ImageBase64Part("AAAA", "image/png"),
		},
	}
	if len(msg.ContentParts) != 3 {
		t.Fatalf("len(ContentParts) = %d, want 3", len(msg.ContentParts))
	}
	if msg.ContentParts[0].Type != "text" || msg.ContentParts[1].Type != "image" || msg.ContentParts[2].Type != "image" {
		t.Errorf("unexpected part types: %+v", msg.ContentParts)
	}
	if msg.ContentParts[2].ImageURL != "data:image/png;base64,AAAA" {
		t.Errorf("base64 part = %q", msg.ContentParts[2].ImageURL)
	}
}

// ToolDefinition.Parameters is a json.RawMessage and must round-trip an
// arbitrary JSON schema verbatim.
func TestToolDefinition_ParametersRawJSON(t *testing.T) {
	schema := `{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}`
	td := ToolDefinition{
		Name:        "get_weather",
		Description: "Get the weather for a city",
		Parameters:  json.RawMessage(schema),
	}

	var got, want map[string]any
	if err := json.Unmarshal(td.Parameters, &got); err != nil {
		t.Fatalf("parameters not valid JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(schema), &want); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Errorf("round-tripped schema mismatch: got %v, want %v", got, want)
	}
}
