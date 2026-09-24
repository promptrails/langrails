package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/internal/jsonschema"
)

// Tool pairs a tool's definition with the function that runs it.
type Tool struct {
	// Definition is what the model sees: name, description, parameter schema.
	Definition langrails.ToolDefinition

	// Func runs the tool on JSON-encoded arguments.
	Func Func
}

// New builds a Tool from a typed Go function. The parameter schema is
// derived from In (see the struct tags documented on [NewSet]), the model's
// arguments are decoded into In before fn runs, and fn's result is encoded
// as JSON — except a string result, which is returned as is.
//
//	type WeatherArgs struct {
//	    City string `json:"city" description:"City name"`
//	    Unit string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit"`
//	}
//
//	weather, err := tools.New("get_weather", "Get current weather for a city",
//	    func(ctx context.Context, in WeatherArgs) (Weather, error) { ... })
//
// New returns an error when In cannot be described as a JSON Schema (for
// example a recursive type or a map with non-string keys).
func New[In, Out any](name, description string, fn func(ctx context.Context, in In) (Out, error)) (Tool, error) {
	schema, err := jsonschema.For[In]()
	if err != nil {
		return Tool{}, fmt.Errorf("tool %s: %w", name, err)
	}
	return Tool{
		Definition: langrails.ToolDefinition{
			Name:        name,
			Description: description,
			Parameters:  schema,
		},
		Func: func(ctx context.Context, arguments string) (string, error) {
			var in In
			if strings.TrimSpace(arguments) != "" {
				if err := json.Unmarshal([]byte(arguments), &in); err != nil {
					return "", fmt.Errorf("tool %s: invalid arguments: %w", name, err)
				}
			}
			out, err := fn(ctx, in)
			if err != nil {
				return "", err
			}
			if s, ok := any(out).(string); ok {
				return s, nil
			}
			b, err := json.Marshal(out)
			if err != nil {
				return "", fmt.Errorf("tool %s: encode result: %w", name, err)
			}
			return string(b), nil
		},
	}, nil
}

// MustNew is like [New] but panics on error. Use it for tools declared at
// package level, where a bad schema is a programming error.
func MustNew[In, Out any](name, description string, fn func(ctx context.Context, in In) (Out, error)) Tool {
	t, err := New(name, description, fn)
	if err != nil {
		panic(err)
	}
	return t
}

// Set is a collection of tools that provides both the definitions to send
// to the model and an [Executor] that runs them.
type Set struct {
	tools  []Tool
	byName map[string]Func
}

// NewSet collects tools into a Set. When two tools share a name, the later
// one wins.
//
// Parameter schemas built by [New] follow encoding/json: the json tag names
// a property and omitempty/omitzero (or a pointer field) makes it optional.
// A `description` tag documents a field, and a `jsonschema` tag adds
// keywords: enum=a|b, minimum=N, maximum=N, minLength=N, maxLength=N,
// minItems=N, maxItems=N, format=F, required, optional.
func NewSet(tools ...Tool) *Set {
	s := &Set{byName: map[string]Func{}}
	for _, t := range tools {
		s.Add(t)
	}
	return s
}

// Add registers a tool, replacing any tool with the same name.
func (s *Set) Add(t Tool) {
	if _, exists := s.byName[t.Definition.Name]; exists {
		for i := range s.tools {
			if s.tools[i].Definition.Name == t.Definition.Name {
				s.tools[i] = t
			}
		}
	} else {
		s.tools = append(s.tools, t)
	}
	s.byName[t.Definition.Name] = t.Func
}

// Definitions returns the tool definitions, in registration order, for
// CompletionRequest.Tools.
func (s *Set) Definitions() []langrails.ToolDefinition {
	defs := make([]langrails.ToolDefinition, len(s.tools))
	for i, t := range s.tools {
		defs[i] = t.Definition
	}
	return defs
}

// Execute runs the named tool. It implements [Executor].
func (s *Set) Execute(ctx context.Context, name string, arguments string) (string, error) {
	fn, ok := s.byName[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return fn(ctx, arguments)
}
