package telemetry

// LatencyBuckets defines bucket boundaries tuned for sub-second to multi-second
// operations: 1ms → 10s, logarithmic progression.
//
// The OTel default buckets (0, 5, 10, 25, 50, 75, 100, 250, 500, 750, 1000, ...)
// are designed for millisecond-scale measurements. When a histogram records in
// seconds (as per OpenTelemetry semantic convention), those defaults make P95/P99
// unusable: every sub-second value falls into a single huge bucket.
//
// These boundaries match typical web service latency distributions and align
// with Prometheus community conventions (prometheus.DefBuckets).
var LatencyBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05,
	0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}
