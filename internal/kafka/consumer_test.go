package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/thogio8/task-forge/internal/model"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

type mockMessageReader struct {
	messages  []kafkago.Message
	index     int
	committed []kafkago.Message
	closed    bool
}

func (m *mockMessageReader) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	for m.index >= len(m.messages) {
		select {
		case <-ctx.Done():
			return kafkago.Message{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	msg := m.messages[m.index]
	m.index++
	return msg, nil
}

func (m *mockMessageReader) CommitMessages(_ context.Context, msgs ...kafkago.Message) error {
	m.committed = append(m.committed, msgs...)
	return nil
}

func (m *mockMessageReader) Close() error {
	m.closed = true
	return nil
}

type mockTaskClaimer struct {
	claimedIDs []uuid.UUID
	returnTask *model.Task
	err        error
}

func (m *mockTaskClaimer) ClaimTask(_ context.Context, _ string, taskID uuid.UUID) (*model.Task, error) {
	m.claimedIDs = append(m.claimedIDs, taskID)
	return m.returnTask, m.err
}

func TestConsumer_ClaimsAndDispatchesTask(t *testing.T) {
	taskID := uuid.New()
	payload, _ := json.Marshal(map[string]string{"task_id": taskID.String()})

	reader := &mockMessageReader{
		messages: []kafkago.Message{
			{Key: []byte(taskID.String()), Value: payload},
		},
	}

	task := &model.Task{ID: taskID, Status: "running", Payload: json.RawMessage(`{"type":"echo"}`)}
	claimer := &mockTaskClaimer{returnTask: task}
	ch := make(chan model.TaskEnvelope, 10)

	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	consumer.Stop()

	if len(claimer.claimedIDs) != 1 {
		t.Fatalf("expected 1 claimed task, got %d", len(claimer.claimedIDs))
	}

	if claimer.claimedIDs[0] != taskID {
		t.Errorf("expected claimed ID %s, got %s", taskID, claimer.claimedIDs[0])
	}

	if len(ch) != 1 {
		t.Fatalf("expected 1 task in channel, got %d", len(ch))
	}

	dispatched := <-ch
	if dispatched.Task.ID != taskID {
		t.Errorf("expected dispatched task ID %s, got %s", taskID, dispatched.Task.ID)
	}

	if len(reader.committed) != 1 {
		t.Fatalf("expected 1 committed message, got %d", len(reader.committed))
	}
}

func TestConsumer_SkipsAlreadyClaimedTask(t *testing.T) {
	taskID := uuid.New()
	payload, _ := json.Marshal(map[string]string{"task_id": taskID.String()})

	reader := &mockMessageReader{
		messages: []kafkago.Message{
			{Key: []byte(taskID.String()), Value: payload},
		},
	}

	claimer := &mockTaskClaimer{returnTask: nil} // nil = already claimed
	ch := make(chan model.TaskEnvelope, 10)

	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	consumer.Stop()

	if len(ch) != 0 {
		t.Fatalf("expected 0 tasks (already claimed), got %d", len(ch))
	}

	if len(reader.committed) != 1 {
		t.Fatalf("expected 1 committed message (skipped task still committed), got %d", len(reader.committed))
	}
}

func TestConsumer_DoesNotCommitOnDBError(t *testing.T) {
	taskID := uuid.New()
	payload, _ := json.Marshal(map[string]string{"task_id": taskID.String()})

	reader := &mockMessageReader{
		messages: []kafkago.Message{
			{Key: []byte(taskID.String()), Value: payload},
		},
	}

	claimer := &mockTaskClaimer{err: fmt.Errorf("db unavailable")}
	ch := make(chan model.TaskEnvelope, 10)

	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	consumer.Stop()

	if len(ch) != 0 {
		t.Fatalf("expected 0 tasks (DB error), got %d", len(ch))
	}

	if len(reader.committed) != 0 {
		t.Fatalf("expected 0 committed messages (DB error should not commit), got %d", len(reader.committed))
	}
}

func TestConsumer_CommitsInvalidJSON(t *testing.T) {
	reader := &mockMessageReader{
		messages: []kafkago.Message{
			{Key: []byte("bad"), Value: []byte("not json")},
		},
	}

	claimer := &mockTaskClaimer{}
	ch := make(chan model.TaskEnvelope, 10)

	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	consumer.Stop()

	if len(ch) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(ch))
	}

	if len(claimer.claimedIDs) != 0 {
		t.Fatalf("expected 0 claim attempts for invalid JSON, got %d", len(claimer.claimedIDs))
	}

	if len(reader.committed) != 1 {
		t.Fatalf("expected 1 committed message (invalid JSON still committed), got %d", len(reader.committed))
	}
}

func TestConsumer_CommitsInvalidUUID(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"task_id": "not-a-uuid"})

	reader := &mockMessageReader{
		messages: []kafkago.Message{
			{Key: []byte("bad"), Value: payload},
		},
	}

	claimer := &mockTaskClaimer{}
	ch := make(chan model.TaskEnvelope, 10)

	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(200 * time.Millisecond)
	cancel()
	consumer.Stop()

	if len(ch) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(ch))
	}

	if len(claimer.claimedIDs) != 0 {
		t.Fatalf("expected 0 claim attempts for invalid UUID, got %d", len(claimer.claimedIDs))
	}

	if len(reader.committed) != 1 {
		t.Fatalf("expected 1 committed message (invalid UUID still committed), got %d", len(reader.committed))
	}
}

func TestConsumer_StopsOnContextCancel(t *testing.T) {
	reader := &mockMessageReader{} // no messages, FetchMessage will block
	claimer := &mockTaskClaimer{}
	ch := make(chan model.TaskEnvelope, 10)

	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	cancel()

	stopped := make(chan struct{})
	go func() {
		consumer.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("consumer did not stop after context cancel")
	}

	if !reader.closed {
		t.Error("expected reader to be closed after stop")
	}
}
