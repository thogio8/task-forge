package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type mockStaleCleanerRepo struct {
	calls atomic.Int64
	count int
	err   error
}

func (m *mockStaleCleanerRepo) UnlockStaleTasks(_ context.Context, _ time.Duration) (int, error) {
	m.calls.Add(1)
	return m.count, m.err
}

func TestStartStaleCleaner_TicksAndStops(t *testing.T) {
	repo := &mockStaleCleanerRepo{count: 3}

	ctx, cancel := context.WithCancel(context.Background())
	StartStaleCleaner(ctx, repo, 20*time.Millisecond, 5*time.Minute, testLogger)

	time.Sleep(80 * time.Millisecond)
	cancel()

	if got := repo.calls.Load(); got < 2 {
		t.Errorf("expected at least 2 cleaner cycles in 80ms, got %d", got)
	}

	before := repo.calls.Load()
	time.Sleep(60 * time.Millisecond)
	if after := repo.calls.Load(); after != before {
		t.Errorf("cleaner continued after ctx cancel: before=%d after=%d", before, after)
	}
}

func TestStartStaleCleaner_KeepsRunningOnError(t *testing.T) {
	repo := &mockStaleCleanerRepo{err: errors.New("db down")}

	ctx, cancel := context.WithCancel(context.Background())
	StartStaleCleaner(ctx, repo, 20*time.Millisecond, 5*time.Minute, testLogger)

	time.Sleep(80 * time.Millisecond)
	cancel()

	if got := repo.calls.Load(); got < 2 {
		t.Errorf("expected at least 2 calls despite errors, got %d", got)
	}
}
