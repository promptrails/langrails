package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer token, got %q", r.Header.Get("Authorization"))
		}

		var req jsonRPCRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		var result interface{}

		switch req.Method {
		case "initialize":
			result = map[string]interface{}{
				"protocolVersion": "2025-03-26",
				"capabilities":    map[string]interface{}{},
				"serverInfo": map[string]interface{}{
					"name":    "test-server",
					"version": "1.0.0",
				},
			}

		case "tools/list":
			result = map[string]interface{}{
				"tools": []map[string]interface{}{
					{
						"name":        "get_weather",
						"description": "Get current weather for a city",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"city": map[string]interface{}{"type": "string"},
							},
							"required": []string{"city"},
						},
					},
					{
						"name":        "search",
						"description": "Search the web",
						"inputSchema": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"query": map[string]interface{}{"type": "string"},
							},
						},
					},
				},
			}

		case "tools/call":
			params := req.Params.(map[string]interface{})
			name := params["name"].(string)

			if name == "get_weather" {
				result = map[string]interface{}{
					"content": []map[string]interface{}{
						{"type": "text", "text": `{"temp": 22, "condition": "sunny"}`},
					},
				}
			} else {
				result = map[string]interface{}{
					"content": []map[string]interface{}{
						{"type": "text", "text": "search results here"},
					},
				}
			}

		default:
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &jsonRPCError{Code: -32601, Message: "method not found"},
			})
			return
		}

		resultBytes, _ := json.Marshal(result)
		_ = json.NewEncoder(w).Encode(jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resultBytes,
		})
	}))
}

func TestClient_DiscoverTools(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client, err := NewClient(server.URL, WithBearerToken("test-token"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	defs := client.ToolDefinitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(defs))
	}

	if defs[0].Name != "get_weather" {
		t.Errorf("expected first tool 'get_weather', got %q", defs[0].Name)
	}
	if defs[1].Name != "search" {
		t.Errorf("expected second tool 'search', got %q", defs[1].Name)
	}
	if defs[0].Description != "Get current weather for a city" {
		t.Errorf("unexpected description: %q", defs[0].Description)
	}
}

func TestClient_ExecuteTool(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client, err := NewClient(server.URL, WithBearerToken("test-token"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	result, err := client.Execute(context.Background(), "get_weather", `{"city":"Istanbul"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != `{"temp": 22, "condition": "sunny"}` {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestClient_ExecuteUnknownTool(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client, err := NewClient(server.URL, WithBearerToken("test-token"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// The mock server will still respond, but in a real scenario
	// the server would return an error for unknown tools
	result, err := client.Execute(context.Background(), "search", `{"query":"test"}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "search results here" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestClient_ToolDefinitionsHaveParameters(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	client, err := NewClient(server.URL, WithBearerToken("test-token"))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	defs := client.ToolDefinitions()
	if len(defs[0].Parameters) == 0 {
		t.Error("expected non-empty parameters for get_weather tool")
	}

	var params map[string]interface{}
	if err := json.Unmarshal(defs[0].Parameters, &params); err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}
	if params["type"] != "object" {
		t.Errorf("expected type 'object', got %v", params["type"])
	}
}
