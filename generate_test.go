package langrails

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type scriptedProvider struct {
	replies []string
	reqs    []*CompletionRequest
}

func (p *scriptedProvider) Complete(_ context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	cp := *req
	cp.Messages = append([]Message(nil), req.Messages...)
	p.reqs = append(p.reqs, &cp)
	if len(p.replies) == 0 {
		return nil, errors.New("no more replies")
	}
	r := p.replies[0]
	p.replies = p.replies[1:]
	return &CompletionResponse{Content: r}, nil
}

func (p *scriptedProvider) Stream(context.Context, *CompletionRequest) (<-chan StreamEvent, error) {
	return nil, errors.New("not implemented")
}

type sentiment struct {
	Label      string  `json:"label" jsonschema:"enum=positive|negative"`
	Confidence float64 `json:"confidence"`
}

func TestGenerateStruct(t *testing.T) {
	p := &scriptedProvider{replies: []string{"```json\n{\"label\":\"positive\",\"confidence\":0.9}\n```"}}
	req := &CompletionRequest{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}}

	got, resp, err := Generate[sentiment](context.Background(), p, req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "positive" || got.Confidence != 0.9 || resp == nil {
		t.Errorf("got %+v", got)
	}
	if req.OutputSchema != nil {
		t.Error("caller's request was modified")
	}
	schema := string(*p.reqs[0].OutputSchema)
	if !strings.Contains(schema, `"enum":["positive","negative"]`) {
		t.Errorf("schema = %s", schema)
	}
}

func TestGenerateWrapsNonObject(t *testing.T) {
	p := &scriptedProvider{replies: []string{`{"value":["a","b"]}`}}
	got, _, err := Generate[[]string](context.Background(), p, &CompletionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("got %v", got)
	}
	schema := string(*p.reqs[0].OutputSchema)
	if schema != `{"properties":{"value":{"type":"array","items":{"type":"string"}}},"type":"object","required":["value"]}` {
		t.Errorf("schema = %s", schema)
	}

	p = &scriptedProvider{replies: []string{`{}`}}
	if _, _, err := Generate[int](context.Background(), p, &CompletionRequest{}); err == nil {
		t.Error("missing value should fail")
	}
}

func TestGenerateParseRetries(t *testing.T) {
	p := &scriptedProvider{replies: []string{"nope", `{"label":"negative","confidence":0.1}`}}
	req := &CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}}

	got, _, err := Generate[sentiment](context.Background(), p, req, WithParseRetries(1))
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "negative" {
		t.Errorf("got %+v", got)
	}
	second := p.reqs[1].Messages
	if len(second) != 3 || second[1].Content != "nope" || !strings.Contains(second[2].Content, "could not be parsed") {
		t.Errorf("retry messages: %+v", second)
	}
	if len(req.Messages) != 1 {
		t.Error("caller's messages were modified")
	}
}

func TestGenerateErrors(t *testing.T) {
	p := &scriptedProvider{replies: []string{"nope"}}
	_, resp, err := Generate[sentiment](context.Background(), p, &CompletionRequest{})
	var pe *ParseError
	if !errors.As(err, &pe) || pe.Content != "nope" || resp == nil {
		t.Errorf("err = %v", err)
	}

	p = &scriptedProvider{}
	if _, _, err := Generate[sentiment](context.Background(), p, &CompletionRequest{}); err == nil {
		t.Error("provider error should propagate")
	}

	if _, _, err := Generate[map[int]string](context.Background(), p, &CompletionRequest{}); err == nil {
		t.Error("schema error expected")
	}
}
