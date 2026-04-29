package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

// mockLeaderRepo records every TryAcquireLeader call and lets tests script
// a sequence of acquired/not-acquired outcomes (one decision per call).
type mockLeaderRepo struct {
	tryCalls     atomic.Int64
	releaseCalls atomic.Int64
	mu           sync.Mutex
	decisions    []bool // [true, true, false, ...] — one per TryAcquireLeader call
	tryErr       error
}

func (m *mockLeaderRepo) TryAcquireLeader(_ context.Context) (bool, error) {
	idx := int(m.tryCalls.Add(1)) - 1
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.tryErr != nil {
		return false, m.tryErr
	}
	if idx < len(m.decisions) {
		return m.decisions[idx], nil
	}
	if len(m.decisions) > 0 {
		return m.decisions[len(m.decisions)-1], nil
	}
	return false, nil
}

func (m *mockLeaderRepo) ReleaseLeader(_ context.Context) (bool, error) {
	m.releaseCalls.Add(1)
	return true, nil
}

// coordMockInstance is a minimal stub satisfying both the
// InstanceMonitorRepository (for the coordinator's instance monitor child
// goroutine) and the leader's expectation of it. The coordinator passes the
// instanceRepo into StartInstanceMonitor when it acquires leadership; we
// simply return empty stale lists so that child goroutine is a no-op.
type coordMockInstance struct{}

func (coordMockInstance) GetStaleInstances(_ context.Context, _ time.Duration) ([]model.Instance, error) {
	return nil, nil
}
func (coordMockInstance) Deregister(_ context.Context, _ string) error { return nil }

type coordMockTask struct{}

func (coordMockTask) RecoverTasksByInstance(_ context.Context, _ string) (int, error) {
	return 0, nil
}

type coordMockStale struct{}

func (coordMockStale) UnlockStaleTasks(_ context.Context, _ time.Duration) (int, error) {
	return 0, nil
}

func newCoordinatorForTest(leader *mockLeaderRepo) *Coordinator {
	return NewCoordinator(
		leader,
		coordMockInstance{},
		coordMockTask{},
		coordMockStale{},
		20*time.Millisecond, // leaderInterval
		time.Hour,           // monitorInterval (effectively disabled in test)
		time.Minute,
		time.Hour, // staleInterval (effectively disabled)
		time.Minute,
		testLogger,
	)
}

func TestCoordinator_TriesAcquiringLeaderOnInterval(t *testing.T) {
	leader := &mockLeaderRepo{decisions: []bool{false, false, false}}
	c := newCoordinatorForTest(leader)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	time.Sleep(80 * time.Millisecond)
	cancel()

	if got := leader.tryCalls.Load(); got < 2 {
		t.Errorf("expected at least 2 TryAcquireLeader calls in 80ms with 20ms interval, got %d", got)
	}
}

func TestCoordinator_AcquireFlipsIsLeaderTrue(t *testing.T) {
	leader := &mockLeaderRepo{decisions: []bool{true, true, true}}
	c := newCoordinatorForTest(leader)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	time.Sleep(60 * time.Millisecond)
	if !c.isLeader {
		t.Error("expected isLeader=true after successful acquire")
	}
	cancel()
}

func TestCoordinator_LosingLeadershipCancelsChildContext(t *testing.T) {
	// Sequence: acquire (true), lose (false), continue (false)
	leader := &mockLeaderRepo{decisions: []bool{true, false, false, false}}
	c := newCoordinatorForTest(leader)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	// Give the loop time to acquire then lose.
	time.Sleep(120 * time.Millisecond)

	if c.isLeader {
		t.Error("expected isLeader=false after losing leadership")
	}
	cancel()
}

func TestCoordinator_AcquireErrorSkipsCycleKeepsRunning(t *testing.T) {
	leader := &mockLeaderRepo{tryErr: errors.New("leader query failed")}
	c := newCoordinatorForTest(leader)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	time.Sleep(80 * time.Millisecond)
	cancel()

	if got := leader.tryCalls.Load(); got < 2 {
		t.Errorf("expected loop to keep ticking despite errors, got %d calls", got)
	}
	if c.isLeader {
		t.Error("must not flip to isLeader=true on error")
	}
}

func TestCoordinator_StopsOnContextCancel(t *testing.T) {
	leader := &mockLeaderRepo{decisions: []bool{false}}
	c := newCoordinatorForTest(leader)

	ctx, cancel := context.WithCancel(context.Background())
	c.Start(ctx)

	time.Sleep(40 * time.Millisecond)
	cancel()

	before := leader.tryCalls.Load()
	time.Sleep(60 * time.Millisecond)
	after := leader.tryCalls.Load()

	if after != before {
		t.Errorf("coordinator continued after ctx cancel: before=%d after=%d", before, after)
	}
}
