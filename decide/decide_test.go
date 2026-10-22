package decide

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/promptrails/langrails"
)

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestValidate(t *testing.T) {
	two := []Option{{Value: "a"}, {Value: "b"}}
	levels := []Level{{Label: "low"}, {Label: "high"}}
	tests := []struct {
		name string
		req  Request
		want string
	}{
		{"no questions", Request{}, "no questions"},
		{"no name", Request{Questions: []Question{{Kind: Predicate}}}, "no name"},
		{"duplicate", Request{Questions: []Question{NewPredicate("q", ""), NewPredicate("q", "")}}, "duplicate"},
		{"one option", Request{Questions: []Question{NewChoice("q", "", Option{Value: "a"})}}, "2 to 255"},
		{"dup option", Request{Questions: []Question{NewChoice("q", "", Option{Value: "a"}, Option{Value: "a"})}}, "duplicate option"},
		{"one level", Request{Questions: []Question{NewScore("q", "", Level{Label: "x"})}}, "2 to 10"},
		{"dup level", Request{Questions: []Question{NewScore("q", "", Level{Label: "x"}, Level{Label: "x"})}}, "duplicate level"},
		{"bad kind", Request{Questions: []Question{{Name: "q", Kind: "box"}}}, "unknown kind"},
		{"audio", Request{Questions: []Question{NewPredicate("q", "")}, Parts: []langrails.ContentPart{langrails.AudioPart("x", "audio/wav")}}, "unsupported"},
		{"ok", Request{Questions: []Question{NewPredicate("p", ""), NewChoice("c", "", two...), NewScore("s", "", levels...)}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPredicateAnswer(t *testing.T) {
	a := PredicateAnswer("p", 0.8)
	if a.Choice != Yes || !approx(a.Confidence, 0.8) || !approx(a.Probability(No), 0.2) {
		t.Fatalf("got %+v", a)
	}
	a = PredicateAnswer("p", 0.3)
	if a.Choice != No || !approx(a.Confidence, 0.7) {
		t.Fatalf("got %+v", a)
	}
}

func TestDistributionAnswer(t *testing.T) {
	a := DistributionAnswer("s", Score, []Probability{{"low", 0.2}, {"mid", 0.3}, {"high", 0.5}})
	if a.Choice != "high" || !approx(a.Score, 1.3) || !approx(a.Confidence, 0.5) {
		t.Fatalf("got %+v", a)
	}
	if a.Probability("missing") != 0 {
		t.Fatal("missing value should be 0")
	}
}

type fakeDecider struct {
	got  *Request
	resp *Response
	err  error
}

func (f *fakeDecider) Decide(_ context.Context, req *Request) (*Response, error) {
	f.got = req
	return f.resp, f.err
}

func TestAsk(t *testing.T) {
	d := &fakeDecider{resp: &Response{Answers: []Answer{PredicateAnswer("q", 0.9)}}}
	a, err := Ask(context.Background(), d, "m", "input", NewPredicate("", "spam?"))
	if err != nil || a.Choice != Yes {
		t.Fatalf("got %+v, %v", a, err)
	}
	if d.got.Model != "m" || d.got.Input != "input" || d.got.Questions[0].Name != "q" {
		t.Fatalf("request = %+v", d.got)
	}

	d.resp = &Response{Answers: []Answer{{Name: "q", Refused: true}}}
	if _, err := Ask(context.Background(), d, "", "x", NewPredicate("q", "")); !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v, want ErrRefused", err)
	}

	d.resp = &Response{}
	if _, err := Ask(context.Background(), d, "", "x", NewPredicate("q", "")); err == nil {
		t.Fatal("expected error for a missing answer")
	}
}

type fakeProvider struct {
	req     *langrails.CompletionRequest
	content string
}

func (f *fakeProvider) Complete(_ context.Context, req *langrails.CompletionRequest) (*langrails.CompletionResponse, error) {
	f.req = req
	return &langrails.CompletionResponse{Content: f.content, Model: req.Model, Usage: langrails.TokenUsage{TotalTokens: 7}}, nil
}

func (f *fakeProvider) Stream(context.Context, *langrails.CompletionRequest) (<-chan langrails.StreamEvent, error) {
	return nil, errors.New("not implemented")
}

func TestFromProvider(t *testing.T) {
	p := &fakeProvider{content: "```json\n" + `{
		"spam": {"yes": 0.1},
		"queue": {"probabilities": {"billing": 3, "tech": 1}},
		"sev": {"probabilities": {"low": 0, "high": 0}}
	}` + "\n```"}
	req := &Request{
		Model: "gpt-x",
		Input: "I was charged twice",
		Questions: []Question{
			{Name: "spam", Kind: Predicate, Instructions: "Is it spam?", YesWhen: "ads"},
			NewChoice("queue", "Which queue?", Option{"billing", "money"}, Option{"tech", ""}),
			NewScore("sev", "How severe?", Level{"low", ""}, Level{"high", ""}),
			NewPredicate("missing", "?"),
		},
	}
	resp, err := FromProvider(p).Decide(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	if p.req.Model != "gpt-x" || len(p.req.OutputSchema) == 0 {
		t.Fatalf("request = %+v", p.req)
	}
	prompt := p.req.Messages[0].ContentParts[0].Text
	for _, want := range []string{"I was charged twice", `"queue" (pick one)`, "- billing: money", "- yes: ads"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q:\n%s", want, prompt)
		}
	}
	var schema map[string]any
	if err := json.Unmarshal(p.req.OutputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if len(schema["required"].([]any)) != 4 {
		t.Errorf("schema required = %v", schema["required"])
	}

	spam, _ := resp.Answer("spam")
	if spam.Choice != No || !approx(spam.YesProbability, 0.1) {
		t.Errorf("spam = %+v", spam)
	}
	queue, _ := resp.Answer("queue")
	if queue.Choice != "billing" || !approx(queue.Probability("billing"), 0.75) {
		t.Errorf("queue = %+v", queue)
	}
	sev, _ := resp.Answer("sev")
	if !approx(sev.Probability("low"), 0.5) || !approx(sev.Score, 0.5) {
		t.Errorf("sev = %+v", sev)
	}
	missing, _ := resp.Answer("missing")
	if !missing.Refused {
		t.Errorf("missing = %+v, want refused", missing)
	}
	if resp.Usage.TotalTokens != 7 {
		t.Errorf("usage = %+v", resp.Usage)
	}
}

func TestFromProviderParseError(t *testing.T) {
	_, err := FromProvider(&fakeProvider{content: "nope"}).Decide(context.Background(),
		&Request{Questions: []Question{NewPredicate("q", "")}})
	var pe *langrails.ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("err = %v, want ParseError", err)
	}
}
