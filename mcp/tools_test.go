package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

type fakeTokens struct {
	tokens      []string
	invalidated int
}

func (f *fakeTokens) Token(context.Context) (string, error) { return f.tokens[f.invalidated], nil }
func (f *fakeTokens) InvalidateToken()                      { f.invalidated++ }

func TestTokenSourceRetriesOn401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fresh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req jsonRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ID == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"tools": []any{}}})
	}))
	defer srv.Close()

	ts := &fakeTokens{tokens: []string{"stale", "fresh"}}
	if _, err := NewClient(srv.URL, WithBearerToken("ignored"), WithTokenSource(ts)); err != nil {
		t.Fatal(err)
	}
	if ts.invalidated != 1 {
		t.Errorf("invalidated %d times", ts.invalidated)
	}
}
