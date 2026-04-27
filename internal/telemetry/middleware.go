package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// HTTPMiddleware records HTTP server metrics and opens a server-side span for
// each incoming request. The span name and route attribute are resolved using
// chi's RoutePattern (not URL.Path) to bound cardinality.
func HTTPMiddleware(meter metric.Meter) func(next http.Handler) http.Handler {
	requestDuration, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(LatencyBuckets...),
	)

	requestCount, _ := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("HTTP request count"),
	)

	activeRequests, _ := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("HTTP active requests"),
	)

	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := propagator.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tracer.Start(ctx, "HTTP "+r.Method)
			defer span.End()

			r = r.WithContext(ctx)

			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()

			activeRequests.Add(ctx, 1)
			next.ServeHTTP(sw, r)
			activeRequests.Add(ctx, -1)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}

			span.SetName("HTTP " + r.Method + " " + route)
			span.SetAttributes(
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.HTTPRouteKey.String(route),
				semconv.HTTPResponseStatusCodeKey.Int(sw.status),
			)
			if sw.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(sw.status))
			}

			attrs := []attribute.KeyValue{
				attribute.String("http.method", r.Method),
				attribute.String("http.route", route),
				attribute.String("http.status_code", strconv.Itoa(sw.status)),
			}

			duration := time.Since(start)

			requestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
			requestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
		})
	}
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status = code
		sw.wroteHeader = true
	}
	sw.ResponseWriter.WriteHeader(code)
}
