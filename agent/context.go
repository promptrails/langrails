package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/promptrails/langrails"
)

// ContextEditing keeps long tool-heavy runs inside the context window by
// clearing the content of old tool results once the history grows past a
// token threshold. The most recent results are kept; older ones are
// replaced with a short placeholder, so the model still sees that the call
// happened. Unlike summarization it costs no model call.
type ContextEditing struct {
	BaseMiddleware
	threshold   int
	keep        int
	placeholder string
}

// ContextEditingOption configures ContextEditing.
type ContextEditingOption func(*ContextEditing)

// WithClearThreshold sets the estimated token count above which old tool
// results are cleared. Default is 100,000.
func WithClearThreshold(tokens int) ContextEditingOption {
	return func(c *ContextEditing) { c.threshold = tokens }
}

// WithKeepToolResults sets how many of the most recent tool results are
// kept intact. Default is 3.
func WithKeepToolResults(n int) ContextEditingOption {
	return func(c *ContextEditing) { c.keep = n }
}

// WithClearedPlaceholder sets the text that replaces a cleared result.
func WithClearedPlaceholder(text string) ContextEditingOption {
	return func(c *ContextEditing) { c.placeholder = text }
}

// NewContextEditing creates the middleware.
func NewContextEditing(opts ...ContextEditingOption) *ContextEditing {
	c := &ContextEditing{threshold: 100_000, keep: 3, placeholder: "[tool result cleared to save context]"}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// BeforeModel clears old tool results when the history is over threshold.
// Token counts use the same ~4 characters per token estimate as
// summarization.
func (c *ContextEditing) BeforeModel(_ context.Context, s *State) error {
	msgs := s.Request.Messages
	if estimateMessages(msgs) <= c.threshold {
		return nil
	}
	seen := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "tool" {
			continue
		}
		seen++
		if seen > c.keep && msgs[i].Content != c.placeholder {
			msgs[i].Content = c.placeholder
			msgs[i].ContentParts = nil
		}
	}
	return nil
}

// ToolSelector narrows a large tool set to the tools relevant to the user's
// request before the agent's model sees them, using a separate (usually
// cheaper) model. Fewer tools means a shorter prompt and fewer wrong
// picks. Selection happens once per run, on the first model call.
type ToolSelector struct {
	BaseMiddleware
	provider langrails.Provider
	model    string
	maxTools int
	always   []string
}

// ToolSelectorOption configures ToolSelector.
type ToolSelectorOption func(*ToolSelector)

// WithMaxTools caps how many tools the selector keeps. Default is 5.
// Selection is skipped when the agent has no more tools than this.
func WithMaxTools(n int) ToolSelectorOption {
	return func(t *ToolSelector) { t.maxTools = n }
}

// WithAlwaysInclude keeps the named tools regardless of the selection; they
// do not count toward the maximum.
func WithAlwaysInclude(names ...string) ToolSelectorOption {
	return func(t *ToolSelector) { t.always = append(t.always, names...) }
}

// NewToolSelector selects tools with model on provider.
func NewToolSelector(provider langrails.Provider, model string, opts ...ToolSelectorOption) *ToolSelector {
	t := &ToolSelector{provider: provider, model: model, maxTools: 5}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

const toolSelectorPrompt = `You choose which tools an assistant will need to answer a user's request.
Pick at most %d tools from the list below that are relevant. Pick none if no tool helps.

%s`

// BeforeModel replaces the request's tools with the selected ones on the
// first iteration. The request is reused for the rest of the run, so the
// selection sticks.
func (t *ToolSelector) BeforeModel(ctx context.Context, s *State) error {
	tools := s.Request.Tools
	if s.Iteration != 1 || len(tools) <= t.maxTools {
		return nil
	}

	var list strings.Builder
	names := make([]string, len(tools))
	for i, def := range tools {
		names[i] = def.Name
		fmt.Fprintf(&list, "- %s: %s\n", def.Name, def.Description)
	}
	schema, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tools": map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string", "enum": names},
				"maxItems": t.maxTools,
			},
		},
		"required": []string{"tools"},
	})
	if err != nil {
		return err
	}

	resp, err := t.provider.Complete(ctx, &langrails.CompletionRequest{
		Model:        t.model,
		SystemPrompt: fmt.Sprintf(toolSelectorPrompt, t.maxTools, list.String()),
		Messages:     []langrails.Message{{Role: "user", Content: lastUserText(s.Request.Messages)}},
		OutputSchema: &schema,
	})
	if err != nil {
		return fmt.Errorf("tool selector: %w", err)
	}
	var picked struct {
		Tools []string `json:"tools"`
	}
	if err := json.Unmarshal([]byte(resp.Content), &picked); err != nil {
		return fmt.Errorf("tool selector: parse selection: %w", err)
	}

	var kept []langrails.ToolDefinition
	for _, def := range tools {
		if slices.Contains(t.always, def.Name) ||
			(slices.Contains(picked.Tools, def.Name) && countSelected(kept, t.always) < t.maxTools) {
			kept = append(kept, def)
		}
	}
	s.Request.Tools = kept
	return nil
}

func countSelected(kept []langrails.ToolDefinition, always []string) int {
	n := 0
	for _, d := range kept {
		if !slices.Contains(always, d.Name) {
			n++
		}
	}
	return n
}

func lastUserText(msgs []langrails.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return messageText(msgs[i])
		}
	}
	return ""
}
