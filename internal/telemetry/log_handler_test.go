package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// makeWrappedLogger returns a logger whose handler is the TracingHandler
// wrapping a JSONHandler that writes to the returned buffer. The buffer can
// be inspected to assert on the emitted record's structure.
func makeWrappedLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	wrapped := WrapWithTracing(slog.NewJSONHandler(&buf, nil))
	return slog.New(wrapped), &buf
}

func decodeRecord(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal log record: %v\nraw: %s", err, buf.String())
	}
	return record
}

func TestTracingHandler_AddsTraceIDsWithActiveSpan(t *testing.T) {
	installTestTracing(t)

	logger, buf := makeWrappedLogger()

	ctx, span := StartSpan(context.Background(), "logging.test")
	logger.InfoContext(ctx, "hello with span")
	span.End()

	record := decodeRecord(t, buf)
	traceID, ok := record["trace_id"].(string)
	if !ok || traceID == "" {
		t.Errorf("expected non-empty trace_id, got: %v", record)
	}
	if len(traceID) != 32 {
		t.Errorf("trace_id length = %d, want 32 hex chars", len(traceID))
	}
	spanID, ok := record["span_id"].(string)
	if !ok || spanID == "" {
		t.Errorf("expected non-empty span_id, got: %v", record)
	}
	if len(spanID) != 16 {
		t.Errorf("span_id length = %d, want 16 hex chars", len(spanID))
	}
}

func TestTracingHandler_NoTraceFieldsWithoutActiveSpan(t *testing.T) {
	installTestTracing(t)

	logger, buf := makeWrappedLogger()
	logger.InfoContext(context.Background(), "no span")

	record := decodeRecord(t, buf)
	if _, ok := record["trace_id"]; ok {
		t.Errorf("expected no trace_id field on background ctx, got: %v", record["trace_id"])
	}
	if _, ok := record["span_id"]; ok {
		t.Errorf("expected no span_id field on background ctx, got: %v", record["span_id"])
	}
}

func TestTracingHandler_NonContextCallsDelegateUnchanged(t *testing.T) {
	installTestTracing(t)

	logger, buf := makeWrappedLogger()

	// Even with an active span context, non-*Context calls don't pass ctx,
	// so the wrapper has nothing to extract — graceful no-op.
	_, span := StartSpan(context.Background(), "ignored.test")
	defer span.End()

	logger.Info("classical Info call without ctx")

	record := decodeRecord(t, buf)
	if _, ok := record["trace_id"]; ok {
		t.Errorf("Info() (no ctx) should not enrich; got trace_id=%v", record["trace_id"])
	}
}

func TestTracingHandler_PreservesUserAttributes(t *testing.T) {
	installTestTracing(t)

	logger, buf := makeWrappedLogger()

	ctx, span := StartSpan(context.Background(), "attrs.test")
	logger.InfoContext(ctx, "user attrs preserved",
		slog.String("task_id", "abc-123"),
		slog.Int("attempt", 2),
	)
	span.End()

	record := decodeRecord(t, buf)
	if got := record["task_id"]; got != "abc-123" {
		t.Errorf("task_id = %v, want %q", got, "abc-123")
	}
	// JSON unmarshal turns ints into float64.
	if got := record["attempt"]; got != float64(2) {
		t.Errorf("attempt = %v, want 2", got)
	}
	if _, ok := record["trace_id"]; !ok {
		t.Error("expected trace_id alongside user attributes")
	}
}

func TestTracingHandler_EnabledDelegates(t *testing.T) {
	innerOpts := &slog.HandlerOptions{Level: slog.LevelWarn}
	wrapped := WrapWithTracing(slog.NewJSONHandler(&bytes.Buffer{}, innerOpts))

	if wrapped.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("Enabled(Debug) should be false when inner level is Warn")
	}
	if !wrapped.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled(Error) should be true when inner level is Warn")
	}
}

func TestTracingHandler_WithAttrsKeepsTracingWrapper(t *testing.T) {
	installTestTracing(t)

	var buf bytes.Buffer
	base := WrapWithTracing(slog.NewJSONHandler(&buf, nil))
	logger := slog.New(base.WithAttrs([]slog.Attr{slog.String("worker_id", "w1")}))

	ctx, span := StartSpan(context.Background(), "withattrs.test")
	logger.InfoContext(ctx, "after WithAttrs")
	span.End()

	record := decodeRecord(t, &buf)
	if got := record["worker_id"]; got != "w1" {
		t.Errorf("worker_id = %v, want w1", got)
	}
	if _, ok := record["trace_id"]; !ok {
		t.Error("WithAttrs result should still inject trace_id (wrapper is preserved)")
	}
}

func TestTracingHandler_WithGroupKeepsTracingWrapper(t *testing.T) {
	installTestTracing(t)

	var buf bytes.Buffer
	base := WrapWithTracing(slog.NewJSONHandler(&buf, nil))
	logger := slog.New(base.WithGroup("g"))

	ctx, span := StartSpan(context.Background(), "withgroup.test")
	logger.InfoContext(ctx, "in group", slog.String("k", "v"))
	span.End()

	out := buf.String()
	// Per slog semantics, attributes added after WithGroup are nested inside
	// the group — including the trace_id/span_id that the wrapper injects on
	// each Handle. What we check here is that the wrapper is still in the
	// chain (trace_id appears anywhere) AND that the user's group nesting is
	// preserved.
	if !strings.Contains(out, `"g":{`) {
		t.Errorf("expected group nesting 'g' in output, got: %s", out)
	}
	if !strings.Contains(out, `"k":"v"`) {
		t.Errorf("expected user attribute k=v, got: %s", out)
	}
	if !strings.Contains(out, `"trace_id":"`) {
		t.Errorf("WithGroup result should still inject trace_id (wrapper preserved), got: %s", out)
	}
}
