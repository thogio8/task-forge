//go:build integration

package repository

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/model"
)

var testPool *pgxpool.Pool
var testLogger *slog.Logger

func TestMain(m *testing.M) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_PORT"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_SSL_MODE"),
		)
	}

	var err error
	testPool, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		slog.Error("failed to connect to test DB", "error", err)
		os.Exit(1)
	}

	testLogger = slog.New(slog.NewTextHandler(os.Stdout, nil))

	code := m.Run()

	testPool.Close()
	os.Exit(code)
}

func TestCreateTask(t *testing.T) {
	cleanTasks(t)
	repo := NewTaskRepository(testPool, testLogger)

	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"email"}`),
	}

	err := repo.Create(context.Background(), &task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if task.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("expected task ID to be set by DB")
	}

	if task.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set by DB")
	}
}

func TestGetByID(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)
	repo := NewTaskRepository(testPool, testLogger)

	found, err := repo.GetById(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if found.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, found.ID)
	}

	if found.Status != model.StatusPending {
		t.Errorf("expected status %s, got %s", model.StatusPending, found.Status)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	cleanTasks(t)
	repo := NewTaskRepository(testPool, testLogger)

	_, err := repo.GetById(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected an error for non-existent task")
	}
}

func TestGetAll(t *testing.T) {
	cleanTasks(t)
	createTestTask(t)
	createTestTask(t)
	repo := NewTaskRepository(testPool, testLogger)

	tasks, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf("failed to get tasks: %v", err)
	}

	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestUpdateStatus(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)
	repo := NewTaskRepository(testPool, testLogger)

	err := repo.UpdateStatus(context.Background(), created.ID, model.StatusRunning)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	updated, err := repo.GetById(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("failed to get updated task: %v", err)
	}

	if updated.Status != model.StatusRunning {
		t.Errorf("expected status %s, got %s", model.StatusRunning, updated.Status)
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	cleanTasks(t)
	repo := NewTaskRepository(testPool, testLogger)

	err := repo.UpdateStatus(context.Background(), uuid.New(), model.StatusRunning)
	if err == nil {
		t.Fatal("expected an error for non-existent task")
	}
}

func TestClaimTasks_Basic(t *testing.T) {
	cleanTasks(t)
	for range 5 {
		createTestTask(t)
	}

	repo := NewTaskRepository(testPool, testLogger)

	tasks := claimTestTasks(t, repo, 3)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks claimed, got %d", len(tasks))
	}

	task, err := repo.GetById(context.Background(), tasks[0].ID)

	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Status != model.StatusRunning {
		t.Fatalf("expected status running, got %s", task.Status)
	}

	if task.LockedBy == nil {
		t.Fatalf("expected task to be locked by a worker")
	}

	if *task.LockedBy != "test-worker-1" {
		t.Fatalf("expected task to be locked by test-worker-1, got %s", *task.LockedBy)
	}
}

func TestClaimTasks_RespectsNextRetryAt(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)

	repo := NewTaskRepository(testPool, testLogger)

	_, err := testPool.Exec(context.Background(),
		"UPDATE tasks SET next_retry_at = NOW() + INTERVAL '3 days' WHERE id = $1", created.ID)

	if err != nil {
		t.Fatalf("failed to update next_retry_at column: %v", err)
	}

	tasks := claimTestTasks(t, repo, 10)

	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks to be claimed, got %d", len(tasks))
	}
}

func TestCompleteTask(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)

	repo := NewTaskRepository(testPool, testLogger)
	claimTestTasks(t, repo, 10)

	err := repo.CompleteTask(context.Background(), created.ID)

	if err != nil {
		t.Fatalf("failed to mark task as completed: %v", err)
	}

	task, err := repo.GetById(context.Background(), created.ID)

	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Status != model.StatusCompleted {
		t.Fatalf("expected task to be completed, got %s", task.Status)
	}

	if task.LockedBy != nil {
		t.Fatal("expected locked_by to be nil after completion")
	}

	if task.LockedAt != nil {
		t.Fatal("expected locked_at to be nil after completion")
	}
}

func TestFailTask_WithRetry(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)

	repo := NewTaskRepository(testPool, testLogger)
	claimTestTasks(t, repo, 10)

	errMsg := "something went wrong"
	retryAt := time.Now().Add(1 * time.Hour)

	err := repo.FailTask(context.Background(), created.ID, errMsg, &retryAt)

	if err != nil {
		t.Fatalf("failed to mark task as pending: %v", err)
	}

	task, err := repo.GetById(context.Background(), created.ID)

	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Status != model.StatusPending {
		t.Fatalf("expected status pending, got %s", task.Status)
	}

	if task.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt count, got %d", task.AttemptCount)
	}

	if *task.LastError != errMsg {
		t.Fatalf("expected error message %s, got %s", errMsg, *task.LastError)
	}

	if task.LockedBy != nil {
		t.Fatal("expected locked_by to be nil after fail")
	}
}

func TestFailTask_Permanent(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)

	repo := NewTaskRepository(testPool, testLogger)
	claimTestTasks(t, repo, 10)

	errMsg := "something went wrong"

	err := repo.FailTask(context.Background(), created.ID, errMsg, nil)

	if err != nil {
		t.Fatalf("failed to mark task as failed: %v", err)
	}

	task, err := repo.GetById(context.Background(), created.ID)

	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Status != model.StatusFailed {
		t.Fatalf("expected status failed, got %s", task.Status)
	}

	if task.AttemptCount != 1 {
		t.Fatalf("expected 1 attempt count, got %d", task.AttemptCount)
	}

	if *task.LastError != errMsg {
		t.Fatalf("expected error message %s, got %s", errMsg, *task.LastError)
	}

	if task.LockedBy != nil {
		t.Fatal("expected locked_by to be nil after fail")
	}
}

func TestUnlockStaleTasks(t *testing.T) {
	cleanTasks(t)
	created := createTestTask(t)

	repo := NewTaskRepository(testPool, testLogger)
	claimTestTasks(t, repo, 10)

	_, err := testPool.Exec(context.Background(),
		"UPDATE tasks SET locked_at = NOW() - INTERVAL '10 minutes' WHERE id = $1", created.ID)

	if err != nil {
		t.Fatalf("failed to update locked_at column: %v", err)
	}

	count, err := repo.UnlockStaleTasks(context.Background(), 5*time.Minute)

	if err != nil {
		t.Fatalf("failed to unlock stale tasks: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 task unlocked, got: %d", count)
	}

	task, err := repo.GetById(context.Background(), created.ID)

	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if task.Status != model.StatusPending {
		t.Fatalf("expected task to be pending, got: %s", task.Status)
	}
}

func cleanTasks(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "DELETE FROM tasks")
	if err != nil {
		t.Fatalf("failed to clean tasks: %v", err)
	}
}

func createTestTask(t *testing.T) model.Task {
	t.Helper()
	repo := NewTaskRepository(testPool, testLogger)
	task := model.Task{
		Status:  model.StatusPending,
		Payload: []byte(`{"type":"test"}`),
	}
	if err := repo.Create(context.Background(), &task); err != nil {
		t.Fatalf("failed to create test task: %v", err)
	}
	return task
}

func claimTestTasks(t *testing.T, repo *TaskRepository, limit int) []model.Task {
	t.Helper()
	tasks, err := repo.ClaimTasks(context.Background(), "test-worker-1", limit)
	if err != nil {
		t.Fatalf("failed to claim tasks: %v", err)
	}
	return tasks
}
