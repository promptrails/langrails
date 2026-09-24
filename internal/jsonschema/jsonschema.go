// Package jsonschema derives JSON Schema documents from Go types by
// reflection, so tool parameters and structured-output schemas can be
// declared as ordinary structs instead of hand-written JSON.
//
// The output deliberately stays inside the subset every supported provider
// accepts: no $ref/$defs, no additionalProperties, no oneOf. Recursive types
// are therefore rejected rather than expanded.
//
// Field names and omission follow encoding/json: the `json` tag names a
// property, "-" skips it, and embedded structs are flattened. A property is
// required unless its json tag has omitempty/omitzero or the field is a
// pointer; `jsonschema:"required"` and `jsonschema:"optional"` override that.
//
// Descriptions come from the `description` tag. Other keywords come from the
// `jsonschema` tag as comma-separated key=value pairs:
//
//	type Args struct {
//	    City  string `json:"city" description:"City name"`
//	    Unit  string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit"`
//	    Days  int    `json:"days" jsonschema:"minimum=1,maximum=14"`
//	}
package jsonschema

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Schema is a JSON Schema node. Only the keywords this package emits are
// modeled; it marshals to the standard JSON form.
type Schema struct {
	Type                 string             `json:"type,omitempty"`
	Description          string             `json:"description,omitempty"`
	Format               string             `json:"format,omitempty"`
	Enum                 []any              `json:"enum,omitempty"`
	Properties           map[string]*Schema `json:"properties,omitempty"`
	Required             []string           `json:"required,omitempty"`
	Items                *Schema            `json:"items,omitempty"`
	AdditionalProperties *Schema            `json:"additionalProperties,omitempty"`
	Minimum              *float64           `json:"minimum,omitempty"`
	Maximum              *float64           `json:"maximum,omitempty"`
	MinLength            *int               `json:"minLength,omitempty"`
	MaxLength            *int               `json:"maxLength,omitempty"`
	MinItems             *int               `json:"minItems,omitempty"`
	MaxItems             *int               `json:"maxItems,omitempty"`

	// order keeps properties in struct-field order when marshaling, so the
	// schema a model sees matches the order the fields were declared in.
	order []string
}

// MarshalJSON writes properties in declaration order; a plain map would sort
// them alphabetically. An object with no fields still gets "properties": {},
// which some providers require for a parameterless tool.
func (s *Schema) MarshalJSON() ([]byte, error) {
	type plain Schema
	if s.Properties == nil {
		return json.Marshal((*plain)(s))
	}
	props := s.Properties
	cp := *s
	cp.Properties = nil
	head, err := json.Marshal((*plain)(&cp))
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(`{"properties":{`)
	for i, name := range s.order {
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(name)
		val, err := json.Marshal(props[name])
		if err != nil {
			return nil, err
		}
		b.Write(key)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	if len(head) > 2 { // not "{}"
		b.WriteByte(',')
		b.Write(head[1:])
	} else {
		b.WriteByte('}')
	}
	return []byte(b.String()), nil
}

// For returns the JSON Schema for T.
func For[T any]() (json.RawMessage, error) {
	return FromType(reflect.TypeFor[T]())
}

// FromType returns the JSON Schema for t.
func FromType(t reflect.Type) (json.RawMessage, error) {
	s, err := Reflect(t)
	if err != nil {
		return nil, err
	}
	return json.Marshal(s)
}

// Reflect builds the Schema tree for t.
func Reflect(t reflect.Type) (*Schema, error) {
	return reflectType(t, map[reflect.Type]bool{})
}

var (
	timeType    = reflect.TypeFor[time.Time]()
	rawJSONType = reflect.TypeFor[json.RawMessage]()
)

func reflectType(t reflect.Type, seen map[reflect.Type]bool) (*Schema, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t {
	case timeType:
		return &Schema{Type: "string", Format: "date-time"}, nil
	case rawJSONType:
		return &Schema{}, nil
	}

	switch t.Kind() {
	case reflect.String:
		return &Schema{Type: "string"}, nil
	case reflect.Bool:
		return &Schema{Type: "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &Schema{Type: "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return &Schema{Type: "number"}, nil
	case reflect.Interface:
		return &Schema{}, nil
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			// encoding/json writes []byte as a base64 string.
			return &Schema{Type: "string", Format: "byte"}, nil
		}
		items, err := reflectType(t.Elem(), seen)
		if err != nil {
			return nil, err
		}
		return &Schema{Type: "array", Items: items}, nil
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("jsonschema: map key must be a string, got %s", t.Key())
		}
		val, err := reflectType(t.Elem(), seen)
		if err != nil {
			return nil, err
		}
		return &Schema{Type: "object", AdditionalProperties: val}, nil
	case reflect.Struct:
		if seen[t] {
			return nil, fmt.Errorf("jsonschema: recursive type %s is not supported", t)
		}
		seen[t] = true
		defer delete(seen, t)
		s := &Schema{Type: "object", Properties: map[string]*Schema{}}
		if err := addFields(s, t, seen); err != nil {
			return nil, err
		}
		return s, nil
	default:
		return nil, fmt.Errorf("jsonschema: unsupported kind %s (%s)", t.Kind(), t)
	}
}

func addFields(s *Schema, t reflect.Type, seen map[reflect.Type]bool) error {
	for i := range t.NumField() {
		f := t.Field(i)
		name, opts, skip := jsonName(f)
		if skip {
			continue
		}

		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				if err := addFields(s, ft, seen); err != nil {
					return err
				}
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}

		prop, err := reflectType(f.Type, seen)
		if err != nil {
			return fmt.Errorf("field %s: %w", f.Name, err)
		}
		prop.Description = f.Tag.Get("description")

		required := !opts["omitempty"] && !opts["omitzero"] && f.Type.Kind() != reflect.Pointer
		if err := applyTag(prop, f.Tag.Get("jsonschema"), &required); err != nil {
			return fmt.Errorf("field %s: %w", f.Name, err)
		}

		if _, dup := s.Properties[name]; !dup {
			s.order = append(s.order, name)
		}
		s.Properties[name] = prop
		if required {
			s.Required = append(s.Required, name)
		}
	}
	return nil
}

// jsonName reads the json tag. An anonymous field without a tag name
// returns "" so the caller can flatten it.
func jsonName(f reflect.StructField) (string, map[string]bool, bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", nil, true
	}
	parts := strings.Split(tag, ",")
	opts := map[string]bool{}
	for _, p := range parts[1:] {
		opts[p] = true
	}
	if !f.Anonymous && !f.IsExported() {
		return "", nil, true
	}
	return parts[0], opts, false
}

func applyTag(s *Schema, tag string, required *bool) error {
	if tag == "" {
		return nil
	}
	for _, kv := range strings.Split(tag, ",") {
		key, val, _ := strings.Cut(strings.TrimSpace(kv), "=")
		switch key {
		case "required":
			*required = true
		case "optional":
			*required = false
		case "format":
			s.Format = val
		case "enum":
			for _, v := range strings.Split(val, "|") {
				ev, err := enumValue(s.Type, v)
				if err != nil {
					return err
				}
				s.Enum = append(s.Enum, ev)
			}
		case "minimum", "maximum":
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return fmt.Errorf("jsonschema: %s=%q: %w", key, val, err)
			}
			if key == "minimum" {
				s.Minimum = &n
			} else {
				s.Maximum = &n
			}
		case "minLength", "maxLength", "minItems", "maxItems":
			n, err := strconv.Atoi(val)
			if err != nil {
				return fmt.Errorf("jsonschema: %s=%q: %w", key, val, err)
			}
			switch key {
			case "minLength":
				s.MinLength = &n
			case "maxLength":
				s.MaxLength = &n
			case "minItems":
				s.MinItems = &n
			case "maxItems":
				s.MaxItems = &n
			}
		default:
			return fmt.Errorf("jsonschema: unknown tag key %q", key)
		}
	}
	return nil
}

func enumValue(typ, v string) (any, error) {
	switch typ {
	case "integer":
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("jsonschema: enum value %q is not an integer", v)
		}
		return n, nil
	case "number":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("jsonschema: enum value %q is not a number", v)
		}
		return n, nil
	case "boolean":
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("jsonschema: enum value %q is not a boolean", v)
		}
		return b, nil
	default:
		return v, nil
	}
}
