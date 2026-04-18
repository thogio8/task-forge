package repository

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
)

type OutboxRepository struct {
	pgxPool *pgxpool.Pool
	logger  *slog.Logger
}

func NewOutboxRepository(pool *pgxpool.Pool, logger *slog.Logger) *OutboxRepository {
	return &OutboxRepository{pgxPool: pool, logger: logger}
}

func (o *OutboxRepository) GetUnpublished(ctx context.Context, limit int) ([]model.OutboxEvent, error) {
	query := `
		SELECT id, event_type, payload, created_at, published_at
		FROM outbox
		WHERE published_at IS NULL
		ORDER BY created_at
		LIMIT $1
	`

	rows, err := o.pgxPool.Query(ctx, query, limit)

	if err != nil {
		o.logger.Error("failed to get outbox events", "error", err)
		return nil, apperror.Internal("failed to get outbox events", err)
	}
	defer rows.Close()

	var outboxEvents []model.OutboxEvent

	for rows.Next() {
		outboxEvent, err := scanOutbox(rows)

		if err != nil {
			o.logger.Error("failed to scan outbox event row", "error", err)
			return nil, apperror.Internal("failed to scan outbox event row", err)
		}
		outboxEvents = append(outboxEvents, outboxEvent)
	}

	if err := rows.Err(); err != nil {
		o.logger.Error("failed to iterate outbox event rows", "error", err)
		return nil, apperror.Internal("failed to iterate outbox event rows", err)
	}

	o.logger.Debug("unpublished outbox events fetched", "count", len(outboxEvents))
	return outboxEvents, nil
}

func (o *OutboxRepository) MarkPublished(ctx context.Context, ids []int64) error {
	query := `
		UPDATE outbox
		SET published_at = NOW()
		WHERE id = ANY($1)
	`

	results, err := o.pgxPool.Exec(ctx, query, ids)

	if err != nil {
		o.logger.Error("failed to mark outbox events as published", "error", err)
		return apperror.Internal("failed to mark outbox events as published", err)
	}

	if results.RowsAffected() > 0 {
		o.logger.Debug("marked outbox events as published", "count", results.RowsAffected())
	}

	return nil
}

func (o *OutboxRepository) CountUnpublished(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM outbox WHERE published_at IS NULL`

	var count int64
	err := o.pgxPool.QueryRow(ctx, query).Scan(&count)

	if err != nil {
		o.logger.Error("failed to count unpublished outbox events", "error", err)
		return 0, apperror.Internal("failed to count unpublished outbox events", err)
	}

	return count, nil
}

func (o *OutboxRepository) Purge(ctx context.Context, retention time.Duration) error {
	query := `
		DELETE FROM outbox
		WHERE published_at IS NOT NULL
		AND published_at < NOW() - $1::interval
	`

	results, err := o.pgxPool.Exec(ctx, query, retention)

	if err != nil {
		o.logger.Error("failed to purge outbox events", "error", err)
		return apperror.Internal("failed to purge outbox events", err)
	}

	if results.RowsAffected() > 0 {
		o.logger.Debug("purged outbox events", "count", results.RowsAffected())
	}

	return nil
}

func scanOutbox(s scanner) (model.OutboxEvent, error) {
	var outbox model.OutboxEvent
	err := s.Scan(
		&outbox.ID, &outbox.EventType, &outbox.Payload,
		&outbox.CreatedAt, &outbox.PublishedAt,
	)
	return outbox, err
}
