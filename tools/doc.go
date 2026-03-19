// Package tools provides automatic tool/function calling loop execution.
//
// When an LLM responds with tool calls, RunLoop handles the full cycle:
// executing tools via an Executor, sending results back to the LLM, and
// repeating until the model gives a final text response.
package tools
