package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/model"
)

type mockDispatcherRepo struct {
	tasks   []model.Task
	err     error
	counter atomic.Int64
}

func (m *mockDispatcherRepo) ClaimTasks(_ context.Context, _ string, _ int) ([]model.Task, error) {
	m.counter.Add(1)
	return m.tasks, m.err
}

func TestDispatcher_ClaimsAndDispatches(t *testing.T) {
	mock := &mockDispatcherRepo{
		tasks: []model.Task{
			{ID: uuid.New(), Payload: json.RawMessage(`{"type":"echo"}`)},
			{ID: uuid.New(), Payload: json.RawMessage(`{"type":"echo"}`)},
		},
	}

	ch := make(chan model.Task, 10)

	dispatcher := NewDispatcher(mock, ch, 50*time.Millisecond, 10, testLogger)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		dispatcher.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	if len(ch) < 2 {
		t.Errorf("expected at least 2 tasks, got %d", len(ch))
	}

	if mock.counter.Load() < 1 {
		t.Errorf("expected at least 1 dispatch, got %d", mock.counter.Load())
	}
}

func TestDispatcher_StopsOnNextCancel(t *testing.T) {
	mock := &mockDispatcherRepo{
		tasks: []model.Task{
			{ID: uuid.New(), Payload: json.RawMessage(`{"type":"echo"}`)},
			{ID: uuid.New(), Payload: json.RawMessage(`{"type":"echo"}`)},
		},
	}

	ch := make(chan model.Task, 10)

	dispatcher := NewDispatcher(mock, ch, 50*time.Millisecond, 10, testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go dispatcher.Run(ctx)
	cancel()

	stopped := make(chan struct{})
	go func() {
		dispatcher.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("dispatcher did not stop after context cancel")
	}
}

func TestDispatcher_ContinuesOnError(t *testing.T) {
	mock := &mockDispatcherRepo{
		tasks: nil,
		err:   fmt.Errorf("db connection lost"),
	}

	ch := make(chan model.Task, 10)

	dispatcher := NewDispatcher(mock, ch, 50*time.Millisecond, 10, testLogger)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		dispatcher.Run(ctx)
	}()

	time.Sleep(200 * time.Millisecond)
	cancel()

	if mock.counter.Load() < 2 {
		t.Errorf("expected at least 2 claims (proves retry after error), got %d", mock.counter.Load())
	}

	if len(ch) != 0 {
		t.Errorf("expected 0 tasks in channel, got %d", len(ch))
	}
}
