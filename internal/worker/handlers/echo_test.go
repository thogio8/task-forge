package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestEcho_LogsPayloadAndSucceeds(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	fn := Echo(logger)
	payload := json.RawMessage(`{"hello":"world"}`)

	err := fn(context.Background(), payload)

	if err != nil {
		t.Fatalf("Echo returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "msg=echo") {
		t.Errorf("expected msg=echo in log output, got: %s", out)
	}
	if !strings.Contains(out, `hello`) || !strings.Contains(out, `world`) {
		t.Errorf("expected payload contents in log, got: %s", out)
	}
}

func TestEcho_WorksWithDiscardLogger(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fn := Echo(logger)
	if err := fn(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Errorf("Echo with discard logger returned error: %v", err)
	}
}
