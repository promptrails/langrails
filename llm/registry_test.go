package llm

import (
	"testing"
)

func TestNew_AllProviders(t *testing.T) {
	for _, name := range AllProviders() {
		p, err := New(name, "test-key")
		if err != nil {
			t.Errorf("New(%q) error: %v", name, err)
			continue
		}
		if p == nil {
			t.Errorf("New(%q) returned nil", name)
		}
	}
}

func TestNew_Unknown(t *testing.T) {
	_, err := New("nonexistent", "key")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestMustNew_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNew("nonexistent", "key")
}

func TestMustNew_Works(t *testing.T) {
	p := MustNew(OpenAI, "key")
	if p == nil {
		t.Fatal("expected non-nil")
	}
}

func TestAllProviders(t *testing.T) {
	providers := AllProviders()
	if len(providers) != 19 {
		t.Errorf("expected 19 providers, got %d", len(providers))
	}
}
