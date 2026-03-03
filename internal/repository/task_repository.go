package repository

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
)

type TaskRepository struct {
	pgxPool *pgxpool.Pool
	logger  *slog.Logger
}

func NewTaskRepository(pool *pgxpool.Pool, logger *slog.Logger) *TaskRepository {
	return &TaskRepository{pgxPool: pool, logger: logger}
}

func (t *TaskRepository) Create(ctx context.Context, task *model.Task) error {
	query := `INSERT INTO tasks (status, payload) VALUES ($1, $2) RETURNING id, created_at, updated_at`

	row := t.pgxPool.QueryRow(ctx, query, task.Status, task.Payload)

	err := row.Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		t.logger.Error("failed to create task", "error", err)
		return apperror.Internal("failed to create task", err)
	}

	t.logger.Info("task created", "task_id", task.ID)
	return nil
}

func (t *TaskRepository) GetAll(ctx context.Context) ([]model.Task, error) {
	query := `SELECT id, status, payload, created_at, updated_at FROM tasks`

	rows, err := t.pgxPool.Query(ctx, query)

	if err != nil {
		t.logger.Error("failed to query tasks", "error", err)
		return nil, apperror.Internal("failed to query tasks", err)
	}
	defer rows.Close()

	var tasks []model.Task

	for rows.Next() {
		var task model.Task
		if err := rows.Scan(&task.ID, &task.Status, &task.Payload, &task.CreatedAt, &task.UpdatedAt); err != nil {
			t.logger.Error("failed to scan task row", "error", err)
			return nil, apperror.Internal("failed to scan task row", err)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		t.logger.Error("failed to iterate task rows", "error", err)
		return nil, apperror.Internal("failed to iterate task rows", err)
	}

	t.logger.Info("all tasks fetched", "count", len(tasks))
	return tasks, nil
}

func (t *TaskRepository) GetById(ctx context.Context, id uuid.UUID) (model.Task, error) {
	query := `SELECT id, status, payload, created_at, updated_at FROM tasks WHERE id = $1`

	row := t.pgxPool.QueryRow(ctx, query, id)

	var task model.Task

	err := row.Scan(&task.ID, &task.Status, &task.Payload, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			t.logger.Warn("task not found", "task_id", task.ID)
			return model.Task{}, apperror.NotFound("task not found", err)
		}

		t.logger.Error("failed to scan task", "error", err)
		return model.Task{}, apperror.Internal("failed to scan task", err)
	}

	t.logger.Info("task fetched", "task_id", task.ID)
	return task, nil
}

func (t *TaskRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	query := `UPDATE tasks SET status = $1 WHERE id = $2`

	results, err := t.pgxPool.Exec(ctx, query, status, id)

	if err != nil {
		t.logger.Error("failed to update task", "error", err)
		return apperror.Internal("failed to update task", err)
	}

	if results.RowsAffected() == 0 {
		t.logger.Warn("task not found", "task_id", id)
		return apperror.NotFound("task not found", nil)
	}

	t.logger.Info("task updated", "task_id", id)
	return nil
}
