package prompt

import (
	"testing"
)

func TestTemplate_SimpleVariable(t *testing.T) {
	tmpl := MustNew("test", "Hello {{ name }}, welcome!")
	result, err := tmpl.Execute(map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello Alice, welcome!" {
		t.Errorf("expected 'Hello Alice, welcome!', got %q", result)
	}
}

func TestTemplate_MultipleVariables(t *testing.T) {
	tmpl := MustNew("test", "{{ name }} is a {{ role }} from {{ city }}.")
	result, err := tmpl.Execute(map[string]any{
		"name": "Bob",
		"role": "developer",
		"city": "Istanbul",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Bob is a developer from Istanbul." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestTemplate_Functions(t *testing.T) {
	tmpl := MustNew("test", "{{ name | upper }}")
	result, err := tmpl.Execute(map[string]any{"name": "alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ALICE" {
		t.Errorf("expected 'ALICE', got %q", result)
	}
}

func TestTemplate_LowerFunction(t *testing.T) {
	tmpl := MustNew("test", "{{ name | lower }}")
	result, err := tmpl.Execute(map[string]any{"name": "ALICE"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "alice" {
		t.Errorf("expected 'alice', got %q", result)
	}
}

func TestTemplate_TrimFunction(t *testing.T) {
	tmpl := MustNew("test", "[{{ text | trim }}]")
	result, err := tmpl.Execute(map[string]any{"text": "  hello  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "[hello]" {
		t.Errorf("expected '[hello]', got %q", result)
	}
}

func TestTemplate_GoTemplateConditional(t *testing.T) {
	tmpl := MustNew("test", "{{if .premium}}Premium{{else}}Free{{end}} user")
	result, err := tmpl.Execute(map[string]any{"premium": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Premium user" {
		t.Errorf("expected 'Premium user', got %q", result)
	}
}

func TestTemplate_GoTemplateRange(t *testing.T) {
	tmpl := MustNew("test", "Items:{{range .items}} {{.}}{{end}}")
	result, err := tmpl.Execute(map[string]any{"items": []string{"a", "b", "c"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Items: a b c" {
		t.Errorf("expected 'Items: a b c', got %q", result)
	}
}

func TestTemplate_MixedSyntax(t *testing.T) {
	tmpl := MustNew("test", "Hello {{ name }}{{if .vip}}, VIP{{end}}!")
	result, err := tmpl.Execute(map[string]any{"name": "Alice", "vip": true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello Alice, VIP!" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestTemplate_InvalidTemplate(t *testing.T) {
	_, err := New("test", "{{ invalid {{ syntax }}")
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestTemplate_MissingVariable(t *testing.T) {
	tmpl := MustNew("test", "Hello {{ name }}")
	result, err := tmpl.Execute(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Go templates render missing keys as "<no value>"
	if result != "Hello <no value>" {
		t.Errorf("expected 'Hello <no value>', got %q", result)
	}
}

func TestTemplate_Name(t *testing.T) {
	tmpl := MustNew("my-template", "test")
	if tmpl.Name() != "my-template" {
		t.Errorf("expected 'my-template', got %q", tmpl.Name())
	}
}

func TestTemplate_Raw(t *testing.T) {
	raw := "Hello {{ name }}"
	tmpl := MustNew("test", raw)
	if tmpl.Raw() != raw {
		t.Errorf("expected raw template, got %q", tmpl.Raw())
	}
}

func TestMustNew_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	MustNew("test", "{{ invalid {{ }}")
}

func TestMustExecute_Works(t *testing.T) {
	tmpl := MustNew("test", "{{ name }}")
	result := tmpl.MustExecute(map[string]any{"name": "Alice"})
	if result != "Alice" {
		t.Errorf("expected 'Alice', got %q", result)
	}
}

func TestBuilder_Basic(t *testing.T) {
	b := NewBuilder()
	b.AddLine("You are a helpful assistant.")
	b.AddLine("The user's name is {{ name }}.")

	result, err := b.Build(map[string]any{"name": "Alice"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "You are a helpful assistant.\nThe user's name is Alice."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuilder_WithSection(t *testing.T) {
	b := NewBuilder()
	b.AddLine("System prompt.")
	b.AddSection("Rules", "- Be concise\n- Be accurate")

	result := b.String()
	if result == "" {
		t.Fatal("expected non-empty string")
	}
}

func TestBuilder_AddTemplate(t *testing.T) {
	b := NewBuilder()
	b.AddTemplate("Hello {{ name }}")

	result, err := b.Build(map[string]any{"name": "Bob"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello Bob" {
		t.Errorf("expected 'Hello Bob', got %q", result)
	}
}
