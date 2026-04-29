package telemetry

import (
	"context"
	"errors"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type Config struct {
	CollectorEndpoint string
	ServiceName       string
	ExportInterval    time.Duration
	TraceSampleRatio  float64
}

type Telemetry struct {
	meterProvider  *sdkmetric.MeterProvider
	tracerProvider *sdktrace.TracerProvider
}

func Setup(ctx context.Context, cfg Config) (*Telemetry, error) {
	res, err := buildResource(ctx, cfg.ServiceName)
	if err != nil {
		return nil, err
	}

	meterProvider, err := buildMeterProvider(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	otel.SetMeterProvider(meterProvider)

	tracerProvider, err := buildTracerProvider(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(tracerProvider)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Telemetry{
		meterProvider:  meterProvider,
		tracerProvider: tracerProvider,
	}, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return errors.Join(
		t.meterProvider.Shutdown(ctx),
		t.tracerProvider.Shutdown(ctx),
	)
}

func buildResource(ctx context.Context, serviceName string) (*resource.Resource, error) {
	instanceID, err := os.Hostname()
	if err != nil {
		instanceID = "unknown"
	}

	return resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			semconv.ServiceInstanceIDKey.String(instanceID),
		),
	)
}

func buildMeterProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	exporter, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(cfg.CollectorEndpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter,
				sdkmetric.WithInterval(cfg.ExportInterval),
			),
		),
	), nil
}

func buildTracerProvider(ctx context.Context, cfg Config, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.CollectorEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(buildSampler(cfg.TraceSampleRatio)),
	), nil
}

// buildSampler picks a sampler based on the configured ratio. Wraps with
// ParentBased so that an inbound traceparent's sampling decision is honored
// (sampled-in propagation across services); otherwise the local ratio applies.
//
// Ratio semantics:
//
//	1.0  -> sample every trace (dev default)
//	0.01 -> sample 1% of root traces (typical prod baseline)
//	0.0  -> sample nothing (drop all telemetry; not recommended)
func buildSampler(ratio float64) sdktrace.Sampler {
	if ratio >= 1.0 {
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
	if ratio <= 0.0 {
		return sdktrace.ParentBased(sdktrace.NeverSample())
	}
	return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))
}
