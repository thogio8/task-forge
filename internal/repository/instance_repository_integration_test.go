//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

func TestRegisterAndHeartbeat(t *testing.T) {
	cleanInstances(t)

	repo := NewInstanceRepository(testPool, testLogger)

	err := repo.Register(context.Background(), "test-instance-1")
	if err != nil {
		t.Fatalf("failed to register instance: %v", err)
	}

	err = repo.Heartbeat(context.Background(), "test-instance-1")
	if err != nil {
		t.Fatalf("failed to send heartbeat: %v", err)
	}
}

func TestGetStaleInstances(t *testing.T) {
	cleanInstances(t)

	repo := NewInstanceRepository(testPool, testLogger)

	err := repo.Register(context.Background(), "test-instance-stale")
	if err != nil {
		t.Fatalf("failed to register instance: %v", err)
	}

	_, err = testPool.Exec(context.Background(),
		"UPDATE instances SET last_heartbeat_at = NOW() - INTERVAL '1 hour' WHERE id = $1", "test-instance-stale")
	if err != nil {
		t.Fatalf("failed to update heartbeat: %v", err)
	}

	stale, err := repo.GetStaleInstances(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("failed to get stale instances: %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("expected 1 stale instance, got %d", len(stale))
	}

	if stale[0].ID != "test-instance-stale" {
		t.Fatalf("expected instance id 'test-instance-stale', got '%s'", stale[0].ID)
	}
}

func TestDeregister(t *testing.T) {
	cleanInstances(t)

	repo := NewInstanceRepository(testPool, testLogger)

	err := repo.Register(context.Background(), "test-instance-remove")
	if err != nil {
		t.Fatalf("failed to register instance: %v", err)
	}

	err = repo.Deregister(context.Background(), "test-instance-remove")
	if err != nil {
		t.Fatalf("failed to deregister instance: %v", err)
	}

	// Set timeout to 0 to get all instances
	stale, err := repo.GetStaleInstances(context.Background(), 0)
	if err != nil {
		t.Fatalf("failed to get stale instances: %v", err)
	}

	if len(stale) != 0 {
		t.Fatalf("expected 0 instances after deregister, got %d", len(stale))
	}
}

func TestRecoverTasksByInstance(t *testing.T) {
	cleanTasks(t)
	cleanInstances(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	instanceRepo := NewInstanceRepository(testPool, testLogger)

	// Register a fake instance
	err := instanceRepo.Register(context.Background(), "dead-instance-1")
	if err != nil {
		t.Fatalf("failed to register instance: %v", err)
	}

	// Create a task
	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"echo"}`),
	}
	_, err = taskRepo.Create(context.Background(), &task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Simulate the task being claimed by the dead instance
	_, err = testPool.Exec(context.Background(),
		"UPDATE tasks SET status = 'running', locked_by = $1, locked_at = NOW() WHERE id = $2",
		"dead-instance-1", task.ID)
	if err != nil {
		t.Fatalf("failed to lock task: %v", err)
	}

	// Verify task is running
	locked, err := taskRepo.GetById(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if locked.Status != model.StatusRunning {
		t.Fatalf("expected running, got %s", locked.Status)
	}

	// Recover tasks from the dead instance
	recovered, err := taskRepo.RecoverTasksByInstance(context.Background(), "dead-instance-1")
	if err != nil {
		t.Fatalf("failed to recover tasks: %v", err)
	}

	if recovered != 1 {
		t.Fatalf("expected 1 recovered task, got %d", recovered)
	}

	// Verify task is back to pending
	task, err = taskRepo.GetById(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Status != model.StatusPending {
		t.Fatalf("expected pending after recovery, got %s", task.Status)
	}

	if task.LockedBy != nil {
		t.Fatal("expected locked_by to be nil after recovery")
	}
}

func cleanInstances(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "DELETE FROM instances")
	if err != nil {
		t.Fatalf("failed to clean instances: %v", err)
	}
}
