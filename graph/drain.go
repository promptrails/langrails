package graph

import "sync"

// Drain asks a run to stop cleanly: the run finishes the node (or fan-out)
// it is executing, saves its checkpoint, and returns with
// RunResult.Drained set instead of continuing. Resume the thread later,
// for example after a deploy, to pick up where it stopped.
//
//	drain := graph.NewDrain()
//	go func() { <-shutdown; drain.Request() }()
//	res, err := g.Run(ctx, state,
//	    graph.WithCheckpointer[State](cp), graph.WithThreadID[State]("t1"),
//	    graph.WithDrain[State](drain))
//
// Unlike cancelling the context, a drain never interrupts a node midway.
// One Drain can be shared by many runs; it cannot be reset.
type Drain struct {
	once sync.Once
	ch   chan struct{}
}

// NewDrain returns a Drain that has not been requested.
func NewDrain() *Drain {
	return &Drain{ch: make(chan struct{})}
}

// Request asks every run using the drain to stop after its current step.
// It is safe to call more than once and from any goroutine.
func (d *Drain) Request() {
	d.once.Do(func() { close(d.ch) })
}

// Requested reports whether Request has been called.
func (d *Drain) Requested() bool {
	select {
	case <-d.ch:
		return true
	default:
		return false
	}
}

// WithDrain lets d stop the run between steps. Without a checkpointer the
// drained run cannot be resumed, so pair it with WithCheckpointer.
func WithDrain[S any](d *Drain) Option[S] {
	return func(c *runConfig[S]) { c.drain = d }
}

// drained reports whether the run should stop before its next step.
func (c *runConfig[S]) drained() bool {
	return c.drain != nil && c.drain.Requested()
}
