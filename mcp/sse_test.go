package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// newSSEServer answers tools/call with an SSE stream that first sends a
// notification and a ping request, waits for the client's answer to the
// ping, and only then sends the call's response.
func newSSEServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var mu sync.Mutex
	var log []string
	pong := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		var msg incoming
		_ = json.NewDecoder(r.Body).Decode(&msg)
		mu.Lock()
		log = append(log, msg.Method+string(msg.ID))
		mu.Unlock()

		switch {
		case msg.Method == "" && string(msg.ID) == `"srv-1"`: // the client's pong
			if string(msg.Result) != "{}" {
				t.Errorf("pong result = %s", msg.Result)
			}
			pong <- struct{}{}
			w.WriteHeader(http.StatusAccepted)
		case len(msg.ID) == 0:
			w.WriteHeader(http.StatusAccepted)
		case msg.Method == "initialize":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{}}`, msg.ID)
		case msg.Method == "tools/list":
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"tools\":[{\"name\":\"echo\"}]}}\n\n", msg.ID)
		case msg.Method == "tools/call":
			w.Header().Set("Content-Type", "text/event-stream")
			fl := w.(http.Flusher)
			fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\n")
			fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":\"srv-1\",\"method\":\"ping\"}\n\n")
			fl.Flush()
			<-pong
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}\n\n", msg.ID)
		}
	}))
	return srv, &log
}

func TestHTTP_SSEResponses(t *testing.T) {
	srv, log := newSSEServer(t)
	defer srv.Close()

	c, err := NewClient(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tools()) != 1 {
		t.Fatalf("tools = %+v", c.Tools())
	}
	out, err := c.Execute(t.Context(), "echo", "{}")
	if err != nil || out != "hi" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if !strings.Contains(strings.Join(*log, ","), `"srv-1"`) {
		t.Errorf("ping was not answered: %v", *log)
	}
}

func TestHTTP_SSEStreamEndsEarly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg incoming
		_ = json.NewDecoder(r.Body).Decode(&msg)
		if len(msg.ID) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"jsonrpc\":\"2.0\",\"id\":999,\"result\":{}}\n\n")
	}))
	defer srv.Close()

	if _, err := NewClient(srv.URL); err == nil || !strings.Contains(err.Error(), errNoResponse.Error()) {
		t.Fatalf("err = %v", err)
	}
}
