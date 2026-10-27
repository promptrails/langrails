package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/graph"
	"github.com/promptrails/langrails/tools"
)

var emailSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"to": {"type": "string"},
		"priority": {"type": "integer", "enum": [1, 2, 3]}
	},
	"required": ["to"],
	"additionalProperties": false
}`)

func TestValidateArguments(t *testing.T) {
	tests := []struct {
		args string
		want string
	}{
		{`{"to": "a@b.c"}`, ""},
		{`{"to": "a@b.c", "priority": 2}`, ""},
		{`[]`, "JSON object"},
		{`{}`, `missing required property "to"`},
		{`{"to": 5}`, "want [string]"},
		{`{"to": "x", "priority": 1.5}`, "want [integer]"},
		{`{"to": "x", "priority": 7}`, "not one of"},
		{`{"to": "x", "cc": "y"}`, `unknown property "cc"`},
	}
	for _, tt := range tests {
		err := ValidateArguments(emailSchema, tt.args)
		if tt.want == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tt.args, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.args, err, tt.want)
		}
	}
	if err := ValidateArguments(nil, `{"any": 1}`); err != nil {
		t.Errorf("no schema: %v", err)
	}
	if err := ValidateArguments(json.RawMessage(`{"properties": {"n": {"type": ["string", "null"]}}}`), `{"n": null}`); err != nil {
		t.Errorf("type list: %v", err)
	}
}

func recordingExec(got *[]string) tools.Executor {
	return tools.NewMap(map[string]tools.Func{
		"send_email": func(_ context.Context, args string) (string, error) {
			*got = append(*got, args)
			return "sent", nil
		},
	})
}

func TestHITL_Edit(t *testing.T) {
	var ran []string
	defs := []langrails.ToolDefinition{{Name: "send_email", Parameters: emailSchema}}
	edit := `{"to": "boss@x.com"}`
	h := NewHumanInLoop(recordingExec(&ran), func(context.Context, langrails.ToolCall) (Decision, error) {
		return Edit(edit), nil
	}, WithToolSchemas(defs))

	if _, err := h.Execute(context.Background(), "send_email", `{"to": "all@x.com"}`); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || ran[0] != edit {
		t.Fatalf("ran with %v", ran)
	}

	edit = `{"to": 42}`
	_, err := h.Execute(context.Background(), "send_email", `{"to": "all@x.com"}`)
	if !errors.Is(err, ErrInvalidEdit) || !aborts(err) || len(ran) != 1 {
		t.Fatalf("err = %v, ran = %v", err, ran)
	}
}

func TestAgent_ApproverErrorEndsRun(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "send_email", Arguments: "{}"}}},
		{Content: "should not get here"},
	}}
	var ran []string
	gate := NewHumanInLoop(recordingExec(&ran), func(context.Context, langrails.ToolCall) (Decision, error) {
		return Decision{}, errors.New("reviewer unavailable")
	})
	a := New(p, WithModel("m"), WithTools(nil, gate), WithMiddleware(NewToolRetry(3, WithToolRetryDelay(time.Millisecond))))
	_, err := a.Run(context.Background(), "email")
	if err == nil || !strings.Contains(err.Error(), "reviewer unavailable") {
		t.Fatalf("err = %v", err)
	}
	if p.calls != 1 || len(ran) != 0 {
		t.Errorf("calls = %d, ran = %v", p.calls, ran)
	}
}

func TestAgent_AbortFromExecutor(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "fail", Arguments: "{}"}}},
	}}
	sentinel := errors.New("disk full")
	exec := tools.NewMap(map[string]tools.Func{
		"fail": func(context.Context, string) (string, error) { return "", Abort(sentinel) },
	})
	_, err := New(p, WithModel("m"), WithTools(nil, exec)).Run(context.Background(), "go")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v", err)
	}
}

type emailState struct {
	Request string
	Answer  string
}

func TestAwaitApproval_DurableAcrossResumes(t *testing.T) {
	// The model asks to send two emails in one turn, then answers. The
	// cache makes the replayed run on each resume see the same calls.
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{
			{ID: "c1", Name: "send_email", Arguments: `{"to": "a@x.com"}`},
			{ID: "c2", Name: "send_email", Arguments: `{"to": "b@x.com"}`},
		}},
		{Content: "done"},
	}}
	cached := langrails.WithCache(p, langrails.NewMemoryCache(10, time.Hour))
	var ran []string
	defs := []langrails.ToolDefinition{{Name: "send_email", Parameters: emailSchema}}
	gate := NewHumanInLoop(recordingExec(&ran), AwaitApproval(defs), WithToolSchemas(defs))
	a := New(cached, WithModel("m"), WithTools(defs, gate))

	g := graph.New[emailState]()
	g.AddNode("agent", func(ctx context.Context, s emailState) (emailState, error) {
		res, err := a.Run(ctx, s.Request)
		if err != nil {
			return s, err
		}
		s.Answer = res.Response.Content
		return s, nil
	})
	g.SetEntryPoint("agent")
	g.AddEdge("agent", graph.END)

	opts := []graph.Option[emailState]{
		graph.WithCheckpointer[emailState](graph.NewMemoryCheckpointer[emailState]()),
		graph.WithThreadID[emailState]("t"),
	}
	res, err := g.Run(context.Background(), emailState{Request: "email a and b"}, opts...)
	if err != nil || res.Interruption == nil {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	req := res.Interruption.Payload.(ApprovalRequest)
	if req.Tool != "send_email" || req.Arguments != `{"to": "a@x.com"}` || len(req.Schema) == 0 {
		t.Fatalf("first request = %+v", req)
	}

	// Approve the first with an edit; the run pauses on the second.
	res, err = g.Resume(context.Background(), append(opts,
		graph.WithResumeValue[emailState](req.Decided.With(req, Edit(`{"to": "a2@x.com"}`))))...)
	if err != nil || res.Interruption == nil {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	req = res.Interruption.Payload.(ApprovalRequest)
	if req.Arguments != `{"to": "b@x.com"}` || len(req.Decided) != 1 {
		t.Fatalf("second request = %+v", req)
	}
	if len(ran) != 1 || ran[0] != `{"to": "a2@x.com"}` {
		t.Fatalf("ran = %v", ran)
	}

	// Reject the second; the replay re-sends the first (already approved)
	// and the run completes.
	res, err = g.Resume(context.Background(), append(opts,
		graph.WithResumeValue[emailState](req.Decided.With(req, Reject("no"))))...)
	if err != nil || res.Interruption != nil || res.State.Answer != "done" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if p.calls != 2 {
		t.Errorf("provider called %d times; the cache should absorb replays", p.calls)
	}
	// The replay ran the approved call again (documented).
	if len(ran) != 2 || ran[1] != `{"to": "a2@x.com"}` {
		t.Errorf("ran = %v", ran)
	}
}
