package prompt

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

var simpleVarPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// Template is a reusable prompt template with Jinja-style variable substitution.
//
// Templates use {{ variable }} syntax for simple variable replacement,
// and support Go's text/template features for advanced use cases
// (conditionals, loops, functions).
//
// Simple syntax (recommended):
//
//	{{ name }}     → variable substitution
//	{{ age }}      → works with any map key
//
// Advanced syntax (Go text/template):
//
//	{{if .premium}}Premium user{{end}}
//	{{range .items}}* {{.}}{{end}}
//	{{ name | upper }}  → built-in functions
//
// Example:
//
//	t := prompt.MustNew("greeting", "Hello {{ name }}, you are a {{ role }}.")
//	result, _ := t.Execute(map[string]any{"name": "Alice", "role": "admin"})
//	// result: "Hello Alice, you are a admin."
type Template struct {
	name string
	raw  string
	tmpl *template.Template
}

// New creates a new prompt template.
//
// Variables use {{ name }} syntax. Built-in functions:
// join, upper, lower, trim, contains, replace, default.
func New(name, text string) (*Template, error) {
	// Convert simple {{ var }} to Go template {{ .var }}
	goTmpl := toGoTemplate(text)

	funcMap := template.FuncMap{
		"join":     strings.Join,
		"upper":    strings.ToUpper,
		"lower":    strings.ToLower,
		"trim":     strings.TrimSpace,
		"contains": strings.Contains,
		"replace":  strings.ReplaceAll,
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
	}

	tmpl, err := template.New(name).Funcs(funcMap).Parse(goTmpl)
	if err != nil {
		return nil, fmt.Errorf("prompt: failed to parse template %q: %w", name, err)
	}

	return &Template{name: name, raw: text, tmpl: tmpl}, nil
}

// MustNew creates a new prompt template and panics on error.
func MustNew(name, text string) *Template {
	t, err := New(name, text)
	if err != nil {
		panic(err)
	}
	return t
}

// Execute renders the template with the given variables.
// Variables should be a map[string]any or a struct.
func (t *Template) Execute(vars any) (string, error) {
	var buf bytes.Buffer
	if err := t.tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("prompt: failed to execute template %q: %w", t.name, err)
	}
	return buf.String(), nil
}

// MustExecute renders the template and panics on error.
func (t *Template) MustExecute(vars any) string {
	result, err := t.Execute(vars)
	if err != nil {
		panic(err)
	}
	return result
}

// Name returns the template name.
func (t *Template) Name() string {
	return t.name
}

// Raw returns the original template text before parsing.
func (t *Template) Raw() string {
	return t.raw
}

// toGoTemplate converts simple {{ var }} and {{ var | func }} to
// Go template {{ .var }} and {{ .var | func }}.
// Leaves advanced syntax ({{ if }}, {{ range }}, {{ .field }}) untouched.
func toGoTemplate(text string) string {
	return simpleVarPattern.ReplaceAllStringFunc(text, func(match string) string {
		inner := strings.TrimSpace(match[2 : len(match)-2])

		// Skip already-dotted vars
		if strings.HasPrefix(inner, ".") {
			return match
		}

		// Skip Go template keywords
		keywords := []string{"if ", "else", "end", "range ", "with ", "define ", "template ", "block "}
		for _, kw := range keywords {
			if strings.HasPrefix(inner, kw) || inner == strings.TrimSpace(kw) {
				return match
			}
		}

		// Handle pipe: {{ var | func }} → {{ .var | func }}
		if idx := strings.Index(inner, "|"); idx > 0 {
			varName := strings.TrimSpace(inner[:idx])
			rest := inner[idx:] // includes the |
			if !strings.HasPrefix(varName, ".") {
				return "{{ ." + varName + " " + rest + " }}"
			}
			return match
		}

		// Simple variable: {{ var }} → {{ .var }}
		return "{{ ." + inner + " }}"
	})
}

// Builder helps construct complex prompts from multiple sections.
//
// Example:
//
//	b := prompt.NewBuilder()
//	b.AddLine("You are a helpful assistant.")
//	b.AddLine("The user's name is {{ name }}.")
//	b.AddSection("Rules", "- Be concise\n- Be accurate")
//	result, _ := b.Build(map[string]any{"name": "Alice"})
type Builder struct {
	parts []string
}

// NewBuilder creates a new prompt builder.
func NewBuilder() *Builder {
	return &Builder{}
}

// AddLine adds a line of text to the prompt.
func (b *Builder) AddLine(line string) *Builder {
	b.parts = append(b.parts, line)
	return b
}

// AddSection adds a named section with a header.
func (b *Builder) AddSection(header, content string) *Builder {
	b.parts = append(b.parts, fmt.Sprintf("\n## %s\n%s", header, content))
	return b
}

// AddTemplate adds a template string that will be rendered with variables.
func (b *Builder) AddTemplate(text string) *Builder {
	b.parts = append(b.parts, text)
	return b
}

// Build renders the complete prompt with the given variables.
func (b *Builder) Build(vars any) (string, error) {
	combined := strings.Join(b.parts, "\n")
	t, err := New("builder", combined)
	if err != nil {
		return "", err
	}
	return t.Execute(vars)
}

// String returns the raw template text without variable substitution.
func (b *Builder) String() string {
	return strings.Join(b.parts, "\n")
}
