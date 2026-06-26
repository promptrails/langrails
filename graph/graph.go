package graph

import (
	"context"
	"fmt"
	"sync"
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

// Send dispatches a copy of the state to a named node as part of a
// fan-out. Each Send becomes one concurrent branch. This mirrors
// LangGraph's Send API for map-reduce style workflows.
type Send[S any] struct {
	// Node is the name of the node to execute for this branch.
	Node string

	// State is the state copy passed to the branch node.
	State S
}

// FanFunc produces the set of branches to run concurrently from the
// current state. Returning an empty slice runs no branches and reduces
// over no results. Returning an error aborts the run.
type FanFunc[S any] func(ctx context.Context, state S) ([]Send[S], error)

// Reducer merges the results of concurrent fan-out branches back into a
// single state. base is the state as it was before the fan-out, and
// results holds each branch's output in branch order. The returned value
// becomes the state passed to the join node.
type Reducer[S any] func(base S, results []S) S

type fanOut[S any] struct {
	fan    FanFunc[S]
	reduce Reducer[S]
	join   string
}

// Graph is a stateful workflow engine. It executes nodes in a graph
// structure, where transitions between nodes are determined by edges.
// S is the state type that flows through the graph.
type Graph[S any] struct {
	nodes            map[string]NodeFunc[S]
	edges            map[string]string
	conditionalEdges map[string]RouterFunc[S]
	fanOuts          map[string]fanOut[S]
	entryPoint       string
	maxSteps         int
	checkpointer     Checkpointer[S]
	threadID         string
}

// New creates a new graph with the given state type.
func New[S any]() *Graph[S] {
	return &Graph[S]{
		nodes:            make(map[string]NodeFunc[S]),
		edges:            make(map[string]string),
		conditionalEdges: make(map[string]RouterFunc[S]),
		fanOuts:          make(map[string]fanOut[S]),
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

// WithCheckpointer enables durable execution. After each superstep the
// graph saves a checkpoint (the next node to run plus the current state)
// under the configured thread ID, allowing a crashed or paused run to be
// continued with [Graph.Resume]. A thread ID is required; set it with
// [WithThreadID].
func WithCheckpointer[S any](cp Checkpointer[S]) Option[S] {
	return func(g *Graph[S]) {
		g.checkpointer = cp
	}
}

// WithThreadID sets the thread ID under which checkpoints are stored. A
// thread identifies one logical run (for example a conversation), so its
// checkpoints can be loaded and resumed independently of other runs.
func WithThreadID[S any](id string) Option[S] {
	return func(g *Graph[S]) {
		g.threadID = id
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

// AddFanOut adds a fan-out edge. After the from node executes, fan is
// called to produce a set of branches; each branch runs its target node
// concurrently on its own state copy. Once every branch completes, reduce
// merges their results back into a single state, and execution continues
// at the join node.
//
// Branch target nodes are executed as leaf workers: their own outgoing
// edges are ignored. Use a subgraph (see AsNode) as the target node when a
// branch needs multiple steps.
//
// A fan-out edge takes precedence over any plain or conditional edge
// registered for the same from node.
func (g *Graph[S]) AddFanOut(from string, fan FanFunc[S], reduce Reducer[S], join string) {
	g.fanOuts[from] = fanOut[S]{fan: fan, reduce: reduce, join: join}
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
	if err := g.checkThread(); err != nil {
		return nil, err
	}

	return g.run(ctx, initialState, g.entryPoint, 0)
}

// Resume continues a checkpointed run from its most recent checkpoint. It
// loads the latest checkpoint for the configured thread ID and executes
// the graph from the saved next node and state. If the saved checkpoint
// marks the run as complete, the persisted final state is returned without
// executing any further nodes.
//
// A checkpointer and thread ID are required; pass them via options here or
// when the graph was first run.
func (g *Graph[S]) Resume(ctx context.Context, opts ...Option[S]) (*RunResult[S], error) {
	for _, opt := range opts {
		opt(g)
	}

	if g.checkpointer == nil {
		return nil, fmt.Errorf("graph: resume requires a checkpointer")
	}
	if g.threadID == "" {
		return nil, fmt.Errorf("graph: resume requires a thread ID")
	}

	cp, ok, err := g.checkpointer.Load(ctx, g.threadID)
	if err != nil {
		return nil, fmt.Errorf("graph: load checkpoint: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("graph: no checkpoint for thread %q", g.threadID)
	}
	if cp.Done || cp.Node == END {
		return &RunResult[S]{State: cp.State}, nil
	}

	return g.run(ctx, cp.State, cp.Node, cp.Step)
}

// run drives the execution loop from a given node and step offset. It is
// shared by Run (start from the entry point at step 0) and Resume (start
// from a loaded checkpoint).
func (g *Graph[S]) run(ctx context.Context, state S, currentNode string, startStep int) (*RunResult[S], error) {
	result := &RunResult[S]{}

	for step := startStep + 1; step <= g.maxSteps; step++ {
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

		// Fan-out edges take precedence: run branches concurrently, then
		// reduce their results and continue at the join node.
		if fo, ok := g.fanOuts[currentNode]; ok {
			results, sends, err := g.executeFanOut(ctx, fo, state)
			if err != nil {
				return nil, err
			}
			for i, snd := range sends {
				step++
				result.Steps = append(result.Steps, StepEvent[S]{
					Node:  snd.Node,
					State: results[i],
					Step:  step,
				})
			}
			state = reduceFanOut(fo.reduce, state, results)
			currentNode = fo.join
			if err := g.saveCheckpoint(ctx, step, currentNode, state); err != nil {
				return nil, err
			}
			continue
		}

		// Determine next node
		if router, ok := g.conditionalEdges[currentNode]; ok {
			currentNode = router(state)
		} else if next, ok := g.edges[currentNode]; ok {
			currentNode = next
		} else {
			return nil, fmt.Errorf("graph: no edge from node %q", currentNode)
		}

		if err := g.saveCheckpoint(ctx, step, currentNode, state); err != nil {
			return nil, err
		}
	}

	return nil, fmt.Errorf("graph: exceeded maximum steps (%d)", g.maxSteps)
}

// saveCheckpoint persists the next node to run and the current state. It
// is a no-op when no checkpointer is configured.
func (g *Graph[S]) saveCheckpoint(ctx context.Context, step int, nextNode string, state S) error {
	if g.checkpointer == nil {
		return nil
	}
	cp := Checkpoint[S]{
		ThreadID: g.threadID,
		Step:     step,
		Node:     nextNode,
		State:    state,
		Done:     nextNode == END,
	}
	if err := g.checkpointer.Save(ctx, g.threadID, cp); err != nil {
		return fmt.Errorf("graph: save checkpoint: %w", err)
	}
	return nil
}

// checkThread validates that a thread ID is present whenever a
// checkpointer is configured.
func (g *Graph[S]) checkThread() error {
	if g.checkpointer != nil && g.threadID == "" {
		return fmt.Errorf("graph: checkpointer set but no thread ID (use WithThreadID)")
	}
	return nil
}

// executeFanOut runs every branch returned by the fan function
// concurrently and collects their results in branch order. If any branch
// fails, the remaining branches are cancelled and the first error is
// returned.
func (g *Graph[S]) executeFanOut(ctx context.Context, fo fanOut[S], state S) ([]S, []Send[S], error) {
	sends, err := fo.fan(ctx, state)
	if err != nil {
		return nil, nil, fmt.Errorf("graph: fan-out failed: %w", err)
	}
	if len(sends) == 0 {
		return nil, sends, nil
	}

	// Validate targets up front so a typo fails fast and deterministically.
	for _, snd := range sends {
		if _, ok := g.nodes[snd.Node]; !ok {
			return nil, nil, fmt.Errorf("graph: unknown fan-out node %q", snd.Node)
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make([]S, len(sends))
	errs := make([]error, len(sends))
	var wg sync.WaitGroup
	for i, snd := range sends {
		wg.Add(1)
		go func(i int, fn NodeFunc[S], st S) {
			defer wg.Done()
			out, e := fn(ctx, st)
			if e != nil {
				errs[i] = e
				cancel()
				return
			}
			results[i] = out
		}(i, g.nodes[snd.Node], snd.State)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			return nil, nil, fmt.Errorf("graph: fan-out node %q failed: %w", sends[i].Node, e)
		}
	}
	return results, sends, nil
}

// reduceFanOut merges branch results back into a single state. A nil
// reducer falls back to the sole result when there is exactly one branch,
// otherwise it keeps the pre-fan-out state.
func reduceFanOut[S any](reduce Reducer[S], base S, results []S) S {
	if reduce != nil {
		return reduce(base, results)
	}
	if len(results) == 1 {
		return results[0]
	}
	return base
}
