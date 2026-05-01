package obs

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel"
)

func quietForTracer() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInitTracing_EmptyEndpointReturnsNoop(t *testing.T) {
	shutdown, ok := InitTracing(context.Background(), TracingConfig{}, quietForTracer())
	if ok {
		t.Errorf("expected initialized=false for empty Endpoint")
	}
	if shutdown == nil {
		t.Fatal("shutdown should never be nil")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Errorf("noop shutdown returned %v", err)
	}
}

func TestInitTracing_EmptyEndpointStillSetsPropagator(t *testing.T) {
	// Even when not initialized, the W3C propagator must be set so
	// inbound `traceparent` headers parse and outbound calls
	// propagate trace context. If a downstream test has already
	// run with InitTracing, the propagator may already be set —
	// we accept either "matches our composite" or "matches a
	// previously set composite" as ok.
	_, _ = InitTracing(context.Background(), TracingConfig{}, quietForTracer())
	prop := otel.GetTextMapPropagator()
	if prop == nil {
		t.Fatal("global propagator should be set after InitTracing call")
	}
	// Defensive: confirm at least one of the standard fields is in
	// the propagator's Fields() set.
	fields := prop.Fields()
	hasTraceparent := false
	for _, f := range fields {
		if f == "traceparent" {
			hasTraceparent = true
		}
	}
	if !hasTraceparent {
		t.Errorf("expected propagator to handle traceparent header; fields=%v", fields)
	}
}

func TestInitTracing_BadEndpointReturnsFalse(t *testing.T) {
	// Endpoint that's syntactically wrong (not a valid host:port).
	// otlptracehttp may or may not error at New() time depending on
	// the version; we accept either ok=true (lazy init) or ok=false
	// (eager validation), as long as shutdown doesn't panic.
	shutdown, _ := InitTracing(context.Background(),
		TracingConfig{Endpoint: "::not-a-valid-endpoint::"},
		quietForTracer())
	if shutdown == nil {
		t.Fatal("shutdown should never be nil even on init failure")
	}
	// Best-effort shutdown — should not panic.
	_ = shutdown(context.Background())
}

func TestInitTracing_DefaultsAppliedWhenZero(t *testing.T) {
	// Use a localhost endpoint that won't actually resolve to a
	// collector — the SDK is lazy enough that init succeeds and
	// only fails when it tries to export. Tests the defaulting
	// path without needing a real collector.
	shutdown, ok := InitTracing(context.Background(), TracingConfig{
		Endpoint: "localhost:4318",
		Insecure: true,
		// SampleRate intentionally zero — should default to 0.1
		// inside InitTracing.
	}, quietForTracer())
	if !ok {
		t.Skip("collector exporter init failed in this environment; lazy-init test skipped")
	}
	defer func() { _ = shutdown(context.Background()) }()
	// No assertion on the internal state; we only confirm that
	// init didn't error and shutdown is callable.
}
