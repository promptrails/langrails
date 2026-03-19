// Package compat provides a base implementation for OpenAI-compatible LLM providers.
//
// Many providers (DeepSeek, Groq, Together, Fireworks, xAI, Mistral, Cohere,
// OpenRouter) expose an API compatible with OpenAI's chat completions endpoint.
// This package implements the shared protocol logic so each provider only needs
// to supply its base URL, name, and optional custom headers.
package compat
