// Package mcp provides a Model Context Protocol (MCP) client.
//
// It connects to MCP servers, discovers available tools, and executes them.
// The client implements tools.Executor so it can be used directly with
// tools.RunLoop for automatic tool calling.
package mcp
