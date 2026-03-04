//go:build integration

package repository

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

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
