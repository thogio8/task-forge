package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

type DispatcherRepository interface {
	ClaimTasks(ctx context.Context, workerID string, limit int) ([]model.Task, error)
}

type Dispatcher struct {
	repo         DispatcherRepository
	tasks        chan<- model.Task
	pollInterval time.Duration
	batchSize    int
	workerID     string
	logger       *slog.Logger
	done         chan struct{}
}

func NewDispatcher(repo DispatcherRepository, tasks chan<- model.Task, pollInterval time.Duration, batchSize int, workerID string, logger *slog.Logger) *Dispatcher {
	return &Dispatcher{
		repo:         repo,
		tasks:        tasks,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		workerID:     workerID,
		logger:       logger,
		done:         make(chan struct{}),
	}
}

func (d *Dispatcher) Run(ctx context.Context) {
	d.logger.Info("dispatcher started", "worker_id", d.workerID)

	ticker := time.NewTicker(d.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tasks, err := d.repo.ClaimTasks(ctx, d.workerID, d.batchSize)

			if err != nil {
				d.logger.Error("failed to claim tasks", "error", err)
				continue
			}

			if len(tasks) > 0 {
				d.logger.Info("tasks claimed", "count", len(tasks))
			}

			for _, task := range tasks {
				d.tasks <- task
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
