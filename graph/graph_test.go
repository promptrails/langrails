package graph

import (
	"context"
	"errors"
	"testing"
)

type testState struct {
	Input     string
	Output    string
	Sentiment string
	Steps     []string
}

func TestGraph_LinearFlow(t *testing.T) {
	g := New[testState]()

	g.AddNode("step1", func(_ context.Context, s testState) (testState, error) {
		s.Steps = append(s.Steps, "step1")
		s.Output = "processed: " + s.Input
		return s, nil
	})
	g.AddNode("step2", func(_ context.Context, s testState) (testState, error) {
		s.Steps = append(s.Steps, "step2")
		s.Output = "final: " + s.Output
		return s, nil
	})

	g.SetEntryPoint("step1")
	g.AddEdge("step1", "step2")
	g.AddEdge("step2", END)

	result, err := g.Run(context.Background(), testState{Input: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.State.Output != "final: processed: hello" {
		t.Errorf("unexpected output: %q", result.State.Output)
	}
	if len(result.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(result.Steps))
	}
}

func TestGraph_ConditionalEdge(t *testing.T) {
	g := New[testState]()

	g.AddNode("classify", func(_ context.Context, s testState) (testState, error) {
		if s.Input == "good" {
			s.Sentiment = "positive"
		} else {
			s.Sentiment = "negative"
		}
		return s, nil
	})
	g.AddNode("handle_positive", func(_ context.Context, s testState) (testState, error) {
		s.Output = "Thank you!"
		return s, nil
	})
	g.AddNode("handle_negative", func(_ context.Context, s testState) (testState, error) {
		s.Output = "Sorry to hear that."
		return s, nil
	})

	g.SetEntryPoint("classify")
	g.AddConditionalEdge("classify", func(s testState) string {
		if s.Sentiment == "positive" {
			return "handle_positive"
		}
		return "handle_negative"
	})
	g.AddEdge("handle_positive", END)
	g.AddEdge("handle_negative", END)

	// Test positive path
	result, err := g.Run(context.Background(), testState{Input: "good"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Output != "Thank you!" {
		t.Errorf("expected 'Thank you!', got %q", result.State.Output)
	}

	// Test negative path
	result, err = g.Run(context.Background(), testState{Input: "bad"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Output != "Sorry to hear that." {
		t.Errorf("expected 'Sorry to hear that.', got %q", result.State.Output)
	}
}

func TestGraph_Loop(t *testing.T) {
	type counterState struct {
		Count int
	}

	g := New[counterState]()

	g.AddNode("increment", func(_ context.Context, s counterState) (counterState, error) {
		s.Count++
		return s, nil
	})

	g.SetEntryPoint("increment")
	g.AddConditionalEdge("increment", func(s counterState) string {
		if s.Count >= 5 {
			return END
		}
		return "increment"
	})

	result, err := g.Run(context.Background(), counterState{Count: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State.Count != 5 {
		t.Errorf("expected count 5, got %d", result.State.Count)
	}
	if len(result.Steps) != 5 {
		t.Errorf("expected 5 steps, got %d", len(result.Steps))
	}
}

func TestGraph_MaxSteps(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.AddNode("loop", func(_ context.Context, s state) (state, error) {
		return s, nil
	})
	g.SetEntryPoint("loop")
	g.AddEdge("loop", "loop") // infinite loop

	_, err := g.Run(context.Background(), state{}, WithMaxSteps[state](5))
	if err == nil {
		t.Fatal("expected max steps error")
	}
}

func TestGraph_NodeError(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.AddNode("fail", func(_ context.Context, s state) (state, error) {
		return s, errors.New("node failed")
	})
	g.SetEntryPoint("fail")
	g.AddEdge("fail", END)

	_, err := g.Run(context.Background(), state{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGraph_UnknownNode(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.SetEntryPoint("nonexistent")

	_, err := g.Run(context.Background(), state{})
	if err == nil {
		t.Fatal("expected unknown node error")
	}
}

func TestGraph_NoEntryPoint(t *testing.T) {
	type state struct{}

	g := New[state]()
	_, err := g.Run(context.Background(), state{})
	if err == nil {
		t.Fatal("expected no entry point error")
	}
}

func TestGraph_NoEdge(t *testing.T) {
	type state struct{}

	g := New[state]()
	g.AddNode("orphan", func(_ context.Context, s state) (state, error) {
		return s, nil
	})
	g.SetEntryPoint("orphan")

	_, err := g.Run(context.Background(), state{})
	if err == nil {
		t.Fatal("expected no edge error")
	}
}
