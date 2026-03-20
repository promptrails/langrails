// Package memory provides conversation history management for LLM interactions.
//
// It stores messages with configurable limits (max messages or estimated token count)
// and automatically truncates old messages to stay within budget. Thread-safe
// for concurrent use.
package memory
