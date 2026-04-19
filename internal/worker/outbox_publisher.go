package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type OutboxPublisherRepository interface {
	GetUnpublished(ctx context.Context, limit int) ([]model.OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []int64) error
	Purge(ctx context.Context, retention time.Duration) error
	CountUnpublished(ctx context.Context) (int64, error)
}

type OutboxProducer interface {
	Publish(ctx context.Context, key string, value []byte) error
}

type OutboxPublisher struct {
	repo             OutboxPublisherRepository
	producer         OutboxProducer
	pollInterval     time.Duration
	batchSize        int
	retention        time.Duration
	workerID         string
	logger           *slog.Logger
	done             chan struct{}
	publishedCounter metric.Int64Counter
	errorsCounter    metric.Int64Counter
	publishDuration  metric.Float64Histogram
}

func NewOutboxPublisher(repo OutboxPublisherRepository, producer OutboxProducer, pollInterval time.Duration, batchSize int, retention time.Duration, workerID string, logger *slog.Logger) *OutboxPublisher {
	meter := otel.Meter("taskforge.outbox")

	_, _ = meter.Int64ObservableGauge(
		"outbox.pending",
		metric.WithDescription("Number of unpublished events in the outbox"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			count, err := repo.CountUnpublished(ctx)
			if err != nil {
				logger.Warn("failed to observe outbox.pending", "error", err)
				return nil
			}
			o.Observe(count)
			return nil
		}),
	)

	publishedCounter, _ := meter.Int64Counter(
		"outbox.published.count",
		metric.WithDescription("Number of outbox events published to Kafka"),
	)

	errorsCounter, _ := meter.Int64Counter(
		"outbox.publish.errors.count",
		metric.WithDescription("Number of failed outbox publish attempts to Kafka"),
	)

	publishDuration, _ := meter.Float64Histogram(
		"outbox.publish.duration",
		metric.WithDescription("Duration of event publishing to Kafka"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(telemetry.LatencyBuckets...),
	)

	return &OutboxPublisher{
		repo:             repo,
		producer:         producer,
		pollInterval:     pollInterval,
		batchSize:        batchSize,
		retention:        retention,
		workerID:         workerID,
		logger:           logger,
		done:             make(chan struct{}),
		publishedCounter: publishedCounter,
		errorsCounter:    errorsCounter,
		publishDuration:  publishDuration,
	}
}

func (o *OutboxPublisher) Run(ctx context.Context) {
	o.logger.Info("outbox publisher started", "worker_id", o.workerID)

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.processCycle(ctx)
		case <-ctx.Done():
			close(o.done)
			o.logger.Info("outbox publisher stopped", "worker_id", o.workerID)
			return
		}
	}
}

func (o *OutboxPublisher) processCycle(ctx context.Context) {
	outboxEvents, err := o.repo.GetUnpublished(ctx, o.batchSize)

	var outboxEventsIDs []int64

	if err != nil {
		o.logger.Error("failed to retrieve unpublished outbox events", "worker_id", o.workerID, "error", err)
		return
	}

	for _, outboxEvent := range outboxEvents {
		taskID, extractErr := extractTaskID(outboxEvent.Payload)
		if extractErr != nil {
			o.logger.Warn("failed to extract task_id from outbox payload", "worker_id", o.workerID, "outbox_event_id", outboxEvent.ID, "error", extractErr)
		}

		start := time.Now()
		err = o.producer.Publish(ctx, taskID, outboxEvent.Payload)
		o.publishDuration.Record(ctx, time.Since(start).Seconds())

		if err != nil {
			o.logger.Error("failed to publish outbox event", "worker_id", o.workerID, "outbox_event_id", outboxEvent.ID, "error", err)
			o.errorsCounter.Add(ctx, 1)
			continue
		}

		o.publishedCounter.Add(ctx, 1)

		outboxEventsIDs = append(outboxEventsIDs, outboxEvent.ID)
	}

	if len(outboxEventsIDs) > 0 {
		err = o.repo.MarkPublished(ctx, outboxEventsIDs)

		if err != nil {
			o.logger.Error("failed to mark outbox events as published", "worker_id", o.workerID, "error", err)
			return
		}
	}

	err = o.repo.Purge(ctx, o.retention)

	if err != nil {
		o.logger.Error("failed to purge outbox events", "worker_id", o.workerID, "error", err)
	}
}

func (o *OutboxPublisher) Stop() {
	<-o.done
}

func extractTaskID(payload json.RawMessage) (string, error) {
	var data struct {
		TaskID string `json:"task_id"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return "", err
	}

	return data.TaskID, nil
}
