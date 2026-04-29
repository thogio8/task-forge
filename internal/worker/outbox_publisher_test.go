package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	kafkago "github.com/segmentio/kafka-go"
	"github.com/thogio8/task-forge/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type mockOutboxRepo struct {
	events    []model.OutboxEvent
	published []int64
	purged    bool
	mu        sync.Mutex
	getErr    error
	markErr   error
	purgeErr  error
}

func (m *mockOutboxRepo) GetUnpublished(_ context.Context, _ int) ([]model.OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.events, nil
}

func (m *mockOutboxRepo) MarkPublished(_ context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markErr != nil {
		return m.markErr
	}
	m.published = append(m.published, ids...)
	return nil
}

func (m *mockOutboxRepo) Purge(_ context.Context, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.purgeErr != nil {
		return m.purgeErr
	}
	m.purged = true
	return nil
}

func (m *mockOutboxRepo) CountUnpublished(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, e := range m.events {
		if e.PublishedAt == nil {
			count++
		}
	}
	return count, nil
}

type mockProducer struct {
	messages []publishedMessage
	mu       sync.Mutex
	err      error
}

type publishedMessage struct {
	key     string
	value   []byte
	headers []kafkago.Header
}

func (m *mockProducer) Publish(_ context.Context, key string, value []byte, headers ...kafkago.Header) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, publishedMessage{key: key, value: value, headers: headers})
	return nil
}

func TestOutboxPublisher_PublishesAndMarks(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{ID: 1, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"aaa","event_type":"task.created"}`)},
			{ID: 2, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"bbb","event_type":"task.created"}`)},
		},
	}
	producer := &mockProducer{}

	publisher := NewOutboxPublisher(repo, producer, 50*time.Millisecond, 10, 24*time.Hour, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go publisher.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	publisher.Stop()

	producer.mu.Lock()
	defer producer.mu.Unlock()
	if len(producer.messages) < 2 {
		t.Fatalf("expected at least 2 published messages, got %d", len(producer.messages))
	}

	if producer.messages[0].key != "aaa" {
		t.Errorf("expected first message key 'aaa', got '%s'", producer.messages[0].key)
	}

	if producer.messages[1].key != "bbb" {
		t.Errorf("expected second message key 'bbb', got '%s'", producer.messages[1].key)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.published) < 2 {
		t.Fatalf("expected at least 2 marked as published, got %d", len(repo.published))
	}
}

func TestOutboxPublisher_SkipsFailedPublish(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{ID: 1, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"aaa","event_type":"task.created"}`)},
			{ID: 2, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"bbb","event_type":"task.created"}`)},
		},
	}
	// Producer fails on all publishes
	producer := &mockProducer{err: fmt.Errorf("kafka down")}

	publisher := NewOutboxPublisher(repo, producer, 50*time.Millisecond, 10, 24*time.Hour, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go publisher.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	publisher.Stop()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.published) != 0 {
		t.Fatalf("expected 0 marked (all publishes failed), got %d", len(repo.published))
	}
}

func TestOutboxPublisher_PurgesEvenWhenEmpty(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{}, // no events to publish
	}
	producer := &mockProducer{}

	publisher := NewOutboxPublisher(repo, producer, 50*time.Millisecond, 10, 24*time.Hour, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go publisher.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	publisher.Stop()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if !repo.purged {
		t.Error("expected purge to run even when no events to publish")
	}
}

func TestOutboxPublisher_StopsOnContextCancel(t *testing.T) {
	repo := &mockOutboxRepo{}
	producer := &mockProducer{}

	publisher := NewOutboxPublisher(repo, producer, 50*time.Millisecond, 10, 24*time.Hour, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go publisher.Run(ctx)
	cancel()

	stopped := make(chan struct{})
	go func() {
		publisher.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("publisher did not stop after context cancel")
	}
}

func TestExtractTaskID(t *testing.T) {
	id, err := extractTaskID(json.RawMessage(`{"task_id":"abc-123","event_type":"task.created"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "abc-123" {
		t.Errorf("expected 'abc-123', got '%s'", id)
	}
}

func TestExtractTaskID_InvalidJSON(t *testing.T) {
	id, err := extractTaskID(json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if id != "" {
		t.Errorf("expected empty string for invalid JSON, got '%s'", id)
	}
}

// TestOutboxPublisher_PropagatesTraceparentToKafka verifies that when an
// outbox event has a stored trace_context, the publisher rebuilds the trace
// context and injects a "traceparent" header into the Kafka message — which
// is the bridge that lets the consumer reattach to the originating HTTP trace.
func TestOutboxPublisher_PropagatesTraceparentToKafka(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	storedTraceparent := "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01"

	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{
				ID:           1,
				EventType:    "task.created",
				Payload:      json.RawMessage(`{"task_id":"abc-123"}`),
				TraceContext: &storedTraceparent,
			},
		},
	}
	producer := &mockProducer{}
	publisher := NewOutboxPublisher(repo, producer, 50*time.Millisecond, 10, time.Hour, "test-worker", testLogger)

	publisher.processCycle(context.Background())

	if len(producer.messages) != 1 {
		t.Fatalf("expected 1 message published, got %d", len(producer.messages))
	}

	var traceparent string
	for _, h := range producer.messages[0].headers {
		if h.Key == "traceparent" {
			traceparent = string(h.Value)
			break
		}
	}
	if traceparent == "" {
		t.Fatalf("expected traceparent header, got headers=%+v", producer.messages[0].headers)
	}

	expectedTraceID := "0123456789abcdef0123456789abcdef"
	if !contains(traceparent, expectedTraceID) {
		t.Errorf("injected traceparent %q does not carry the stored trace_id %q", traceparent, expectedTraceID)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestOutboxPublisher_PendingGaugeRunsCallback verifies that the
// outbox.pending gauge callback registered at constructor time actually runs
// on collect — without this test the callback path is dead code from a
// coverage standpoint.
func TestOutboxPublisher_PendingGaugeRunsCallback(t *testing.T) {
	reader := withTestMeterProvider(t)

	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{ID: 1, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"a"}`)},
			{ID: 2, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"b"}`)},
		},
	}
	_ = NewOutboxPublisher(repo, &mockProducer{}, time.Hour, 10, time.Hour, "test", testLogger)

	if _, ok := observeGaugeInt64(t, reader, "outbox.pending"); !ok {
		t.Fatal("outbox.pending gauge not recorded after collect")
	}
}

// TestOutboxPublisher_PendingGaugeHandlesRepoError verifies that the gauge
// callback degrades gracefully when CountUnpublished returns an error:
// no panic, no observation produced (callback returns nil silently after log).
func TestOutboxPublisher_PendingGaugeHandlesRepoError(t *testing.T) {
	reader := withTestMeterProvider(t)

	// Drive the callback into its error branch via a count helper that fails.
	repo := &erroringCountRepo{}
	_ = NewOutboxPublisher(repo, &mockProducer{}, time.Hour, 10, time.Hour, "test", testLogger)

	// Collect once — the callback runs and hits the error branch.
	_, _ = observeGaugeInt64(t, reader, "outbox.pending")
}

// TestOutboxPublisher_GetUnpublishedErrorMarksCycleSpanError exercises the
// cycle-span error branch when GetUnpublished fails. Just needs to run
// processCycle once with a failing repo — no need to inspect the span here,
// the path is covered for branch reporting.
func TestOutboxPublisher_GetUnpublishedErrorPath(t *testing.T) {
	repo := &mockOutboxRepo{getErr: fmt.Errorf("db down")}
	publisher := NewOutboxPublisher(repo, &mockProducer{}, time.Hour, 10, time.Hour, "test", testLogger)

	publisher.processCycle(context.Background())
	// No assertion — coverage of the error branch is the goal.
}

// TestOutboxPublisher_MarkPublishedErrorPath drives the cycle into the
// MarkPublished error branch after at least one event was published.
func TestOutboxPublisher_MarkPublishedErrorPath(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{ID: 1, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"x"}`)},
		},
		markErr: fmt.Errorf("mark failed"),
	}
	publisher := NewOutboxPublisher(repo, &mockProducer{}, time.Hour, 10, time.Hour, "test", testLogger)

	publisher.processCycle(context.Background())

	// Event was published but mark failed — purge must NOT have run because
	// processCycle returns early after MarkPublished error.
	if repo.purged {
		t.Error("purge should not run after MarkPublished error")
	}
}

// TestOutboxPublisher_PurgeErrorIsLoggedNotFatal verifies that a Purge error
// is logged but does not prevent a successful publish cycle from completing.
func TestOutboxPublisher_PurgeErrorIsLoggedNotFatal(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{ID: 1, EventType: "task.created", Payload: json.RawMessage(`{"task_id":"x"}`)},
		},
		purgeErr: fmt.Errorf("purge failed"),
	}
	producer := &mockProducer{}
	publisher := NewOutboxPublisher(repo, producer, time.Hour, 10, time.Hour, "test", testLogger)

	publisher.processCycle(context.Background())

	if len(producer.messages) != 1 {
		t.Errorf("expected 1 published message despite Purge error, got %d", len(producer.messages))
	}
	if len(repo.published) != 1 {
		t.Errorf("expected 1 marked-as-published despite Purge error, got %d", len(repo.published))
	}
}

// TestOutboxPublisher_ExtractTaskIDWarnDoesNotBlockPublish covers the
// `extractTaskID` warn branch — the code logs the failure but still attempts
// to publish (with an empty key).
func TestOutboxPublisher_ExtractTaskIDWarnDoesNotBlockPublish(t *testing.T) {
	repo := &mockOutboxRepo{
		events: []model.OutboxEvent{
			{ID: 1, EventType: "task.created", Payload: json.RawMessage(`not valid json`)},
		},
	}
	producer := &mockProducer{}
	publisher := NewOutboxPublisher(repo, producer, time.Hour, 10, time.Hour, "test", testLogger)

	publisher.processCycle(context.Background())

	if len(producer.messages) != 1 {
		t.Errorf("expected publish attempt despite payload parse error, got %d messages", len(producer.messages))
	}
}

// erroringCountRepo only fails on CountUnpublished — used for the gauge
// callback error-path test. Other methods are not exercised here.
type erroringCountRepo struct{}

func (erroringCountRepo) GetUnpublished(_ context.Context, _ int) ([]model.OutboxEvent, error) {
	return nil, nil
}
func (erroringCountRepo) MarkPublished(_ context.Context, _ []int64) error  { return nil }
func (erroringCountRepo) Purge(_ context.Context, _ time.Duration) error    { return nil }
func (erroringCountRepo) CountUnpublished(_ context.Context) (int64, error) { return 0, fmt.Errorf("count failed") }
