// Package chain provides sequential prompt chain execution.
//
// A chain is a series of steps where each step's output feeds into the next
// step's input. Steps can use different providers, models, and transforms,
// which makes a chain a convenient way to express multi-stage prompting (for
// example: summarize, then translate, then critique).
//
// # Usage
//
//	c := chain.New(provider, []chain.Step{
//		{SystemPrompt: "Summarize the user's text in one sentence."},
//		{
//			SystemPrompt:  "Translate the text to French.",
//			InputTemplate: "Translate: {input}",
//		},
//	}, chain.WithModel("gpt-4o"))
//
//	result, err := c.Run(ctx, "Some long article text...")
//	fmt.Println(result.Output)       // final step output
//	fmt.Println(result.TotalUsage)   // token usage across all steps
//
// The initial Run input is the user message for the first step; each
// subsequent step receives the previous step's output. Use the optional
// [Step].InputTemplate with the "{input}" placeholder to wrap that output, and
// [Step].Transform to post-process it. Per-step Provider, Model, Temperature,
// and MaxTokens override the chain defaults.
//
// For stateful workflows with branching, conditional routing, and loops, use
// the graph package instead.
package chain
