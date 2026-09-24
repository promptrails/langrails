// Package tools provides typed tool definitions and automatic tool/function
// calling loop execution.
//
// New builds a tool from a typed Go function, deriving the parameter schema
// from the input struct; a Set collects tools and serves both their
// definitions and an Executor.
//
// When an LLM responds with tool calls, RunLoop handles the full cycle:
// executing tools via an Executor, sending results back to the LLM, and
// repeating until the model gives a final text response.
package tools
