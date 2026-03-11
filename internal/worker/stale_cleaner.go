package worker

import (
	"context"
	"log/slog"
	"time"
)

type StaleCleanerRepository interface {
	UnlockStaleTasks(ctx context.Context, staleDuration time.Duration) (int, error)
}

func StartStaleCleaner(ctx context.Context, repo StaleCleanerRepository, interval time.Duration, duration time.Duration, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				count, err := repo.UnlockStaleTasks(ctx, duration)
				if err != nil {
					logger.Error("failed to unlock stale tasks", "error", err)
				}
				if count > 0 {
					logger.Info("unlocked stale tasks", "count", count)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
