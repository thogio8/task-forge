package kafka

import (
	"context"
	"fmt"
	"testing"

	kafkago "github.com/segmentio/kafka-go"
)

type messageRecord struct {
	key     string
	value   []byte
	headers []kafkago.Header
}

func TestProducer_Publish(t *testing.T) {
	var messages []messageRecord

	producer := &Producer{
		writeFn: func(_ context.Context, key string, value []byte, headers []kafkago.Header) error {
			messages = append(messages, messageRecord{key: key, value: value, headers: headers})
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

func TestProducer_Publish_PassesHeaders(t *testing.T) {
	var captured []kafkago.Header

	producer := &Producer{
		writeFn: func(_ context.Context, _ string, _ []byte, headers []kafkago.Header) error {
			captured = headers
			return nil
		},
		closeFn: func() error { return nil },
	}

	err := producer.Publish(context.Background(), "k", []byte("v"),
		kafkago.Header{Key: "traceparent", Value: []byte("00-abc-def-01")},
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(captured) != 1 || captured[0].Key != "traceparent" {
		t.Errorf("expected traceparent header, got %+v", captured)
	}
}

func TestProducer_Publish_Error(t *testing.T) {
	producer := &Producer{
		writeFn: func(_ context.Context, _ string, _ []byte, _ []kafkago.Header) error {
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
		writeFn: func(_ context.Context, _ string, _ []byte, _ []kafkago.Header) error { return nil },
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
