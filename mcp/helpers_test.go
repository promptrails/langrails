package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rpcHandler answers one JSON-RPC method call for a test server.
type rpcHandler func(method string, params json.RawMessage) (any, *jsonRPCError)

// newRPCServer serves JSON-RPC over plain JSON responses, answering
// initialize itself and every other request with handle.
func newRPCServer(t *testing.T, handle rpcHandler) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.ID == nil { // notification
			w.WriteHeader(http.StatusAccepted)
			return
		}
		var result any
		var rpcErr *jsonRPCError
		if req.Method == "initialize" {
			result = map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}}
		} else {
			result, rpcErr = handle(req.Method, req.Params)
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": *req.ID}
		if rpcErr != nil {
			resp["error"] = rpcErr
		} else {
			resp["result"] = result
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}
