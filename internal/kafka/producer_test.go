package kafka

import (
	"context"
	"fmt"
	"testing"
)

type messageRecord struct {
	key   string
	value []byte
}

func TestProducer_Publish(t *testing.T) {
	var messages []messageRecord

	producer := &Producer{
		writeFn: func(_ context.Context, key string, value []byte) error {
			messages = append(messages, messageRecord{key: key, value: value})
			return nil
		},
		closeFn: func() error { return nil },
	}

	err := producer.Publish(context.Background(), "task-123", []byte(`{"task_id":"task-123"}`))
	if err != nil {
		t.Fatal(err)
	}

	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}

	if messages[0].key != "task-123" {
		t.Errorf("expected key 'task-123', got '%s'", messages[0].key)
	}

	if string(messages[0].value) != `{"task_id":"task-123"}` {
		t.Errorf("expected value '{\"task_id\":\"task-123\"}', got '%s'", string(messages[0].value))
	}
}

func TestProducer_Publish_Error(t *testing.T) {
	producer := &Producer{
		writeFn: func(_ context.Context, _ string, _ []byte) error {
			return fmt.Errorf("kafka unavailable")
		},
		closeFn: func() error { return nil },
	}

	err := producer.Publish(context.Background(), "task-123", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error when kafka is unavailable")
	}

	if err.Error() != "kafka unavailable" {
		t.Errorf("expected 'kafka unavailable', got '%s'", err.Error())
	}
}

func TestProducer_Close(t *testing.T) {
	closed := false

	producer := &Producer{
		writeFn: func(_ context.Context, _ string, _ []byte) error { return nil },
		closeFn: func() error {
			closed = true
			return nil
		},
	}

	err := producer.Close()
	if err != nil {
		t.Fatal(err)
	}

	if !closed {
		t.Error("expected writer to be closed")
	}
}
