package telemetry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestHTTPMetricsMiddleware_PassesThrough(t *testing.T) {
	meter := otel.Meter("taskforge.http.test")
	mw := HTTPMetrics(meter)

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

func TestHTTPMetricsMiddleware_CapturesStatusCode(t *testing.T) {
	meter := otel.Meter("taskforge.http.test")
	mw := HTTPMetrics(meter)

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

func TestHTTPMetricsMiddleware_DefaultStatusOK(t *testing.T) {
	meter := otel.Meter("taskforge.http.test")
	mw := HTTPMetrics(meter)

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
