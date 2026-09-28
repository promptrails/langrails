package llm

import (
	"fmt"
	"os"
	"strings"

	"github.com/promptrails/langrails"
)

// apiKeyEnv lists, per provider, the environment variables FromString reads
// an API key from when none is passed, in order of preference. These are
// the names each provider's own SDKs and docs use.
var apiKeyEnv = map[ProviderName][]string{
	OpenAI:      {"OPENAI_API_KEY"},
	Anthropic:   {"ANTHROPIC_API_KEY"},
	Gemini:      {"GEMINI_API_KEY", "GOOGLE_API_KEY"},
	DeepSeek:    {"DEEPSEEK_API_KEY"},
	Groq:        {"GROQ_API_KEY"},
	Fireworks:   {"FIREWORKS_API_KEY"},
	XAI:         {"XAI_API_KEY"},
	OpenRouter:  {"OPENROUTER_API_KEY"},
	Together:    {"TOGETHER_API_KEY"},
	Mistral:     {"MISTRAL_API_KEY"},
	Cohere:      {"COHERE_API_KEY", "CO_API_KEY"},
	Chutes:      {"CHUTES_API_KEY"},
	ZAI:         {"ZAI_API_KEY"},
	Moonshot:    {"MOONSHOT_API_KEY"},
	Novita:      {"NOVITA_API_KEY"},
	DeepInfra:   {"DEEPINFRA_API_KEY", "DEEPINFRA_TOKEN"},
	Friendli:    {"FRIENDLI_TOKEN"},
	Cerebras:    {"CEREBRAS_API_KEY"},
	SambaNova:   {"SAMBANOVA_API_KEY"},
	Hyperbolic:  {"HYPERBOLIC_API_KEY"},
	DashScope:   {"DASHSCOPE_API_KEY"},
	HuggingFace: {"HF_TOKEN", "HUGGINGFACE_API_KEY"},
	// Ollama needs no key; Bedrock reads the AWS environment itself.
}

// ParseModel splits a "provider:model" string, such as
// "anthropic:claude-sonnet-5" or "ollama:llama3:8b", at its first colon.
// The provider name is case-insensitive and must be a known provider.
func ParseModel(spec string) (ProviderName, string, error) {
	prov, model, ok := strings.Cut(spec, ":")
	if !ok || prov == "" || model == "" {
		return "", "", fmt.Errorf("langrails: model %q is not in provider:model form", spec)
	}
	name := ProviderName(strings.ToLower(strings.TrimSpace(prov)))
	for _, known := range AllProviders() {
		if name == known {
			return name, strings.TrimSpace(model), nil
		}
	}
	return "", "", fmt.Errorf("langrails: unknown provider %q in %q", prov, spec)
}

// FromString creates a provider from a "provider:model" string and returns
// it with the model name, ready for CompletionRequest.Model. It suits
// applications that pick the model from configuration:
//
//	provider, model, err := llm.FromString(os.Getenv("LLM_MODEL"), "")
//	// LLM_MODEL=anthropic:claude-sonnet-5 → reads ANTHROPIC_API_KEY
//	resp, err := provider.Complete(ctx, &langrails.CompletionRequest{Model: model, ...})
//
// When apiKey is empty, the key is read from the provider's conventional
// environment variable (OPENAI_API_KEY, ANTHROPIC_API_KEY, GEMINI_API_KEY,
// ...). It is an error if the provider needs a key and none is found.
func FromString(spec, apiKey string) (langrails.Provider, string, error) {
	name, model, err := ParseModel(spec)
	if err != nil {
		return nil, "", err
	}
	if apiKey == "" {
		apiKey = envAPIKey(name)
		if envs := apiKeyEnv[name]; apiKey == "" && len(envs) > 0 {
			return nil, "", fmt.Errorf("langrails: no API key for %s: pass one or set %s", name, strings.Join(envs, " or "))
		}
	}
	p, err := New(name, apiKey)
	if err != nil {
		return nil, "", err
	}
	return p, model, nil
}

// MustFromString is like FromString but panics on error.
func MustFromString(spec, apiKey string) (langrails.Provider, string) {
	p, model, err := FromString(spec, apiKey)
	if err != nil {
		panic(err)
	}
	return p, model
}

func envAPIKey(name ProviderName) string {
	for _, env := range apiKeyEnv[name] {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}
