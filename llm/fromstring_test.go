package llm

import (
	"strings"
	"testing"
)

func TestParseModel(t *testing.T) {
	cases := []struct {
		spec  string
		name  ProviderName
		model string
	}{
		{"openai:gpt-4o", OpenAI, "gpt-4o"},
		{"Anthropic:claude-sonnet-5", Anthropic, "claude-sonnet-5"},
		{"ollama:llama3:8b", Ollama, "llama3:8b"},
		{"openrouter:meta-llama/llama-3-70b", OpenRouter, "meta-llama/llama-3-70b"},
	}
	for _, c := range cases {
		name, model, err := ParseModel(c.spec)
		if err != nil || name != c.name || model != c.model {
			t.Errorf("ParseModel(%q) = %q, %q, %v", c.spec, name, model, err)
		}
	}

	for _, bad := range []string{"gpt-4o", "openai:", ":gpt-4o", "nope:model", ""} {
		if _, _, err := ParseModel(bad); err == nil {
			t.Errorf("ParseModel(%q) should fail", bad)
		}
	}
}

func TestFromString(t *testing.T) {
	p, model, err := FromString("openai:gpt-4o", "sk-explicit")
	if err != nil || p == nil || model != "gpt-4o" {
		t.Fatalf("explicit key: %v, %q, %v", p, model, err)
	}

	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "g-key")
	if _, _, err := FromString("gemini:gemini-3-pro", ""); err != nil {
		t.Errorf("fallback env var: %v", err)
	}

	t.Setenv("ANTHROPIC_API_KEY", "")
	_, _, err = FromString("anthropic:claude-sonnet-5", "")
	if err == nil || !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("missing key: err = %v", err)
	}

	// Keyless providers need nothing.
	if _, model, err := FromString("ollama:llama3", ""); err != nil || model != "llama3" {
		t.Errorf("ollama: %q, %v", model, err)
	}
	if _, _, err := FromString("bedrock:anthropic.claude-3-haiku", ""); err != nil {
		t.Errorf("bedrock: %v", err)
	}

	if _, _, err := FromString("nope:x", "k"); err == nil {
		t.Error("unknown provider should fail")
	}
}

func TestMustFromString(t *testing.T) {
	if _, model := MustFromString("groq:llama-3.3-70b", "k"); model != "llama-3.3-70b" {
		t.Errorf("model = %q", model)
	}
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	MustFromString("bad", "")
}

func TestAPIKeyEnvCoversKeyedProviders(t *testing.T) {
	for _, name := range AllProviders() {
		if name == Ollama || name == Bedrock {
			continue
		}
		if len(apiKeyEnv[name]) == 0 {
			t.Errorf("no API key env var for %s", name)
		}
	}
}
