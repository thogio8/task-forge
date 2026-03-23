//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/worker"
)

func TestScenario_FailThenSucceed(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	dlqRepo := NewDeadLetterRepository(testPool, testLogger)
	executor := worker.NewExecutor(taskRepo, 5*time.Second, testLogger, dlqRepo)

	var callCount atomic.Int32
	executor.Register("flaky", func(_ context.Context, _ json.RawMessage) error {
		if callCount.Add(1) <= 1 {
			return fmt.Errorf("temporary error")
		}
		return nil
	})

	task := model.Task{
		Status:     model.StatusPending,
		Payload:    []byte(`{"type":"flaky"}`),
		MaxRetries: 3,
	}
	_, err := taskRepo.Create(context.Background(), &task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	claimed := claimTestTasks(t, taskRepo, 1)
	if len(claimed) != 1 {
		t.Fatalf("expected 1 task claimed, got %d", len(claimed))
	}

	executor.Execute(context.Background(), claimed[0])

	task, err = taskRepo.GetById(context.Background(), claimed[0].ID)
	if err != nil {
		t.Fatalf("failed to retrieve task: %v", err)
	}

	if task.Status != model.StatusPending {
		t.Fatalf("expected pending task, got %s", task.Status)
	}

	if task.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", task.AttemptCount)
	}

	if task.NextRetryAt == nil {
		t.Fatal("expected next retry time, got nil")
	}

	_, err = testPool.Exec(context.Background(), "UPDATE tasks SET next_retry_at = NOW() - INTERVAL '1 hour' WHERE id = $1", task.ID)
	if err != nil {
		t.Fatalf("failed to update next retry time: %v", err)
	}

	claimed = claimTestTasks(t, taskRepo, 1)
	if len(claimed) != 1 {
		t.Fatalf("expected 1 task claimed, got %d", len(claimed))
	}

	executor.Execute(context.Background(), claimed[0])

	task, err = taskRepo.GetById(context.Background(), claimed[0].ID)
	if err != nil {
		t.Fatalf("failed to retrieve task: %v", err)
	}

	if task.Status != model.StatusCompleted {
		t.Fatalf("expected completed task, got %s", task.Status)
	}
}

func TestScenario_ExhaustedRetries_MovesToDLQ(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	dlqRepo := NewDeadLetterRepository(testPool, testLogger)
	executor := worker.NewExecutor(taskRepo, 5*time.Second, testLogger, dlqRepo)

	executor.Register("broken", func(_ context.Context, _ json.RawMessage) error {
		return fmt.Errorf("always fails")
	})

	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"broken"}`),
	}
	_, err := taskRepo.Create(context.Background(), &task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	_, err = testPool.Exec(context.Background(), "UPDATE tasks SET max_retries = 2 WHERE id = $1", task.ID)
	if err != nil {
		t.Fatalf("failed to update max_retries: %v", err)
	}

	claimed := claimTestTasks(t, taskRepo, 1)
	if len(claimed) != 1 {
		t.Fatalf("expected 1 task claimed, got %d", len(claimed))
	}

	executor.Execute(context.Background(), claimed[0])

	task, err = taskRepo.GetById(context.Background(), claimed[0].ID)
	if err != nil {
		t.Fatalf("failed to retrieve task: %v", err)
	}

	if task.Status != model.StatusPending {
		t.Fatalf("expected pending task, got %s", task.Status)
	}

	if task.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt, got %d", task.AttemptCount)
	}

	if task.NextRetryAt == nil {
		t.Fatal("expected next retry time, got nil")
	}

	_, err = testPool.Exec(context.Background(), "UPDATE tasks SET next_retry_at = NOW() - INTERVAL '1 hour' WHERE id = $1", task.ID)
	if err != nil {
		t.Fatalf("failed to update next retry time: %v", err)
	}

	claimed = claimTestTasks(t, taskRepo, 1)
	if len(claimed) != 1 {
		t.Fatalf("expected 1 task claimed, got %d", len(claimed))
	}

	executor.Execute(context.Background(), claimed[0])

	_, err = taskRepo.GetById(context.Background(), claimed[0].ID)
	if err == nil {
		t.Fatal("expected error during retry attempt")
	}

	deadLetterTasks, err := dlqRepo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("failed to retrieve dead letter tasks: %v", err)
	}

	if len(deadLetterTasks) != 1 {
		t.Fatalf("expected 1 dead letter task, got %d", len(deadLetterTasks))
	}

	if deadLetterTasks[0].OriginalTaskID != task.ID {
		t.Fatalf("expected original task id to have the same id, got %s", deadLetterTasks[0].OriginalTaskID)
	}
}

func TestScenario_RetryFromDLQ(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	dlqRepo := NewDeadLetterRepository(testPool, testLogger)
	executor := worker.NewExecutor(taskRepo, 5*time.Second, testLogger, dlqRepo)

	executor.Register("broken", func(_ context.Context, _ json.RawMessage) error {
		return fmt.Errorf("always fails")
	})

	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"broken"}`),
	}
	_, err := taskRepo.Create(context.Background(), &task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	_, err = testPool.Exec(context.Background(), "UPDATE tasks SET max_retries = 2 WHERE id = $1", task.ID)
	if err != nil {
		t.Fatalf("failed to update max_retries: %v", err)
	}

	claimed := claimTestTasks(t, taskRepo, 1)
	executor.Execute(context.Background(), claimed[0])

	_, err = testPool.Exec(context.Background(), "UPDATE tasks SET next_retry_at = NOW() - INTERVAL '1 hour' WHERE id = $1", task.ID)
	if err != nil {
		t.Fatalf("failed to update next retry time: %v", err)
	}

	claimed = claimTestTasks(t, taskRepo, 1)
	executor.Execute(context.Background(), claimed[0])

	dlqTasks, err := dlqRepo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("failed to get DLQ tasks: %v", err)
	}
	if len(dlqTasks) != 1 {
		t.Fatalf("expected 1 DLQ task, got %d", len(dlqTasks))
	}

	retriedTask, err := dlqRepo.Retry(context.Background(), dlqTasks[0].ID)
	if err != nil {
		t.Fatalf("failed to retry DLQ task: %v", err)
	}

	if retriedTask.ID == task.ID {
		t.Fatal("expected new task created after retry")
	}

	if retriedTask.Status != model.StatusPending {
		t.Fatalf("expected new task status to be pending, got %s", retriedTask.Status)
	}

	if string(retriedTask.Payload) != `{"type": "broken"}` {
		t.Fatalf("expected retried task to have the same payload, got %s", string(retriedTask.Payload))
	}

	dlqTasks, err = dlqRepo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("failed to get DLQ tasks: %v", err)
	}
	if len(dlqTasks) != 1 {
		t.Fatalf("expected 1 DLQ task, got %d", len(dlqTasks))
	}

	if dlqTasks[0].RetriedAt == nil {
		t.Fatal("expected DLQ task retry time, got nil")
	}
}
