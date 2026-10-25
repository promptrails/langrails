package mcp

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestToolAnnotations(t *testing.T) {
	srv := newRPCServer(t, func(method string, _ json.RawMessage) (any, *jsonRPCError) {
		if method != "tools/list" {
			return nil, &jsonRPCError{Code: -32601, Message: "method not found"}
		}
		return map[string]any{"tools": []map[string]any{
			{"name": "read_file", "annotations": map[string]any{"title": "Read file", "readOnlyHint": true}},
			{"name": "delete_file", "annotations": map[string]any{"destructiveHint": true, "idempotentHint": true}},
			{"name": "append_log", "annotations": map[string]any{"readOnlyHint": false, "destructiveHint": false, "openWorldHint": false}},
			{"name": "unannotated"},
		}}, nil
	})
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	tools := c.Tools()
	if len(tools) != 4 || tools[0].Annotations.Title != "Read file" || !tools[0].Annotations.ReadOnly() {
		t.Fatalf("tools = %+v", tools)
	}
	if !tools[1].Annotations.Idempotent() || tools[2].Annotations.OpenWorld() || !tools[3].Annotations.OpenWorld() {
		t.Errorf("hints = %+v", tools)
	}
	if got, want := c.DestructiveTools(), []string{"delete_file", "unannotated"}; !slices.Equal(got, want) {
		t.Errorf("DestructiveTools() = %v, want %v", got, want)
	}
	if got := c.ToolNames(func(t Tool) bool { return t.Annotations.ReadOnly() }); !slices.Equal(got, []string{"read_file"}) {
		t.Errorf("read-only = %v", got)
	}
}
