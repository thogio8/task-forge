package worker

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
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
}

func NewDispatcher(repo DispatcherRepository, tasks chan<- model.Task, pollInterval time.Duration, batchSize int, logger *slog.Logger) *Dispatcher {
	hostName, err := os.Hostname()
	if err != nil {
		logger.Warn("failed to get kernel hostname", "error", err)
		hostName = "unknown"
	}

	workerID := fmt.Sprintf("%s-%d-%s", hostName, os.Getpid(), uuid.New().String()[:8])

	return &Dispatcher{
		repo:         repo,
		tasks:        tasks,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		workerID:     workerID,
		logger:       logger,
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
			d.logger.Info("dispatcher stopped")
			return
		}
	}
}
