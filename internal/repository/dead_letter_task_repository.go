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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type DeadLetterTaskRepository struct {
	pgxPool *pgxpool.Pool
	logger  *slog.Logger
}

func NewDeadLetterRepository(pool *pgxpool.Pool, logger *slog.Logger) *DeadLetterTaskRepository {
	repo := &DeadLetterTaskRepository{pgxPool: pool, logger: logger}

	meter := otel.Meter("taskforge.dlq")
	_, _ = meter.Int64ObservableGauge(
		"dlq.size",
		metric.WithDescription("Number of tasks currently in the dead letter queue"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			count, err := repo.CountAll(ctx)
			if err != nil {
				logger.WarnContext(ctx, "failed to observe dlq.size", "error", err)
				return nil
			}
			o.Observe(count)
			return nil
		}),
	)

	return repo
}

func (d *DeadLetterTaskRepository) CountAll(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM dead_letter_tasks`

	var count int64
	err := d.pgxPool.QueryRow(ctx, query).Scan(&count)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to count dead letter tasks", "error", err)
		return 0, apperror.Internal("failed to count dead letter tasks", err)
	}

	return count, nil
}

func (d *DeadLetterTaskRepository) MoveToDLQ(ctx context.Context, taskID uuid.UUID) error {
	tx, err := d.pgxPool.Begin(ctx)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to begin transaction", "error", err)
		return apperror.Internal("failed to begin transaction", err)
	}

	defer tx.Rollback(ctx)

	selectQuery := `
		SELECT id, status, payload, created_at, updated_at, locked_by, locked_at,
		attempt_count, max_retries, last_error, next_retry_at, idempotency_key
		FROM tasks
		WHERE id = $1
	`
	row := tx.QueryRow(ctx, selectQuery, taskID)

	task, err := scanTask(row)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			d.logger.WarnContext(ctx, "task not found", "task_id", taskID)
			return apperror.NotFound("task not found", err)
		}

		d.logger.ErrorContext(ctx, "failed to scan task", "error", err)
		return apperror.Internal("failed to scan task", err)
	}

	insertQuery := `
		INSERT INTO dead_letter_tasks (original_task_id, payload, last_error, attempt_count)
		VALUES ($1, $2, $3, $4)
	`

	_, err = tx.Exec(ctx, insertQuery, task.ID, task.Payload, task.LastError, task.AttemptCount)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to insert dead letter task", "error", err)
		return apperror.Internal("failed to insert dead letter task", err)
	}

	deleteQuery := `
		DELETE FROM tasks
		WHERE id = $1
	`

	_, err = tx.Exec(ctx, deleteQuery, task.ID)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to delete task", "error", err)
		return apperror.Internal("failed to delete task", err)
	}

	err = tx.Commit(ctx)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return apperror.Internal("failed to commit transaction", err)
	}

	return nil
}

func (d *DeadLetterTaskRepository) GetAll(ctx context.Context) ([]model.DeadLetterTask, error) {
	query := `
		SELECT id, original_task_id, payload, last_error, attempt_count, failed_at, retried_at,
		created_at
		FROM dead_letter_tasks
	`

	rows, err := d.pgxPool.Query(ctx, query)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to get all dead letter tasks", "error", err)
		return nil, apperror.Internal("failed to get all dead letter tasks", err)
	}
	defer rows.Close()

	var deadLetterTasks []model.DeadLetterTask

	for rows.Next() {
		deadLetterTask, err := scanDeadLetterTask(rows)

		if err != nil {
			d.logger.ErrorContext(ctx, "failed to scan dead letter task row", "error", err)
			return nil, apperror.Internal("failed to scan dead letter task row", err)
		}
		deadLetterTasks = append(deadLetterTasks, deadLetterTask)
	}

	if err := rows.Err(); err != nil {
		d.logger.ErrorContext(ctx, "failed to iterate dead letter task rows", "error", err)
		return nil, apperror.Internal("failed to iterate dead letter task rows", err)
	}

	d.logger.InfoContext(ctx, "all dead letter tasks fetched", "count", len(deadLetterTasks))
	return deadLetterTasks, nil
}

func (d *DeadLetterTaskRepository) GetById(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error) {
	query := `
		SELECT id, original_task_id, payload, last_error, attempt_count, failed_at, retried_at,
		created_at
		FROM dead_letter_tasks
		WHERE id = $1
		`

	row := d.pgxPool.QueryRow(ctx, query, id)

	deadLetterTask, err := scanDeadLetterTask(row)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			d.logger.WarnContext(ctx, "dead letter task not found", "dead_letter_task_id", deadLetterTask.ID)
			return model.DeadLetterTask{}, apperror.NotFound("dead letter task not found", err)
		}

		d.logger.ErrorContext(ctx, "failed to scan dead letter task", "error", err)
		return model.DeadLetterTask{}, apperror.Internal("failed to scan dead letter task", err)
	}

	d.logger.InfoContext(ctx, "dead letter task fetched", "dead_letter_task_id", deadLetterTask.ID)
	return deadLetterTask, nil
}

func (d *DeadLetterTaskRepository) Retry(ctx context.Context, id uuid.UUID) (model.Task, error) {
	tx, err := d.pgxPool.Begin(ctx)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to begin transaction", "error", err)
		return model.Task{}, apperror.Internal("failed to begin transaction", err)
	}

	defer tx.Rollback(ctx)

	selectQuery := `
		SELECT id, original_task_id, payload, last_error, attempt_count, failed_at, retried_at,
		created_at
		FROM dead_letter_tasks
		WHERE id = $1
	`

	row := tx.QueryRow(ctx, selectQuery, id)

	deadLetterTask, err := scanDeadLetterTask(row)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			d.logger.WarnContext(ctx, "dead letter task not found", "dead_letter_task_id", id)
			return model.Task{}, apperror.NotFound("dead letter task not found", err)
		}

		d.logger.ErrorContext(ctx, "failed to scan dead letter task", "error", err)
		return model.Task{}, apperror.Internal("failed to scan dead letter task", err)
	}

	insertQuery := `
		INSERT INTO tasks (status, payload)
		VALUES ($1, $2)
		RETURNING id, created_at, updated_at
	`

	var task model.Task

	row = tx.QueryRow(ctx, insertQuery, model.StatusPending, deadLetterTask.Payload)

	err = row.Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		d.logger.ErrorContext(ctx, "failed to insert task", "error", err)
		return model.Task{}, apperror.Internal("failed to insert task", err)
	}

	task.Status = model.StatusPending
	task.Payload = deadLetterTask.Payload

	updateQuery := `
		UPDATE dead_letter_tasks
		SET retried_at = NOW()
		WHERE id = $1
	`

	_, err = tx.Exec(ctx, updateQuery, deadLetterTask.ID)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to update dead letter task", "error", err)
		return model.Task{}, apperror.Internal("failed to update dead letter task", err)
	}

	err = tx.Commit(ctx)

	if err != nil {
		d.logger.ErrorContext(ctx, "failed to commit transaction", "error", err)
		return model.Task{}, apperror.Internal("failed to commit transaction", err)
	}

	return task, nil
}

func scanDeadLetterTask(s scanner) (model.DeadLetterTask, error) {
	var deadLetterTask model.DeadLetterTask
	err := s.Scan(
		&deadLetterTask.ID, &deadLetterTask.OriginalTaskID,
		&deadLetterTask.Payload, &deadLetterTask.LastError,
		&deadLetterTask.AttemptCount, &deadLetterTask.FailedAt,
		&deadLetterTask.RetriedAt, &deadLetterTask.CreatedAt,
	)
	return deadLetterTask, err
}
