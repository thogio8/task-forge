package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/model"
)

type TaskFunc func(ctx context.Context, payload json.RawMessage) error

type TaskRepository interface {
	CompleteTask(ctx context.Context, id uuid.UUID) error
	FailTask(ctx context.Context, id uuid.UUID, errMsg string, nextRetryAt *time.Time) error
}

type Executor struct {
	handlers    map[string]TaskFunc
	repo        TaskRepository
	taskTimeout time.Duration
	logger      *slog.Logger
}

func NewExecutor(repo TaskRepository, taskTimeout time.Duration, logger *slog.Logger) *Executor {
	return &Executor{handlers: make(map[string]TaskFunc), repo: repo, taskTimeout: taskTimeout, logger: logger}
}

func (e *Executor) Register(taskType string, fn TaskFunc) {
	e.handlers[taskType] = fn
}

func (e *Executor) Execute(ctx context.Context, task model.Task) {
	var payload struct {
		Type string `json:"type"`
	}
	err := json.Unmarshal(task.Payload, &payload)

	if err != nil {
		e.logger.Error("invalid payload", "error", err)
		e.repo.FailTask(ctx, task.ID, "invalid payload: "+err.Error(), nil)
		return
	}

	handler, exists := e.handlers[payload.Type]

	if !exists {
		e.logger.Error("unknown task type", "task_id", task.ID, "type", payload.Type)
		e.repo.FailTask(ctx, task.ID, "unknown task type: "+payload.Type, nil)
		return
	}

	e.logger.Info("executing task", "task_id", task.ID, "type", payload.Type)

	ctx, cancel := context.WithTimeout(ctx, e.taskTimeout)
	defer cancel()
	err = handler(ctx, task.Payload)

	if err != nil {
		if task.AttemptCount+1 < task.MaxRetries {
			backoff := calculateBackoff(task.AttemptCount + 1)
			nextRetryAt := time.Now().Add(backoff)

			failErr := e.repo.FailTask(ctx, task.ID, err.Error(), &nextRetryAt)

			if failErr != nil {
				e.logger.Error("failed to mark task as pending", "task_id", task.ID, "error", failErr)
			} else {
				e.logger.Warn("task failed, scheduling retry", "task_id", task.ID, "attempt", task.AttemptCount+1, "next_retry", nextRetryAt)
			}
		} else {
			failErr := e.repo.FailTask(ctx, task.ID, err.Error(), nil)

			if failErr != nil {
				e.logger.Error("failed to mark task as failed", "task_id", task.ID, "error", failErr)
			} else {
				e.logger.Error("task permanently failed", "task_id", task.ID, "attempt", task.AttemptCount+1)
			}
		}

	} else {
		completeErr := e.repo.CompleteTask(ctx, task.ID)
		if completeErr != nil {
			e.logger.Error("failed to mark task as completed", "task_id", task.ID, "error", completeErr)
		} else {
			e.logger.Info("task completed", "task_id", task.ID)
		}
	}
}

func calculateBackoff(attempt int) time.Duration {
	base := 1 * time.Second

	backOff := base * time.Duration(1<<(attempt-1))

	maxBackOff := 5 * time.Minute
	if backOff > maxBackOff {
		backOff = maxBackOff
	}

	jitter := time.Duration(rand.Int63n(int64(backOff/5))) - backOff/10
	backOff += jitter

	return backOff
}
