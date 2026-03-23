package repository

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
)

type InstanceRepository struct {
	pgxPool *pgxpool.Pool
	logger  *slog.Logger
}

func NewInstanceRepository(pool *pgxpool.Pool, logger *slog.Logger) *InstanceRepository {
	return &InstanceRepository{pgxPool: pool, logger: logger}
}

func (i *InstanceRepository) Register(ctx context.Context, instanceID string) error {
	query := `
		INSERT INTO instances (id) 
		VALUES ($1)
	`

	_, err := i.pgxPool.Exec(ctx, query, instanceID)

	if err != nil {
		i.logger.Error("failed to create instance", "instance_id", instanceID, "error", err)
		return apperror.Internal("failed to create instance", err)
	}

	i.logger.Info("instance registered", "instance_id", instanceID)
	return nil
}

func (i *InstanceRepository) Heartbeat(ctx context.Context, instanceID string) error {
	query := `
		UPDATE instances
		SET last_heartbeat_at = NOW()
		WHERE id = $1
	`

	results, err := i.pgxPool.Exec(ctx, query, instanceID)

	if err != nil {
		i.logger.Error("failed to update heartbeat", "instance_id", instanceID, "error", err)
		return apperror.Internal("failed to update heartbeat", err)
	}

	if results.RowsAffected() == 0 {
		i.logger.Warn("instance not found", "instance_id", instanceID)
		return apperror.NotFound("instance not found", nil)
	}

	i.logger.Debug("instance updated", "instance_id", instanceID)
	return nil
}

func (i *InstanceRepository) GetStaleInstances(ctx context.Context, timeout time.Duration) ([]model.Instance, error) {
	query := `
		SELECT id, last_heartbeat_at, started_at
		FROM instances
		WHERE last_heartbeat_at < NOW() - $1::interval
	`

	rows, err := i.pgxPool.Query(ctx, query, timeout)
	if err != nil {
		i.logger.Error("failed to get stale instances", "error", err)
		return nil, apperror.Internal("failed to get stale instances", err)
	}
	defer rows.Close()

	var instances []model.Instance

	for rows.Next() {
		instance, err := scanInstance(rows)

		if err != nil {
			i.logger.Error("failed to scan stale instance row", "error", err)
			return nil, apperror.Internal("failed to scan stale instance row", err)
		}
		instances = append(instances, instance)
	}

	if err := rows.Err(); err != nil {
		i.logger.Error("failed to iterate stale instance rows", "error", err)
		return nil, apperror.Internal("failed to iterate stale instance rows", err)
	}

	i.logger.Info("all stale instances fetched", "count", len(instances))
	return instances, nil
}

func (i *InstanceRepository) Deregister(ctx context.Context, instanceID string) error {
	query := `
		DELETE FROM instances 
		WHERE id = $1
	`

	_, err := i.pgxPool.Exec(ctx, query, instanceID)

	if err != nil {
		i.logger.Error("failed to deregister instance", "instance_id", instanceID, "error", err)
		return apperror.Internal("failed to deregister instance", err)
	}

	i.logger.Info("instance deregistered", "instance_id", instanceID)
	return nil
}

func scanInstance(s scanner) (model.Instance, error) {
	var instance model.Instance
	err := s.Scan(&instance.ID, &instance.LastHeartbeatAt, &instance.StartedAt)
	return instance, err
}
