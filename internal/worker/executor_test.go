package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/model"
)

type mockRepo struct {
	completedIDs []uuid.UUID
	failedIDs    []uuid.UUID
	lastErrMsg   string
	lastRetryAt  *time.Time
}

func (m *mockRepo) CompleteTask(_ context.Context, id uuid.UUID) error {
	m.completedIDs = append(m.completedIDs, id)
	return nil
}

func (m *mockRepo) FailTask(_ context.Context, id uuid.UUID, errMsg string, nextRetryAt *time.Time) error {
	m.failedIDs = append(m.failedIDs, id)
	m.lastErrMsg = errMsg
	m.lastRetryAt = nextRetryAt
	return nil
}

func TestExecute_Success(t *testing.T) {
	mock := &mockRepo{}

	executor := NewExecutor(mock, 5*time.Second, testLogger)

	executor.Register("echo", func(_ context.Context, _ json.RawMessage) error {
		return nil
	})

	task := model.Task{
		ID:         uuid.New(),
		Payload:    json.RawMessage(`{"type": "echo"}`),
		MaxRetries: 3,
	}

	executor.Execute(context.Background(), task)

	if len(mock.completedIDs) != 1 {
		t.Fatalf("expected 1 completed, got %d", len(mock.completedIDs))
	}

	if mock.completedIDs[0] != task.ID {
		t.Fatalf("expected completed id to be %s, got %s", task.ID, mock.completedIDs[0])
	}

	if len(mock.failedIDs) != 0 {
		t.Fatalf("expected 0 failed, got %d", len(mock.failedIDs))
	}
}

func TestExecute_UnknownType(t *testing.T) {
	mock := &mockRepo{}

	executor := NewExecutor(mock, 5*time.Second, testLogger)

	task := model.Task{
		ID:         uuid.New(),
		Payload:    json.RawMessage(`{"type": "blabla"}`),
		MaxRetries: 3,
	}

	executor.Execute(context.Background(), task)

	if len(mock.completedIDs) != 0 {
		t.Fatalf("expected 0 completed, got %d", len(mock.completedIDs))
	}

	if len(mock.failedIDs) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(mock.failedIDs))
	}

	if mock.failedIDs[0] != task.ID {
		t.Fatalf("expected failed id to be %s, got %s", task.ID, mock.failedIDs[0])
	}

	if mock.lastRetryAt != nil {
		t.Fatal("expected permanent failure (no retry)")
	}
}

func TestExecute_HandleError(t *testing.T) {
	mock := &mockRepo{}

	executor := NewExecutor(mock, 5*time.Second, testLogger)

	executor.Register("echo", func(_ context.Context, _ json.RawMessage) error {
		return fmt.Errorf("something broke")
	})

	task := model.Task{
		ID:           uuid.New(),
		Payload:      json.RawMessage(`{"type": "echo"}`),
		MaxRetries:   3,
		AttemptCount: 0,
	}

	executor.Execute(context.Background(), task)

	if len(mock.completedIDs) != 0 {
		t.Fatalf("expected 0 completed, got %d", len(mock.completedIDs))
	}

	if len(mock.failedIDs) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(mock.failedIDs))
	}

	if mock.failedIDs[0] != task.ID {
		t.Fatalf("expected failed id to be %s, got %s", task.ID, mock.failedIDs[0])
	}

	if mock.lastRetryAt == nil {
		t.Fatal("expected retry after failure")
	}

	if mock.lastErrMsg != "something broke" {
		t.Fatalf("expected error message 'something broke', got: '%s'", mock.lastErrMsg)
	}
}

func TestExecute_Timeout(t *testing.T) {
	mock := &mockRepo{}

	executor := NewExecutor(mock, 100*time.Millisecond, testLogger)

	executor.Register("slow", func(ctx context.Context, _ json.RawMessage) error {
		select {
		case <-time.After(2 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	task := model.Task{
		ID:           uuid.New(),
		Payload:      json.RawMessage(`{"type": "slow"}`),
		MaxRetries:   3,
		AttemptCount: 0,
	}

	executor.Execute(context.Background(), task)

	if len(mock.completedIDs) != 0 {
		t.Fatalf("expected 0 completed, got %d", len(mock.completedIDs))
	}

	if len(mock.failedIDs) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(mock.failedIDs))
	}

	if mock.failedIDs[0] != task.ID {
		t.Fatalf("expected failed id to be %s, got %s", task.ID, mock.failedIDs[0])
	}
}

func TestExecute_InvalidPayload(t *testing.T) {
	mock := &mockRepo{}

	executor := NewExecutor(mock, 5*time.Second, testLogger)

	task := model.Task{
		ID:         uuid.New(),
		Payload:    json.RawMessage(`not json`),
		MaxRetries: 3,
	}

	executor.Execute(context.Background(), task)

	if len(mock.completedIDs) != 0 {
		t.Fatalf("expected 0 completed, got %d", len(mock.completedIDs))
	}

	if len(mock.failedIDs) != 1 {
		t.Fatalf("expected 1 failed, got %d", len(mock.failedIDs))
	}

	if mock.lastRetryAt != nil {
		t.Fatal("expected permanent failure (no retry)")
	}
}

func TestCalculateBackoff(t *testing.T) {
	var backoff time.Duration

	backoff = calculateBackoff(1)
	if backoff < 900*time.Millisecond || backoff > 1100*time.Millisecond {
		t.Fatalf("expected ~1s, got %v", backoff)
	}

	backoff = calculateBackoff(2)
	if backoff < 1800*time.Millisecond || backoff > 2200*time.Millisecond {
		t.Fatalf("expected ~2s, got %v", backoff)
	}

	backoff = calculateBackoff(3)
	if backoff < 3600*time.Millisecond || backoff > 4400*time.Millisecond {
		t.Fatalf("expected ~4s, got %v", backoff)
	}

	backoff = calculateBackoff(20)
	maxWithJitter := 5*time.Minute + 5*time.Minute/10
	if backoff > maxWithJitter {
		t.Fatalf("expected backoff capped, got %v", backoff)
	}
}
