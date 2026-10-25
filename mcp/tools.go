package mcp

import "encoding/json"

// ToolAnnotations are hints a server gives about how a tool behaves. They
// are hints, not guarantees: only rely on them for servers you trust. Use
// the methods, which apply the spec's defaults for hints the server left
// out.
type ToolAnnotations struct {
	// Title is a human-readable name for the tool.
	Title string `json:"title,omitempty"`

	// ReadOnlyHint says the tool does not modify its environment.
	ReadOnlyHint *bool `json:"readOnlyHint,omitempty"`

	// DestructiveHint says a modifying tool may destroy or overwrite data,
	// as opposed to only adding.
	DestructiveHint *bool `json:"destructiveHint,omitempty"`

	// IdempotentHint says calling the tool again with the same arguments
	// has no further effect.
	IdempotentHint *bool `json:"idempotentHint,omitempty"`

	// OpenWorldHint says the tool reaches outside the server (the web, a
	// third-party API) rather than a closed domain.
	OpenWorldHint *bool `json:"openWorldHint,omitempty"`
}

// ReadOnly reports whether the tool leaves its environment unchanged.
// Defaults to false.
func (a ToolAnnotations) ReadOnly() bool { return hint(a.ReadOnlyHint, false) }

// Destructive reports whether the tool may destroy or overwrite data. A
// read-only tool never is; otherwise it defaults to true.
func (a ToolAnnotations) Destructive() bool {
	return !a.ReadOnly() && hint(a.DestructiveHint, true)
}

// Idempotent reports whether repeating a call has no further effect.
// Defaults to false.
func (a ToolAnnotations) Idempotent() bool { return hint(a.IdempotentHint, false) }

// OpenWorld reports whether the tool reaches outside the server. Defaults
// to true.
func (a ToolAnnotations) OpenWorld() bool { return hint(a.OpenWorldHint, true) }

func hint(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

// Tool is a tool the server offers, with its annotations.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Annotations ToolAnnotations
}

// Tools returns the tools discovered on the server.
func (c *Client) Tools() []Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]Tool, len(c.tools))
	for i, t := range c.tools {
		schema, _ := json.Marshal(t.InputSchema)
		out[i] = Tool{Name: t.Name, Description: t.Description, InputSchema: schema, Annotations: t.Annotations}
	}
	return out
}

// ToolNames returns the names of the tools for which keep returns true.
func (c *Client) ToolNames(keep func(Tool) bool) []string {
	var names []string
	for _, t := range c.Tools() {
		if keep(t) {
			names = append(names, t.Name)
		}
	}
	return names
}

// DestructiveTools returns the names of the tools that may destroy or
// overwrite data, including tools that do not say (the spec's default).
// Gate them behind approval:
//
//	h := agent.NewHumanInLoop(client, approver, agent.WithInterruptOn(client.DestructiveTools()...))
func (c *Client) DestructiveTools() []string {
	return c.ToolNames(func(t Tool) bool { return t.Annotations.Destructive() })
}
