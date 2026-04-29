package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/telemetry"
	kafkago "github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type OutboxPublisherRepository interface {
	GetUnpublished(ctx context.Context, limit int) ([]model.OutboxEvent, error)
	MarkPublished(ctx context.Context, ids []int64) error
	Purge(ctx context.Context, retention time.Duration) error
	CountUnpublished(ctx context.Context) (int64, error)
}

type OutboxProducer interface {
	Publish(ctx context.Context, key string, value []byte, headers ...kafkago.Header) error
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
				logger.WarnContext(ctx, "failed to observe outbox.pending", "error", err)
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
	o.logger.InfoContext(ctx, "outbox publisher started", "worker_id", o.workerID)

	ticker := time.NewTicker(o.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			o.processCycle(ctx)
		case <-ctx.Done():
			close(o.done)
			o.logger.InfoContext(ctx, "outbox publisher stopped", "worker_id", o.workerID)
			return
		}
	}
}

func (o *OutboxPublisher) processCycle(ctx context.Context) {
	cycleCtx, cycleSpan := telemetry.StartSpan(ctx, "outbox.publish.cycle",
		attribute.String("worker.id", o.workerID),
	)
	defer cycleSpan.End()

	outboxEvents, err := o.repo.GetUnpublished(cycleCtx, o.batchSize)

	if err != nil {
		telemetry.SpanError(cycleSpan, err)
		o.logger.ErrorContext(cycleCtx, "failed to retrieve unpublished outbox events", "worker_id", o.workerID, "error", err)
		return
	}

	cycleSpan.SetAttributes(attribute.Int("batch.size", len(outboxEvents)))

	var outboxEventsIDs []int64

	for _, outboxEvent := range outboxEvents {
		o.publishEvent(cycleCtx, outboxEvent, &outboxEventsIDs)
	}

	if len(outboxEventsIDs) > 0 {
		if err := o.repo.MarkPublished(cycleCtx, outboxEventsIDs); err != nil {
			telemetry.SpanError(cycleSpan, err)
			o.logger.ErrorContext(cycleCtx, "failed to mark outbox events as published", "worker_id", o.workerID, "error", err)
			return
		}
	}

	if err := o.repo.Purge(cycleCtx, o.retention); err != nil {
		o.logger.ErrorContext(cycleCtx, "failed to purge outbox events", "worker_id", o.workerID, "error", err)
	}
}

// publishEvent publishes a single outbox event to Kafka, propagating the
// trace context stored at INSERT time so that the trace continues from the
// originating HTTP request through Kafka into the consumer.
func (o *OutboxPublisher) publishEvent(rootCtx context.Context, event model.OutboxEvent, publishedIDs *[]int64) {
	publishCtx := rootCtx
	if event.TraceContext != nil {
		publishCtx = telemetry.ContextFromTraceparent(rootCtx, *event.TraceContext)
	}

	publishCtx, span := telemetry.StartSpanKind(publishCtx, "messaging.publish", trace.SpanKindProducer,
		attribute.String("messaging.system", "kafka"),
		attribute.String("messaging.destination.name", "tasks"),
		attribute.String("messaging.operation", "publish"),
		attribute.Int64("outbox.event.id", event.ID),
	)
	defer span.End()

	var headers telemetry.KafkaHeaderCarrier
	otel.GetTextMapPropagator().Inject(publishCtx, &headers)

	taskID, extractErr := extractTaskID(event.Payload)
	if extractErr != nil {
		o.logger.WarnContext(publishCtx, "failed to extract task_id from outbox payload", "worker_id", o.workerID, "outbox_event_id", event.ID, "error", extractErr)
	}

	start := time.Now()
	err := o.producer.Publish(publishCtx, taskID, event.Payload, headers...)
	o.publishDuration.Record(publishCtx, time.Since(start).Seconds())

	if err != nil {
		telemetry.SpanError(span, err)
		o.logger.ErrorContext(publishCtx, "failed to publish outbox event", "worker_id", o.workerID, "outbox_event_id", event.ID, "error", err)
		o.errorsCounter.Add(publishCtx, 1)
		return
	}

	o.publishedCounter.Add(publishCtx, 1)
	*publishedIDs = append(*publishedIDs, event.ID)
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
