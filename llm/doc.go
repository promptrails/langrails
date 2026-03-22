// Package llm provides a registry of all LLM providers and a convenience
// constructor for creating providers by name.
//
// # Usage
//
//	provider, err := llm.New(llm.OpenAI, "sk-...")
//	// or
//	provider := llm.MustNew(llm.OpenAI, "sk-...")
//
//	resp, err := provider.Complete(ctx, &langrails.CompletionRequest{
//		Model:    "gpt-4o",
//		Messages: []langrails.Message{{Role: "user", Content: "Hello!"}},
//	})
//
// All providers are registered automatically. No additional imports needed.
package llm
