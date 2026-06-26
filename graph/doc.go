// Package graph provides a LangGraph-style stateful workflow engine.
//
// A graph defines nodes connected by edges. Each node processes the current
// state and returns an updated state. Edges can be conditional, routing to
// different nodes based on state. Supports loops, branching, and multi-agent
// coordination patterns.
//
// The graph is generic over the state type S, which flows through every node.
//
// # Usage
//
//	type State struct {
//		Input  string
//		Result string
//	}
//
//	g := graph.New[State]()
//	g.AddNode("work", func(ctx context.Context, s State) (State, error) {
//		s.Result = "processed: " + s.Input
//		return s, nil
//	})
//	g.SetEntryPoint("work")
//	g.AddEdge("work", graph.END)
//
//	res, err := g.Run(ctx, State{Input: "hello"})
//	fmt.Println(res.State.Result) // processed: hello
//
// Use [Graph.AddConditionalEdge] with a [RouterFunc] to branch or loop based on
// state, and [WithMaxSteps] to bound execution (the default cap is 100 steps,
// which guards against infinite loops). [RunResult].Steps records the node
// execution history for observability.
//
// # Parallel fan-out
//
// Use [Graph.AddFanOut] to fan a node out into concurrent branches and merge
// their results — the map-reduce pattern. A [FanFunc] turns the current state
// into a slice of [Send] values (one per branch), each branch runs its target
// node on its own state copy, and a [Reducer] folds the branch results back
// into a single state before execution continues at the join node:
//
//	g.AddFanOut("split",
//		func(ctx context.Context, s State) ([]graph.Send[State], error) {
//			sends := make([]graph.Send[State], len(s.Docs))
//			for i, d := range s.Docs {
//				sends[i] = graph.Send[State]{Node: "summarize", State: State{Doc: d}}
//			}
//			return sends, nil
//		},
//		func(base State, results []State) State {
//			for _, r := range results {
//				base.Summaries = append(base.Summaries, r.Summary)
//			}
//			return base
//		},
//		"reduce",
//	)
//
// # Durable execution
//
// Pass [WithCheckpointer] and [WithThreadID] to make a run durable. After
// every superstep the graph saves a [Checkpoint] (the next node plus the
// current state) so a crashed or paused run can be continued with
// [Graph.Resume]. [MemoryCheckpointer] is the built-in in-memory store;
// any type implementing [Checkpointer] (SQLite, Postgres, ...) can be used.
// [Checkpointer.History] exposes the full checkpoint sequence for
// time-travel debugging.
//
// For simple linear pipelines without branching, the chain package is lighter
// weight.
package graph
