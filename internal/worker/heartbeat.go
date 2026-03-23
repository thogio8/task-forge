package worker

import (
	"context"
	"log/slog"
	"time"
)

type HeartbeatRepository interface {
	Heartbeat(ctx context.Context, instanceID string) error
}

func StartHeartbeat(ctx context.Context, repo HeartbeatRepository, instanceID string, interval time.Duration, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				err := repo.Heartbeat(ctx, instanceID)
				if err != nil {
					logger.Error("failed to send heartbeat", "instance_id", instanceID, "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
