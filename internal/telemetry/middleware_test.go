package telemetry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// withTestTracerProvider installs a fresh TracerProvider backed by an
// in-memory SpanRecorder for the duration of the test, restoring the previous
// provider afterwards. The returned recorder can be queried with .Ended().
func withTestTracerProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})

	return recorder
}

// withTestMeterProvider installs a fresh MeterProvider with a manual reader
// for the duration of the test, restoring the previous provider afterwards.
// The returned reader can be used to collect emitted metrics on demand.
func withTestMeterProvider(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	prev := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() {
		otel.SetMeterProvider(prev)
		_ = mp.Shutdown(context.Background())
	})

	return reader
}

// findMetric returns the metric data matching the given name, or nil.
func findMetric(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}

// hasAttribute reports whether the attribute set contains a key/value pair.
func hasAttribute(set attribute.Set, key, value string) bool {
	v, ok := set.Value(attribute.Key(key))
	if !ok {
		return false
	}
	return v.AsString() == value
}

func TestHTTPMiddleware_PassesThrough(t *testing.T) {
	meter := otel.Meter("taskforge.http.test")
	mw := HTTPMiddleware(meter)

	called := false
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("middleware did not call next handler")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHTTPMiddleware_CapturesStatusCode(t *testing.T) {
	meter := otel.Meter("taskforge.http.test")
	mw := HTTPMiddleware(meter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHTTPMiddleware_DefaultStatusOK(t *testing.T) {
	meter := otel.Meter("taskforge.http.test")
	mw := HTTPMiddleware(meter)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestStatusWriter_WriteHeaderCalledOnce(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

	sw.WriteHeader(http.StatusCreated)
	sw.WriteHeader(http.StatusInternalServerError)

	if sw.status != http.StatusCreated {
		t.Errorf("status = %d, want %d (first call should win)", sw.status, http.StatusCreated)
	}
}

func TestStatusWriter_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}

	if sw.status != http.StatusOK {
		t.Errorf("status = %d, want %d", sw.status, http.StatusOK)
	}
}

// TestHTTPMiddleware_RecordsCounterAndHistogram verifies that the
// middleware actually emits the expected counter and histogram with the
// proper attributes when wired behind a chi router.
func TestHTTPMiddleware_RecordsCounterAndHistogram(t *testing.T) {
	reader := withTestMeterProvider(t)
	meter := otel.Meter("taskforge.http")

	router := chi.NewRouter()
	router.Use(HTTPMiddleware(meter))
	router.Get("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/tasks/abc-123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	count := findMetric(rm, "http.server.request.count")
	if count == nil {
		t.Fatal("http.server.request.count not recorded")
	}

	sum, ok := count.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("http.server.request.count is not Sum[int64], got %T", count.Data)
	}
	if len(sum.DataPoints) != 1 {
		t.Fatalf("expected 1 data point, got %d", len(sum.DataPoints))
	}
	if sum.DataPoints[0].Value != 1 {
		t.Errorf("counter value = %d, want 1", sum.DataPoints[0].Value)
	}
	attrs := sum.DataPoints[0].Attributes
	if !hasAttribute(attrs, "http.method", "GET") {
		t.Errorf("missing or wrong http.method attribute: %v", attrs.ToSlice())
	}
	if !hasAttribute(attrs, "http.route", "/tasks/{id}") {
		t.Errorf("expected http.route=/tasks/{id} (chi pattern), got: %v", attrs.ToSlice())
	}
	if !hasAttribute(attrs, "http.status_code", "200") {
		t.Errorf("missing or wrong http.status_code attribute: %v", attrs.ToSlice())
	}

	duration := findMetric(rm, "http.server.request.duration")
	if duration == nil {
		t.Fatal("http.server.request.duration not recorded")
	}

	hist, ok := duration.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("http.server.request.duration is not Histogram[float64], got %T", duration.Data)
	}
	if len(hist.DataPoints) != 1 {
		t.Fatalf("expected 1 histogram data point, got %d", len(hist.DataPoints))
	}
	if hist.DataPoints[0].Count != 1 {
		t.Errorf("histogram count = %d, want 1", hist.DataPoints[0].Count)
	}
}

// TestHTTPMiddleware_UnmatchedRouteFallback verifies that the
// middleware labels requests with `unmatched` when no chi route pattern is
// resolved (e.g. unrouted requests, raw http.Handler usage).
func TestHTTPMiddleware_UnmatchedRouteFallback(t *testing.T) {
	reader := withTestMeterProvider(t)
	meter := otel.Meter("taskforge.http")

	mw := HTTPMiddleware(meter)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/does/not/exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	count := findMetric(rm, "http.server.request.count")
	if count == nil {
		t.Fatal("http.server.request.count not recorded")
	}

	sum := count.Data.(metricdata.Sum[int64])
	if !hasAttribute(sum.DataPoints[0].Attributes, "http.route", "unmatched") {
		t.Errorf("expected http.route=unmatched for unrouted request, got: %v", sum.DataPoints[0].Attributes.ToSlice())
	}
}

// TestHTTPMiddleware_ActiveRequestsReturnsToZero verifies that the
// in-flight gauge is correctly decremented after the request completes.
func TestHTTPMiddleware_ActiveRequestsReturnsToZero(t *testing.T) {
	reader := withTestMeterProvider(t)
	meter := otel.Meter("taskforge.http")

	router := chi.NewRouter()
	router.Use(HTTPMiddleware(meter))
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}

	active := findMetric(rm, "http.server.active_requests")
	if active == nil {
		t.Fatal("http.server.active_requests not recorded")
	}

	sum := active.Data.(metricdata.Sum[int64])
	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	if total != 0 {
		t.Errorf("active_requests total = %d after all requests done, want 0", total)
	}
}

// TestHTTPMiddleware_OpensSpanWithRoute verifies that the middleware opens a
// server-side span with the chi RoutePattern (not URL.Path) and the standard
// HTTP semantic-convention attributes.
func TestHTTPMiddleware_OpensSpanWithRoute(t *testing.T) {
	recorder := withTestTracerProvider(t)
	meter := otel.Meter("taskforge.http")

	router := chi.NewRouter()
	router.Use(HTTPMiddleware(meter))
	router.Get("/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/tasks/abc-123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name() != "HTTP GET /tasks/{id}" {
		t.Errorf("span name = %q, want %q", span.Name(), "HTTP GET /tasks/{id}")
	}

	attrs := span.Attributes()
	want := map[string]string{
		"http.request.method":      "GET",
		"http.route":               "/tasks/{id}",
		"http.response.status_code": "200",
	}
	for key, expected := range want {
		found := false
		for _, kv := range attrs {
			if string(kv.Key) != key {
				continue
			}
			got := kv.Value.Emit()
			if got != expected {
				t.Errorf("attr %s = %q, want %q", key, got, expected)
			}
			found = true
			break
		}
		if !found {
			t.Errorf("missing span attribute %q", key)
		}
	}
}

// TestHTTPMiddleware_SpanErrorOnServerError verifies that the span status is
// set to Error when the handler returns 5xx.
func TestHTTPMiddleware_SpanErrorOnServerError(t *testing.T) {
	recorder := withTestTracerProvider(t)
	meter := otel.Meter("taskforge.http")

	router := chi.NewRouter()
	router.Use(HTTPMiddleware(meter))
	router.Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if code := spans[0].Status().Code.String(); code != "Error" {
		t.Errorf("span status = %q, want Error (5xx should mark span as error)", code)
	}
}
