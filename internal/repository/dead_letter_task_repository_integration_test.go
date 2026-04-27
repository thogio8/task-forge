//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/thogio8/task-forge/internal/model"
)

func TestMoveToDLQ(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	deadLetterTaskRepo := NewDeadLetterRepository(testPool, testLogger)

	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"echo"}`),
	}

	created, err := taskRepo.Create(context.Background(), &task)

	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if !created {
		t.Fatal("expected task to be created")
	}

	claimTestTasks(t, taskRepo, 1)

	err = taskRepo.FailTask(context.Background(), task.ID, "something went wrong", nil)

	if err != nil {
		t.Fatalf("failed to mark task as failed: %v", err)
	}

	err = deadLetterTaskRepo.MoveToDLQ(context.Background(), task.ID)

	if err != nil {
		t.Fatalf("failed to move to DLQ: %v", err)
	}

	_, err = taskRepo.GetById(context.Background(), task.ID)

	if err == nil {
		t.Error("expected no task found error")
	}

	deadLetterTasks, err := deadLetterTaskRepo.GetAll(context.Background())

	if err != nil {
		t.Fatalf("failed to fetch dead letter tasks: %v", err)
	}

	if len(deadLetterTasks) != 1 {
		t.Errorf("expected one dead letter task, got %d", len(deadLetterTasks))
	}

	if deadLetterTasks[0].OriginalTaskID != task.ID {
		t.Errorf("expected id : %s, got %s", task.ID, deadLetterTasks[0].OriginalTaskID)
	}
}

func TestRetry(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	deadLetterTaskRepo := NewDeadLetterRepository(testPool, testLogger)

	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"echo"}`),
	}

	created, err := taskRepo.Create(context.Background(), &task)

	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if !created {
		t.Fatal("expected task to be created")
	}

	claimTestTasks(t, taskRepo, 1)

	err = taskRepo.FailTask(context.Background(), task.ID, "something went wrong", nil)

	if err != nil {
		t.Fatalf("failed to mark task as failed: %v", err)
	}

	err = deadLetterTaskRepo.MoveToDLQ(context.Background(), task.ID)

	if err != nil {
		t.Fatalf("failed to move to DLQ: %v", err)
	}

	deadLetterTasks, err := deadLetterTaskRepo.GetAll(context.Background())

	if err != nil {
		t.Fatalf("failed to fetch dead letter tasks: %v", err)
	}

	retriedTask, err := deadLetterTaskRepo.Retry(context.Background(), deadLetterTasks[0].ID)

	if err != nil {
		t.Fatalf("failed to retry task from the dead letter queue: %v", err)
	}

	if task.ID == retriedTask.ID {
		t.Error("expected retried task to have a different task ID")
	}

	if retriedTask.Status != model.StatusPending {
		t.Errorf("expected retried task to be pending, got %s", retriedTask.Status)
	}

	if string(retriedTask.Payload) != `{"type": "echo"}` {
		t.Error("expected retried task to have the same payload as the original task")
	}

	deadLetterTasks, err = deadLetterTaskRepo.GetAll(context.Background())

	if err != nil {
		t.Fatalf("failed to fetch dead letter tasks: %v", err)
	}

	if deadLetterTasks[0].RetriedAt == nil {
		t.Error("expected dead letter task to be retried")
	}
}

func TestGetAll_Empty(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	deadLetterTaskRepo := NewDeadLetterRepository(testPool, testLogger)

	deadLetterTasks, err := deadLetterTaskRepo.GetAll(context.Background())

	if err != nil {
		t.Fatalf("failed to fetch dead letter tasks: %v", err)
	}

	if len(deadLetterTasks) != 0 {
		t.Errorf("expected no dead letter tasks, got %d", len(deadLetterTasks))
	}
}

func TestDLQ_CountAll(t *testing.T) {
	cleanTasks(t)
	cleanDeadLetterTasks(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	deadLetterTaskRepo := NewDeadLetterRepository(testPool, testLogger)

	for i := range 3 {
		task := model.Task{
			Status:  model.StatusPending,
			Payload: []byte(`{"type":"echo"}`),
		}

		_, err := taskRepo.Create(context.Background(), &task)
		if err != nil {
			t.Fatalf("failed to create task %d: %v", i, err)
		}

		claimTestTasks(t, taskRepo, 1)

		if err := taskRepo.FailTask(context.Background(), task.ID, "boom", nil); err != nil {
			t.Fatalf("failed to fail task %d: %v", i, err)
		}

		if err := deadLetterTaskRepo.MoveToDLQ(context.Background(), task.ID); err != nil {
			t.Fatalf("failed to move task %d to DLQ: %v", i, err)
		}
	}

	count, err := deadLetterTaskRepo.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll error: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 dead letter tasks, got %d", count)
	}
}

func TestDLQ_CountAll_Empty(t *testing.T) {
	cleanDeadLetterTasks(t)

	deadLetterTaskRepo := NewDeadLetterRepository(testPool, testLogger)

	count, err := deadLetterTaskRepo.CountAll(context.Background())
	if err != nil {
		t.Fatalf("CountAll error: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 dead letter tasks in empty DLQ, got %d", count)
	}
}

func cleanDeadLetterTasks(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "DELETE FROM dead_letter_tasks")
	if err != nil {
		t.Fatalf("failed to clean dead letter tasks: %v", err)
	}
}
