package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/futurebuildai/buildos/internal/brain"
)

// newTestLogger returns a logger writing JSON to a buffer the test can
// inspect, wrapped in CorrelatingHandler.
func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(NewCorrelatingHandler(inner))
}

func TestCorrelatingHandler_StampsTraceIDFromActiveSpan(t *testing.T) {
	// When ctx carries a valid OTel span, the handler should stamp
	// trace_id + span_id alongside request_id. Form the trio every
	// observability stack expects for cross-correlation.
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// Create a real span via a TracerProvider that records spans
	// in-memory. We don't need an exporter — just a span context
	// with a valid trace_id/span_id pair.
	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test.op")
	defer span.End()

	logger.InfoContext(ctx, "test message")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if record["trace_id"] == nil {
		t.Errorf("trace_id missing: %v", record)
	}
	if record["span_id"] == nil {
		t.Errorf("span_id missing: %v", record)
	}
	// trace_id should be 32 hex chars; span_id should be 16. Defends
	// against a future change that accidentally truncates either.
	if traceID, _ := record["trace_id"].(string); len(traceID) != 32 {
		t.Errorf("trace_id len = %d, want 32 (got %q)", len(traceID), traceID)
	}
	if spanID, _ := record["span_id"].(string); len(spanID) != 16 {
		t.Errorf("span_id len = %d, want 16 (got %q)", len(spanID), spanID)
	}
}

func TestCorrelatingHandler_StampsRequestIDFromContext(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx := brain.ContextWithRequestID(context.Background(), "req-abc-123")
	logger.InfoContext(ctx, "test message", "extra", "value")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal log line: %v\n%s", err, buf.String())
	}
	if record["request_id"] != "req-abc-123" {
		t.Errorf("request_id = %v, want req-abc-123", record["request_id"])
	}
	if record["msg"] != "test message" {
		t.Errorf("msg = %v", record["msg"])
	}
	if record["extra"] != "value" {
		t.Errorf("extra attr lost: %v", record["extra"])
	}
}

func TestCorrelatingHandler_NoRequestIDOnContextlessLog(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// slog.Logger.Info (no ctx) routes through Handle with the
	// background context. No request_id should appear.
	logger.Info("plain message")

	out := buf.String()
	if strings.Contains(out, "request_id") {
		t.Errorf("expected no request_id field; got %s", out)
	}
}

func TestCorrelatingHandler_EmptyRequestIDNotStamped(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx := brain.ContextWithRequestID(context.Background(), "")
	logger.InfoContext(ctx, "empty id")

	out := buf.String()
	if strings.Contains(out, "request_id") {
		t.Errorf("empty request_id should be omitted; got %s", out)
	}
}

func TestCorrelatingHandler_PreservesWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// Logger-level attrs should survive wrapping.
	logger = logger.With("component", "agents")

	ctx := brain.ContextWithRequestID(context.Background(), "req-xyz")
	logger.InfoContext(ctx, "test")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if record["component"] != "agents" {
		t.Errorf("With attr lost: %v", record)
	}
	if record["request_id"] != "req-xyz" {
		t.Errorf("request_id lost: %v", record)
	}
}

func TestCorrelatingHandler_GroupScopesRequestID(t *testing.T) {
	// When the logger sits inside a group, slog's contract is that
	// AddAttrs adds inside the group — including the request_id we
	// inject. Top-level placement would require intercepting
	// WithGroup which is non-trivial; for a simple correlation ID
	// nested-under-group is acceptable. BuildOS's own server/worker
	// loggers don't use WithGroup, so this is mostly a documented
	// edge case rather than the common path.
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger = logger.WithGroup("svc")

	ctx := brain.ContextWithRequestID(context.Background(), "req-grp")
	logger.InfoContext(ctx, "test", "key", "val")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	svc, ok := record["svc"].(map[string]any)
	if !ok {
		t.Fatalf("expected svc group, got %v", record)
	}
	if svc["key"] != "val" {
		t.Errorf("user attr lost: %v", svc)
	}
	if svc["request_id"] != "req-grp" {
		t.Errorf("request_id should land in svc group: %v", svc)
	}
}
