package jsonschema

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type weatherArgs struct {
	City string `json:"city" description:"City name"`
	Unit string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit"`
	Days int    `json:"days" jsonschema:"minimum=1,maximum=14"`
	Note *string
	Skip string `json:"-"`
}

func TestForStruct(t *testing.T) {
	got, err := For[weatherArgs]()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"properties":{"city":{"type":"string","description":"City name"},` +
		`"unit":{"type":"string","enum":["celsius","fahrenheit"]},` +
		`"days":{"type":"integer","minimum":1,"maximum":14},` +
		`"Note":{"type":"string"}},` +
		`"type":"object","required":["city","days"]}`
	if string(got) != want {
		t.Errorf("schema:\n got %s\nwant %s", got, want)
	}
}

func TestNestedTypes(t *testing.T) {
	type item struct {
		Name string `json:"name"`
	}
	type order struct {
		Items    []item            `json:"items" jsonschema:"minItems=1"`
		Tags     map[string]string `json:"tags,omitempty"`
		At       time.Time         `json:"at"`
		Blob     []byte            `json:"blob,omitzero"`
		Anything any               `json:"anything,omitempty"`
		Raw      json.RawMessage   `json:"raw,omitempty"`
		Score    float64           `json:"score" jsonschema:"optional"`
	}
	s, err := Reflect(reflectTypeOf[order]())
	if err != nil {
		t.Fatal(err)
	}
	if s.Properties["items"].Type != "array" || s.Properties["items"].Items.Properties["name"].Type != "string" {
		t.Errorf("items: %+v", s.Properties["items"])
	}
	if *s.Properties["items"].MinItems != 1 {
		t.Error("minItems not applied")
	}
	if s.Properties["tags"].AdditionalProperties.Type != "string" {
		t.Error("map value schema missing")
	}
	if s.Properties["at"].Format != "date-time" {
		t.Error("time.Time should be a date-time string")
	}
	if s.Properties["blob"].Format != "byte" {
		t.Error("[]byte should be a base64 string")
	}
	if s.Properties["anything"].Type != "" || s.Properties["raw"].Type != "" {
		t.Error("any/RawMessage should be unconstrained")
	}
	if strings.Join(s.Required, ",") != "items,at" {
		t.Errorf("required = %v", s.Required)
	}
}

func TestEmbeddedFlattened(t *testing.T) {
	type Base struct {
		ID string `json:"id"`
	}
	type withBase struct {
		Base
		Name string `json:"name"`
	}
	got, err := For[withBase]()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"properties":{"id":`) || !strings.Contains(string(got), `"required":["id","name"]`) {
		t.Errorf("embedded not flattened: %s", got)
	}
}

func TestEmptyStructHasProperties(t *testing.T) {
	got, err := For[struct{}]()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"properties":{},"type":"object"}` {
		t.Errorf("got %s", got)
	}
}

type node struct {
	Children []node `json:"children"`
}

func TestErrors(t *testing.T) {
	if _, err := For[node](); err == nil || !strings.Contains(err.Error(), "recursive") {
		t.Errorf("recursive type: err = %v", err)
	}
	if _, err := For[map[int]string](); err == nil {
		t.Error("non-string map key should fail")
	}
	if _, err := For[chan int](); err == nil {
		t.Error("chan should fail")
	}
	type badTag struct {
		N int `json:"n" jsonschema:"minimum=x"`
	}
	if _, err := For[badTag](); err == nil {
		t.Error("bad minimum should fail")
	}
	type badEnum struct {
		N int `json:"n" jsonschema:"enum=1|two"`
	}
	if _, err := For[badEnum](); err == nil {
		t.Error("non-integer enum on int should fail")
	}
	type unknown struct {
		N int `json:"n" jsonschema:"nope"`
	}
	if _, err := For[unknown](); err == nil {
		t.Error("unknown key should fail")
	}
}

func TestTypedEnums(t *testing.T) {
	type e struct {
		I int     `json:"i" jsonschema:"enum=1|2"`
		F float64 `json:"f" jsonschema:"enum=0.5|1.5"`
		B bool    `json:"b" jsonschema:"enum=true"`
	}
	got, err := For[e]()
	if err != nil {
		t.Fatal(err)
	}
	for _, frag := range []string{`"enum":[1,2]`, `"enum":[0.5,1.5]`, `"enum":[true]`} {
		if !strings.Contains(string(got), frag) {
			t.Errorf("missing %s in %s", frag, got)
		}
	}
}

func reflectTypeOf[T any]() reflect.Type { return reflect.TypeFor[T]() }
