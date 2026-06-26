package graph

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// research subgraph state
type research struct {
	Topic  string
	Result string
}

// write subgraph state
type write struct {
	Brief string
	Draft string
}

// parent supervisor state
type article struct {
	Topic    string
	Findings string
	Draft    string
}

func newResearchGraph() *Graph[research] {
	g := New[research]()
	g.AddNode("gather", func(_ context.Context, s research) (research, error) {
		s.Result = "facts about " + s.Topic
		return s, nil
	})
	g.AddNode("refine", func(_ context.Context, s research) (research, error) {
		s.Result = strings.ToUpper(s.Result)
		return s, nil
	})
	g.SetEntryPoint("gather")
	g.AddEdge("gather", "refine")
	g.AddEdge("refine", END)
	return g
}

func newWriteGraph() *Graph[write] {
	g := New[write]()
	g.AddNode("draft", func(_ context.Context, s write) (write, error) {
		s.Draft = "draft based on: " + s.Brief
		return s, nil
	})
	g.SetEntryPoint("draft")
	g.AddEdge("draft", END)
	return g
}

func TestAsNode_Supervisor(t *testing.T) {
	parent := New[article]()

	parent.AddNode("research", AsNode(newResearchGraph(),
		func(p article) research { return research{Topic: p.Topic} },
		func(r research, p article) article { p.Findings = r.Result; return p },
	))
	parent.AddNode("write", AsNode(newWriteGraph(),
		func(p article) write { return write{Brief: p.Findings} },
		func(w write, p article) article { p.Draft = w.Draft; return p },
	))

	parent.SetEntryPoint("research")
	parent.AddEdge("research", "write")
	parent.AddEdge("write", END)

	result, err := parent.Run(context.Background(), article{Topic: "otters"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Findings != "FACTS ABOUT OTTERS" {
		t.Errorf("unexpected findings: %q", result.State.Findings)
	}
	if result.State.Draft != "draft based on: FACTS ABOUT OTTERS" {
		t.Errorf("unexpected draft: %q", result.State.Draft)
	}
	// Two parent steps: research, write (subgraph internals are not parent steps).
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 parent steps, got %d", len(result.Steps))
	}
}

func TestAsNode_SubgraphErrorPropagates(t *testing.T) {
	sub := New[research]()
	sub.AddNode("boom", func(_ context.Context, s research) (research, error) {
		return s, errors.New("subgraph node failed")
	})
	sub.SetEntryPoint("boom")
	sub.AddEdge("boom", END)

	parent := New[article]()
	parent.AddNode("research", AsNode(sub,
		func(p article) research { return research{Topic: p.Topic} },
		func(r research, p article) article { p.Findings = r.Result; return p },
	))
	parent.SetEntryPoint("research")
	parent.AddEdge("research", END)

	_, err := parent.Run(context.Background(), article{Topic: "x"})
	if err == nil {
		t.Fatal("expected subgraph error to propagate")
	}
	if !strings.Contains(err.Error(), "subgraph") {
		t.Errorf("expected wrapped subgraph error, got: %v", err)
	}
}
