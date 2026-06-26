package graph

import (
	"context"
	"sync"
)

// Checkpoint is a durable snapshot of a graph run taken at a superstep
// boundary. It records the next node to execute and the state going into
// it, so a run can be resumed exactly where it left off.
type Checkpoint[S any] struct {
	// ThreadID identifies the run this checkpoint belongs to.
	ThreadID string

	// Step is the step number after which this checkpoint was taken.
	Step int

	// Node is the next node to execute on resume. It is END when the run
	// has completed.
	Node string

	// State is the state to feed into Node on resume.
	State S

	// Done is true when the run reached END.
	Done bool
}

// Checkpointer persists and restores graph checkpoints, keyed by thread
// ID. Implementations must be safe for concurrent use. The standard
// implementation, [MemoryCheckpointer], keeps checkpoints in memory;
// durable backends (SQLite, Postgres, ...) can satisfy the same interface.
type Checkpointer[S any] interface {
	// Save stores a checkpoint as the latest for its thread and appends it
	// to the thread's history.
	Save(ctx context.Context, threadID string, cp Checkpoint[S]) error

	// Load returns the most recent checkpoint for a thread. The boolean is
	// false when the thread has no checkpoints.
	Load(ctx context.Context, threadID string) (Checkpoint[S], bool, error)

	// History returns every checkpoint for a thread in the order it was
	// saved, enabling replay and time-travel debugging.
	History(ctx context.Context, threadID string) ([]Checkpoint[S], error)
}

// MemoryCheckpointer is an in-memory Checkpointer. It is safe for
// concurrent use and is the default when durability across process
// restarts is not required.
type MemoryCheckpointer[S any] struct {
	mu      sync.RWMutex
	history map[string][]Checkpoint[S]
}

// NewMemoryCheckpointer creates an empty in-memory checkpointer.
func NewMemoryCheckpointer[S any]() *MemoryCheckpointer[S] {
	return &MemoryCheckpointer[S]{history: make(map[string][]Checkpoint[S])}
}

// Save appends a checkpoint to the thread's history.
func (m *MemoryCheckpointer[S]) Save(_ context.Context, threadID string, cp Checkpoint[S]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history[threadID] = append(m.history[threadID], cp)
	return nil
}

// Load returns the latest checkpoint for the thread, or false if none.
func (m *MemoryCheckpointer[S]) Load(_ context.Context, threadID string) (Checkpoint[S], bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h := m.history[threadID]
	if len(h) == 0 {
		var zero Checkpoint[S]
		return zero, false, nil
	}
	return h[len(h)-1], true, nil
}

// History returns a copy of the thread's checkpoints in save order.
func (m *MemoryCheckpointer[S]) History(_ context.Context, threadID string) ([]Checkpoint[S], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h := m.history[threadID]
	out := make([]Checkpoint[S], len(h))
	copy(out, h)
	return out, nil
}
