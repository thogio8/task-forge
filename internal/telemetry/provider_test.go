package telemetry

import (
	"context"
	"testing"
	"time"
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
