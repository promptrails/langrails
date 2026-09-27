package graph

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type deploy struct {
	Version  string
	Built    int
	Approved bool
	Deployed bool
}

func deployGraph(buildRuns *int) *Graph[deploy] {
	g := New[deploy]()
	g.AddNode("build", func(_ context.Context, s deploy) (deploy, error) {
		*buildRuns++
		s.Built++
		return s, nil
	})
	g.AddNode("approve", func(ctx context.Context, s deploy) (deploy, error) {
		ok, err := Await[bool](ctx, "Deploy "+s.Version+"?")
		if err != nil {
			return s, err
		}
		s.Approved = ok
		return s, nil
	})
	g.AddNode("deploy", func(_ context.Context, s deploy) (deploy, error) {
		s.Deployed = s.Approved
		return s, nil
	})
	g.SetEntryPoint("build")
	g.AddEdge("build", "approve")
	g.AddEdge("approve", "deploy")
	g.AddEdge("deploy", END)
	return g
}

func TestInterrupt_PauseAndResume(t *testing.T) {
	var buildRuns int
	g := deployGraph(&buildRuns)
	cp := NewMemoryCheckpointer[deploy]()
	opts := []Option[deploy]{WithCheckpointer[deploy](cp), WithThreadID[deploy]("t1")}

	res, err := g.Run(context.Background(), deploy{Version: "v2"}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Interruption == nil || res.Interruption.Node != "approve" || res.Interruption.Payload != "Deploy v2?" {
		t.Fatalf("interruption = %+v", res.Interruption)
	}
	if res.State.Built != 1 || res.State.Deployed {
		t.Errorf("paused state = %+v", res.State)
	}

	saved, _, _ := cp.Load(context.Background(), "t1")
	if saved.Interruption == nil || saved.Node != "approve" || saved.Done {
		t.Errorf("checkpoint = %+v", saved)
	}

	// Resuming without an answer asks again.
	res, err = g.Resume(context.Background(), opts...)
	if err != nil || res.Interruption == nil {
		t.Fatalf("resume without value: %+v, %v", res, err)
	}

	// A fresh graph value (as after a restart) resumes from the checkpoint.
	g2 := deployGraph(&buildRuns)
	res, err = g2.Resume(context.Background(), append(opts, WithResumeValue[deploy](true))...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Interruption != nil || !res.State.Approved || !res.State.Deployed || res.State.Built != 1 {
		t.Errorf("resumed state = %+v, interruption = %+v", res.State, res.Interruption)
	}
	if buildRuns != 1 {
		t.Errorf("build ran %d times; completed nodes must not re-run", buildRuns)
	}

	final, _, _ := cp.Load(context.Background(), "t1")
	if !final.Done || final.Interruption != nil {
		t.Errorf("final checkpoint = %+v", final)
	}
}

func TestInterrupt_WrongResumeType(t *testing.T) {
	var n int
	g := deployGraph(&n)
	cp := NewMemoryCheckpointer[deploy]()
	opts := []Option[deploy]{WithCheckpointer[deploy](cp), WithThreadID[deploy]("t")}
	if _, err := g.Run(context.Background(), deploy{}, opts...); err != nil {
		t.Fatal(err)
	}
	_, err := g.Resume(context.Background(), append(opts, WithResumeValue[deploy]("yes"))...)
	if err == nil || !strings.Contains(err.Error(), "resume value is string, want bool") {
		t.Errorf("err = %v", err)
	}
}

func TestInterrupt_WithoutCheckpointer(t *testing.T) {
	var n int
	_, err := deployGraph(&n).Run(context.Background(), deploy{})
	var ie *InterruptError
	if !errors.As(err, &ie) || !strings.Contains(err.Error(), "no checkpointer") {
		t.Errorf("err = %v", err)
	}
}

func TestInterrupt_InsideSubgraph(t *testing.T) {
	var buildRuns int
	sub := deployGraph(&buildRuns)

	type parent struct{ Deployed bool }
	g := New[parent]()
	g.AddNode("release", AsNode(sub,
		func(parent) deploy { return deploy{Version: "v3"} },
		func(d deploy, p parent) parent { p.Deployed = d.Deployed; return p },
	))
	g.SetEntryPoint("release")
	g.AddEdge("release", END)

	cp := NewMemoryCheckpointer[parent]()
	opts := []Option[parent]{WithCheckpointer[parent](cp), WithThreadID[parent]("p")}
	res, err := g.Run(context.Background(), parent{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Interruption == nil || res.Interruption.Node != "release" || res.Interruption.Payload != "Deploy v3?" {
		t.Fatalf("interruption = %+v", res.Interruption)
	}

	res, err = g.Resume(context.Background(), append(opts, WithResumeValue[parent](true))...)
	if err != nil {
		t.Fatal(err)
	}
	if !res.State.Deployed {
		t.Errorf("state = %+v", res.State)
	}
	// The subgraph restarts from its entry point on resume.
	if buildRuns != 2 {
		t.Errorf("sub build ran %d times, want 2", buildRuns)
	}
}

func TestInterrupt_FanOutBranchUnsupported(t *testing.T) {
	g := New[int]()
	g.AddNode("split", func(_ context.Context, s int) (int, error) { return s, nil })
	g.AddNode("ask", func(ctx context.Context, s int) (int, error) {
		_, err := Await[int](ctx, "?")
		return s, err
	})
	g.AddNode("join", func(_ context.Context, s int) (int, error) { return s, nil })
	g.SetEntryPoint("split")
	g.AddFanOut("split", func(context.Context, int) ([]Send[int], error) {
		return []Send[int]{{Node: "ask"}}, nil
	}, nil, "join")
	g.AddEdge("join", END)

	cp := NewMemoryCheckpointer[int]()
	_, err := g.Run(context.Background(), 0, WithCheckpointer[int](cp), WithThreadID[int]("f"))
	if err == nil || !strings.Contains(err.Error(), "not supported inside fan-out") {
		t.Errorf("err = %v", err)
	}
}

func TestInterrupt_Stream(t *testing.T) {
	var n int
	cp := NewMemoryCheckpointer[deploy]()
	events, errc := deployGraph(&n).Stream(context.Background(), deploy{},
		WithCheckpointer[deploy](cp), WithThreadID[deploy]("s"))

	var last StepEvent[deploy]
	for ev := range events {
		last = ev
	}
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if last.Interruption == nil || last.Node != "approve" {
		t.Errorf("last event = %+v", last)
	}
}
