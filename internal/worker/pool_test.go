package worker

import (
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestPool_AllTasksProcessed(t *testing.T) {
	var counter atomic.Int64

	workerCount := 3
	processFunc := func(_ model.Task) { counter.Add(1) }

	pool := NewPool(workerCount, processFunc, testLogger)

	pool.Start()

	for range 20 {
		pool.Submit(model.Task{})
	}

	pool.Stop()

	if counter.Load() != 20 {
		t.Fatalf("expected counter to be 20, got: %d", counter.Load())
	}
}

func TestPool_StopBlocksUntilDone(t *testing.T) {
	var counter atomic.Int64

	workerCount := 3
	processFunc := func(_ model.Task) {
		time.Sleep(50 * time.Millisecond)
		counter.Add(1)
	}

	pool := NewPool(workerCount, processFunc, testLogger)

	pool.Start()

	for range 10 {
		pool.Submit(model.Task{})
	}

	pool.Stop()

	if counter.Load() != 10 {
		t.Fatalf("expected counter to be 10, got: %d", counter.Load())
	}
}
