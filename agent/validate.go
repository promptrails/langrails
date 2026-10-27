package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
)

// ValidateArguments checks JSON tool arguments against a tool's parameter
// schema: they must be an object, carry every required property, and
// each property's value must have the declared type and, if one is given,
// be one of the enum values. When the schema sets additionalProperties to
// false, unknown properties are rejected too. Nested values are only
// type-checked at the top level. Use it to validate a reviewer's edit
// before resuming a paused run.
func ValidateArguments(schema json.RawMessage, arguments string) error {
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil || args == nil {
		return fmt.Errorf("arguments must be a JSON object")
	}
	if len(schema) == 0 {
		return nil
	}
	var s struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	if err := json.Unmarshal(schema, &s); err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}
	for _, name := range s.Required {
		if _, ok := args[name]; !ok {
			return fmt.Errorf("missing required property %q", name)
		}
	}
	for name, v := range args {
		raw, ok := s.Properties[name]
		if !ok {
			if s.AdditionalProperties != nil && !*s.AdditionalProperties {
				return fmt.Errorf("unknown property %q", name)
			}
			continue
		}
		if err := checkProperty(raw, v); err != nil {
			return fmt.Errorf("property %q: %w", name, err)
		}
	}
	return nil
}

func checkProperty(raw json.RawMessage, v any) error {
	var p struct {
		Type json.RawMessage `json:"type"`
		Enum []any           `json:"enum"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil // not a schema we can read; accept
	}
	var types []string
	if len(p.Type) > 0 {
		var one string
		if json.Unmarshal(p.Type, &one) == nil {
			types = []string{one}
		} else {
			_ = json.Unmarshal(p.Type, &types)
		}
	}
	if len(types) > 0 && !slices.ContainsFunc(types, func(t string) bool { return hasType(v, t) }) {
		return fmt.Errorf("want %v, got %s", types, jsonType(v))
	}
	if len(p.Enum) > 0 && !slices.ContainsFunc(p.Enum, func(e any) bool { return equalJSON(e, v) }) {
		return fmt.Errorf("value %v is not one of %v", v, p.Enum)
	}
	return nil
}

func hasType(v any, t string) bool {
	switch t {
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "number":
		_, ok := v.(float64)
		return ok
	default:
		return jsonType(v) == t
	}
}

func jsonType(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	default:
		return "object"
	}
}

func equalJSON(a, b any) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	return err1 == nil && err2 == nil && string(ja) == string(jb)
}
