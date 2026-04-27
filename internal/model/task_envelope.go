package model

import "context"

// TaskEnvelope carries a Task across the worker pool channel along with the
// context that spawned it. The pool channel is a process-internal boundary:
// without an envelope, the originating ctx (and any active span) cannot reach
// the executor, breaking trace continuity from the producer to task execution.
type TaskEnvelope struct {
	Task Task
	Ctx  context.Context
}
