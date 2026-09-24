package compat

import (
	"encoding/json"
	"testing"
)

func TestEnforceStrictSchemaRecursive(t *testing.T) {
	in := `{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"unit": {"type": "string", "enum": ["c", "f"]},
			"address": {
				"type": "object",
				"properties": {"city": {"type": "string"}, "zip": {"type": "string"}},
				"required": ["city"]
			},
			"items": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "integer"}}}},
			"tags": {"type": "object", "additionalProperties": {"type": "string"}}
		},
		"required": ["name", "address", "items", "tags"]
	}`
	var got map[string]any
	if err := json.Unmarshal(enforceStrictSchema([]byte(in)), &got); err != nil {
		t.Fatal(err)
	}
	want := `{"additionalProperties":false,"properties":{` +
		`"address":{"additionalProperties":false,"properties":{"city":{"type":"string"},"zip":{"type":["string","null"]}},"required":["city","zip"],"type":"object"},` +
		`"items":{"items":{"additionalProperties":false,"properties":{"id":{"type":["integer","null"]}},"required":["id"],"type":"object"},"type":"array"},` +
		`"name":{"type":"string"},` +
		`"tags":{"additionalProperties":{"type":"string"},"type":"object"},` +
		`"unit":{"enum":["c","f",null],"type":["string","null"]}},` +
		`"required":["address","items","name","tags","unit"],"type":"object"}`
	b, _ := json.Marshal(got)
	if string(b) != want {
		t.Errorf("strict schema:\n got %s\nwant %s", b, want)
	}
}

func TestEnforceStrictSchemaInvalidPassthrough(t *testing.T) {
	if got := string(enforceStrictSchema([]byte("not json"))); got != "not json" {
		t.Errorf("got %s", got)
	}
}
