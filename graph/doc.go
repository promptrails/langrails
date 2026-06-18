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
// For simple linear pipelines without branching, the chain package is lighter
// weight.
package graph
