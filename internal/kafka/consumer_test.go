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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

type mockMessageReader struct {
	messages  []kafkago.Message
	index     int
	committed []kafkago.Message
	closed    bool
	closeErr  error
	fetchErr  error
}

func (m *mockMessageReader) FetchMessage(ctx context.Context) (kafkago.Message, error) {
	if m.fetchErr != nil {
		// One-shot fetch error: emit it, then clear so subsequent calls
		// don't keep returning the same error (lets the consumer loop progress).
		err := m.fetchErr
		m.fetchErr = nil
		return kafkago.Message{}, err
	}
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
	return m.closeErr
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

// TestConsumer_StopLogsCloseError covers the close-error branch in Stop().
// We can't observe the log directly without a fixture, but exercising the
// branch ensures no panic and that the reader was closed.
func TestConsumer_StopLogsCloseError(t *testing.T) {
	reader := &mockMessageReader{closeErr: fmt.Errorf("close failed")}
	claimer := &mockTaskClaimer{}
	ch := make(chan model.TaskEnvelope, 1)
	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(80 * time.Millisecond)
	cancel()
	consumer.Stop()

	if !reader.closed {
		t.Error("reader.Close should still have been called even when it returns an error")
	}
}

// TestConsumer_RunLogsFetchErrorAndContinues exercises the path where
// FetchMessage returns an error WHILE ctx is still active — the loop must
// log and continue (not exit), which is the resilience we need against
// transient broker hiccups.
func TestConsumer_RunLogsFetchErrorAndContinues(t *testing.T) {
	reader := &mockMessageReader{fetchErr: fmt.Errorf("transient broker error")}
	claimer := &mockTaskClaimer{}
	ch := make(chan model.TaskEnvelope, 10)
	consumer := NewConsumerWithReader(reader, claimer, ch, "test-worker", testLogger)

	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	time.Sleep(60 * time.Millisecond)
	cancel()
	consumer.Stop()

	// Reader stayed alive past the error and was closed cleanly on cancel.
	if !reader.closed {
		t.Error("expected reader to close after ctx cancel post fetch error")
	}
}

// TestConsumer_NewConsumerSmokeReturnsNonNil is a minimal construction smoke
// test for the public NewConsumer constructor. It does not connect to Kafka
// (the underlying reader connects lazily) but it ensures the wiring code
// runs without panic and returns an initialized Consumer.
func TestConsumer_NewConsumerSmokeReturnsNonNil(t *testing.T) {
	ch := make(chan model.TaskEnvelope, 1)
	c := NewConsumer([]string{"localhost:9999"}, "tasks", "test-group", &mockTaskClaimer{}, ch, "smoke-worker", testLogger)

	if c == nil {
		t.Fatal("NewConsumer returned nil")
	}
	if c.workerID != "smoke-worker" {
		t.Errorf("workerID = %q, want %q", c.workerID, "smoke-worker")
	}
	// Close the underlying reader to release the kafkago goroutines started
	// at construction time, otherwise they keep dialing in the background.
	_ = c.reader.Close()
}

// TestConsumer_ExtractsTraceparentFromHeaders verifies that the consumer
// reads the W3C "traceparent" Kafka header and uses it as the parent of the
// messaging.receive span — without this, the trace cascade breaks at the
// Kafka boundary and the worker side has its own orphan trace.
func TestConsumer_ExtractsTraceparentFromHeaders(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})

	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	const incomingTraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const incomingSpanID = "bbbbbbbbbbbbbbbb"
	incomingTraceparent := "00-" + incomingTraceID + "-" + incomingSpanID + "-01"

	taskID := uuid.New()
	payload, _ := json.Marshal(map[string]string{"task_id": taskID.String()})

	reader := &mockMessageReader{
		messages: []kafkago.Message{
			{
				Key:   []byte(taskID.String()),
				Value: payload,
				Headers: []kafkago.Header{
					{Key: "traceparent", Value: []byte(incomingTraceparent)},
				},
			},
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

	spans := recorder.Ended()
	if len(spans) == 0 {
		t.Fatal("expected at least 1 span (messaging.receive), got 0")
	}

	var receiveSpan = spans[0]
	if receiveSpan.Name() != "messaging.receive" {
		t.Errorf("first span name = %q, want %q", receiveSpan.Name(), "messaging.receive")
	}
	if got := receiveSpan.SpanContext().TraceID().String(); got != incomingTraceID {
		t.Errorf("messaging.receive trace_id = %q, want %q (must inherit from header)", got, incomingTraceID)
	}
	if got := receiveSpan.Parent().SpanID().String(); got != incomingSpanID {
		t.Errorf("messaging.receive parent span_id = %q, want %q (must be the publisher's span)", got, incomingSpanID)
	}
}
