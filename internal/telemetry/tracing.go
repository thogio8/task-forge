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

// StartSpan opens an Internal-kind child span anchored on the current span in
// ctx, attaches the given attributes, and returns the new context and span.
// The caller is responsible for ending the span — typically via defer
// EndSpanWithError. Use StartSpanKind for spans that should expose a specific
// SpanKind (Server/Client/Producer/Consumer) for the OTel service-graph.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartSpanKind opens a child span with an explicit SpanKind. Convention:
// Server for HTTP server handlers, Client for outbound DB/HTTP calls, Producer
// for messaging publish, Consumer for messaging receive. Tempo/Grafana use
// SpanKind to derive service graphs and infer client/server boundaries.
func StartSpanKind(ctx context.Context, name string, kind trace.SpanKind, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	return otel.Tracer(tracerName).Start(ctx, name,
		trace.WithSpanKind(kind),
		trace.WithAttributes(attrs...),
	)
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

// SpanError records an error on a span and sets its status to Error. No-op
// when err is nil. Use when the span outlives the current function (per-event
// loops, void-return functions, multi-stage flows) where the named-return
// EndSpanWithError pattern does not apply.
func SpanError(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
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

// ContextFromTraceparent rebuilds a context with a remote span context from a
// "traceparent" string previously serialized by ExtractTraceparent. Used to
// resume an async trace across a process or storage boundary (DB, queue).
// Returns parent unchanged if traceparent is empty or malformed.
func ContextFromTraceparent(parent context.Context, traceparent string) context.Context {
	if traceparent == "" {
		return parent
	}
	carrier := propagation.MapCarrier{"traceparent": traceparent}
	return otel.GetTextMapPropagator().Extract(parent, carrier)
}
