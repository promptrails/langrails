package graph

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"testing"
)

func TestGraph_FanOut_MapReduce(t *testing.T) {
	type state struct {
		Inputs []string
		Sum    int
		Done   []string
	}

	g := New[state]()

	// "split" is the fan-out source; "score" is the per-branch worker.
	g.AddNode("split", func(_ context.Context, s state) (state, error) {
		return s, nil
	})
	g.AddNode("score", func(_ context.Context, s state) (state, error) {
		// Each branch sees exactly one input and scores it by length.
		s.Sum = len(s.Done[0])
		return s, nil
	})
	g.AddNode("collect", func(_ context.Context, s state) (state, error) {
		return s, nil
	})

	g.SetEntryPoint("split")
	g.AddFanOut("split",
		func(_ context.Context, s state) ([]Send[state], error) {
			sends := make([]Send[state], len(s.Inputs))
			for i, in := range s.Inputs {
				sends[i] = Send[state]{Node: "score", State: state{Done: []string{in}}}
			}
			return sends, nil
		},
		func(base state, results []state) state {
			for _, r := range results {
				base.Sum += r.Sum
				base.Done = append(base.Done, r.Done...)
			}
			return base
		},
		"collect",
	)
	g.AddEdge("collect", END)

	result, err := g.Run(context.Background(), state{Inputs: []string{"a", "bb", "ccc"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Sum != 6 { // 1 + 2 + 3
		t.Errorf("expected sum 6, got %d", result.State.Sum)
	}
	sort.Strings(result.State.Done)
	if got := len(result.State.Done); got != 3 {
		t.Errorf("expected 3 branch outputs, got %d", got)
	}
	// split + 3 branches + collect = 5 step events.
	if len(result.Steps) != 5 {
		t.Errorf("expected 5 steps, got %d", len(result.Steps))
	}
}

func TestGraph_FanOut_RespectsMaxSteps(t *testing.T) {
	type state struct{}

	var branchRuns int32
	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("work", func(_ context.Context, s state) (state, error) {
		atomic.AddInt32(&branchRuns, 1)
		return s, nil
	})
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) {
			return []Send[state]{
				{Node: "work", State: state{}},
				{Node: "work", State: state{}},
				{Node: "work", State: state{}},
			}, nil
		},
		func(base state, _ []state) state { return base },
		"done",
	)
	g.AddEdge("done", END)

	// Budget of 1 covers the seed node only; the 3 branches must not run.
	_, err := g.Run(context.Background(), state{}, WithMaxSteps[state](1))
	if err == nil {
		t.Fatal("expected max steps error before branches run")
	}
	if n := atomic.LoadInt32(&branchRuns); n != 0 {
		t.Errorf("expected no branch executions when over budget, got %d", n)
	}
}

func TestGraph_FanOut_RunsConcurrently(t *testing.T) {
	type state struct{ N int }

	var inFlight, maxInFlight int32
	started := make(chan struct{}, 3)

	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("work", func(ctx context.Context, s state) (state, error) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if cur <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, cur) {
				break
			}
		}
		started <- struct{}{}
		// Block until all branches have started, proving real concurrency.
		for len(started) < 3 {
			select {
			case <-ctx.Done():
				return s, ctx.Err()
			default:
			}
		}
		atomic.AddInt32(&inFlight, -1)
		return s, nil
	})
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) {
			return []Send[state]{
				{Node: "work", State: state{N: 1}},
				{Node: "work", State: state{N: 2}},
				{Node: "work", State: state{N: 3}},
			}, nil
		},
		func(base state, _ []state) state { return base },
		"done",
	)
	g.AddEdge("done", END)

	if _, err := g.Run(context.Background(), state{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if maxInFlight < 2 {
		t.Errorf("expected branches to overlap, max concurrent was %d", maxInFlight)
	}
}

func TestGraph_FanOut_BranchError(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("ok", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("boom", func(_ context.Context, s state) (state, error) {
		return s, errors.New("branch exploded")
	})
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) {
			return []Send[state]{{Node: "ok", State: state{}}, {Node: "boom", State: state{}}}, nil
		},
		func(base state, _ []state) state { return base },
		"done",
	)
	g.AddEdge("done", END)

	_, err := g.Run(context.Background(), state{})
	if err == nil {
		t.Fatal("expected fan-out branch error")
	}
}

func TestGraph_FanOut_UnknownNode(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) {
			return []Send[state]{{Node: "ghost", State: state{}}}, nil
		},
		func(base state, _ []state) state { return base },
		"done",
	)
	g.AddEdge("done", END)

	_, err := g.Run(context.Background(), state{})
	if err == nil {
		t.Fatal("expected unknown fan-out node error")
	}
}

func TestGraph_FanOut_FanError(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) {
			return nil, errors.New("cannot plan branches")
		},
		func(base state, _ []state) state { return base },
		"done",
	)
	g.AddEdge("done", END)

	if _, err := g.Run(context.Background(), state{}); err == nil {
		t.Fatal("expected fan function error")
	}
}

func TestGraph_FanOut_EmptyReducesToBase(t *testing.T) {
	type state struct{ Tag string }

	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) {
		s.Tag = "seeded"
		return s, nil
	})
	g.AddNode("worker", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) { return nil, nil },
		func(base state, _ []state) state { return base },
		"done",
	)
	g.AddEdge("done", END)

	result, err := g.Run(context.Background(), state{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Tag != "seeded" {
		t.Errorf("expected base state to survive empty fan-out, got %q", result.State.Tag)
	}
}

func TestGraph_FanOut_NilReducerSingleBranch(t *testing.T) {
	type state struct{ Val int }

	g := New[state]()
	g.AddNode("seed", func(_ context.Context, s state) (state, error) { return s, nil })
	g.AddNode("worker", func(_ context.Context, s state) (state, error) {
		s.Val = 42
		return s, nil
	})
	g.AddNode("done", func(_ context.Context, s state) (state, error) { return s, nil })

	g.SetEntryPoint("seed")
	g.AddFanOut("seed",
		func(_ context.Context, _ state) ([]Send[state], error) {
			return []Send[state]{{Node: "worker", State: state{}}}, nil
		},
		nil, // nil reducer falls back to the sole branch result
		"done",
	)
	g.AddEdge("done", END)

	result, err := g.Run(context.Background(), state{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Val != 42 {
		t.Errorf("expected nil reducer to adopt single branch result, got %d", result.State.Val)
	}
}
