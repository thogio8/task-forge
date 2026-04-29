package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

// TracingHandler wraps an inner slog.Handler and injects the active span's
// trace_id and span_id as record attributes when ctx carries an OTel span.
// When no valid span is active in ctx, the record is delegated unchanged —
// callers can keep using non-Context logging variants (e.g. logger.Info)
// without breakage; only *Context calls (InfoContext, ErrorContext, ...)
// receive the trace correlation.
type TracingHandler struct {
	inner slog.Handler
}

// WrapWithTracing returns a slog.Handler that adds trace_id and span_id
// attributes when an OTel span is active in the record's context. Compose
// with any underlying handler — JSONHandler in production, TextHandler in
// dev. The wrapper itself is format-agnostic.
func WrapWithTracing(inner slog.Handler) slog.Handler {
	return &TracingHandler{inner: inner}
}

func (h *TracingHandler) Handle(ctx context.Context, r slog.Record) error {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, r)
}

func (h *TracingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *TracingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &TracingHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *TracingHandler) WithGroup(name string) slog.Handler {
	return &TracingHandler{inner: h.inner.WithGroup(name)}
}
