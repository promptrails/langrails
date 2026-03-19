# Graph (LangGraph-style Workflows)

The `graph` package provides a stateful workflow engine inspired by LangGraph. It enables complex agent architectures with conditional branching, loops, and multi-step reasoning.

## Concepts

- **Node**: A function that processes the current state and returns an updated state
- **Edge**: A connection from one node to another
- **Conditional Edge**: An edge that routes to different nodes based on state
- **State**: A typed struct that flows through the graph
- **END**: A special sentinel that signals graph completion

## Basic Graph

```go
import "github.com/promptrails/unillm/graph"

type State struct {
    Input  string
    Output string
}

g := graph.New[State]()

g.AddNode("process", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
        Model:    "gpt-4o",
        Messages: []unillm.Message{{Role: "user", Content: s.Input}},
    })
    s.Output = resp.Content
    return s, nil
})

g.SetEntryPoint("process")
g.AddEdge("process", graph.END)

result, err := g.Run(ctx, State{Input: "Hello!"})
fmt.Println(result.State.Output)
```

## Conditional Routing

Route to different nodes based on state:

```go
type State struct {
    Input     string
    Category  string
    Output    string
}

g := graph.New[State]()

// Classification node
g.AddNode("classify", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Classify the input as 'question', 'complaint', or 'feedback'. Respond with one word.",
        Messages:     []unillm.Message{{Role: "user", Content: s.Input}},
    })
    s.Category = strings.TrimSpace(resp.Content)
    return s, nil
})

// Handler nodes
g.AddNode("answer", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Answer the following question helpfully.",
        Messages:     []unillm.Message{{Role: "user", Content: s.Input}},
    })
    s.Output = resp.Content
    return s, nil
})

g.AddNode("resolve", func(ctx context.Context, s State) (State, error) {
    s.Output = "We're sorry for the inconvenience. A support agent will contact you."
    return s, nil
})

g.AddNode("thank", func(ctx context.Context, s State) (State, error) {
    s.Output = "Thank you for your feedback!"
    return s, nil
})

// Wiring
g.SetEntryPoint("classify")
g.AddConditionalEdge("classify", func(s State) string {
    switch s.Category {
    case "question":
        return "answer"
    case "complaint":
        return "resolve"
    default:
        return "thank"
    }
})
g.AddEdge("answer", graph.END)
g.AddEdge("resolve", graph.END)
g.AddEdge("thank", graph.END)

result, _ := g.Run(ctx, State{Input: "How do I reset my password?"})
// Routes to "answer" node
```

## Loops (Iterative Refinement)

```go
type State struct {
    Draft    string
    Score    int
    Rounds   int
}

g := graph.New[State]()

g.AddNode("write", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Improve this draft. Make it more concise and engaging.",
        Messages:     []unillm.Message{{Role: "user", Content: s.Draft}},
    })
    s.Draft = resp.Content
    s.Rounds++
    return s, nil
})

g.AddNode("evaluate", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &unillm.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Rate this text quality 1-10. Respond with just the number.",
        Messages:     []unillm.Message{{Role: "user", Content: s.Draft}},
    })
    s.Score, _ = strconv.Atoi(strings.TrimSpace(resp.Content))
    return s, nil
})

g.SetEntryPoint("write")
g.AddEdge("write", "evaluate")
g.AddConditionalEdge("evaluate", func(s State) string {
    if s.Score >= 8 || s.Rounds >= 3 {
        return graph.END
    }
    return "write" // Loop back for another round
})

result, _ := g.Run(ctx, State{Draft: "Initial rough draft..."})
```

## Execution History

```go
result, _ := g.Run(ctx, initialState)

for _, step := range result.Steps {
    fmt.Printf("Step %d: node=%s\n", step.Step, step.Node)
    // step.State contains the state after this node executed
}
```

## Max Steps

Prevent infinite loops:

```go
result, err := g.Run(ctx, state, graph.WithMaxSteps[State](50))
if err != nil {
    // "graph: exceeded maximum steps (50)"
}
```

Default max steps is 100.

## Multi-Agent Pattern

```go
type State struct {
    Task      string
    Research  string
    Plan      string
    Code      string
    Review    string
}

g := graph.New[State]()

g.AddNode("researcher", func(ctx context.Context, s State) (State, error) {
    // Use one provider/model for research
    resp, _ := researchProvider.Complete(ctx, ...)
    s.Research = resp.Content
    return s, nil
})

g.AddNode("planner", func(ctx context.Context, s State) (State, error) {
    // Use another provider for planning
    resp, _ := plannerProvider.Complete(ctx, ...)
    s.Plan = resp.Content
    return s, nil
})

g.AddNode("coder", func(ctx context.Context, s State) (State, error) {
    resp, _ := coderProvider.Complete(ctx, ...)
    s.Code = resp.Content
    return s, nil
})

g.AddNode("reviewer", func(ctx context.Context, s State) (State, error) {
    resp, _ := reviewProvider.Complete(ctx, ...)
    s.Review = resp.Content
    return s, nil
})

g.SetEntryPoint("researcher")
g.AddEdge("researcher", "planner")
g.AddEdge("planner", "coder")
g.AddEdge("coder", "reviewer")
g.AddConditionalEdge("reviewer", func(s State) string {
    if strings.Contains(s.Review, "APPROVED") {
        return graph.END
    }
    return "coder" // Send back for revision
})
```
