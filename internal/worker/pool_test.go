package worker

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thogio8/task-forge/internal/model"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

var testLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// withTestMeterProvider installs a fresh MeterProvider with a manual reader
// for the duration of the test, restoring the previous provider afterwards.
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

// observeGaugeInt64 fetches the most recent observation of a named Int64
// observable gauge. Returns (value, true) when found, (0, false) otherwise.
func observeGaugeInt64(t *testing.T, reader *sdkmetric.ManualReader, name string) (int64, bool) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			gauge, ok := m.Data.(metricdata.Gauge[int64])
			if !ok || len(gauge.DataPoints) == 0 {
				return 0, false
			}
			return gauge.DataPoints[0].Value, true
		}
	}
	return 0, false
}

func TestPool_AllTasksProcessed(t *testing.T) {
	var counter atomic.Int64

	workerCount := 3
	processFunc := func(_ context.Context, _ model.Task) { counter.Add(1) }

	pool := NewPool(workerCount, processFunc, testLogger)

	pool.Start()

	for range 20 {
		pool.Submit(context.Background(), model.Task{})
	}

	pool.Stop()

	if counter.Load() != 20 {
		t.Fatalf("expected counter to be 20, got: %d", counter.Load())
	}
}

// TestPool_QueueDepthGaugeIsRegistered verifies that the pool.queue_depth
// observable gauge is registered on the meter and that its callback executes
// on collect. We don't assert a specific value — the OTel SDK caches meters
// by name globally, so when several Pool instances are created in different
// tests the callback may observe channels other than this test's instance.
// What matters for coverage is that the callback path runs at least once.
func TestPool_QueueDepthGaugeIsRegistered(t *testing.T) {
	reader := withTestMeterProvider(t)

	_ = NewPool(2, func(_ context.Context, _ model.Task) {}, testLogger)

	if _, ok := observeGaugeInt64(t, reader, "pool.queue_depth"); !ok {
		t.Fatal("pool.queue_depth gauge not recorded")
	}
}

// TestPool_AccessorsExposeInternalState exercises the small Tasks(), Active(),
// Processed() accessors so they are not silent-dead-code in coverage.
func TestPool_AccessorsExposeInternalState(t *testing.T) {
	pool := NewPool(2, func(_ context.Context, _ model.Task) {}, testLogger)

	if pool.Tasks() == nil {
		t.Fatal("Tasks() returned nil channel")
	}
	if got := pool.Active(); got != 0 {
		t.Errorf("Active() before Start = %d, want 0", got)
	}
	if got := pool.Processed(); got != 0 {
		t.Errorf("Processed() before Start = %d, want 0", got)
	}

	pool.Start()
	for range 5 {
		pool.Submit(context.Background(), model.Task{})
	}
	pool.Stop()

	if got := pool.Processed(); got != 5 {
		t.Errorf("Processed() after Stop = %d, want 5", got)
	}
	if got := pool.Active(); got != 0 {
		t.Errorf("Active() after Stop = %d, want 0 (no tasks running)", got)
	}
}

func TestPool_StopBlocksUntilDone(t *testing.T) {
	var counter atomic.Int64

	workerCount := 3
	processFunc := func(_ context.Context, _ model.Task) {
		time.Sleep(50 * time.Millisecond)
		counter.Add(1)
	}

	pool := NewPool(workerCount, processFunc, testLogger)

	pool.Start()

	for range 10 {
		pool.Submit(context.Background(), model.Task{})
	}

	pool.Stop()

	if counter.Load() != 10 {
		t.Fatalf("expected counter to be 10, got: %d", counter.Load())
	}
}
