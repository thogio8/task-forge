package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockHeartbeatRepo struct {
	calls atomic.Int64
	mu    sync.Mutex
	err   error
}

func (m *mockHeartbeatRepo) Heartbeat(_ context.Context, _ string) error {
	m.calls.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.err
}

func TestStartHeartbeat_TicksAndStopsOnContextCancel(t *testing.T) {
	repo := &mockHeartbeatRepo{}

	ctx, cancel := context.WithCancel(context.Background())
	StartHeartbeat(ctx, repo, "worker-1", 20*time.Millisecond, testLogger)

	time.Sleep(80 * time.Millisecond)
	cancel()

	got := repo.calls.Load()
	if got < 2 {
		t.Errorf("expected at least 2 heartbeat calls in 80ms with 20ms interval, got %d", got)
	}

	// After cancel, the goroutine should stop. We verify by sleeping longer
	// than the interval and confirming no further calls land.
	before := repo.calls.Load()
	time.Sleep(60 * time.Millisecond)
	after := repo.calls.Load()
	if after != before {
		t.Errorf("heartbeat continued after ctx cancel: before=%d after=%d", before, after)
	}
}

func TestStartHeartbeat_LogsRepoErrorButKeepsRunning(t *testing.T) {
	repo := &mockHeartbeatRepo{err: errors.New("db down")}

	ctx, cancel := context.WithCancel(context.Background())
	StartHeartbeat(ctx, repo, "worker-1", 20*time.Millisecond, testLogger)

	time.Sleep(80 * time.Millisecond)
	cancel()

	// Even with errors, the loop must keep ticking — no goroutine exit.
	got := repo.calls.Load()
	if got < 2 {
		t.Errorf("expected at least 2 calls despite errors, got %d", got)
	}
}
