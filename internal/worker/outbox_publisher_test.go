package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/thogio8/task-forge/internal/model"
)

type mockOutboxRepo struct {
	events    []model.OutboxEvent
	published []int64
	purged    bool
	mu        sync.Mutex
}

func (m *mockOutboxRepo) GetUnpublished(_ context.Context, _ int) ([]model.OutboxEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.events, nil
}

func (m *mockOutboxRepo) MarkPublished(_ context.Context, ids []int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, ids...)
	return nil
}

func (m *mockOutboxRepo) Purge(_ context.Context, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
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
	key   string
	value []byte
}

func (m *mockProducer) Publish(_ context.Context, key string, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.messages = append(m.messages, publishedMessage{key: key, value: value})
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
