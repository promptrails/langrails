package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/promptrails/langrails"
)

// Resource is a piece of context an MCP server exposes by URI (a file, a
// database row, an API response).
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MIMEType    string `json:"mimeType,omitempty"`
}

// ResourceContent is the content of a resource. Exactly one of Text and
// Blob (base64) is set.
type ResourceContent struct {
	URI      string `json:"uri"`
	MIMEType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"`
}

// Prompt is a prompt template an MCP server offers.
type Prompt struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	Arguments   []PromptArgument `json:"arguments,omitempty"`
}

// PromptArgument describes one argument of a Prompt.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListResources returns every resource the server exposes, following
// pagination.
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	var all []Resource
	err := paginate(ctx, c.t, "resources/list", func(raw json.RawMessage) (string, error) {
		var page struct {
			Resources  []Resource `json:"resources"`
			NextCursor string     `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return "", err
		}
		all = append(all, page.Resources...)
		return page.NextCursor, nil
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: list resources: %w", err)
	}
	return all, nil
}

// ReadResource returns the contents of the resource at uri. A resource may
// have several parts (for example a directory listing).
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	raw, err := c.t.call(ctx, "resources/read", map[string]interface{}{"uri": uri})
	if err != nil {
		return nil, fmt.Errorf("mcp: read resource %q: %w", uri, err)
	}
	var result struct {
		Contents []ResourceContent `json:"contents"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: read resource %q: %w", uri, err)
	}
	return result.Contents, nil
}

// ListPrompts returns every prompt the server offers, following pagination.
func (c *Client) ListPrompts(ctx context.Context) ([]Prompt, error) {
	var all []Prompt
	err := paginate(ctx, c.t, "prompts/list", func(raw json.RawMessage) (string, error) {
		var page struct {
			Prompts    []Prompt `json:"prompts"`
			NextCursor string   `json:"nextCursor"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return "", err
		}
		all = append(all, page.Prompts...)
		return page.NextCursor, nil
	})
	if err != nil {
		return nil, fmt.Errorf("mcp: list prompts: %w", err)
	}
	return all, nil
}

// GetPrompt renders a server prompt with arguments and returns it as
// messages ready to send in a CompletionRequest. Text content becomes
// Content; image content becomes an image part; embedded text resources
// are inlined as text.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) ([]langrails.Message, error) {
	params := map[string]interface{}{"name": name}
	if len(args) > 0 {
		params["arguments"] = args
	}
	raw, err := c.t.call(ctx, "prompts/get", params)
	if err != nil {
		return nil, fmt.Errorf("mcp: get prompt %q: %w", name, err)
	}

	var result struct {
		Messages []struct {
			Role    string        `json:"role"`
			Content promptContent `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("mcp: get prompt %q: %w", name, err)
	}

	msgs := make([]langrails.Message, 0, len(result.Messages))
	for _, m := range result.Messages {
		msg := langrails.Message{Role: langrails.Role(m.Role)}
		switch m.Content.Type {
		case "image":
			msg.ContentParts = []langrails.ContentPart{langrails.ImageBase64Part(m.Content.Data, m.Content.MIMEType)}
		case "resource":
			if m.Content.Resource != nil {
				msg.Content = m.Content.Resource.Text
			}
		default:
			msg.Content = m.Content.Text
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

type promptContent struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	Data     string           `json:"data,omitempty"`
	MIMEType string           `json:"mimeType,omitempty"`
	Resource *ResourceContent `json:"resource,omitempty"`
}
