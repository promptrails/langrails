package agent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/tools"
)

// streamProvider streams scripted event sequences, one per call.
type streamProvider struct {
	scripts [][]langrails.StreamEvent
	calls   int
	openErr error
}

func (p *streamProvider) Complete(context.Context, *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	return nil, errors.New("Complete should not be called when streaming")
}

func (p *streamProvider) Stream(_ context.Context, _ *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	if p.openErr != nil {
		return nil, p.openErr
	}
	script := p.scripts[p.calls]
	p.calls++
	ch := make(chan langrails.StreamEvent, len(script))
	for _, ev := range script {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func collect(ch <-chan Event) []Event {
	var evs []Event
	for ev := range ch {
		evs = append(evs, ev)
	}
	return evs
}

func TestAgent_StreamToolLoop(t *testing.T) {
	p := &streamProvider{scripts: [][]langrails.StreamEvent{
		{
			{Type: langrails.EventReasoning, Reasoning: "need weather"},
			{Type: langrails.EventToolCall, ToolCall: &langrails.ToolCall{ID: "c1", Name: "weather", Arguments: `{"city":"Ankara"}`}},
			{Type: langrails.EventDone, Usage: &langrails.TokenUsage{TotalTokens: 5}},
		},
		{
			{Type: langrails.EventContent, Content: "It is "},
			{Type: langrails.EventContent, Content: "sunny."},
			{Type: langrails.EventDone, Usage: &langrails.TokenUsage{TotalTokens: 7}},
		},
	}}
	exec := tools.NewMap(map[string]tools.Func{
		"weather": func(context.Context, string) (string, error) { return `{"sky":"clear"}`, nil },
	})

	a := New(p, WithModel("m"), WithTools(nil, exec))
	evs := collect(a.Stream(context.Background(), "weather?"))

	var kinds []string
	for _, ev := range evs {
		kinds = append(kinds, string(ev.Type))
	}
	want := "reasoning,tool_call,tool_result,content,content,done"
	if got := strings.Join(kinds, ","); got != want {
		t.Fatalf("events = %s, want %s", got, want)
	}
	if evs[2].Content != `{"sky":"clear"}` || evs[2].ToolCall.Name != "weather" || evs[2].Iteration != 1 {
		t.Errorf("tool result event = %+v", evs[2])
	}
	if evs[3].Iteration != 2 {
		t.Errorf("content iteration = %d", evs[3].Iteration)
	}
	res := evs[len(evs)-1].Result
	if res.Response.Content != "It is sunny." || res.Iterations != 2 || res.TotalUsage.TotalTokens != 12 {
		t.Errorf("result = %+v", res)
	}
	if last := res.Messages[len(res.Messages)-1]; last.Role != "tool" {
		t.Errorf("history should end with the tool result, got %+v", last)
	}
}

type cannedReply struct{ BaseMiddleware }

func (cannedReply) WrapModelCall(CallFunc) CallFunc {
	return func(context.Context, *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
		return &langrails.CompletionResponse{Content: "from cache"}, nil
	}
}

func TestAgent_StreamShortCircuitedModel(t *testing.T) {
	a := New(&streamProvider{}, WithModel("m"), WithMiddleware(cannedReply{}))
	evs := collect(a.Stream(context.Background(), "hi"))
	if len(evs) != 2 || evs[0].Type != EventContent || evs[0].Content != "from cache" || evs[1].Type != EventDone {
		t.Errorf("events = %+v", evs)
	}
}

func TestAgent_StreamErrors(t *testing.T) {
	a := New(&streamProvider{openErr: errors.New("down")}, WithModel("m"))
	evs := collect(a.Stream(context.Background(), "hi"))
	if len(evs) != 1 || evs[0].Type != EventError || !strings.Contains(evs[0].Error.Error(), "down") {
		t.Errorf("open error events = %+v", evs)
	}

	a = New(&streamProvider{scripts: [][]langrails.StreamEvent{
		{{Type: langrails.EventContent, Content: "par"}, {Type: langrails.EventError, Error: errors.New("cut")}},
	}}, WithModel("m"))
	evs = collect(a.Stream(context.Background(), "hi"))
	last := evs[len(evs)-1]
	if last.Type != EventError || !strings.Contains(last.Error.Error(), "cut") {
		t.Errorf("mid-stream error events = %+v", evs)
	}

	evs = collect(New(&streamProvider{}).Stream(context.Background(), "hi"))
	if len(evs) != 1 || evs[0].Type != EventError {
		t.Errorf("missing model: %+v", evs)
	}
}

func TestAgent_StreamCancelledReader(t *testing.T) {
	p := &streamProvider{scripts: [][]langrails.StreamEvent{
		{{Type: langrails.EventContent, Content: "a"}, {Type: langrails.EventContent, Content: "b"}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	ch := New(p, WithModel("m")).Stream(ctx, "hi")
	<-ch
	cancel()
	for range ch {
	} // must terminate
}
