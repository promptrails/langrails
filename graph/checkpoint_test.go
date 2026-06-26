package graph

import (
	"context"
	"errors"
	"testing"
)

type counterState struct {
	Count int
	Trace []string
}

// buildCountingGraph returns a graph that increments Count three times
// across three nodes, recording which nodes ran in Trace.
func buildCountingGraph() *Graph[counterState] {
	g := New[counterState]()
	for _, name := range []string{"a", "b", "c"} {
		name := name
		g.AddNode(name, func(_ context.Context, s counterState) (counterState, error) {
			s.Count++
			s.Trace = append(s.Trace, name)
			return s, nil
		})
	}
	g.SetEntryPoint("a")
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", END)
	return g
}

func TestCheckpointer_SavesAfterEachStep(t *testing.T) {
	cp := NewMemoryCheckpointer[counterState]()
	g := buildCountingGraph()

	_, err := g.Run(context.Background(), counterState{}, WithCheckpointer[counterState](cp), WithThreadID[counterState]("t1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hist, err := cp.History(context.Background(), "t1")
	if err != nil {
		t.Fatalf("history error: %v", err)
	}
	// One checkpoint per routing decision: a->b, b->c, c->END.
	if len(hist) != 3 {
		t.Fatalf("expected 3 checkpoints, got %d", len(hist))
	}
	if hist[0].Node != "b" || hist[1].Node != "c" || hist[2].Node != END {
		t.Errorf("unexpected checkpoint nodes: %q, %q, %q", hist[0].Node, hist[1].Node, hist[2].Node)
	}
	if !hist[2].Done {
		t.Error("expected final checkpoint to be marked Done")
	}
}

func TestResume_ContinuesFromCrash(t *testing.T) {
	cp := NewMemoryCheckpointer[counterState]()

	// First graph fails inside node "c", after a->b checkpoints are saved.
	failing := New[counterState]()
	failing.AddNode("a", func(_ context.Context, s counterState) (counterState, error) {
		s.Count++
		s.Trace = append(s.Trace, "a")
		return s, nil
	})
	failing.AddNode("b", func(_ context.Context, s counterState) (counterState, error) {
		s.Count++
		s.Trace = append(s.Trace, "b")
		return s, nil
	})
	failing.AddNode("c", func(_ context.Context, s counterState) (counterState, error) {
		return s, errors.New("boom in c")
	})
	failing.SetEntryPoint("a")
	failing.AddEdge("a", "b")
	failing.AddEdge("b", "c")
	failing.AddEdge("c", END)

	_, err := failing.Run(context.Background(), counterState{},
		WithCheckpointer[counterState](cp), WithThreadID[counterState]("job"))
	if err == nil {
		t.Fatal("expected failure in node c")
	}

	// The latest checkpoint should point at "c" with state after b.
	latest, ok, _ := cp.Load(context.Background(), "job")
	if !ok || latest.Node != "c" {
		t.Fatalf("expected checkpoint at c, got node=%q ok=%v", latest.Node, ok)
	}
	if latest.State.Count != 2 {
		t.Fatalf("expected count 2 at checkpoint, got %d", latest.State.Count)
	}

	// A fixed graph resumes from the saved checkpoint and runs only c.
	fixed := buildCountingGraph()
	result, err := fixed.Resume(context.Background(),
		WithCheckpointer[counterState](cp), WithThreadID[counterState]("job"))
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}
	if result.State.Count != 3 {
		t.Errorf("expected resumed count 3, got %d", result.State.Count)
	}
	if len(result.Steps) != 1 || result.Steps[0].Node != "c" {
		t.Errorf("expected resume to run only node c, got %+v", result.Steps)
	}
}

func TestResume_DoneReturnsFinalState(t *testing.T) {
	cp := NewMemoryCheckpointer[counterState]()
	g := buildCountingGraph()

	if _, err := g.Run(context.Background(), counterState{},
		WithCheckpointer[counterState](cp), WithThreadID[counterState]("t")); err != nil {
		t.Fatalf("run error: %v", err)
	}

	// Resuming a completed run replays no nodes.
	result, err := g.Resume(context.Background(),
		WithCheckpointer[counterState](cp), WithThreadID[counterState]("t"))
	if err != nil {
		t.Fatalf("resume error: %v", err)
	}
	if result.State.Count != 3 {
		t.Errorf("expected final count 3, got %d", result.State.Count)
	}
	if len(result.Steps) != 0 {
		t.Errorf("expected no replayed steps, got %d", len(result.Steps))
	}
}

func TestResume_NoCheckpoint(t *testing.T) {
	cp := NewMemoryCheckpointer[counterState]()
	g := buildCountingGraph()

	_, err := g.Resume(context.Background(),
		WithCheckpointer[counterState](cp), WithThreadID[counterState]("missing"))
	if err == nil {
		t.Fatal("expected error resuming a thread with no checkpoint")
	}
}

func TestResume_RequiresCheckpointer(t *testing.T) {
	g := buildCountingGraph()
	if _, err := g.Resume(context.Background()); err == nil {
		t.Fatal("expected error: resume without checkpointer")
	}
}

func TestRun_CheckpointerWithoutThreadID(t *testing.T) {
	cp := NewMemoryCheckpointer[counterState]()
	g := buildCountingGraph()

	_, err := g.Run(context.Background(), counterState{}, WithCheckpointer[counterState](cp))
	if err == nil {
		t.Fatal("expected error: checkpointer without thread ID")
	}
}

func TestCheckpointer_IndependentThreads(t *testing.T) {
	cp := NewMemoryCheckpointer[counterState]()
	g := buildCountingGraph()
	ctx := context.Background()

	if _, err := g.Run(ctx, counterState{}, WithCheckpointer[counterState](cp), WithThreadID[counterState]("x")); err != nil {
		t.Fatalf("run x: %v", err)
	}
	if _, err := g.Run(ctx, counterState{}, WithCheckpointer[counterState](cp), WithThreadID[counterState]("y")); err != nil {
		t.Fatalf("run y: %v", err)
	}

	hx, _ := cp.History(ctx, "x")
	hy, _ := cp.History(ctx, "y")
	if len(hx) != 3 || len(hy) != 3 {
		t.Errorf("expected 3 checkpoints per thread, got x=%d y=%d", len(hx), len(hy))
	}
}
