package repository

import (
	"context"
	"errors"
	"log/slog"
	"time"

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
	query := `
		SELECT id, status, payload, created_at, updated_at, locked_by, locked_at,
    	attempt_count, max_retries, last_error, next_retry_at
		FROM tasks
	`

	rows, err := t.pgxPool.Query(ctx, query)

	if err != nil {
		t.logger.Error("failed to query tasks", "error", err)
		return nil, apperror.Internal("failed to query tasks", err)
	}
	defer rows.Close()

	var tasks []model.Task

	for rows.Next() {
		var task model.Task
		if err := rows.Scan(
			&task.ID,
			&task.Status,
			&task.Payload,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.LockedBy,
			&task.LockedAt,
			&task.AttemptCount,
			&task.MaxRetries,
			&task.LastError,
			&task.NextRetryAt,
		); err != nil {
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
	query := `
		SELECT id, status, payload, created_at, updated_at, locked_by, locked_at,
    	attempt_count, max_retries, last_error, next_retry_at
		FROM tasks
		WHERE id = $1
		`

	row := t.pgxPool.QueryRow(ctx, query, id)

	var task model.Task

	err := row.Scan(
		&task.ID,
		&task.Status,
		&task.Payload,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.LockedBy,
		&task.LockedAt,
		&task.AttemptCount,
		&task.MaxRetries,
		&task.LastError,
		&task.NextRetryAt,
	)

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

	t.logger.Info("task updated", "task_id", id, "new_status", status)
	return nil
}

func (t *TaskRepository) ClaimTasks(ctx context.Context, workerID string, limit int) ([]model.Task, error) {
	tx, err := t.pgxPool.Begin(ctx)

	if err != nil {
		t.logger.Error("failed to begin transaction", "error", err)
		return nil, apperror.Internal("failed to begin transaction", err)
	}
	defer tx.Rollback(ctx)

	selectQuery := `
		SELECT id, status, payload, created_at, updated_at, locked_by, locked_at,
    	attempt_count, max_retries, last_error, next_retry_at
  		FROM tasks
  		WHERE status = 'pending'
    	AND locked_by IS NULL
    	AND (next_retry_at IS NULL OR next_retry_at <= NOW())
  		ORDER BY created_at
  		LIMIT $1
  		FOR UPDATE SKIP LOCKED;
		`

	rows, err := tx.Query(ctx, selectQuery, limit)

	if err != nil {
		t.logger.Error("failed to fetch claimable tasks", "error", err)
		return nil, apperror.Internal("failed to fetch claimable tasks", err)
	}
	defer rows.Close()

	var tasks []model.Task
	var ids []uuid.UUID

	for rows.Next() {
		var task model.Task

		if err := rows.Scan(
			&task.ID,
			&task.Status,
			&task.Payload,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.LockedBy,
			&task.LockedAt,
			&task.AttemptCount,
			&task.MaxRetries,
			&task.LastError,
			&task.NextRetryAt,
		); err != nil {
			t.logger.Error("failed to scan claimable task", "error", err)
			return nil, apperror.Internal("failed to scan claimable task", err)
		}

		tasks = append(tasks, task)
		ids = append(ids, task.ID)
	}

	if err := rows.Err(); err != nil {
		t.logger.Error("failed to iterate claimable tasks", "error", err)
		return nil, apperror.Internal("failed to iterate claimable tasks", err)
	}

	if len(ids) == 0 {
		return tasks, nil
	}

	updateQuery := `UPDATE tasks SET status = 'running', locked_by = $1, locked_at = NOW() WHERE id = ANY($2)`

	_, err = tx.Exec(ctx, updateQuery, workerID, ids)

	if err != nil {
		t.logger.Error("failed to claim tasks", "error", err)
		return nil, apperror.Internal("failed to claim tasks", err)
	}

	err = tx.Commit(ctx)

	if err != nil {
		t.logger.Error("failed to commit claim transaction", "error", err)
		return nil, apperror.Internal("failed to commit claim transaction", err)
	}

	return tasks, nil
}

func (t *TaskRepository) CompleteTask(ctx context.Context, id uuid.UUID) error {
	query := "UPDATE tasks SET status = 'completed', locked_by = NULL, locked_at = NULL WHERE id = $1"

	results, err := t.pgxPool.Exec(ctx, query, id)

	if err != nil {
		t.logger.Error("failed to mark task as completed", "task_id", id, "error", err)
		return apperror.Internal("failed to mark task as completed", err)
	}

	if results.RowsAffected() == 0 {
		t.logger.Warn("task not found", "task_id", id)
		return apperror.NotFound("task not found", nil)
	}

	return nil
}

func (t *TaskRepository) FailTask(ctx context.Context, id uuid.UUID, errMsg string, nextRetryAt *time.Time) error {
	if nextRetryAt != nil {
		query := `
			UPDATE tasks
			SET status = 'pending', locked_by = NULL, locked_at = NULL, attempt_count = attempt_count + 1,
			last_error = $1, next_retry_at = $2 
			WHERE id = $3
		`

		results, err := t.pgxPool.Exec(ctx, query, errMsg, nextRetryAt, id)

		if err != nil {
			t.logger.Error("failed to mark task as pending", "task_id", id, "error", err)
			return apperror.Internal("failed to mark task as pending", err)
		}

		if results.RowsAffected() == 0 {
			t.logger.Warn("task not found", "task_id", id)
			return apperror.NotFound("task not found", nil)
		}

	} else {
		query := `
			UPDATE tasks
			SET status = 'failed', locked_by = NULL, locked_at = NULL, attempt_count = attempt_count + 1,
			last_error = $1
			WHERE id = $2
		`

		results, err := t.pgxPool.Exec(ctx, query, errMsg, id)

		if err != nil {
			t.logger.Error("failed to mark task as failed", "task_id", id, "error", err)
			return apperror.Internal("failed to mark task as failed", err)
		}

		if results.RowsAffected() == 0 {
			t.logger.Warn("task not found", "task_id", id)
			return apperror.NotFound("task not found", nil)
		}

	}

	return nil
}

func (t *TaskRepository) UnlockStaleTasks(ctx context.Context, staleDuration time.Duration) (int, error) {
	query := `
		UPDATE tasks
		SET status = 'pending', locked_by = NULL, locked_at = NULL
		WHERE status = 'running'
		AND locked_at < NOW() - $1::interval
	`

	results, err := t.pgxPool.Exec(ctx, query, staleDuration)

	if err != nil {
		t.logger.Error("failed to unlock stale tasks", "error", err)
		return 0, apperror.Internal("failed to unlock stale tasks", err)
	}

	unlocked := int(results.RowsAffected())

	return unlocked, nil
}
