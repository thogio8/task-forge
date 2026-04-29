package telemetry

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// installTestTracing installs an in-memory TracerProvider with a SpanRecorder
// plus the global TextMapPropagator (TraceContext) for the duration of the
// test, restoring previous globals afterwards. Returned recorder lets tests
// inspect span names, kinds, attributes and statuses.
func installTestTracing(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	prevTP := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
		_ = tp.Shutdown(context.Background())
	})

	return recorder
}

func TestStartSpan_OpensInternalSpanWithAttrs(t *testing.T) {
	rec := installTestTracing(t)

	_, span := StartSpan(context.Background(), "test.op",
		attribute.String("custom.key", "v"),
	)
	span.End()

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name() != "test.op" {
		t.Errorf("name = %q, want %q", spans[0].Name(), "test.op")
	}
	if got := spans[0].SpanKind(); got != trace.SpanKindInternal {
		t.Errorf("kind = %v, want Internal", got)
	}
	if !hasSpanAttr(spans[0], "custom.key", "v") {
		t.Errorf("expected attribute custom.key=v, got %v", spans[0].Attributes())
	}
}

func TestStartSpanKind_AppliesExplicitKind(t *testing.T) {
	rec := installTestTracing(t)

	tests := []struct {
		name string
		kind trace.SpanKind
	}{
		{"server", trace.SpanKindServer},
		{"client", trace.SpanKindClient},
		{"producer", trace.SpanKindProducer},
		{"consumer", trace.SpanKindConsumer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, span := StartSpanKind(context.Background(), "kind."+tt.name, tt.kind)
			span.End()
		})
	}

	spans := rec.Ended()
	if len(spans) != len(tests) {
		t.Fatalf("expected %d spans, got %d", len(tests), len(spans))
	}
	for i, tt := range tests {
		if got := spans[i].SpanKind(); got != tt.kind {
			t.Errorf("span %d kind = %v, want %v", i, got, tt.kind)
		}
	}
}

func TestEndSpanWithError_RecordsErrorAndSetsErrorStatus(t *testing.T) {
	rec := installTestTracing(t)

	_, span := StartSpan(context.Background(), "fails")
	err := errors.New("boom")
	EndSpanWithError(span, &err)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].Status().Code; got != codes.Error {
		t.Errorf("status code = %v, want Error", got)
	}
	if spans[0].Status().Description != "boom" {
		t.Errorf("status description = %q, want %q", spans[0].Status().Description, "boom")
	}
	if len(spans[0].Events()) == 0 {
		t.Error("expected RecordError to add an event")
	}
}

func TestEndSpanWithError_NilErrorLeavesUnsetStatus(t *testing.T) {
	rec := installTestTracing(t)

	_, span := StartSpan(context.Background(), "ok")
	var err error
	EndSpanWithError(span, &err)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].Status().Code; got != codes.Unset {
		t.Errorf("status code = %v, want Unset on nil error", got)
	}
}

func TestEndSpanWithError_NilPointerSafe(t *testing.T) {
	rec := installTestTracing(t)

	_, span := StartSpan(context.Background(), "no-pointer")
	EndSpanWithError(span, nil)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got := spans[0].Status().Code; got != codes.Unset {
		t.Errorf("nil errPtr should not set Error status, got %v", got)
	}
}

func TestSpanError_RecordsAndSetsStatus(t *testing.T) {
	rec := installTestTracing(t)

	_, span := StartSpan(context.Background(), "explicit-error")
	SpanError(span, errors.New("kaboom"))
	span.End()

	spans := rec.Ended()
	if got := spans[0].Status().Code; got != codes.Error {
		t.Errorf("status code = %v, want Error", got)
	}
	if spans[0].Status().Description != "kaboom" {
		t.Errorf("status description = %q, want %q", spans[0].Status().Description, "kaboom")
	}
}

func TestSpanError_NilIsNoop(t *testing.T) {
	rec := installTestTracing(t)

	_, span := StartSpan(context.Background(), "nil-noop")
	SpanError(span, nil)
	span.End()

	spans := rec.Ended()
	if got := spans[0].Status().Code; got != codes.Unset {
		t.Errorf("status code = %v, want Unset (no-op)", got)
	}
	if len(spans[0].Events()) != 0 {
		t.Errorf("expected 0 events on nil error, got %d", len(spans[0].Events()))
	}
}

func TestExtractTraceparent_WithActiveSpan(t *testing.T) {
	installTestTracing(t)

	ctx, span := StartSpan(context.Background(), "active")
	defer span.End()

	tp := ExtractTraceparent(ctx)
	if tp == nil {
		t.Fatal("expected non-nil traceparent with active span")
	}
	if len(*tp) != 55 {
		t.Errorf("traceparent length = %d, want 55 (W3C format)", len(*tp))
	}
	if (*tp)[:3] != "00-" {
		t.Errorf("traceparent should start with version '00-', got %q", *tp)
	}
}

func TestExtractTraceparent_NoActiveSpanReturnsNil(t *testing.T) {
	installTestTracing(t)

	tp := ExtractTraceparent(context.Background())
	if tp != nil {
		t.Errorf("expected nil traceparent without span, got %q", *tp)
	}
}

func TestContextFromTraceparent_EmptyReturnsParentUnchanged(t *testing.T) {
	installTestTracing(t)

	parent := context.Background()
	got := ContextFromTraceparent(parent, "")
	if got != parent {
		t.Error("empty traceparent should return parent ctx unchanged")
	}
}

func TestContextFromTraceparent_ValidExtractsSpanContext(t *testing.T) {
	installTestTracing(t)

	const traceID = "0123456789abcdef0123456789abcdef"
	const spanID = "0123456789abcdef"
	traceparent := "00-" + traceID + "-" + spanID + "-01"

	ctx := ContextFromTraceparent(context.Background(), traceparent)
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		t.Fatal("expected valid SpanContext after extract")
	}
	if got := sc.TraceID().String(); got != traceID {
		t.Errorf("trace_id = %q, want %q", got, traceID)
	}
	if got := sc.SpanID().String(); got != spanID {
		t.Errorf("span_id = %q, want %q", got, spanID)
	}
	if !sc.IsRemote() {
		t.Error("extracted SpanContext should be flagged as remote")
	}
}

func TestContextFromTraceparent_MalformedReturnsParent(t *testing.T) {
	installTestTracing(t)

	parent := context.Background()
	got := ContextFromTraceparent(parent, "not-a-traceparent")
	sc := trace.SpanContextFromContext(got)
	if sc.IsValid() {
		t.Error("malformed traceparent must not yield a valid SpanContext")
	}
}

// hasSpanAttr returns true if the span carries an attribute key with the given
// string value. Local helper because tracetest doesn't expose a finder.
func hasSpanAttr(span sdktrace.ReadOnlySpan, key, value string) bool {
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key && kv.Value.AsString() == value {
			return true
		}
	}
	return false
}
