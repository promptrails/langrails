// Package unillm provides a unified interface for interacting with multiple
// LLM (Large Language Model) providers through a single, consistent API.
//
// It supports 11+ providers including OpenAI, Anthropic, Google Gemini,
// DeepSeek, Groq, Fireworks, xAI, OpenRouter, Together, Mistral, and Cohere.
//
// # Core Interface
//
// All providers implement the [Provider] interface with two methods:
//
//   - Complete: sends a request and returns the full response
//   - Stream: sends a request and returns a channel of streaming events
//
// # Quick Start
//
//	import (
//		"github.com/promptrails/unillm/openai"
//	)
//
//	provider := openai.New("sk-...")
//	resp, err := provider.Complete(ctx, &unillm.CompletionRequest{
//		Model:    "gpt-4o",
//		Messages: []unillm.Message{{Role: "user", Content: "Hello!"}},
//	})
//
// # Streaming
//
//	events, err := provider.Stream(ctx, &unillm.CompletionRequest{
//		Model:    "gpt-4o",
//		Messages: []unillm.Message{{Role: "user", Content: "Hello!"}},
//	})
//	for event := range events {
//		if event.Type == unillm.EventContent {
//			fmt.Print(event.Content)
//		}
//	}
//
// # Provider Decorators
//
// Providers can be wrapped with decorators for retry and fallback behavior:
//
//	provider := unillm.WithRetry(openai.New("sk-..."), 3)
//	provider = unillm.WithFallback(provider, anthropic.New("sk-..."))
package unillm
