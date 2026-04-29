package telemetry

import (
	"context"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func TestSetupAndShutdown(t *testing.T) {
	ctx := context.Background()

	tel, err := Setup(ctx, Config{
		CollectorEndpoint: "localhost:4317",
		ServiceName:       "taskforge-test",
		ExportInterval:    1 * time.Second,
	})

	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	if tel == nil {
		t.Fatal("Setup() returned nil")
	}

	if tel.meterProvider == nil {
		t.Fatal("Setup() returned Telemetry with nil meterProvider")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Shutdown tente de flush les métriques vers le Collector.
	// Sans Collector actif, l'export échoue — c'est attendu en test.
	tel.Shutdown(shutdownCtx)

	if tel.tracerProvider == nil {
		t.Fatal("Setup() returned Telemetry with nil tracerProvider")
	}
}

func TestSetupWithEmptyServiceName(t *testing.T) {
	ctx := context.Background()

	tel, err := Setup(ctx, Config{
		CollectorEndpoint: "localhost:4317",
		ServiceName:       "",
		ExportInterval:    1 * time.Second,
	})

	if err != nil {
		t.Fatalf("Setup() error = %v", err)
	}

	if tel == nil {
		t.Fatal("Setup() returned nil")
	}

	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Shutdown tente de flush les métriques vers le Collector.
	// Sans Collector actif, l'export échoue — c'est attendu en test.
	tel.Shutdown(shutdownCtx)
}

func stableTraceID(t *testing.T, hex string) trace.TraceID {
	t.Helper()
	tid, err := trace.TraceIDFromHex(hex)
	if err != nil {
		t.Fatalf("TraceIDFromHex(%q) error: %v", hex, err)
	}
	return tid
}

func TestBuildSampler_RatioOneAlwaysSamples(t *testing.T) {
	sampler := buildSampler(1.0)

	got := sampler.ShouldSample(sdktrace.SamplingParameters{
		TraceID: stableTraceID(t, "0123456789abcdef0123456789abcdef"),
		Name:    "x",
	})
	if got.Decision != sdktrace.RecordAndSample {
		t.Errorf("ratio=1.0 decision = %v, want RecordAndSample", got.Decision)
	}

	if desc := sampler.Description(); desc == "" {
		t.Error("expected non-empty sampler description")
	}
}

func TestBuildSampler_RatioZeroNeverSamples(t *testing.T) {
	sampler := buildSampler(0.0)

	got := sampler.ShouldSample(sdktrace.SamplingParameters{
		TraceID: stableTraceID(t, "fedcba9876543210fedcba9876543210"),
		Name:    "x",
	})
	if got.Decision != sdktrace.Drop {
		t.Errorf("ratio=0.0 decision = %v, want Drop", got.Decision)
	}
}

func TestBuildSampler_RatioBetweenIsTraceIDBased(t *testing.T) {
	sampler := buildSampler(0.5)

	desc := sampler.Description()
	if !strings.Contains(desc, "TraceIDRatioBased") {
		t.Errorf("description = %q, expected to mention TraceIDRatioBased", desc)
	}
}

func TestBuildSampler_NegativeRatioBehavesAsZero(t *testing.T) {
	sampler := buildSampler(-0.1)

	got := sampler.ShouldSample(sdktrace.SamplingParameters{
		TraceID: stableTraceID(t, "0123456789abcdef0123456789abcdef"),
		Name:    "x",
	})
	if got.Decision != sdktrace.Drop {
		t.Errorf("negative ratio decision = %v, want Drop", got.Decision)
	}
}

func TestBuildSampler_RatioAboveOneBehavesAsOne(t *testing.T) {
	sampler := buildSampler(2.0)

	got := sampler.ShouldSample(sdktrace.SamplingParameters{
		TraceID: stableTraceID(t, "0123456789abcdef0123456789abcdef"),
		Name:    "x",
	})
	if got.Decision != sdktrace.RecordAndSample {
		t.Errorf("ratio>1 decision = %v, want RecordAndSample", got.Decision)
	}
}

func TestBuildResource_HasServiceAttributes(t *testing.T) {
	res, err := buildResource(context.Background(), "taskforge-test")
	if err != nil {
		t.Fatalf("buildResource error: %v", err)
	}
	if res == nil {
		t.Fatal("expected non-nil resource")
	}

	var sawServiceName, sawInstanceID bool
	for _, kv := range res.Attributes() {
		switch string(kv.Key) {
		case "service.name":
			if kv.Value.AsString() != "taskforge-test" {
				t.Errorf("service.name = %q, want %q", kv.Value.AsString(), "taskforge-test")
			}
			sawServiceName = true
		case "service.instance.id":
			if kv.Value.AsString() == "" {
				t.Error("service.instance.id should not be empty")
			}
			sawInstanceID = true
		}
	}
	if !sawServiceName {
		t.Error("missing service.name attribute")
	}
	if !sawInstanceID {
		t.Error("missing service.instance.id attribute")
	}
}
