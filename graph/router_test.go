package graph

import (
	"context"
	"errors"
	"testing"
)

type ctxKey struct{}

func TestGraph_ConditionalEdgeCtx(t *testing.T) {
	g := New[testState]()
	g.AddNode("classify", func(_ context.Context, s testState) (testState, error) { return s, nil })
	g.AddNode("yes", func(_ context.Context, s testState) (testState, error) {
		s.Output = "yes"
		return s, nil
	})
	g.SetEntryPoint("classify")
	g.AddConditionalEdgeCtx("classify", func(ctx context.Context, s testState) (string, error) {
		if ctx.Value(ctxKey{}) != "v" {
			t.Error("router did not get the run's context")
		}
		if s.Input == "fail" {
			return "", errors.New("model down")
		}
		return "yes", nil
	})
	g.AddEdge("yes", END)

	ctx := context.WithValue(context.Background(), ctxKey{}, "v")
	result, err := g.Run(ctx, testState{Input: "ok"})
	if err != nil || result.State.Output != "yes" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}

	_, err = g.Run(ctx, testState{Input: "fail"})
	if err == nil || err.Error() != `graph: router for node "classify" failed: model down` {
		t.Fatalf("err = %v", err)
	}
}
