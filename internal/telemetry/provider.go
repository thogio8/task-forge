package telemetry

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Config struct {
	CollectorEndpoint string
	ServiceName       string
	ExportInterval    time.Duration
}

type Telemetry struct {
	meterProvider *metric.MeterProvider
}

func Setup(ctx context.Context, cfg Config) (*Telemetry, error) {
	instanceID, err := os.Hostname()
	if err != nil {
		instanceID = "unknown"
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceInstanceIDKey.String(instanceID),
		),
	)

	if err != nil {
		return nil, err
	}

	metricExporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.CollectorEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)

	if err != nil {
		return nil, err
	}

	meterProvider := metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(
			metric.NewPeriodicReader(metricExporter,
				metric.WithInterval(cfg.ExportInterval),
			),
		),
	)

	otel.SetMeterProvider(meterProvider)

	return &Telemetry{meterProvider: meterProvider}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return t.meterProvider.Shutdown(ctx)
}
