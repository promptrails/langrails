package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

func toolCall(name string) *langrails.CompletionResponse {
	return &langrails.CompletionResponse{ToolCalls: []langrails.ToolCall{{ID: name, Name: name}}}
}

func echoExec(calls *[]string) tools.Executor {
	return tools.NewMap(map[string]tools.Func{
		"a": func(context.Context, string) (string, error) { *calls = append(*calls, "a"); return "ok", nil },
		"b": func(context.Context, string) (string, error) { *calls = append(*calls, "b"); return "ok", nil },
	})
}

func TestModelCallLimit(t *testing.T) {
	var calls []string
	p := &mockProvider{responses: []*langrails.CompletionResponse{toolCall("a"), toolCall("a"), toolCall("a")}}
	a := New(p, WithModel("m"), WithTools(nil, echoExec(&calls)), WithMiddleware(NewModelCallLimit(2)))
	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Iterations != 2 || len(calls) != 1 || len(res.Response.ToolCalls) != 1 {
		t.Errorf("iterations = %d, tool calls = %v", res.Iterations, calls)
	}

	p = &mockProvider{responses: []*langrails.CompletionResponse{toolCall("a"), toolCall("a")}}
	a = New(p, WithModel("m"), WithTools(nil, echoExec(&calls)), WithMiddleware(NewModelCallLimit(2).FailOnLimit()))
	if _, err := a.Run(context.Background(), "go"); !errors.Is(err, ErrCallLimit) {
		t.Errorf("err = %v", err)
	}

	// A final answer within the limit is unaffected.
	p = &mockProvider{responses: []*langrails.CompletionResponse{{Content: "hi"}}}
	a = New(p, WithModel("m"), WithMiddleware(NewModelCallLimit(1).FailOnLimit()))
	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Error(err)
	}
}

func TestToolCallLimit(t *testing.T) {
	var calls []string
	resps := []*langrails.CompletionResponse{toolCall("a"), toolCall("b"), toolCall("a"), toolCall("a")}

	p := &mockProvider{responses: resps}
	a := New(p, WithModel("m"), WithTools(nil, echoExec(&calls)), WithMiddleware(NewToolCallLimit(2)))
	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "a,b" {
		t.Errorf("executed = %v", calls)
	}
	last := res.Messages[len(res.Messages)-1]
	if !strings.Contains(last.Content, "limit of 2 calls reached") {
		t.Errorf("model was not told about the limit: %q", last.Content)
	}

	// Per-tool limit; limits reset for each run of the same agent.
	for run := range 2 {
		calls = nil
		p = &mockProvider{responses: resps}
		a = New(p, WithModel("m"), WithTools(nil, echoExec(&calls)), WithMiddleware(NewToolCallLimit(1, "a")))
		if _, err := a.Run(context.Background(), "go"); err != nil {
			t.Fatal(err)
		}
		if strings.Join(calls, ",") != "a,b" {
			t.Errorf("run %d: executed = %v", run, calls)
		}
	}
}

func TestToolRetry(t *testing.T) {
	failures := 2
	var attempts int
	exec := tools.NewMap(map[string]tools.Func{
		"a": func(context.Context, string) (string, error) {
			attempts++
			if attempts <= failures {
				return "", errors.New("flaky")
			}
			return "ok", nil
		},
		"b": func(context.Context, string) (string, error) { attempts++; return "", errors.New("hard") },
	})
	run := func(mw Middleware, tool string) string {
		attempts = 0
		p := &mockProvider{responses: []*langrails.CompletionResponse{toolCall(tool)}}
		res, err := New(p, WithModel("m"), WithTools(nil, exec), WithMiddleware(mw)).Run(context.Background(), "go")
		if err != nil {
			t.Fatal(err)
		}
		return res.Messages[len(res.Messages)-1].Content
	}

	if out := run(NewToolRetry(2, WithToolRetryDelay(time.Millisecond)), "a"); out != "ok" || attempts != 3 {
		t.Errorf("retry: out = %q, attempts = %d", out, attempts)
	}
	if out := run(NewToolRetry(1, WithToolRetryDelay(time.Millisecond)), "a"); !strings.Contains(out, "flaky") || attempts != 2 {
		t.Errorf("exhausted: out = %q, attempts = %d", out, attempts)
	}
	if run(NewToolRetry(3, WithToolRetryIf(func(error) bool { return false })), "b"); attempts != 1 {
		t.Errorf("non-retryable: attempts = %d", attempts)
	}
	if run(NewToolRetry(3, WithToolRetryOn("a"), WithToolRetryDelay(time.Millisecond)), "b"); attempts != 1 {
		t.Errorf("unlisted tool: attempts = %d", attempts)
	}
}

type modelProvider struct {
	fail  map[string]bool
	calls []string
}

func (p *modelProvider) Complete(_ context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	p.calls = append(p.calls, req.Model)
	if p.fail[req.Model] {
		return nil, errors.New(req.Model + " down")
	}
	return &langrails.CompletionResponse{Content: "from " + req.Model}, nil
}

func (p *modelProvider) Stream(context.Context, *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return nil, errors.New("no")
}

func TestModelFallback(t *testing.T) {
	p := &modelProvider{fail: map[string]bool{"big": true, "mid": true}}
	a := New(p, WithModel("big"), WithMiddleware(NewModelFallback("mid", "small", "never")))
	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	if res.Response.Content != "from small" || strings.Join(p.calls, ",") != "big,mid,small" {
		t.Errorf("content = %q, calls = %v", res.Response.Content, p.calls)
	}

	p = &modelProvider{fail: map[string]bool{"big": true, "mid": true}}
	_, err = New(p, WithModel("big"), WithMiddleware(NewModelFallback("mid"))).Run(context.Background(), "go")
	if err == nil || !strings.Contains(err.Error(), "big down") || !strings.Contains(err.Error(), "mid down") {
		t.Errorf("err = %v", err)
	}
}

func TestContextEditing(t *testing.T) {
	long := strings.Repeat("x", 400) // ~100 tokens
	msgs := []langrails.Message{{Role: "user", Content: "go"}}
	for range 5 {
		msgs = append(msgs, langrails.Message{Role: "assistant"}, langrails.Message{Role: "tool", Content: long})
	}
	s := &State{Request: &langrails.CompletionRequest{Messages: msgs}}

	ce := NewContextEditing(WithClearThreshold(300), WithKeepToolResults(2), WithClearedPlaceholder("[x]"))
	if err := ce.BeforeModel(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range s.Request.Messages {
		if m.Role == "tool" {
			got = append(got, map[bool]string{true: "cleared", false: "kept"}[m.Content == "[x]"])
		}
	}
	if strings.Join(got, ",") != "cleared,cleared,cleared,kept,kept" {
		t.Errorf("tool results = %v", got)
	}

	small := &State{Request: &langrails.CompletionRequest{Messages: []langrails.Message{{Role: "tool", Content: "short"}}}}
	_ = NewContextEditing(WithKeepToolResults(0)).BeforeModel(context.Background(), small)
	if small.Request.Messages[0].Content != "short" {
		t.Error("under threshold should not clear")
	}
}

type selectorProvider struct {
	reply string
	req   *langrails.CompletionRequest
}

func (p *selectorProvider) Complete(_ context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	p.req = req
	return &langrails.CompletionResponse{Content: p.reply}, nil
}

func (p *selectorProvider) Stream(context.Context, *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return nil, errors.New("no")
}

func TestToolSelector(t *testing.T) {
	defs := []langrails.ToolDefinition{{Name: "weather"}, {Name: "email"}, {Name: "search"}, {Name: "calc"}, {Name: "help"}}
	sel := &selectorProvider{reply: `{"tools":["search","weather","calc"]}`}
	ts := NewToolSelector(sel, "cheap", WithMaxTools(2), WithAlwaysInclude("help"))

	s := &State{Iteration: 1, Request: &langrails.CompletionRequest{
		Tools:    defs,
		Messages: []langrails.Message{{Role: "user", Content: "weather in Rome?"}, {Role: "assistant", Content: "..."}},
	}}
	if err := ts.BeforeModel(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, d := range s.Request.Tools {
		names = append(names, d.Name)
	}
	// Declaration order, capped at 2 selected, plus the always-included tool.
	if strings.Join(names, ",") != "weather,search,help" {
		t.Errorf("tools = %v", names)
	}
	if sel.req.Model != "cheap" || sel.req.Messages[0].Content != "weather in Rome?" ||
		!strings.Contains(string(*sel.req.OutputSchema), `"enum":["weather","email","search","calc","help"]`) {
		t.Errorf("selector request = %+v", sel.req)
	}

	// Later iterations and small tool sets are left alone.
	sel.req = nil
	s.Iteration = 2
	_ = ts.BeforeModel(context.Background(), s)
	small := &State{Iteration: 1, Request: &langrails.CompletionRequest{Tools: defs[:2]}}
	_ = ts.BeforeModel(context.Background(), small)
	if sel.req != nil {
		t.Error("selector should not have been called")
	}

	bad := NewToolSelector(&selectorProvider{reply: "nope"}, "cheap", WithMaxTools(1))
	if err := bad.BeforeModel(context.Background(), &State{Iteration: 1, Request: &langrails.CompletionRequest{Tools: defs}}); err == nil {
		t.Error("expected parse error")
	}
}
