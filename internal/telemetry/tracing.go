package telemetry

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "taskforge"

// StartSpan opens a child span anchored on the current span in ctx, attaches
// the given attributes, and returns the new context and the span. The caller
// is responsible for ending the span — typically via defer EndSpanWithError.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

// EndSpanWithError ends a span and, if err is non-nil, records it and sets the
// span status to Error. Designed for `defer telemetry.EndSpanWithError(span, err)`
// patterns where err is a named return value.
func EndSpanWithError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}

// ExtractTraceparent serializes the W3C Trace Context from ctx into the
// "traceparent" header value. Returns nil when no active span exists in ctx,
// allowing callers to store NULL in DB columns rather than empty strings.
func ExtractTraceparent(ctx context.Context) *string {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	tp, ok := carrier["traceparent"]
	if !ok || tp == "" {
		return nil
	}
	return &tp
}
