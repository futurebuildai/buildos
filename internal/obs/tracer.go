package obs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// TracingConfig is the operator-controlled subset of OTel SDK
// parameters. Mirrors the cfg fields in internal/config so cmd/*
// entrypoints pass through without translation.
type TracingConfig struct {
	// Endpoint is the OTLP-HTTP collector URL (e.g.
	// "https://otel-collector.example.com"). Empty disables
	// tracing initialization entirely — the SDK falls back to a
	// no-op tracer that's safe to call but emits nothing.
	Endpoint string

	// ServiceName tags every span with a service.name attribute.
	// Defaults to "buildos" when empty.
	ServiceName string

	// Environment is the deploy.environment.name attribute
	// ("production" / "staging" / "dev"). Defaults to "dev".
	Environment string

	// Release is the service.version attribute (build SHA / git
	// tag). Empty omits the attribute.
	Release string

	// SampleRate is the head-based sample rate, [0.0, 1.0]. 1.0
	// records every span; 0.0 records none (the no-op tracer).
	// Production deployments typically set 0.05–0.1; high-traffic
	// services lower further. Default 0.1.
	SampleRate float64

	// Insecure controls whether the OTLP exporter requires TLS to
	// the collector. False (default) requires HTTPS. True allows
	// HTTP — only for local dev / sidecar collectors on the same
	// pod over loopback.
	Insecure bool
}

// InitTracing boots OpenTelemetry. Returns a shutdown function the
// caller must defer at process exit (drains pending spans within a
// 5-second budget) and a bool reporting whether the SDK actually
// initialized.
//
// Empty Endpoint is intentionally a no-op return — many environments
// (local dev, CI) don't run a collector. The returned shutdown is
// always safe to call.
//
// On success, the global TracerProvider + Propagator are set; any
// `otel.Tracer(...)` call elsewhere in the codebase routes through
// these. Propagator is the W3C tracecontext + baggage standard so
// trace_id flows over HTTP headers (`traceparent`) to The Brain.
func InitTracing(ctx context.Context, cfg TracingConfig, logger *slog.Logger) (shutdown func(context.Context) error, initialized bool) {
	if cfg.Endpoint == "" {
		logger.Info("otel: endpoint unset, skipping tracing initialization")
		// Even when not initialized, set the W3C propagator so
		// inbound `traceparent` headers are still parsed (and our
		// outbound calls still propagate trace context if the caller
		// has one). Spans go nowhere; trace_ids still correlate.
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{}, propagation.Baggage{},
		))
		return func(context.Context) error { return nil }, false
	}

	if cfg.ServiceName == "" {
		cfg.ServiceName = "buildos"
	}
	if cfg.Environment == "" {
		cfg.Environment = "dev"
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 0.1
	}
	if cfg.SampleRate > 1.0 {
		cfg.SampleRate = 1.0
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.Endpoint),
	}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	if err != nil {
		logger.Error("otel: exporter init failed; continuing without",
			"error", err, "endpoint", cfg.Endpoint)
		return func(context.Context) error { return nil }, false
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.DeploymentEnvironmentName(cfg.Environment),
		),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithHost(),
	)
	if err != nil {
		logger.Warn("otel: resource detection partial failure; continuing",
			"error", err)
	}
	if cfg.Release != "" {
		res, err = resource.Merge(res, resource.NewSchemaless(
			semconv.ServiceVersion(cfg.Release),
		))
		if err != nil {
			logger.Warn("otel: release attribute merge failed", "error", err)
		}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.SampleRate)),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	logger.Info("otel tracing initialized",
		"endpoint", cfg.Endpoint,
		"service", cfg.ServiceName,
		"environment", cfg.Environment,
		"release", cfg.Release,
		"sample_rate", cfg.SampleRate,
	)

	return func(shutdownCtx context.Context) error {
		shutdownCtx, cancel := context.WithTimeout(shutdownCtx, 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("otel tracer shutdown: %w", err)
		}
		return nil
	}, true
}
