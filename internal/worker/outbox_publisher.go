package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

type OutboxPublisherRepository interface {
	GetUnpublished(ctx context.Context, limit int) ([]model.OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []int64) error
	Purge(ctx context.Context, retention time.Duration) error
}

type OutboxProducer interface {
	Publish(ctx context.Context, key string, value []byte) error
}

type OutboxPublisher struct {
	repo         OutboxPublisherRepository
	producer     OutboxProducer
	pollInterval time.Duration
	batchSize    int
	retention    time.Duration
	logger       *slog.Logger
	done         chan struct{}
}

func NewOutboxPublisher(repo OutboxPublisherRepository, producer OutboxProducer, pollInterval time.Duration, batchSize int, retention time.Duration, logger *slog.Logger) *OutboxPublisher {
	return &OutboxPublisher{
		repo:         repo,
		producer:     producer,
		pollInterval: pollInterval,
		batchSize:    batchSize,
		retention:    retention,
		logger:       logger,
		done:         make(chan struct{}),
	}
}

func (o *OutboxPublisher) Run(ctx context.Context) {
	o.logger.Info("outbox publisher started")

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.processCycle(ctx)
		case <-ctx.Done():
			close(o.done)
			o.logger.Info("outbox publisher stopped")
			return
		}
	}
}

func (o *OutboxPublisher) processCycle(ctx context.Context) {
	outboxEvents, err := o.repo.GetUnpublished(ctx, o.batchSize)

	var outboxEventsIDs []int64

	if err != nil {
		o.logger.Error("failed to retrieve unpublished outbox events", "error", err)
		return
	}

	for _, outboxEvent := range outboxEvents {
		err = o.producer.Publish(ctx, extractTaskID(outboxEvent.Payload), outboxEvent.Payload)

		if err != nil {
			o.logger.Error("failed to publish outbox event", "outbox_event_id", outboxEvent.ID, "error", err)
			continue
		}

		outboxEventsIDs = append(outboxEventsIDs, outboxEvent.ID)
	}

	if len(outboxEventsIDs) > 0 {
		err = o.repo.MarkPublished(ctx, outboxEventsIDs)

		if err != nil {
			o.logger.Error("failed to mark outbox events as published", "error", err)
			return
		}
	}

	err = o.repo.Purge(ctx, o.retention)

	if err != nil {
		o.logger.Error("failed to purge outbox events", "error", err)
		return
	}
}

func (o *OutboxPublisher) Stop() {
	<-o.done
}

func extractTaskID(payload json.RawMessage) string {
	var data struct {
		TaskID string `json:"task_id"`
	}

	err := json.Unmarshal(payload, &data)

	if err != nil {
		return ""
	}

	return data.TaskID
}
