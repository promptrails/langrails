// Package graph provides a LangGraph-style stateful workflow engine.
//
// A graph defines nodes connected by edges. Each node processes the current
// state and returns an updated state. Edges can be conditional, routing to
// different nodes based on state. Supports loops, branching, and multi-agent
// coordination patterns.
package graph
