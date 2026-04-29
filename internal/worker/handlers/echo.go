package handlers

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/thogio8/task-forge/internal/worker"
)

func Echo(logger *slog.Logger) worker.TaskFunc {
	return func(ctx context.Context, payload json.RawMessage) error {
		logger.InfoContext(ctx, "echo", "payload", string(payload))
		return nil
	}
}
