// Package agent provides a middleware-driven tool-calling agent.
//
// An agent runs the same loop as tools.RunLoop — call the model, execute
// any requested tools, feed results back, repeat — but wraps each model
// call with middleware hooks. This is the extension model popularized by
// LangChain's create_agent: instead of rewriting the loop, you attach
// middleware that intercepts it.
//
// Three hooks are available (see [Middleware]):
//
//   - BeforeModel runs before each model call, in registration order. Use
//     it to rewrite the request — trim or summarize history, redact input.
//   - AfterModel runs after each model call, in reverse registration order.
//     Use it to inspect or rewrite the response, or call State.Stop to end
//     the loop.
//   - WrapModelCall composes around the model call (first middleware
//     outermost) for retries, timing, or caching.
//
// Embed [BaseMiddleware] to implement only the hooks you need.
//
// # Usage
//
//	a := agent.New(provider,
//		agent.WithModel("claude-sonnet-4-6"),
//		agent.WithSystemPrompt("You are a helpful assistant."),
//		agent.WithTools(defs, executor),
//		agent.WithMiddleware(agent.NewSummarization(provider, "claude-haiku-4-5-20251001")),
//	)
//
//	result, err := a.Run(ctx, "What's the weather in Istanbul?")
//	fmt.Println(result.Response.Content)
//
// Built-in middleware covers common needs: [SummarizationMiddleware]
// compresses long histories and [PIIRedactionMiddleware] masks sensitive
// data. For human-in-the-loop approval, [HumanInLoop] wraps a
// tools.Executor with an approval gate so a reviewer can approve or reject
// tool calls before they run.
package agent
