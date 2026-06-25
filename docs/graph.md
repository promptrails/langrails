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
import "github.com/promptrails/langrails/graph"

type State struct {
    Input  string
    Output string
}

g := graph.New[State]()

g.AddNode("process", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:    "gpt-4o",
        Messages: []langrails.Message{{Role: "user", Content: s.Input}},
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
    resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Classify the input as 'question', 'complaint', or 'feedback'. Respond with one word.",
        Messages:     []langrails.Message{{Role: "user", Content: s.Input}},
    })
    s.Category = strings.TrimSpace(resp.Content)
    return s, nil
})

// Handler nodes
g.AddNode("answer", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Answer the following question helpfully.",
        Messages:     []langrails.Message{{Role: "user", Content: s.Input}},
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
    resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Improve this draft. Make it more concise and engaging.",
        Messages:     []langrails.Message{{Role: "user", Content: s.Draft}},
    })
    s.Draft = resp.Content
    s.Rounds++
    return s, nil
})

g.AddNode("evaluate", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Rate this text quality 1-10. Respond with just the number.",
        Messages:     []langrails.Message{{Role: "user", Content: s.Draft}},
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

## Parallel Fan-Out (Map-Reduce)

`AddFanOut` runs a node's work across concurrent branches and merges the
results — the map-reduce pattern, equivalent to LangGraph's Send API.

Three pieces define a fan-out:

- A **fan function** that turns the current state into a slice of `Send` values, one per branch. Each `Send` names the target node and carries the state copy that branch should run on.
- A **reducer** that folds the branch results back into a single state.
- A **join** node where execution continues once every branch completes.

Branches run concurrently on their own state copies; if any branch returns an
error, the rest are cancelled and the run fails.

```go
type State struct {
    Docs      []string
    Summaries []string
}

g := graph.New[State]()

g.AddNode("split", func(ctx context.Context, s State) (State, error) {
    return s, nil // nothing to do; fan-out reads s.Docs below
})

// Per-branch worker: each branch sees one document in s.Docs[0].
g.AddNode("summarize", func(ctx context.Context, s State) (State, error) {
    resp, _ := provider.Complete(ctx, &langrails.CompletionRequest{
        Model:        "gpt-4o",
        SystemPrompt: "Summarize the document in one sentence.",
        Messages:     []langrails.Message{{Role: "user", Content: s.Docs[0]}},
    })
    s.Summaries = []string{resp.Content}
    return s, nil
})

g.AddNode("reduce", func(ctx context.Context, s State) (State, error) {
    return s, nil
})

g.SetEntryPoint("split")
g.AddFanOut("split",
    // fan: one branch per document
    func(ctx context.Context, s State) ([]graph.Send[State], error) {
        sends := make([]graph.Send[State], len(s.Docs))
        for i, d := range s.Docs {
            sends[i] = graph.Send[State]{Node: "summarize", State: State{Docs: []string{d}}}
        }
        return sends, nil
    },
    // reduce: gather every branch's summary
    func(base State, results []State) State {
        for _, r := range results {
            base.Summaries = append(base.Summaries, r.Summaries...)
        }
        return base
    },
    "reduce", // join node
)
g.AddEdge("reduce", graph.END)

result, _ := g.Run(ctx, State{Docs: []string{"doc a", "doc b", "doc c"}})
// result.State.Summaries holds one summary per document
```

Notes:

- Fan-out target nodes are leaf workers — their own outgoing edges are ignored. Use a subgraph (`graph.AsNode`) as the target when a branch needs multiple steps.
- A fan-out edge takes precedence over any plain or conditional edge on the same source node.
- A `nil` reducer falls back to the single branch result when there is exactly one branch, otherwise it keeps the pre-fan-out state.
- An empty `Send` slice runs no branches and continues at the join node with the unchanged state.

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
