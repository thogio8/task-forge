package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
)

type TaskFunc func(ctx context.Context, payload json.RawMessage) error

type TaskRepository interface {
	CompleteTask(ctx context.Context, id uuid.UUID) error
	FailTask(ctx context.Context, id uuid.UUID, errMsg string, nextRetryAt *time.Time) error
}

type DLQRepository interface {
	MoveToDLQ(ctx context.Context, taskID uuid.UUID) error
}

type Executor struct {
	handlers         map[string]TaskFunc
	repo             TaskRepository
	taskTimeout      time.Duration
	logger           *slog.Logger
	dlqRepo          DLQRepository
	execDuration     metric.Float64Histogram
	completedCounter metric.Int64Counter
	failedCounter    metric.Int64Counter
	retriedCounter   metric.Int64Counter
}

func NewExecutor(repo TaskRepository, taskTimeout time.Duration, logger *slog.Logger, dlqRepo DLQRepository) *Executor {
	meter := otel.Meter("taskforge.executor")

	execDuration, _ := meter.Float64Histogram(
		"task.execution.duration",
		metric.WithDescription("Execution duration in seconds."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(telemetry.LatencyBuckets...),
	)

	completedCounter, _ := meter.Int64Counter(
		"task.completed.count",
		metric.WithDescription("Number of completed tasks."),
	)

	failedCounter, _ := meter.Int64Counter(
		"task.failed.count",
		metric.WithDescription("Number of failed tasks."),
	)

	retriedCounter, _ := meter.Int64Counter(
		"task.retried.count",
		metric.WithDescription("Number of retried tasks."),
	)

	return &Executor{
		handlers:         make(map[string]TaskFunc),
		repo:             repo,
		taskTimeout:      taskTimeout,
		logger:           logger,
		dlqRepo:          dlqRepo,
		execDuration:     execDuration,
		completedCounter: completedCounter,
		failedCounter:    failedCounter,
		retriedCounter:   retriedCounter,
	}
}

func (e *Executor) Register(taskType string, fn TaskFunc) {
	e.handlers[taskType] = fn
}

func (e *Executor) Execute(ctx context.Context, task model.Task) {
	var payload struct {
		Type string `json:"type"`
	}
	payloadErr := json.Unmarshal(task.Payload, &payload)

	spanTaskType := "unknown"
	if payloadErr == nil && payload.Type != "" {
		if _, ok := e.handlers[payload.Type]; ok {
			spanTaskType = payload.Type
		}
	}

	ctx, span := telemetry.StartSpan(ctx, "task.execute "+spanTaskType,
		attribute.String("task.id", task.ID.String()),
		attribute.String("task.type", spanTaskType),
		attribute.Int("task.attempt", task.AttemptCount+1),
	)
	defer span.End()

	if payloadErr != nil {
		span.RecordError(payloadErr)
		span.SetStatus(codes.Error, "invalid payload")
		e.logger.Error("invalid payload", "task_id", task.ID, "error", payloadErr)
		if failErr := e.repo.FailTask(ctx, task.ID, "invalid payload: "+payloadErr.Error(), nil); failErr != nil {
			e.logger.Error("failed to fail task with invalid payload", "task_id", task.ID, "error", failErr)
		}
		e.failedCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("task_type", "unknown"),
			attribute.String("error_type", "validation"),
		))
		return
	}

	if payload.Type == "" {
		span.SetStatus(codes.Error, "missing task type")
		e.logger.Error("missing task type in payload", "task_id", task.ID)
		if failErr := e.repo.FailTask(ctx, task.ID, "missing task type in payload", nil); failErr != nil {
			e.logger.Error("failed to fail task with missing type", "task_id", task.ID, "error", failErr)
		}
		e.failedCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("task_type", "unknown"),
			attribute.String("error_type", "validation"),
		))
		return
	}

	handler, exists := e.handlers[payload.Type]

	if !exists {
		span.SetStatus(codes.Error, "unknown task type")
		e.logger.Error("unknown task type", "task_id", task.ID, "type", payload.Type)
		if failErr := e.repo.FailTask(ctx, task.ID, "unknown task type: "+payload.Type, nil); failErr != nil {
			e.logger.Error("failed to fail task with unknown type", "task_id", task.ID, "error", failErr)
		}
		e.failedCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("task_type", "unknown"),
			attribute.String("error_type", "validation"),
		))
		return
	}

	e.logger.Info("executing task", "task_id", task.ID, "type", payload.Type)

	handlerCtx, cancel := context.WithTimeout(ctx, e.taskTimeout)
	defer cancel()
	start := time.Now()
	err := handler(handlerCtx, task.Payload)
	e.execDuration.Record(handlerCtx, time.Since(start).Seconds(), metric.WithAttributes(
		attribute.String("task_type", payload.Type),
	))

	if err != nil {
		errorType := "handler"
		if errors.Is(handlerCtx.Err(), context.DeadlineExceeded) {
			errorType = "timeout"
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("error.type", errorType))

		if task.AttemptCount+1 < task.MaxRetries {
			backoff := calculateBackoff(task.AttemptCount + 1)
			nextRetryAt := time.Now().Add(backoff)

			failErr := e.repo.FailTask(ctx, task.ID, err.Error(), &nextRetryAt)

			if failErr != nil {
				e.logger.Error("failed to mark task as pending", "task_id", task.ID, "error", failErr)
			} else {
				e.logger.Warn("task failed, scheduling retry", "task_id", task.ID, "attempt", task.AttemptCount+1, "next_retry", nextRetryAt)
				span.SetAttributes(attribute.String("task.outcome", "retry_scheduled"))
				e.retriedCounter.Add(ctx, 1, metric.WithAttributes(
					attribute.String("task_type", payload.Type),
					attribute.String("error_type", errorType),
				))
			}
		} else {
			moveErr := e.dlqRepo.MoveToDLQ(ctx, task.ID)

			if moveErr != nil {
				e.logger.Error("failed to move task to DLQ", "task_id", task.ID, "error", moveErr)
			} else {
				e.logger.Error("task permanently failed, moved to DLQ", "task_id", task.ID, "attempt", task.AttemptCount+1)
				span.SetAttributes(attribute.String("task.outcome", "dlq"))
				e.failedCounter.Add(ctx, 1, metric.WithAttributes(
					attribute.String("task_type", payload.Type),
					attribute.String("error_type", errorType),
				))
			}
		}

		return
	}

	completeErr := e.repo.CompleteTask(ctx, task.ID)
	if completeErr != nil {
		span.RecordError(completeErr)
		span.SetStatus(codes.Error, completeErr.Error())
		e.logger.Error("failed to mark task as completed", "task_id", task.ID, "error", completeErr)
		return
	}

	e.logger.Info("task completed", "task_id", task.ID)
	span.SetAttributes(attribute.String("task.outcome", "completed"))
	e.completedCounter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("task_type", payload.Type),
	))
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
