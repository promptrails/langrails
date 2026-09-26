package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

type toolRecorder struct {
	BaseMiddleware
	name string
	log  *[]string
}

func (r toolRecorder) WrapToolCall(next ToolFunc) ToolFunc {
	return func(ctx context.Context, call langrails.ToolCall) (string, error) {
		*r.log = append(*r.log, r.name+">"+call.Name)
		out, err := next(ctx, call)
		*r.log = append(*r.log, r.name+"<")
		return out, err
	}
}

type toolShortCircuit struct{ BaseMiddleware }

func (toolShortCircuit) WrapToolCall(ToolFunc) ToolFunc {
	return func(context.Context, langrails.ToolCall) (string, error) {
		return "", errors.New("blocked")
	}
}

func TestAgent_WrapToolCallOrder(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "echo", Arguments: "x"}}},
		{Content: "done"},
	}}
	var log []string
	exec := tools.NewMap(map[string]tools.Func{
		"echo": func(_ context.Context, args string) (string, error) {
			log = append(log, "exec")
			return args, nil
		},
	})

	a := New(p, WithModel("m"), WithTools(nil, exec), WithMiddleware(
		toolRecorder{name: "outer", log: &log},
		BaseMiddleware{}, // plain middleware without WrapToolCall is skipped
		toolRecorder{name: "inner", log: &log},
	))
	if _, err := a.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(log, ","); got != "outer>echo,inner>echo,exec,inner<,outer<" {
		t.Errorf("order = %s", got)
	}
}

func TestAgent_WrapToolCallErrorGoesToModel(t *testing.T) {
	p := &mockProvider{responses: []*langrails.CompletionResponse{
		{ToolCalls: []langrails.ToolCall{{ID: "c1", Name: "echo"}}},
		{Content: "done"},
	}}
	exec := tools.NewMap(map[string]tools.Func{"echo": func(context.Context, string) (string, error) {
		t.Error("executor should not run")
		return "", nil
	}})

	a := New(p, WithModel("m"), WithTools(nil, exec), WithMiddleware(toolShortCircuit{}))
	res, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatal(err)
	}
	last := res.Messages[len(res.Messages)-1]
	if last.Role != "tool" || !strings.Contains(last.Content, "blocked") {
		t.Errorf("tool result = %+v", last)
	}
}
