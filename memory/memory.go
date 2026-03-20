package memory

import (
	"sync"

	"github.com/promptrails/langrails"
)

// estimateTokens estimates the number of tokens in a string.
// Uses the rough heuristic of ~4 characters per token.
func estimateTokens(text string) int {
	return len(text)/4 + 1
}

// Memory stores conversation history with configurable limits.
// It is safe for concurrent use.
//
// Example:
//
//	mem := memory.New(memory.WithMaxMessages(50))
//	mem.Add(langrails.Message{Role: "user", Content: "Hello!"})
//	mem.Add(langrails.Message{Role: "assistant", Content: "Hi there!"})
//	messages := mem.Messages() // returns conversation history
type Memory struct {
	mu          sync.RWMutex
	messages    []langrails.Message
	maxMessages int
	maxTokens   int
}

// Option configures the memory.
type Option func(*Memory)

// WithMaxMessages sets the maximum number of messages to keep.
// When exceeded, the oldest messages (excluding system messages) are removed.
// Default is 0 (unlimited).
func WithMaxMessages(n int) Option {
	return func(m *Memory) {
		m.maxMessages = n
	}
}

// WithMaxTokens sets the maximum estimated token count for all messages.
// When exceeded, the oldest messages (excluding system messages) are removed.
// Uses a rough heuristic of ~4 characters per token.
// Default is 0 (unlimited).
func WithMaxTokens(n int) Option {
	return func(m *Memory) {
		m.maxTokens = n
	}
}

// New creates a new conversation memory.
func New(opts ...Option) *Memory {
	m := &Memory{}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Add appends a message to the conversation history and trims if needed.
func (m *Memory) Add(msg langrails.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.messages = append(m.messages, msg)
	m.trim()
}

// AddUserMessage is a convenience method to add a user message.
func (m *Memory) AddUserMessage(content string) {
	m.Add(langrails.Message{Role: "user", Content: content})
}

// AddAssistantMessage is a convenience method to add an assistant message.
func (m *Memory) AddAssistantMessage(content string) {
	m.Add(langrails.Message{Role: "assistant", Content: content})
}

// Messages returns a copy of the current conversation history.
func (m *Memory) Messages() []langrails.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]langrails.Message, len(m.messages))
	copy(result, m.messages)
	return result
}

// Len returns the number of messages in memory.
func (m *Memory) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.messages)
}

// TokenCount returns the estimated total token count of all messages.
func (m *Memory) TokenCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tokenCount()
}

// Clear removes all messages from memory.
func (m *Memory) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}

// Last returns the last n messages. If n > len, returns all messages.
func (m *Memory) Last(n int) []langrails.Message {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if n >= len(m.messages) {
		result := make([]langrails.Message, len(m.messages))
		copy(result, m.messages)
		return result
	}

	result := make([]langrails.Message, n)
	copy(result, m.messages[len(m.messages)-n:])
	return result
}

func (m *Memory) tokenCount() int {
	total := 0
	for _, msg := range m.messages {
		total += estimateTokens(msg.Content)
		// Account for role overhead (~4 tokens per message)
		total += 4
	}
	return total
}

// trim removes the oldest non-system messages to stay within limits.
func (m *Memory) trim() {
	// Trim by message count
	if m.maxMessages > 0 && len(m.messages) > m.maxMessages {
		excess := len(m.messages) - m.maxMessages
		m.messages = m.removeOldest(m.messages, excess)
	}

	// Trim by token count
	if m.maxTokens > 0 {
		for m.tokenCount() > m.maxTokens && len(m.messages) > 1 {
			m.messages = m.removeOldest(m.messages, 1)
		}
	}
}

// removeOldest removes n oldest messages, preserving system messages at the start.
func (m *Memory) removeOldest(msgs []langrails.Message, n int) []langrails.Message {
	// Find where non-system messages start
	start := 0
	for start < len(msgs) && msgs[start].Role == "system" {
		start++
	}

	// Remove from the start of non-system messages
	removed := 0
	for removed < n && start < len(msgs)-1 {
		msgs = append(msgs[:start], msgs[start+1:]...)
		removed++
	}

	return msgs
}
