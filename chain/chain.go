// Package chain provides sequential prompt chain execution.
//
// A chain is a series of steps where each step's output can be used as
// input to the next step. This enables multi-step LLM workflows like:
// summarize → translate → format.
//
// # Usage
//
//	c := chain.New(provider,
//		chain.Step{
//			SystemPrompt: "Summarize the following text in 2 sentences.",
//		},
//		chain.Step{
//			SystemPrompt: "Translate the following to Turkish.",
//		},
//	)
//
//	result, err := c.Run(ctx, "Long article text here...")
package chain

import (
	"context"
	"fmt"
	"strings"

	"github.com/promptrails/unillm"
)

// Step defines a single step in a chain.
type Step struct {
	// SystemPrompt is the system instruction for this step.
	SystemPrompt string

	// Model overrides the chain's default model for this step.
	// If empty, the chain's model is used.
	Model string

	// Provider overrides the chain's default provider for this step.
	// If nil, the chain's provider is used.
	Provider unillm.Provider

	// Temperature overrides for this step.
	Temperature *float64

	// MaxTokens overrides for this step.
	MaxTokens *int

	// Transform is an optional function that transforms the output
	// of this step before passing it to the next step.
	// If nil, the raw LLM output is used.
	Transform func(output string) string

	// InputTemplate is an optional template for the user message.
	// Use {input} as a placeholder for the previous step's output.
	// If empty, the previous output is used directly as the user message.
	InputTemplate string
}

// Chain executes a sequence of LLM calls where each step's output
// feeds into the next step's input.
type Chain struct {
	provider unillm.Provider
	model    string
	steps    []Step
}

// Option configures the chain.
type Option func(*Chain)

// WithModel sets the default model for all steps.
func WithModel(model string) Option {
	return func(c *Chain) {
		c.model = model
	}
}

// New creates a new chain with the given provider and steps.
func New(provider unillm.Provider, steps []Step, opts ...Option) *Chain {
	c := &Chain{
		provider: provider,
		steps:    steps,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// StepResult contains the output and metadata from a single step.
type StepResult struct {
	// Output is the text output from this step.
	Output string

	// Usage is the token usage for this step.
	Usage unillm.TokenUsage

	// Model is the model used for this step.
	Model string
}

// Result contains the output from all steps in the chain.
type Result struct {
	// Output is the final output from the last step.
	Output string

	// Steps contains the result from each step.
	Steps []StepResult

	// TotalUsage is the accumulated token usage across all steps.
	TotalUsage unillm.TokenUsage
}

// Run executes the chain with the given initial input.
// The input is passed as the user message to the first step.
// Each subsequent step receives the previous step's output.
func (c *Chain) Run(ctx context.Context, input string) (*Result, error) {
	result := &Result{}
	currentInput := input

	for i, step := range c.steps {
		provider := step.Provider
		if provider == nil {
			provider = c.provider
		}

		model := step.Model
		if model == "" {
			model = c.model
		}
		if model == "" {
			return nil, fmt.Errorf("chain step %d: no model specified", i+1)
		}

		// Build user message
		userContent := currentInput
		if step.InputTemplate != "" {
			userContent = strings.ReplaceAll(step.InputTemplate, "{input}", currentInput)
		}

		req := &unillm.CompletionRequest{
			Model:        model,
			SystemPrompt: step.SystemPrompt,
			Messages: []unillm.Message{
				{Role: "user", Content: userContent},
			},
			Temperature: step.Temperature,
			MaxTokens:   step.MaxTokens,
		}

		resp, err := provider.Complete(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("chain step %d: %w", i+1, err)
		}

		output := resp.Content
		if step.Transform != nil {
			output = step.Transform(output)
		}

		stepResult := StepResult{
			Output: output,
			Usage:  resp.Usage,
			Model:  resp.Model,
		}
		result.Steps = append(result.Steps, stepResult)
		result.TotalUsage.PromptTokens += resp.Usage.PromptTokens
		result.TotalUsage.CompletionTokens += resp.Usage.CompletionTokens
		result.TotalUsage.TotalTokens += resp.Usage.TotalTokens

		currentInput = output
	}

	if len(result.Steps) > 0 {
		result.Output = result.Steps[len(result.Steps)-1].Output
	}

	return result, nil
}
