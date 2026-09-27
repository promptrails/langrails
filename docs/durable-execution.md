# Durable Execution (Checkpointing & Resume)

A graph run can be made **durable**: after every superstep the graph saves a
checkpoint — the next node to run plus the state going into it — under a thread
ID. If the process crashes, is paused for human approval, or is interrupted for
any reason, the run can be continued from its last checkpoint with `Resume`.

This mirrors LangGraph's persistence model: short-term, thread-scoped state that
enables resume-after-failure, human-in-the-loop pauses, and time-travel
debugging.

## Concepts

- **Checkpoint**: a snapshot taken at a superstep boundary holding the next node, the state, the step number, and whether the run is done.
- **Checkpointer**: stores and restores checkpoints, keyed by thread ID.
- **Thread ID**: identifies one logical run (e.g. a conversation) so its checkpoints are independent of other runs.

## Enabling checkpointing

Pass a checkpointer and a thread ID. `MemoryCheckpointer` keeps checkpoints in
memory and is the default; any type satisfying the `Checkpointer` interface
(SQLite, Postgres, ...) can be substituted.

```go
import "github.com/promptrails/langrails/graph"

cp := graph.NewMemoryCheckpointer[State]()

result, err := g.Run(ctx, State{Input: "..."},
    graph.WithCheckpointer[State](cp),
    graph.WithThreadID[State]("conversation-42"),
)
```

Each routing decision saves a checkpoint, so a three-node linear graph produces
three checkpoints (`a→b`, `b→c`, `c→END`). The final checkpoint is marked
`Done`.

## Resuming after a crash

If a node fails, the latest checkpoint points at that node with the state that
went into it. Calling `Resume` re-runs from there:

```go
// First attempt fails inside a node.
_, err := g.Run(ctx, initial,
    graph.WithCheckpointer[State](cp),
    graph.WithThreadID[State]("job-7"),
)
// ... process restarts, transient failure resolved ...

result, err := g.Resume(ctx,
    graph.WithCheckpointer[State](cp),
    graph.WithThreadID[State]("job-7"),
)
// Resumes from the last checkpoint; already-completed nodes are not re-run.
```

Resuming a run that already reached `END` replays no nodes — it returns the
persisted final state.

## Interrupts (human-in-the-loop)

A node can **pause the run** until someone answers — approve a deployment,
review a draft, pick an option — and the run continues later, even in another
process, days after. This is LangGraph's `interrupt()` / resume pattern.

Call `graph.Await[T](ctx, payload)` in the node and return its error:

```go
g.AddNode("approve", func(ctx context.Context, s State) (State, error) {
    ok, err := graph.Await[bool](ctx, "Deploy "+s.Version+" to production?")
    if err != nil {
        return s, err // pauses the run
    }
    s.Approved = ok
    return s, nil
})
```

The first time, `Await` returns an interrupt. The graph saves a checkpoint and
`Run` returns **without an error**, with `RunResult.Interruption` set:

```go
opts := []graph.Option[State]{
    graph.WithCheckpointer[State](cp),
    graph.WithThreadID[State]("deploy-42"),
}

res, err := g.Run(ctx, State{Version: "v2"}, opts...)
if res.Interruption != nil {
    // res.Interruption.Node    == "approve"
    // res.Interruption.Payload == "Deploy v2 to production?"
    notifyReviewer(res.Interruption.Payload)
    return
}
```

When the answer arrives, resume with it. The paused node runs again from the
top, and this time `Await` returns the value:

```go
res, err := g.Resume(ctx, append(opts, graph.WithResumeValue[State](true))...)
// res.State.Approved == true; the run continued to END
```

Rules of thumb:

- **A checkpointer is required.** Without one the interrupt is returned as an
  error wrapping `*graph.InterruptError`.
- **The paused node re-runs on resume**, so code before `Await` runs twice.
  Keep side effects after it. Nodes that completed earlier do not re-run.
- **Resuming without `WithResumeValue`** simply pauses again at the same node.
- **The value's type must match** `Await[T]`; otherwise the node fails.
- **Subgraphs** (`AsNode`) may interrupt: the parent pauses at the subgraph
  node, and on resume the subgraph starts again from its entry point, with the
  value delivered to the `Await` that asked.
- **Fan-out branches** cannot interrupt; the run fails with an error.
- The pending interruption is stored in `Checkpoint.Interruption`, and `Stream`
  ends with a `StepEvent` carrying it. If your checkpointer serializes state,
  make the payload serializable too.

## Time travel

`History` returns every checkpoint for a thread in save order, so you can
inspect or replay intermediate states:

```go
hist, _ := cp.History(ctx, "job-7")
for _, c := range hist {
    fmt.Printf("step %d → next=%s done=%v\n", c.Step, c.Node, c.Done)
}
```

## Custom checkpointers

Implement the `Checkpointer` interface to persist to a durable store:

```go
type Checkpointer[S any] interface {
    Save(ctx context.Context, threadID string, cp Checkpoint[S]) error
    Load(ctx context.Context, threadID string) (Checkpoint[S], bool, error)
    History(ctx context.Context, threadID string) ([]Checkpoint[S], error)
}
```

`Save` should store the checkpoint as the latest for its thread and append it to
the thread's history. `Load` returns the most recent checkpoint (the boolean is
`false` when the thread has none). Implementations must be safe for concurrent
use.

## Notes

- A thread ID is required whenever a checkpointer is set; `Run` errors otherwise.
- Fan-out supersteps checkpoint once, at the join — branch results are reduced before the checkpoint is taken.
- `maxSteps` is a total budget across the original run and any resumes (the resumed step counter continues from the checkpoint's step).
