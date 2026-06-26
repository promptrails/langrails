package graph

import (
	"context"
	"errors"
	"testing"
)

func TestStream_EmitsEventsInOrder(t *testing.T) {
	g := buildCountingGraph() // a -> b -> c -> END, increments Count

	events, errc := g.Stream(context.Background(), counterState{})

	var nodes []string
	var last counterState
	for ev := range events {
		nodes = append(nodes, ev.Node)
		last = ev.State
	}
	if err := <-errc; err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"a", "b", "c"}
	if len(nodes) != len(want) {
		t.Fatalf("expected %d events, got %d (%v)", len(want), len(nodes), nodes)
	}
	for i := range want {
		if nodes[i] != want[i] {
			t.Errorf("event %d: expected node %q, got %q", i, want[i], nodes[i])
		}
	}
	if last.Count != 3 {
		t.Errorf("expected final count 3 from last event, got %d", last.Count)
	}
}

func TestStream_PropagatesError(t *testing.T) {
	g := New[counterState]()
	g.AddNode("ok", func(_ context.Context, s counterState) (counterState, error) {
		s.Count++
		return s, nil
	})
	g.AddNode("boom", func(_ context.Context, s counterState) (counterState, error) {
		return s, errors.New("stream node failed")
	})
	g.SetEntryPoint("ok")
	g.AddEdge("ok", "boom")
	g.AddEdge("boom", END)

	events, errc := g.Stream(context.Background(), counterState{})
	count := 0
	for range events {
		count++
	}
	if err := <-errc; err == nil {
		t.Fatal("expected error from failing node")
	}
	// Only "ok" emits before the failure.
	if count != 1 {
		t.Errorf("expected 1 event before failure, got %d", count)
	}
}

func TestStream_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	g := New[counterState]()
	g.AddNode("loop", func(_ context.Context, s counterState) (counterState, error) {
		s.Count++
		return s, nil
	})
	g.SetEntryPoint("loop")
	g.AddEdge("loop", "loop") // would run until maxSteps without cancel

	events, errc := g.Stream(ctx, counterState{}, WithMaxSteps[counterState](1_000_000))

	// Read a couple of events, then cancel.
	<-events
	cancel()
	// Drain remaining events so the producer can observe cancellation.
	for range events {
	}
	if err := <-errc; err == nil {
		t.Fatal("expected context cancellation error")
	}
}
