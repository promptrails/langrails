package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestHelperMCPServer is not a real test: when run as a child process with
// MCP_FAKE_SERVER=1 it acts as a stdio MCP server.
func TestHelperMCPServer(t *testing.T) {
	if os.Getenv("MCP_FAKE_SERVER") != "1" {
		return
	}
	fakeServer(os.Stdin, os.Stdout)
	os.Exit(0)
}

func fakeServer(in *os.File, out *os.File) {
	fmt.Fprintln(out, "not json: servers sometimes log to stdout")
	// A server-initiated request the client must answer.
	fmt.Fprintln(out, `{"jsonrpc":"2.0","id":"srv-1","method":"ping"}`)

	sc := bufio.NewScanner(in)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage        `json:"id"`
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue // notifications and our ping answer
		}
		if req.Method == "" {
			continue
		}

		var result interface{}
		switch req.Method {
		case "initialize":
			result = map[string]interface{}{"protocolVersion": protocolVersion, "capabilities": map[string]interface{}{}}
		case "tools/list":
			if req.Params["cursor"] == nil {
				result = map[string]interface{}{
					"tools":      []map[string]interface{}{{"name": "echo", "inputSchema": map[string]interface{}{"type": "object"}}},
					"nextCursor": "p2",
				}
			} else {
				result = map[string]interface{}{"tools": []map[string]interface{}{{"name": "fail"}}}
			}
		case "tools/call":
			args, _ := json.Marshal(req.Params["arguments"])
			if req.Params["name"] == "fail" {
				result = map[string]interface{}{"isError": true, "content": []map[string]string{{"type": "text", "text": "nope"}}}
			} else {
				result = map[string]interface{}{"content": []map[string]string{
					{"type": "text", "text": "echo"}, {"type": "text", "text": string(args)},
				}}
			}
		case "slow":
			time.Sleep(time.Second)
			result = map[string]interface{}{}
		case "exit":
			os.Exit(0)
		default:
			fmt.Fprintf(out, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32601,"message":"no %s"}}`+"\n", req.ID, req.Method)
			continue
		}
		b, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": req.ID, "result": result})
		fmt.Fprintln(out, string(b))
	}
}

func newStdioTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewStdioClient(os.Args[0], []string{"-test.run=^TestHelperMCPServer$"},
		WithEnv("MCP_FAKE_SERVER=1"), WithSetupTimeout(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestStdioClient_ToolsRoundTrip(t *testing.T) {
	c := newStdioTestClient(t)

	defs := c.ToolDefinitions()
	if len(defs) != 2 || defs[0].Name != "echo" || defs[1].Name != "fail" {
		t.Fatalf("paginated tools = %+v", defs)
	}

	out, err := c.Execute(context.Background(), "echo", `{"x":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "echo\n{\"x\":1}" {
		t.Errorf("out = %q", out)
	}

	if _, err := c.Execute(context.Background(), "fail", `{}`); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Errorf("isError result: err = %v", err)
	}
}

func TestStdioClient_ConcurrentCalls(t *testing.T) {
	c := newStdioTestClient(t)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			out, err := c.Execute(context.Background(), "echo", fmt.Sprintf(`{"i":%d}`, i))
			if err != nil || !strings.Contains(out, fmt.Sprintf(`"i":%d`, i)) {
				t.Errorf("call %d: %q, %v", i, out, err)
			}
		})
	}
	wg.Wait()
}

func TestStdioClient_ContextAndExit(t *testing.T) {
	c := newStdioTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.t.call(ctx, "slow", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("slow call: err = %v", err)
	}

	if _, err := c.t.call(context.Background(), "unknown", nil); err == nil || !strings.Contains(err.Error(), "-32601") {
		t.Errorf("rpc error: %v", err)
	}

	if _, err := c.t.call(context.Background(), "exit", nil); !errors.Is(err, errServerExited) {
		t.Errorf("exit: err = %v", err)
	}
	if _, err := c.Execute(context.Background(), "echo", "{}"); err == nil {
		t.Error("call after exit should fail")
	}
}

func TestStdioClient_StartFailure(t *testing.T) {
	if _, err := NewStdioClient("/definitely/not/a/binary", nil); err == nil {
		t.Error("expected start error")
	}
}

func TestHTTPClient_SessionResourcesPrompts(t *testing.T) {
	var sawSession, sawInitialized bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     *int64                 `json:"id"`
			Method string                 `json:"method"`
			Params map[string]interface{} `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Method != "initialize" && r.Header.Get(sessionHeader) == "sess-1" {
			sawSession = true
		}
		if req.ID == nil {
			sawInitialized = req.Method == "notifications/initialized"
			w.WriteHeader(http.StatusAccepted)
			return
		}

		var result interface{}
		switch req.Method {
		case "initialize":
			w.Header().Set(sessionHeader, "sess-1")
			result = map[string]interface{}{}
		case "tools/list":
			result = map[string]interface{}{"tools": []interface{}{}}
		case "resources/list":
			result = map[string]interface{}{"resources": []map[string]string{{"uri": "file:///a.txt", "name": "a"}}}
		case "resources/read":
			result = map[string]interface{}{"contents": []map[string]string{{"uri": req.Params["uri"].(string), "text": "hello"}}}
		case "prompts/list":
			result = map[string]interface{}{"prompts": []map[string]interface{}{
				{"name": "review", "arguments": []map[string]interface{}{{"name": "code", "required": true}}},
			}}
		case "prompts/get":
			args := req.Params["arguments"].(map[string]interface{})
			result = map[string]interface{}{"messages": []map[string]interface{}{
				{"role": "user", "content": map[string]string{"type": "text", "text": "Review: " + args["code"].(string)}},
				{"role": "user", "content": map[string]string{"type": "image", "data": "QUFB", "mimeType": "image/png"}},
				{"role": "user", "content": map[string]interface{}{"type": "resource", "resource": map[string]string{"uri": "x", "text": "ctx"}}},
			}}
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
	defer server.Close()

	c, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, err := c.ListResources(ctx)
	if err != nil || len(res) != 1 || res[0].URI != "file:///a.txt" {
		t.Fatalf("resources = %+v, %v", res, err)
	}
	contents, err := c.ReadResource(ctx, "file:///a.txt")
	if err != nil || len(contents) != 1 || contents[0].Text != "hello" {
		t.Fatalf("contents = %+v, %v", contents, err)
	}

	prompts, err := c.ListPrompts(ctx)
	if err != nil || len(prompts) != 1 || !prompts[0].Arguments[0].Required {
		t.Fatalf("prompts = %+v, %v", prompts, err)
	}
	msgs, err := c.GetPrompt(ctx, "review", map[string]string{"code": "x := 1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[0].Content != "Review: x := 1" ||
		msgs[1].ContentParts[0].ImageURL != "data:image/png;base64,QUFB" || msgs[2].Content != "ctx" {
		t.Errorf("messages = %+v", msgs)
	}

	if !sawSession {
		t.Error("session ID was not echoed")
	}
	if !sawInitialized {
		t.Error("notifications/initialized was not sent")
	}
}
