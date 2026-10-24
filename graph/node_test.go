package graph

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNode_Timeout(t *testing.T) {
	g := New[testState]()
	g.AddNodeWithOptions("slow", func(ctx context.Context, s testState) (testState, error) {
		<-ctx.Done()
		return s, ctx.Err()
	}, WithNodeTimeout[testState](20*time.Millisecond))
	g.AddNodeWithOptions("stuck", func(_ context.Context, s testState) (testState, error) {
		time.Sleep(200 * time.Millisecond) // ignores its context
		return s, nil
	}, WithNodeTimeout[testState](20*time.Millisecond))
	g.AddEdge("slow", END)
	g.AddEdge("stuck", END)

	for _, node := range []string{"slow", "stuck"} {
		g.SetEntryPoint(node)
		start := time.Now()
		_, err := g.Run(context.Background(), testState{})
		if !errors.Is(err, ErrNodeTimeout) {
			t.Fatalf("%s: err = %v, want ErrNodeTimeout", node, err)
		}
		if time.Since(start) > 150*time.Millisecond {
			t.Errorf("%s: run did not stop at the timeout", node)
		}
	}
}

func TestNode_TimeoutNotHitPassesThrough(t *testing.T) {
	g := New[testState]()
	g.AddNodeWithOptions("fast", func(_ context.Context, s testState) (testState, error) {
		return s, errors.New("boom")
	}, WithNodeTimeout[testState](time.Second))
	g.SetEntryPoint("fast")
	_, err := g.Run(context.Background(), testState{})
	if err == nil || errors.Is(err, ErrNodeTimeout) {
		t.Fatalf("err = %v", err)
	}
}

func TestNode_Retry(t *testing.T) {
	var calls atomic.Int32
	g := New[testState]()
	g.AddNodeWithOptions("flaky", func(_ context.Context, s testState) (testState, error) {
		if calls.Add(1) < 3 {
			return s, errors.New("transient")
		}
		s.Output = "ok"
		return s, nil
	}, WithNodeRetry[testState](2, time.Millisecond))
	g.SetEntryPoint("flaky")
	g.AddEdge("flaky", END)

	res, err := g.Run(context.Background(), testState{})
	if err != nil || res.State.Output != "ok" || calls.Load() != 3 {
		t.Fatalf("res = %+v, err = %v, calls = %d", res, err, calls.Load())
	}

	calls.Store(-10)
	if _, err := g.Run(context.Background(), testState{}); err == nil {
		t.Fatal("expected failure once retries are exhausted")
	}
	if calls.Load() != -7 {
		t.Errorf("calls = %d, want 3 attempts", calls.Load()+10)
	}
}

func TestNode_RetryNotOnInterrupt(t *testing.T) {
	var calls atomic.Int32
	g := New[testState]()
	g.AddNodeWithOptions("ask", func(ctx context.Context, s testState) (testState, error) {
		calls.Add(1)
		_, err := Await[string](ctx, "?")
		return s, err
	}, WithNodeRetry[testState](3, 0), WithErrorHandler(func(context.Context, testState, error) (testState, string, error) {
		t.Error("error handler saw an interrupt")
		return testState{}, "", nil
	}))
	g.SetEntryPoint("ask")
	g.AddEdge("ask", END)

	cp := NewMemoryCheckpointer[testState]()
	res, err := g.Run(context.Background(), testState{}, WithCheckpointer[testState](cp), WithThreadID[testState]("t"))
	if err != nil || res.Interruption == nil || calls.Load() != 1 {
		t.Fatalf("res = %+v, err = %v, calls = %d", res, err, calls.Load())
	}
}

func TestNode_ErrorHandler(t *testing.T) {
	g := New[testState]()
	handler := func(_ context.Context, s testState, err error) (testState, string, error) {
		s.Output = "recovered: " + err.Error()
		if s.Input == "reroute" {
			return s, "apologize", nil
		}
		if s.Input == "give up" {
			return s, "", errors.New("handler gave up")
		}
		return s, "", nil
	}
	g.AddNodeWithOptions("work", func(_ context.Context, s testState) (testState, error) {
		return s, errors.New("boom")
	}, WithErrorHandler(handler))
	g.AddNode("next", func(_ context.Context, s testState) (testState, error) {
		s.Steps = append(s.Steps, "next")
		return s, nil
	})
	g.AddNode("apologize", func(_ context.Context, s testState) (testState, error) {
		s.Steps = append(s.Steps, "apologize")
		return s, nil
	})
	g.SetEntryPoint("work")
	g.AddEdge("work", "next")
	g.AddEdge("next", END)
	g.AddEdge("apologize", END)

	res, err := g.Run(context.Background(), testState{})
	if err != nil || res.State.Output != "recovered: boom" || len(res.State.Steps) != 1 || res.State.Steps[0] != "next" {
		t.Fatalf("follow edges: res = %+v, err = %v", res, err)
	}
	res, err = g.Run(context.Background(), testState{Input: "reroute"})
	if err != nil || len(res.State.Steps) != 1 || res.State.Steps[0] != "apologize" {
		t.Fatalf("reroute: res = %+v, err = %v", res, err)
	}
	if _, err = g.Run(context.Background(), testState{Input: "give up"}); err == nil || err.Error() != `graph: node "work" failed: handler gave up` {
		t.Fatalf("give up: err = %v", err)
	}
}

func TestNode_ErrorHandlerInFanOut(t *testing.T) {
	g := New[testState]()
	g.AddNode("start", func(_ context.Context, s testState) (testState, error) { return s, nil })
	g.AddNodeWithOptions("branch", func(_ context.Context, s testState) (testState, error) {
		if s.Input == "bad" {
			return s, errors.New("boom")
		}
		s.Output = s.Input
		return s, nil
	}, WithErrorHandler(func(_ context.Context, s testState, _ error) (testState, string, error) {
		s.Output = "fallback"
		return s, "ignored", nil
	}))
	g.AddNode("join", func(_ context.Context, s testState) (testState, error) { return s, nil })
	g.SetEntryPoint("start")
	g.AddFanOut("start", func(_ context.Context, _ testState) ([]Send[testState], error) {
		return []Send[testState]{{Node: "branch", State: testState{Input: "good"}}, {Node: "branch", State: testState{Input: "bad"}}}, nil
	}, func(base testState, results []testState) testState {
		for _, r := range results {
			base.Steps = append(base.Steps, r.Output)
		}
		return base
	}, "join")
	g.AddEdge("join", END)

	res, err := g.Run(context.Background(), testState{})
	if err != nil || len(res.State.Steps) != 2 || res.State.Steps[1] != "fallback" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestNode_AddNodeClearsOptions(t *testing.T) {
	g := New[testState]()
	g.AddNodeWithOptions("n", func(_ context.Context, s testState) (testState, error) { return s, nil },
		WithErrorHandler(func(_ context.Context, s testState, _ error) (testState, string, error) { return s, "", nil }))
	g.AddNode("n", func(_ context.Context, s testState) (testState, error) { return s, errors.New("boom") })
	g.SetEntryPoint("n")
	g.AddEdge("n", END)
	if _, err := g.Run(context.Background(), testState{}); err == nil {
		t.Fatal("replaced node kept the old error handler")
	}
}
