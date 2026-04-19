package telemetry

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func HTTPMetrics(meter metric.Meter) func(next http.Handler) http.Handler {
	requestDuration, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP request duration in seconds"),
		metric.WithUnit("s"),
	)

	requestCount, _ := meter.Int64Counter(
		"http.server.request.count",
		metric.WithDescription("HTTP request count"),
	)

	activeRequests, _ := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithDescription("HTTP active requests"),
	)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			start := time.Now()

			activeRequests.Add(r.Context(), 1)
			next.ServeHTTP(sw, r)
			activeRequests.Add(r.Context(), -1)

			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched"
			}

			attrs := []attribute.KeyValue{
				attribute.String("http.method", r.Method),
				attribute.String("http.route", route),
				attribute.String("http.status_code", strconv.Itoa(sw.status)),
			}

			duration := time.Since(start)

			requestDuration.Record(r.Context(), duration.Seconds(), metric.WithAttributes(attrs...))
			requestCount.Add(r.Context(), 1, metric.WithAttributes(attrs...))
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
