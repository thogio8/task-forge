package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

type InstanceMonitorRepository interface {
	GetStaleInstances(ctx context.Context, timeout time.Duration) ([]model.Instance, error)
	Deregister(ctx context.Context, instance string) error
}

type TaskRecoveryRepository interface {
	RecoverTasksByInstance(ctx context.Context, instanceID string) (int, error)
}

func StartInstanceMonitor(ctx context.Context, instanceRepo InstanceMonitorRepository, taskRepo TaskRecoveryRepository, interval time.Duration, heartbeatTimeout time.Duration, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				staleInstances, err := instanceRepo.GetStaleInstances(ctx, heartbeatTimeout)

				if err != nil {
					logger.Error("failed to get stale instances", "error", err)
					continue
				}

				for _, instance := range staleInstances {
					recovered, err := taskRepo.RecoverTasksByInstance(ctx, instance.ID)

					if err != nil {
						logger.Error("failed to recover tasks", "instance_id", instance.ID, "error", err)
						continue
					}

					err = instanceRepo.Deregister(ctx, instance.ID)
					if err != nil {
						logger.Error("failed to deregister instance", "instance_id", instance.ID, "error", err)
						continue
					}

					logger.Warn("dead instance detected", "instance_id", instance.ID, "tasks_recovered", recovered)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
