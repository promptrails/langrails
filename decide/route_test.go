package decide

import (
	"context"
	"errors"
	"testing"

	"github.com/promptrails/langrails"
	"github.com/promptrails/langrails/graph"
)

func TestRoute(t *testing.T) {
	small, large := &fakeProvider{content: "small"}, &fakeProvider{content: "large"}
	d := &fakeDecider{}
	var decisions []Answer
	r := Route(d, "dm", NewPredicate("", "hard?"), map[string]Target{
		No:  {Provider: small, Model: "mini"},
		Yes: {Provider: large},
	},
		WithMinConfidence(0.8),
		WithFallback(Target{Provider: large, Model: "big"}),
		WithOnDecision(func(_ context.Context, a Answer) { decisions = append(decisions, a) }),
	)
	req := &langrails.CompletionRequest{Model: "orig", Messages: []langrails.Message{
		{Role: langrails.RoleUser, Content: "first"},
		{Role: langrails.RoleAssistant, Content: "ok"},
		{Role: langrails.RoleUser, ContentParts: []langrails.ContentPart{langrails.TextPart("what is"), langrails.TextPart("2+2")}},
	}}

	tests := []struct {
		name      string
		answer    Answer
		wantText  string
		wantModel string
	}{
		{"easy", PredicateAnswer("route", 0.1), "small", "mini"},
		{"hard keeps model", PredicateAnswer("route", 0.95), "large", "orig"},
		{"unsure falls back", PredicateAnswer("route", 0.6), "large", "big"},
		{"refused falls back", Answer{Name: "route", Refused: true}, "large", "big"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			small.req, large.req = nil, nil
			d.resp = &Response{Answers: []Answer{tt.answer}}
			resp, err := r.Complete(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.Content != tt.wantText || resp.Model != tt.wantModel {
				t.Errorf("got %q on %q, want %q on %q", resp.Content, resp.Model, tt.wantText, tt.wantModel)
			}
		})
	}
	if d.got.Input != "what is\n2+2" || d.got.Model != "dm" || d.got.Questions[0].Name != "route" {
		t.Errorf("decision request = %+v", d.got)
	}
	if req.Model != "orig" {
		t.Error("caller's request was modified")
	}
	if len(decisions) != 4 {
		t.Errorf("OnDecision called %d times", len(decisions))
	}
}

func TestRouteNoFallback(t *testing.T) {
	d := &fakeDecider{resp: &Response{Answers: []Answer{DistributionAnswer("route", Choice, []Probability{{"a", 0.1}, {"b", 0.9}})}}}
	r := Route(d, "", NewChoice("", "?", Option{Value: "a"}, Option{Value: "b"}),
		map[string]Target{"a": {Provider: &fakeProvider{}}})
	if _, err := r.Complete(context.Background(), &langrails.CompletionRequest{}); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
	if _, err := r.Stream(context.Background(), &langrails.CompletionRequest{}); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("stream err = %v, want ErrNoRoute", err)
	}

	d.resp, d.err = nil, errors.New("down")
	if _, err := r.Complete(context.Background(), &langrails.CompletionRequest{}); err == nil || errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want the decider's error", err)
	}
}

type ticket struct {
	Body  string
	Queue string
}

func TestGraphRouter(t *testing.T) {
	d := &fakeDecider{}
	g := graph.New[ticket]()
	g.AddNode("intake", func(_ context.Context, s ticket) (ticket, error) { return s, nil })
	for _, n := range []string{"refund", "debug", "human"} {
		g.AddNode(n, func(_ context.Context, s ticket) (ticket, error) {
			s.Queue = n
			return s, nil
		})
		g.AddEdge(n, graph.END)
	}
	g.SetEntryPoint("intake")
	q := NewChoice("queue", "?", Option{Value: "billing"}, Option{Value: "tech"}, Option{Value: "sales"})
	g.AddConditionalEdgeCtx("intake", GraphRouter(d, "", q,
		func(s ticket) string { return s.Body },
		map[string]string{"billing": "refund", "tech": "debug"},
		WithMinConfidence(0.7), WithFallbackNode("human"),
	))

	tests := []struct {
		probs []Probability
		want  string
	}{
		{[]Probability{{"billing", 0.9}, {"tech", 0.05}, {"sales", 0.05}}, "refund"},
		{[]Probability{{"billing", 0.1}, {"tech", 0.8}, {"sales", 0.1}}, "debug"},
		{[]Probability{{"billing", 0.5}, {"tech", 0.4}, {"sales", 0.1}}, "human"},
		{[]Probability{{"billing", 0}, {"tech", 0}, {"sales", 1}}, "human"},
	}
	for _, tt := range tests {
		d.resp = &Response{Answers: []Answer{DistributionAnswer("queue", Choice, tt.probs)}}
		res, err := g.Run(context.Background(), ticket{Body: "charged twice"})
		if err != nil {
			t.Fatal(err)
		}
		if res.State.Queue != tt.want {
			t.Errorf("probs %v: queue = %q, want %q", tt.probs, res.State.Queue, tt.want)
		}
	}
	if d.got.Input != "charged twice" {
		t.Errorf("input = %q", d.got.Input)
	}
}

func TestGraphRouterChoiceIsNode(t *testing.T) {
	d := &fakeDecider{resp: &Response{Answers: []Answer{PredicateAnswer("route", 0.9)}}}
	router := GraphRouter(d, "", NewPredicate("", "?"), func(s string) string { return s }, nil)
	node, err := router(context.Background(), "x")
	if err != nil || node != Yes {
		t.Fatalf("node = %q, err = %v", node, err)
	}

	d.resp = &Response{Answers: []Answer{{Name: "route", Refused: true}}}
	if _, err := router(context.Background(), "x"); !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}
