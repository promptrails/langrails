package llm

import (
	"fmt"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/llm/anthropic"
	"github.com/promptrails/langrails/llm/cohere"
	"github.com/promptrails/langrails/llm/deepseek"
	"github.com/promptrails/langrails/llm/fireworks"
	"github.com/promptrails/langrails/llm/gemini"
	"github.com/promptrails/langrails/llm/groq"
	"github.com/promptrails/langrails/llm/mistral"
	"github.com/promptrails/langrails/llm/ollama"
	"github.com/promptrails/langrails/llm/openai"
	"github.com/promptrails/langrails/llm/openrouter"
	"github.com/promptrails/langrails/llm/together"
	"github.com/promptrails/langrails/llm/xai"
)

// ProviderName identifies a supported LLM provider.
type ProviderName string

const (
	OpenAI     ProviderName = "openai"
	Anthropic  ProviderName = "anthropic"
	Gemini     ProviderName = "gemini"
	DeepSeek   ProviderName = "deepseek"
	Groq       ProviderName = "groq"
	Fireworks  ProviderName = "fireworks"
	XAI        ProviderName = "xai"
	OpenRouter ProviderName = "openrouter"
	Together   ProviderName = "together"
	Mistral    ProviderName = "mistral"
	Cohere     ProviderName = "cohere"
	Ollama     ProviderName = "ollama"
)

// New creates a new LLM provider by name.
//
//	provider, err := llm.New(llm.OpenAI, "sk-...")
//	provider, err := llm.New(llm.Anthropic, "sk-ant-...")
//	provider, err := llm.New(llm.Ollama, "")  // no key needed
func New(name ProviderName, apiKey string) (langrails.Provider, error) {
	switch name {
	case OpenAI:
		return openai.New(apiKey), nil
	case Anthropic:
		return anthropic.New(apiKey), nil
	case Gemini:
		return gemini.New(apiKey), nil
	case DeepSeek:
		return deepseek.New(apiKey), nil
	case Groq:
		return groq.New(apiKey), nil
	case Fireworks:
		return fireworks.New(apiKey), nil
	case XAI:
		return xai.New(apiKey), nil
	case OpenRouter:
		return openrouter.New(apiKey), nil
	case Together:
		return together.New(apiKey), nil
	case Mistral:
		return mistral.New(apiKey), nil
	case Cohere:
		return cohere.New(apiKey), nil
	case Ollama:
		return ollama.New(), nil
	default:
		return nil, fmt.Errorf("langrails: unknown provider %q", name)
	}
}

// MustNew creates a new provider and panics on error.
func MustNew(name ProviderName, apiKey string) langrails.Provider {
	p, err := New(name, apiKey)
	if err != nil {
		panic(err)
	}
	return p
}

// AllProviders returns all registered provider names.
func AllProviders() []ProviderName {
	return []ProviderName{
		OpenAI, Anthropic, Gemini, DeepSeek, Groq, Fireworks,
		XAI, OpenRouter, Together, Mistral, Cohere, Ollama,
	}
}
