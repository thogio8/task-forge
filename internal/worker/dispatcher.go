package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type DispatcherRepository interface {
	ClaimTasks(ctx context.Context, workerID string, limit int) ([]model.Task, error)
}

type Dispatcher struct {
	repo           DispatcherRepository
	tasks          chan<- model.TaskEnvelope
	pollInterval   time.Duration
	batchSize      int
	workerID       string
	logger         *slog.Logger
	done           chan struct{}
	cycleCounter   metric.Int64Counter
	claimedCounter metric.Int64Counter
	pollDuration   metric.Float64Histogram
}

func NewDispatcher(repo DispatcherRepository, tasks chan<- model.TaskEnvelope, pollInterval time.Duration, batchSize int, workerID string, logger *slog.Logger) *Dispatcher {
	meter := otel.Meter("taskforge.dispatcher")

	cycleCounter, _ := meter.Int64Counter(
		"dispatcher.cycle.count",
		metric.WithDescription("Number of dispatcher polling cycles executed."),
	)

	claimedCounter, _ := meter.Int64Counter(
		"dispatcher.claimed.count",
		metric.WithDescription("Number of tasks claimed by dispatcher polling."),
	)

	pollDuration, _ := meter.Float64Histogram(
		"dispatcher.poll.duration",
		metric.WithDescription("Duration of dispatcher DB poll in seconds."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(telemetry.LatencyBuckets...),
	)

	return &Dispatcher{
		repo:           repo,
		tasks:          tasks,
		pollInterval:   pollInterval,
		batchSize:      batchSize,
		workerID:       workerID,
		logger:         logger,
		done:           make(chan struct{}),
		cycleCounter:   cycleCounter,
		claimedCounter: claimedCounter,
		pollDuration:   pollDuration,
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	d.logger.Info("dispatcher started", "worker_id", d.workerID)

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.cycleCounter.Add(ctx, 1)
			start := time.Now()
			tasks, err := d.repo.ClaimTasks(ctx, d.workerID, d.batchSize)
			d.pollDuration.Record(ctx, time.Since(start).Seconds())

			if err != nil {
				d.logger.Error("failed to claim tasks", "error", err)
				continue
			}

			if len(tasks) > 0 {
				d.claimedCounter.Add(ctx, int64(len(tasks)))
				d.logger.Info("tasks claimed", "count", len(tasks))
			}

			for _, task := range tasks {
				d.tasks <- model.TaskEnvelope{Task: task, Ctx: ctx}
			}
		case <-ctx.Done():
			close(d.done)
			d.logger.Info("dispatcher stopped")
			return
		}
	}
}

func (d *Dispatcher) Stop() {
	<-d.done
}
