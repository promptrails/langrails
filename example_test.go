package langrails_test

import (
	"fmt"

	"github.com/promptrails/langrails"
)

// Build a multimodal user message mixing text and an image URL.
func ExampleTextPart() {
	msg := langrails.Message{
		Role: "user",
		ContentParts: []langrails.ContentPart{
			langrails.TextPart("What is in this image?"),
			langrails.ImageURLPart("https://example.com/cat.png"),
		},
	}
	fmt.Println(msg.ContentParts[0].Text)
	fmt.Println(msg.ContentParts[1].ImageURL)
	// Output:
	// What is in this image?
	// https://example.com/cat.png
}

// Embed image bytes inline as a base64 data URI.
func ExampleImageBase64Part() {
	part := langrails.ImageBase64Part("iVBORw0KGgo", "image/png")
	fmt.Println(part.ImageURL)
	// Output: data:image/png;base64,iVBORw0KGgo
}

// Force the model to call a specific tool via ToolChoice.
func ExampleForceTool() {
	req := &langrails.CompletionRequest{
		Model:      "gpt-4o",
		Messages:   []langrails.Message{{Role: "user", Content: "Weather in Paris?"}},
		ToolChoice: langrails.ForceTool("get_weather"),
	}
	fmt.Println(req.ToolChoice.Mode, req.ToolChoice.Name)
	// Output: tool get_weather
}

// Enable provider-native web search with domain filtering.
func ExampleWebSearch() {
	req := &langrails.CompletionRequest{
		Model:    "gpt-4o",
		Messages: []langrails.Message{{Role: "user", Content: "Latest Go release?"}},
		ServerTools: []langrails.ServerTool{
			langrails.WebSearch(&langrails.WebSearchOptions{
				MaxUses:        2,
				AllowedDomains: []string{"go.dev"},
			}),
		},
	}
	st := req.ServerTools[0]
	fmt.Println(st.Type, st.WebSearch.MaxUses, st.WebSearch.AllowedDomains[0])
	// Output: web_search 2 go.dev
}
