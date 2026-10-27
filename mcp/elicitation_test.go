package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/promptrails/langrails/graph"
)

// newElicitServer asks for an environment name during tools/call, over SSE,
// and reports the answer it got as the tool result.
func newElicitServer(t *testing.T, capability *bool) *httptest.Server {
	t.Helper()
	answers := make(chan incoming, 1)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg incoming
		_ = json.NewDecoder(r.Body).Decode(&msg)
		switch {
		case msg.Method == "" && len(msg.ID) > 0: // the client's answer
			answers <- msg
			w.WriteHeader(http.StatusAccepted)
		case len(msg.ID) == 0:
			w.WriteHeader(http.StatusAccepted)
		case msg.Method == "initialize":
			*capability = strings.Contains(string(msg.Params), `"elicitation"`)
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{}}`, msg.ID)
		case msg.Method == "tools/list":
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":{"tools":[{"name":"deploy"}]}}`, msg.ID)
		case msg.Method == "tools/call":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, `data: {"jsonrpc":"2.0","id":7,"method":"elicitation/create","params":{"message":"Which environment?","requestedSchema":{"type":"object","properties":{"env":{"type":"string"}}}}}`+"\n\n")
			w.(http.Flusher).Flush()
			a := <-answers
			text, _ := json.Marshal(string(a.Result))
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%s,\"result\":{\"content\":[{\"type\":\"text\",\"text\":%s}]}}\n\n", msg.ID, text)
		}
	}))
}

func TestElicitation_Handler(t *testing.T) {
	var announced bool
	srv := newElicitServer(t, &announced)
	defer srv.Close()

	var asked ElicitRequest
	c, err := NewClient(srv.URL, WithElicitation(func(_ context.Context, req ElicitRequest) (ElicitResult, error) {
		asked = req
		return AcceptElicitation(map[string]any{"env": "prod"}), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !announced {
		t.Error("elicitation capability not announced")
	}
	out, err := c.Execute(context.Background(), "deploy", "{}")
	if err != nil {
		t.Fatal(err)
	}
	if asked.Message != "Which environment?" || !strings.Contains(string(asked.Schema), `"env"`) {
		t.Errorf("asked = %+v", asked)
	}
	if out != `{"action":"accept","content":{"env":"prod"}}` {
		t.Errorf("server got %s", out)
	}
}

func TestElicitation_HandlerError(t *testing.T) {
	var announced bool
	srv := newElicitServer(t, &announced)
	defer srv.Close()

	sentinel := errors.New("no human around")
	c, err := NewClient(srv.URL, WithElicitation(func(context.Context, ElicitRequest) (ElicitResult, error) {
		return ElicitResult{}, sentinel
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Execute(context.Background(), "deploy", "{}"); !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

func TestElicitation_NotAnnouncedWithoutHandler(t *testing.T) {
	var announced bool
	srv := newElicitServer(t, &announced)
	defer srv.Close()
	if _, err := NewClient(srv.URL); err != nil {
		t.Fatal(err)
	}
	if announced {
		t.Error("capability announced without a handler")
	}
}

func TestAwaitElicitation(t *testing.T) {
	var announced bool
	srv := newElicitServer(t, &announced)
	defer srv.Close()
	c, err := NewClient(srv.URL, WithElicitation(AwaitElicitation()))
	if err != nil {
		t.Fatal(err)
	}

	g := graph.New[string]()
	g.AddNode("deploy", func(ctx context.Context, _ string) (string, error) {
		return c.Execute(ctx, "deploy", "{}")
	})
	g.SetEntryPoint("deploy")
	g.AddEdge("deploy", graph.END)
	opts := []graph.Option[string]{
		graph.WithCheckpointer[string](graph.NewMemoryCheckpointer[string]()),
		graph.WithThreadID[string]("t"),
	}

	res, err := g.Run(context.Background(), "", opts...)
	if err != nil || res.Interruption == nil {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	p := res.Interruption.Payload.(PendingElicitation)
	if p.Request.Message != "Which environment?" {
		t.Fatalf("payload = %+v", p)
	}
	res, err = g.Resume(context.Background(), append(opts, graph.WithResumeValue[string](
		p.Answered.With(p, AcceptElicitation(map[string]any{"env": "staging"}))))...)
	if err != nil || res.Interruption != nil || !strings.Contains(res.State, `"staging"`) {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestElicitErrsAttribution(t *testing.T) {
	var e elicitErrs
	_, a := e.begin(context.Background())
	e.record(context.Background(), errors.New("only call")) // stdio: no call context
	if a.get() == nil {
		t.Fatal("error not attributed to the only call in flight")
	}
	_, b := e.begin(context.Background())
	_, c := e.begin(context.Background())
	e.record(context.Background(), errors.New("ambiguous"))
	if b.get() != nil || c.get() != nil {
		t.Fatal("error attributed with several calls in flight")
	}
}
