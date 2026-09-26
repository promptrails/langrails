// Package mcp provides a Model Context Protocol (MCP) client.
//
// It connects to MCP servers over streamable HTTP (NewClient) or as a child
// process over stdio (NewStdioClient), discovers and executes their tools,
// and reads their resources and prompts. The client implements
// tools.Executor so it can be used directly with tools.RunLoop.
package mcp
