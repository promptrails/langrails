package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rpcServer dispatches each JSON-RPC method to a per-method response builder.
// A builder returning a non-nil *jsonRPCError emits an RPC error frame.
func rpcServer(t *testing.T, methods map[string]func(req jsonRPCRequest) (any, *jsonRPCError)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		build, ok := methods[req.Method]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID,
				Error: &jsonRPCError{Code: -32601, Message: "method not found"}})
			return
		}
		result, rpcErr := build(req)
		if rpcErr != nil {
			_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr})
			return
		}
		resultBytes, _ := json.Marshal(result)
		_ = json.NewEncoder(w).Encode(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: resultBytes})
	}))
}

// okInitialize is a minimal successful initialize response.
func okInitialize(jsonRPCRequest) (any, *jsonRPCError) {
	return map[string]any{"protocolVersion": "2025-03-26"}, nil
}

func TestNewClient_InitializeRPCError(t *testing.T) {
	server := rpcServer(t, map[string]func(jsonRPCRequest) (any, *jsonRPCError){
		"initialize": func(jsonRPCRequest) (any, *jsonRPCError) {
			return nil, &jsonRPCError{Code: -32603, Message: "init failed"}
		},
	})
	defer server.Close()

	if _, err := NewClient(server.URL); err == nil {
		t.Fatal("expected NewClient to fail when initialize errors")
	}
}

func TestNewClient_DiscoverParseError(t *testing.T) {
	server := rpcServer(t, map[string]func(jsonRPCRequest) (any, *jsonRPCError){
		"initialize": okInitialize,
		"tools/list": func(jsonRPCRequest) (any, *jsonRPCError) {
			// A bare string is valid JSON but not a tool list → parse error.
			return "not-a-tool-list", nil
		},
	})
	defer server.Close()

	if _, err := NewClient(server.URL); err == nil {
		t.Fatal("expected NewClient to fail when tools list is malformed")
	}
}

func TestNewClient_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	if _, err := NewClient(server.URL); err == nil {
		t.Fatal("expected NewClient to fail on non-200 status")
	}
}

func TestNewClient_NetworkError(t *testing.T) {
	server := rpcServer(t, nil)
	url := server.URL
	server.Close() // close immediately so the connection is refused

	if _, err := NewClient(url); err == nil {
		t.Fatal("expected NewClient to fail when the server is unreachable")
	}
}

// Non-JSON arguments must be wrapped as {"input": <raw>} before being sent.
func TestClient_Execute_NonJSONArgs(t *testing.T) {
	var gotInput any
	server := rpcServer(t, map[string]func(jsonRPCRequest) (any, *jsonRPCError){
		"initialize": okInitialize,
		"tools/list": func(jsonRPCRequest) (any, *jsonRPCError) {
			return map[string]any{"tools": []any{}}, nil
		},
		"tools/call": func(req jsonRPCRequest) (any, *jsonRPCError) {
			params, _ := req.Params.(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			gotInput = args["input"]
			return map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}, nil
		},
	})
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if _, err := client.Execute(context.Background(), "echo", "plain text"); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotInput != "plain text" {
		t.Errorf("server received arguments.input = %v, want %q", gotInput, "plain text")
	}
}

func TestClient_Execute_CallError(t *testing.T) {
	server := rpcServer(t, map[string]func(jsonRPCRequest) (any, *jsonRPCError){
		"initialize": okInitialize,
		"tools/list": func(jsonRPCRequest) (any, *jsonRPCError) {
			return map[string]any{"tools": []any{}}, nil
		},
		"tools/call": func(jsonRPCRequest) (any, *jsonRPCError) {
			return nil, &jsonRPCError{Code: -32000, Message: "tool blew up"}
		},
	})
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	if _, err := client.Execute(context.Background(), "boom", `{"x":1}`); err == nil {
		t.Fatal("expected Execute to return the server's RPC error")
	}
}

// When the result has no text content, Execute falls back to the raw result.
func TestClient_Execute_NoTextContent(t *testing.T) {
	server := rpcServer(t, map[string]func(jsonRPCRequest) (any, *jsonRPCError){
		"initialize": okInitialize,
		"tools/list": func(jsonRPCRequest) (any, *jsonRPCError) {
			return map[string]any{"tools": []any{}}, nil
		},
		"tools/call": func(jsonRPCRequest) (any, *jsonRPCError) {
			return map[string]any{"content": []map[string]any{{"type": "image", "text": ""}}}, nil
		},
	})
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	out, err := client.Execute(context.Background(), "img", `{}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == "" {
		t.Error("expected raw result fallback, got empty string")
	}
}
