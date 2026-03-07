// Package graph provides a LangGraph-style stateful workflow engine.
//
// A graph defines a set of nodes connected by edges. Each node processes
// the current state and returns an updated state. Edges can be conditional,
// routing to different nodes based on the state.
//
// This enables complex agent architectures like:
//   - Multi-step reasoning with branching logic
//   - Human-in-the-loop approval flows
//   - Iterative refinement loops
//   - Multi-agent coordination
//
// # Usage
//
//	g := graph.New[MyState]()
//	g.AddNode("classify", classifyFn)
//	g.AddNode("handle_positive", handlePositiveFn)
//	g.AddNode("handle_negative", handleNegativeFn)
//
//	g.SetEntryPoint("classify")
//	g.AddConditionalEdge("classify", func(s MyState) string {
//		if s.Sentiment == "positive" {
//			return "handle_positive"
//		}
//		return "handle_negative"
//	})
//	g.AddEdge("handle_positive", graph.END)
//	g.AddEdge("handle_negative", graph.END)
//
//	result, err := g.Run(ctx, MyState{Input: "I love this!"})
package graph

import (
	"context"
	"fmt"
)

// END is a special node name that signals the graph should stop.
const END = "__end__"

// NodeFunc is a function that processes the current state and returns
// an updated state. It receives the context and current state, and
// should return the new state or an error.
type NodeFunc[S any] func(ctx context.Context, state S) (S, error)

// RouterFunc determines the next node based on the current state.
// It should return the name of the next node to execute.
type RouterFunc[S any] func(state S) string

// Graph is a stateful workflow engine. It executes nodes in a graph
// structure, where transitions between nodes are determined by edges.
// S is the state type that flows through the graph.
type Graph[S any] struct {
	nodes            map[string]NodeFunc[S]
	edges            map[string]string
	conditionalEdges map[string]RouterFunc[S]
	entryPoint       string
	maxSteps         int
}

// New creates a new graph with the given state type.
func New[S any]() *Graph[S] {
	return &Graph[S]{
		nodes:            make(map[string]NodeFunc[S]),
		edges:            make(map[string]string),
		conditionalEdges: make(map[string]RouterFunc[S]),
		maxSteps:         100,
	}
}

// Option configures the graph.
type Option[S any] func(*Graph[S])

// WithMaxSteps sets the maximum number of node executions.
// This prevents infinite loops. Default is 100.
func WithMaxSteps[S any](n int) Option[S] {
	return func(g *Graph[S]) {
		g.maxSteps = n
	}
}

// AddNode registers a node with the given name and function.
func (g *Graph[S]) AddNode(name string, fn NodeFunc[S]) {
	g.nodes[name] = fn
}

// SetEntryPoint sets the first node to execute.
func (g *Graph[S]) SetEntryPoint(name string) {
	g.entryPoint = name
}

// AddEdge adds a direct edge from one node to another.
// Use END as the target to signal completion.
func (g *Graph[S]) AddEdge(from, to string) {
	g.edges[from] = to
}

// AddConditionalEdge adds an edge that uses a router function to
// determine the next node based on the current state.
func (g *Graph[S]) AddConditionalEdge(from string, router RouterFunc[S]) {
	g.conditionalEdges[from] = router
}

// StepEvent is emitted after each node execution for observability.
type StepEvent[S any] struct {
	// Node is the name of the node that was executed.
	Node string

	// State is the state after the node executed.
	State S

	// Step is the step number (1-indexed).
	Step int
}

// RunResult contains the final state and execution history.
type RunResult[S any] struct {
	// State is the final state after the graph completes.
	State S

	// Steps contains the history of node executions.
	Steps []StepEvent[S]
}

// Run executes the graph starting from the entry point with the given
// initial state. It follows edges until it reaches END or the maximum
// number of steps is exceeded.
func (g *Graph[S]) Run(ctx context.Context, initialState S, opts ...Option[S]) (*RunResult[S], error) {
	for _, opt := range opts {
		opt(g)
	}

	if g.entryPoint == "" {
		return nil, fmt.Errorf("graph: no entry point set")
	}

	state := initialState
	currentNode := g.entryPoint
	result := &RunResult[S]{}

	for step := 1; step <= g.maxSteps; step++ {
		if currentNode == END {
			result.State = state
			return result, nil
		}

		fn, ok := g.nodes[currentNode]
		if !ok {
			return nil, fmt.Errorf("graph: unknown node %q", currentNode)
		}

		newState, err := fn(ctx, state)
		if err != nil {
			return nil, fmt.Errorf("graph: node %q failed: %w", currentNode, err)
		}

		state = newState
		result.Steps = append(result.Steps, StepEvent[S]{
			Node:  currentNode,
			State: state,
			Step:  step,
		})

		// Determine next node
		if router, ok := g.conditionalEdges[currentNode]; ok {
			currentNode = router(state)
		} else if next, ok := g.edges[currentNode]; ok {
			currentNode = next
		} else {
			return nil, fmt.Errorf("graph: no edge from node %q", currentNode)
		}
	}

	return nil, fmt.Errorf("graph: exceeded maximum steps (%d)", g.maxSteps)
}
