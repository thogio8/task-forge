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

// EndSpanWithError ends a span and, if *errPtr is non-nil, records it and
// sets the span status to Error. Takes a pointer to capture the final value
// of a named return value via defer:
//
//	defer telemetry.EndSpanWithError(span, &err)
//
// Passing the address (not the value) is required because Go evaluates defer
// arguments at registration time; the pointer lets the helper read the latest
// error when the function actually returns.
func EndSpanWithError(span trace.Span, errPtr *error) {
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
		span.SetStatus(codes.Error, (*errPtr).Error())
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
