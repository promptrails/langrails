package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/promptrails/langrails"
)

// Default summarization settings.
const (
	defaultSummaryThreshold = 3000
	defaultKeepRecent       = 4
	defaultSummaryPrompt    = "Summarize the following conversation concisely, " +
		"preserving key facts, decisions, names, and any context needed to continue. " +
		"Write the summary as plain prose."
	summaryPrefix = "Summary of the earlier conversation: "
)

// SummarizationMiddleware compresses long message histories before each
// model call. When the estimated token count of the messages exceeds a
// threshold, the older messages are replaced with a single LLM-generated
// summary while the most recent messages are kept verbatim. This is the
// BeforeModel equivalent of LangChain's SummarizationMiddleware and avoids
// context-window overflow on long-running agents.
type SummarizationMiddleware struct {
	BaseMiddleware
	provider   langrails.Provider
	model      string
	threshold  int
	keepRecent int
	prompt     string
}

// SummarizationOption configures a SummarizationMiddleware.
type SummarizationOption func(*SummarizationMiddleware)

// WithSummaryThreshold sets the estimated token count above which
// summarization triggers. Default is 3000.
func WithSummaryThreshold(tokens int) SummarizationOption {
	return func(m *SummarizationMiddleware) { m.threshold = tokens }
}

// WithKeepRecent sets how many of the most recent messages are kept
// verbatim (never summarized). Default is 4.
func WithKeepRecent(n int) SummarizationOption {
	return func(m *SummarizationMiddleware) { m.keepRecent = n }
}

// WithSummaryPrompt overrides the system prompt used when summarizing.
func WithSummaryPrompt(prompt string) SummarizationOption {
	return func(m *SummarizationMiddleware) { m.prompt = prompt }
}

// NewSummarization creates a SummarizationMiddleware. The provider and
// model are used for the summarization call and may differ from the
// agent's main model (a cheaper model is a common choice).
func NewSummarization(provider langrails.Provider, model string, opts ...SummarizationOption) *SummarizationMiddleware {
	m := &SummarizationMiddleware{
		provider:   provider,
		model:      model,
		threshold:  defaultSummaryThreshold,
		keepRecent: defaultKeepRecent,
		prompt:     defaultSummaryPrompt,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// BeforeModel summarizes the older portion of the history when the
// estimated token count exceeds the threshold.
func (m *SummarizationMiddleware) BeforeModel(ctx context.Context, state *State) error {
	msgs := state.Request.Messages
	if estimateMessages(msgs) <= m.threshold {
		return nil
	}

	cut := len(msgs) - m.keepRecent
	if cut <= 0 {
		return nil // not enough history to summarize
	}
	// Never leave a tool result at the head of the kept tail without its
	// originating assistant message: pull such messages into the summary.
	for cut < len(msgs) && msgs[cut].Role == "tool" {
		cut++
	}

	older := msgs[:cut]
	recent := msgs[cut:]

	summary, err := m.summarize(ctx, older)
	if err != nil {
		return fmt.Errorf("summarize: %w", err)
	}

	newMsgs := make([]langrails.Message, 0, 1+len(recent))
	newMsgs = append(newMsgs, langrails.Message{Role: "user", Content: summaryPrefix + summary})
	newMsgs = append(newMsgs, recent...)
	state.Request.Messages = newMsgs
	return nil
}

// summarize renders the messages to text and asks the model for a summary.
func (m *SummarizationMiddleware) summarize(ctx context.Context, msgs []langrails.Message) (string, error) {
	var b strings.Builder
	for _, msg := range msgs {
		b.WriteString(msg.Role)
		b.WriteString(": ")
		b.WriteString(messageText(msg))
		b.WriteByte('\n')
	}

	resp, err := m.provider.Complete(ctx, &langrails.CompletionRequest{
		Model:        m.model,
		SystemPrompt: m.prompt,
		Messages:     []langrails.Message{{Role: "user", Content: b.String()}},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// estimateMessages estimates the total token count of a message slice using
// the rough heuristic of ~4 characters per token plus per-message overhead.
// This intentionally trades accuracy for zero dependencies; it can
// underestimate for multilingual or code-heavy content, so set
// WithSummaryThreshold conservatively if exact token budgets matter.
func estimateMessages(msgs []langrails.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(messageText(m))/4 + 1
		total += 4 // role / formatting overhead
	}
	return total
}

// messageText returns a message's textual content. When ContentParts is
// set it takes precedence over Content (matching langrails.Message
// semantics), so text parts are joined; image parts contribute no text.
func messageText(m langrails.Message) string {
	if len(m.ContentParts) == 0 {
		return m.Content
	}
	var b strings.Builder
	for _, p := range m.ContentParts {
		if p.Type == "text" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(p.Text)
		}
	}
	return b.String()
}
