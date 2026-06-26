package graph

import (
	"context"
	"fmt"
)

// AsNode embeds a compiled graph as a node in a parent graph. When the
// node runs, in maps the parent state to the subgraph's input state, the
// subgraph executes to completion, and out merges the subgraph's final
// state back into the parent state.
//
// This is the building block for multi-agent and supervisor patterns: a
// coordinator graph delegates to specialist subgraphs, each with its own
// nodes, edges, and state type, and folds their results back in.
//
// The subgraph runs as a single parent step; a failure inside it is
// returned wrapped to the parent.
//
//	parent.AddNode("research", graph.AsNode(researchGraph,
//		func(p Parent) Research { return Research{Topic: p.Topic} },
//		func(r Research, p Parent) Parent { p.Findings = r.Result; return p },
//	))
func AsNode[P, S any](sub *Graph[S], in func(P) S, out func(S, P) P) NodeFunc[P] {
	return func(ctx context.Context, p P) (P, error) {
		res, err := sub.Run(ctx, in(p))
		if err != nil {
			return p, fmt.Errorf("graph: subgraph: %w", err)
		}
		return out(res.State, p), nil
	}
}
