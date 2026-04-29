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

type mockInstanceMonitorRepo struct {
	staleCalls   atomic.Int64
	deregCalls   atomic.Int64
	mu           sync.Mutex
	staleResult  []model.Instance
	staleErr     error
	deregErr     error
	deregistered []string
}

func (m *mockInstanceMonitorRepo) GetStaleInstances(_ context.Context, _ time.Duration) ([]model.Instance, error) {
	m.staleCalls.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.staleResult, m.staleErr
}

func (m *mockInstanceMonitorRepo) Deregister(_ context.Context, instanceID string) error {
	m.deregCalls.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deregistered = append(m.deregistered, instanceID)
	return m.deregErr
}

type mockTaskRecoveryRepo struct {
	calls    atomic.Int64
	recovers int
	err      error
}

func (m *mockTaskRecoveryRepo) RecoverTasksByInstance(_ context.Context, _ string) (int, error) {
	m.calls.Add(1)
	return m.recovers, m.err
}

func TestStartInstanceMonitor_RecoversAndDeregistersDeadInstances(t *testing.T) {
	instanceRepo := &mockInstanceMonitorRepo{
		staleResult: []model.Instance{{ID: "dead-worker-1"}, {ID: "dead-worker-2"}},
	}
	taskRepo := &mockTaskRecoveryRepo{recovers: 5}

	ctx, cancel := context.WithCancel(context.Background())
	StartInstanceMonitor(ctx, instanceRepo, taskRepo, 20*time.Millisecond, time.Minute, testLogger)

	time.Sleep(60 * time.Millisecond)
	cancel()

	if instanceRepo.staleCalls.Load() < 1 {
		t.Errorf("expected at least 1 GetStaleInstances call, got %d", instanceRepo.staleCalls.Load())
	}
	if taskRepo.calls.Load() < 2 {
		t.Errorf("expected at least 2 RecoverTasksByInstance calls (one per dead instance), got %d", taskRepo.calls.Load())
	}
	if instanceRepo.deregCalls.Load() < 2 {
		t.Errorf("expected at least 2 Deregister calls, got %d", instanceRepo.deregCalls.Load())
	}
}

func TestStartInstanceMonitor_GetStaleErrorSkipsCycleAndKeepsRunning(t *testing.T) {
	instanceRepo := &mockInstanceMonitorRepo{staleErr: errors.New("db down")}
	taskRepo := &mockTaskRecoveryRepo{}

	ctx, cancel := context.WithCancel(context.Background())
	StartInstanceMonitor(ctx, instanceRepo, taskRepo, 20*time.Millisecond, time.Minute, testLogger)

	time.Sleep(60 * time.Millisecond)
	cancel()

	if got := instanceRepo.staleCalls.Load(); got < 2 {
		t.Errorf("expected at least 2 GetStaleInstances calls (proves loop continues after error), got %d", got)
	}
	if got := taskRepo.calls.Load(); got != 0 {
		t.Errorf("expected 0 RecoverTasksByInstance calls when stale fetch errored, got %d", got)
	}
}

func TestStartInstanceMonitor_RecoverErrorSkipsThatInstance(t *testing.T) {
	instanceRepo := &mockInstanceMonitorRepo{
		staleResult: []model.Instance{{ID: "dead-1"}, {ID: "dead-2"}},
	}
	taskRepo := &mockTaskRecoveryRepo{err: errors.New("recovery failed")}

	ctx, cancel := context.WithCancel(context.Background())
	StartInstanceMonitor(ctx, instanceRepo, taskRepo, 20*time.Millisecond, time.Minute, testLogger)

	time.Sleep(60 * time.Millisecond)
	cancel()

	// Recover errored on every instance, so Deregister should NEVER run.
	if got := instanceRepo.deregCalls.Load(); got != 0 {
		t.Errorf("expected 0 Deregister calls when recovery errors, got %d", got)
	}
}

func TestStartInstanceMonitor_DeregisterErrorIsLoggedNotFatal(t *testing.T) {
	instanceRepo := &mockInstanceMonitorRepo{
		staleResult: []model.Instance{{ID: "dead-1"}},
		deregErr:    errors.New("deregister failed"),
	}
	taskRepo := &mockTaskRecoveryRepo{recovers: 1}

	ctx, cancel := context.WithCancel(context.Background())
	StartInstanceMonitor(ctx, instanceRepo, taskRepo, 20*time.Millisecond, time.Minute, testLogger)

	time.Sleep(60 * time.Millisecond)
	cancel()

	// Deregister errored, but the loop must continue ticking.
	if got := instanceRepo.staleCalls.Load(); got < 2 {
		t.Errorf("expected loop to continue after Deregister error, got %d cycles", got)
	}
}
