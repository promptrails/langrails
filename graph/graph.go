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

// defaultMaxSteps bounds execution to guard against infinite loops.
const defaultMaxSteps = 100

// Graph is a stateful workflow engine. It executes nodes in a graph
// structure, where transitions between nodes are determined by edges.
// S is the state type that flows through the graph.
//
// A Graph holds only structural configuration (nodes and edges) and is not
// mutated by Run, Resume, or Stream — per-run settings such as the
// checkpointer and step budget are supplied as options on each call. The
// same graph can therefore be run concurrently with different options
// without interference.
type Graph[S any] struct {
	nodes            map[string]NodeFunc[S]
	edges            map[string]string
	conditionalEdges map[string]RouterFunc[S]
	fanOuts          map[string]fanOut[S]
	entryPoint       string
}

// New creates a new graph with the given state type.
func New[S any]() *Graph[S] {
	return &Graph[S]{
		nodes:            make(map[string]NodeFunc[S]),
		edges:            make(map[string]string),
		conditionalEdges: make(map[string]RouterFunc[S]),
		fanOuts:          make(map[string]fanOut[S]),
	}
}

// runConfig holds the per-run settings supplied via options. It is built
// fresh for each Run/Resume/Stream call so options never leak between runs.
type runConfig[S any] struct {
	maxSteps     int
	checkpointer Checkpointer[S]
	threadID     string
}

func (g *Graph[S]) buildConfig(opts ...Option[S]) *runConfig[S] {
	cfg := &runConfig[S]{maxSteps: defaultMaxSteps}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// Option configures a single run.
type Option[S any] func(*runConfig[S])

// WithMaxSteps sets the maximum number of node executions for the run.
// This prevents infinite loops. Default is 100.
func WithMaxSteps[S any](n int) Option[S] {
	return func(c *runConfig[S]) {
		c.maxSteps = n
	}
}

// WithCheckpointer enables durable execution for the run. After each
// superstep the graph saves a checkpoint (the next node to run plus the
// current state) under the configured thread ID, allowing a crashed or
// paused run to be continued with [Graph.Resume]. A thread ID is required;
// set it with [WithThreadID]. Resume must be given the same checkpointer
// and thread ID, as options are not retained between calls.
func WithCheckpointer[S any](cp Checkpointer[S]) Option[S] {
	return func(c *runConfig[S]) {
		c.checkpointer = cp
	}
}

// WithThreadID sets the thread ID under which checkpoints are stored. A
// thread identifies one logical run (for example a conversation), so its
// checkpoints can be loaded and resumed independently of other runs.
func WithThreadID[S any](id string) Option[S] {
	return func(c *runConfig[S]) {
		c.threadID = id
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
	cfg := g.buildConfig(opts...)

	if g.entryPoint == "" {
		return nil, fmt.Errorf("graph: no entry point set")
	}
	if err := cfg.checkThread(); err != nil {
		return nil, err
	}

	return g.run(ctx, cfg, initialState, g.entryPoint, 0, nil)
}

// Stream executes the graph like Run but emits each node's StepEvent as it
// completes. It returns a channel of step updates and a one-shot error
// channel; both are closed when the run finishes. Range over events, then
// read the error:
//
//	events, errc := g.Stream(ctx, initial)
//	for ev := range events {
//		fmt.Printf("node %s done\n", ev.Node)
//	}
//	if err := <-errc; err != nil {
//		// handle failure
//	}
//
// The final state is the State of the last emitted event. Cancel ctx to
// stop the run between steps. Checkpointing applies here exactly as it does
// for Run.
func (g *Graph[S]) Stream(ctx context.Context, initialState S, opts ...Option[S]) (<-chan StepEvent[S], <-chan error) {
	events := make(chan StepEvent[S])
	errc := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errc)

		cfg := g.buildConfig(opts...)
		if g.entryPoint == "" {
			errc <- fmt.Errorf("graph: no entry point set")
			return
		}
		if err := cfg.checkThread(); err != nil {
			errc <- err
			return
		}

		emit := func(ev StepEvent[S]) {
			select {
			case events <- ev:
			case <-ctx.Done():
			}
		}
		if _, err := g.run(ctx, cfg, initialState, g.entryPoint, 0, emit); err != nil {
			errc <- err
		}
	}()

	return events, errc
}

// Resume continues a checkpointed run from its most recent checkpoint. It
// loads the latest checkpoint for the configured thread ID and executes
// the graph from the saved next node and state. If the saved checkpoint
// marks the run as complete, the persisted final state is returned without
// executing any further nodes.
//
// Because options are not retained between calls, Resume must be given the
// same checkpointer and thread ID that the original run used.
func (g *Graph[S]) Resume(ctx context.Context, opts ...Option[S]) (*RunResult[S], error) {
	cfg := g.buildConfig(opts...)

	if cfg.checkpointer == nil {
		return nil, fmt.Errorf("graph: resume requires a checkpointer")
	}
	if cfg.threadID == "" {
		return nil, fmt.Errorf("graph: resume requires a thread ID")
	}

	cp, ok, err := cfg.checkpointer.Load(ctx, cfg.threadID)
	if err != nil {
		return nil, fmt.Errorf("graph: load checkpoint: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("graph: no checkpoint for thread %q", cfg.threadID)
	}
	if cp.Done || cp.Node == END {
		return &RunResult[S]{State: cp.State}, nil
	}

	return g.run(ctx, cfg, cp.State, cp.Node, cp.Step, nil)
}

// run drives the execution loop from a given node and step offset. It is
// shared by Run (start from the entry point at step 0) and Resume (start
// from a loaded checkpoint). If emit is non-nil it is called with each
// StepEvent as the event is produced, which is how Stream observes
// progress live.
func (g *Graph[S]) run(ctx context.Context, cfg *runConfig[S], state S, currentNode string, startStep int, emit func(StepEvent[S])) (*RunResult[S], error) {
	result := &RunResult[S]{}
	record := func(ev StepEvent[S]) {
		result.Steps = append(result.Steps, ev)
		if emit != nil {
			emit(ev)
		}
	}

	// step is an explicit execution counter: it is incremented once per
	// node execution (including each fan-out branch) and is the authority
	// for both StepEvent numbering and the maxSteps budget.
	step := startStep
	for {
		if currentNode == END {
			result.State = state
			return result, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if step >= cfg.maxSteps {
			return nil, fmt.Errorf("graph: exceeded maximum steps (%d)", cfg.maxSteps)
		}

		fn, ok := g.nodes[currentNode]
		if !ok {
			return nil, fmt.Errorf("graph: unknown node %q", currentNode)
		}

		newState, err := fn(ctx, state)
		if err != nil {
			return nil, fmt.Errorf("graph: node %q failed: %w", currentNode, err)
		}

		step++
		state = newState
		record(StepEvent[S]{
			Node:  currentNode,
			State: state,
			Step:  step,
		})

		// Fan-out edges take precedence: run branches concurrently, then
		// reduce their results and continue at the join node.
		if fo, ok := g.fanOuts[currentNode]; ok {
			sends, err := g.planFanOut(ctx, fo, state)
			if err != nil {
				return nil, err
			}
			// Each branch is a node execution, so it must fit within the
			// remaining step budget before any branch (and its side effects)
			// runs.
			if step+len(sends) > cfg.maxSteps {
				return nil, fmt.Errorf("graph: exceeded maximum steps (%d)", cfg.maxSteps)
			}
			results, err := g.runBranches(ctx, sends)
			if err != nil {
				return nil, err
			}
			for i, snd := range sends {
				step++
				record(StepEvent[S]{
					Node:  snd.Node,
					State: results[i],
					Step:  step,
				})
			}
			state = reduceFanOut(fo.reduce, state, results)
			currentNode = fo.join
			if err := saveCheckpoint(ctx, cfg, step, currentNode, state); err != nil {
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

		if err := saveCheckpoint(ctx, cfg, step, currentNode, state); err != nil {
			return nil, err
		}
	}
}

// saveCheckpoint persists the next node to run and the current state. It
// is a no-op when no checkpointer is configured.
func saveCheckpoint[S any](ctx context.Context, cfg *runConfig[S], step int, nextNode string, state S) error {
	if cfg.checkpointer == nil {
		return nil
	}
	cp := Checkpoint[S]{
		ThreadID: cfg.threadID,
		Step:     step,
		Node:     nextNode,
		State:    state,
		Done:     nextNode == END,
	}
	if err := cfg.checkpointer.Save(ctx, cfg.threadID, cp); err != nil {
		return fmt.Errorf("graph: save checkpoint: %w", err)
	}
	return nil
}

// checkThread validates that a thread ID is present whenever a
// checkpointer is configured.
func (c *runConfig[S]) checkThread() error {
	if c.checkpointer != nil && c.threadID == "" {
		return fmt.Errorf("graph: checkpointer set but no thread ID (use WithThreadID)")
	}
	return nil
}

// planFanOut calls the fan function and validates that every branch target
// exists, so a typo fails fast and deterministically before any branch
// runs. It does not execute branches.
func (g *Graph[S]) planFanOut(ctx context.Context, fo fanOut[S], state S) ([]Send[S], error) {
	sends, err := fo.fan(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("graph: fan-out failed: %w", err)
	}
	for _, snd := range sends {
		if _, ok := g.nodes[snd.Node]; !ok {
			return nil, fmt.Errorf("graph: unknown fan-out node %q", snd.Node)
		}
	}
	return sends, nil
}

// runBranches runs every branch concurrently and collects their results in
// branch order. If any branch fails, the remaining branches are cancelled
// and the first error is returned.
func (g *Graph[S]) runBranches(ctx context.Context, sends []Send[S]) ([]S, error) {
	if len(sends) == 0 {
		return nil, nil
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
			return nil, fmt.Errorf("graph: fan-out node %q failed: %w", sends[i].Node, e)
		}
	}
	return results, nil
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
