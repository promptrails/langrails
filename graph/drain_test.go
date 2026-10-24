package graph

import (
	"context"
	"testing"
)

func TestDrain(t *testing.T) {
	drain := NewDrain()
	g := New[testState]()
	g.AddNode("a", func(_ context.Context, s testState) (testState, error) {
		s.Steps = append(s.Steps, "a")
		drain.Request() // e.g. a shutdown signal arriving mid-node
		return s, nil
	})
	g.AddNode("b", func(_ context.Context, s testState) (testState, error) {
		s.Steps = append(s.Steps, "b")
		return s, nil
	})
	g.SetEntryPoint("a")
	g.AddEdge("a", "b")
	g.AddEdge("b", END)

	cp := NewMemoryCheckpointer[testState]()
	opts := []Option[testState]{WithCheckpointer[testState](cp), WithThreadID[testState]("t")}
	res, err := g.Run(context.Background(), testState{}, append(opts, WithDrain[testState](drain))...)
	if err != nil || !res.Drained || len(res.State.Steps) != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}

	res, err = g.Resume(context.Background(), opts...)
	if err != nil || res.Drained || len(res.State.Steps) != 2 || res.State.Steps[1] != "b" {
		t.Fatalf("resume: res = %+v, err = %v", res, err)
	}
}

func TestDrain_LastStepCompletes(t *testing.T) {
	drain := NewDrain()
	drain.Request()
	drain.Request() // idempotent
	g := New[testState]()
	g.AddNode("only", func(_ context.Context, s testState) (testState, error) { return s, nil })
	g.SetEntryPoint("only")
	g.AddEdge("only", END)
	res, err := g.Run(context.Background(), testState{}, WithDrain[testState](drain))
	if err != nil || res.Drained {
		t.Fatalf("a run that reaches END is not drained: res = %+v, err = %v", res, err)
	}
}
