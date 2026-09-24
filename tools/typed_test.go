package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

type weatherArgs struct {
	City string `json:"city" description:"City name"`
	Unit string `json:"unit,omitempty" jsonschema:"enum=celsius|fahrenheit"`
}

type weather struct {
	Temp int    `json:"temp"`
	City string `json:"city"`
}

func TestNewTypedTool(t *testing.T) {
	tool, err := New("get_weather", "Get weather", func(_ context.Context, in weatherArgs) (weather, error) {
		return weather{Temp: 22, City: in.City}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if tool.Definition.Name != "get_weather" || tool.Definition.Description != "Get weather" {
		t.Errorf("definition: %+v", tool.Definition)
	}
	if !strings.Contains(string(tool.Definition.Parameters), `"required":["city"]`) {
		t.Errorf("schema: %s", tool.Definition.Parameters)
	}

	out, err := tool.Func(context.Background(), `{"city":"Istanbul"}`)
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"temp":22,"city":"Istanbul"}` {
		t.Errorf("out = %s", out)
	}
}

func TestTypedToolStringResultAndEmptyArgs(t *testing.T) {
	tool := MustNew("now", "Current time", func(context.Context, struct{}) (string, error) {
		return "12:00", nil
	})
	out, err := tool.Func(context.Background(), "")
	if err != nil || out != "12:00" {
		t.Errorf("out = %q, err = %v", out, err)
	}
}

func TestTypedToolErrors(t *testing.T) {
	tool := MustNew("fail", "", func(context.Context, weatherArgs) (string, error) {
		return "", errors.New("boom")
	})
	if _, err := tool.Func(context.Background(), `{"city":1}`); err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Errorf("bad args: err = %v", err)
	}
	if _, err := tool.Func(context.Background(), `{"city":"x"}`); err == nil || err.Error() != "boom" {
		t.Errorf("fn error: err = %v", err)
	}

	if _, err := New("bad", "", func(context.Context, map[int]string) (string, error) { return "", nil }); err == nil {
		t.Error("expected schema error")
	}
	defer func() {
		if recover() == nil {
			t.Error("MustNew should panic")
		}
	}()
	MustNew("bad", "", func(context.Context, map[int]string) (string, error) { return "", nil })
}

func TestSet(t *testing.T) {
	a := MustNew("a", "first", func(context.Context, struct{}) (string, error) { return "a1", nil })
	b := MustNew("b", "", func(context.Context, struct{}) (string, error) { return "b", nil })
	a2 := MustNew("a", "second", func(context.Context, struct{}) (string, error) { return "a2", nil })

	set := NewSet(a, b, a2)
	defs := set.Definitions()
	if len(defs) != 2 || defs[0].Name != "a" || defs[0].Description != "second" || defs[1].Name != "b" {
		t.Errorf("definitions: %+v", defs)
	}
	if out, _ := set.Execute(context.Background(), "a", ""); out != "a2" {
		t.Errorf("a = %s", out)
	}
	if _, err := set.Execute(context.Background(), "missing", ""); err == nil {
		t.Error("expected unknown tool error")
	}
}

func TestSetWithRunLoop(t *testing.T) {
	set := NewSet(MustNew("get_weather", "", func(_ context.Context, in weatherArgs) (weather, error) {
		return weather{Temp: 22, City: in.City}, nil
	}))

	var calls int
	p := providerFunc(func(req *langrails.CompletionRequest) *langrails.CompletionResponse {
		calls++
		if calls == 1 {
			return &langrails.CompletionResponse{ToolCalls: []langrails.ToolCall{
				{ID: "1", Name: "get_weather", Arguments: `{"city":"Ankara"}`},
			}}
		}
		last := req.Messages[len(req.Messages)-1]
		return &langrails.CompletionResponse{Content: last.Content}
	})

	res, err := RunLoop(context.Background(), p, &langrails.CompletionRequest{Tools: set.Definitions()}, set)
	if err != nil {
		t.Fatal(err)
	}
	if res.Response.Content != `{"temp":22,"city":"Ankara"}` {
		t.Errorf("content = %s", res.Response.Content)
	}
}

type providerFunc func(req *langrails.CompletionRequest) *langrails.CompletionResponse

func (f providerFunc) Complete(_ context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	return f(req), nil
}

func (f providerFunc) Stream(context.Context, *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return nil, errors.New("not implemented")
}
