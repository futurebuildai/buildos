package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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

	ctx := ContextWithRequestID(context.Background(), "req-abc-123")
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

	ctx := ContextWithRequestID(context.Background(), "")
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

	ctx := ContextWithRequestID(context.Background(), "req-xyz")
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

// TestCorrelatingHandler_ScrubsRestrictedPIIAttrs — per-call attrs
// whose key matches a Restricted-class field in pii.FieldClass must
// land at the inner handler with the value redacted. Confidential
// and Internal class attrs pass through unchanged.
func TestCorrelatingHandler_ScrubsRestrictedPIIAttrs(t *testing.T) {
	cases := []struct {
		name     string
		attrKey  string
		attrVal  any
		mustHide string // value substring that must NOT appear in output
	}{
		{"email", "email", "alice@example.com", "alice@example.com"},
		{"phone", "phone", "+1-415-555-0199", "415-555-0199"},
		{"phone_number", "phone_number", "+1-555-0001", "555-0001"},
		{"first_name", "first_name", "Alice", "Alice"},
		{"last_name", "last_name", "Liddell", "Liddell"},
		{"display_name", "display_name", "Alice L.", "Alice L."},
		{"address", "address", "123 Main St", "123 Main St"},
		{"street_address", "street_address", "456 Elm Ave", "456 Elm Ave"},
		{"oidc_sub", "sub", "auth0|abc123", "auth0|abc123"},
		{"user_sub", "user_sub", "auth0|xyz", "auth0|xyz"},
		{"ip_address_string", "ip_address", "203.0.113.42", "203.0.113.42"},
		{"remote_addr", "remote_addr", "198.51.100.7:55321", "198.51.100.7"},
		// Non-string Restricted: the typed value should be replaced.
		{"gps_lat_float", "gps_lat", 37.7749, "37.7749"},
		{"gps_lng_int", "gps_lng", -122, "-122"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newTestLogger(&buf)
			logger.Info("event", tc.attrKey, tc.attrVal)

			out := buf.String()
			if strings.Contains(out, tc.mustHide) {
				t.Errorf("Restricted value leaked: key=%s want absent=%q got line=%s",
					tc.attrKey, tc.mustHide, out)
			}
		})
	}
}

// TestCorrelatingHandler_PreservesConfidentialAttrs — vendor names,
// *_cents amounts, project names should NOT be masked. The slog/SIEM
// surface needs them clear to triage incidents (per the
// classification doc in pii/pii.go).
func TestCorrelatingHandler_PreservesConfidentialAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)
	logger.Info("invoice processed",
		"vendor_name", "Acme Lumber",
		"amount_cents", 150000,
		"project_name", "Maple Ridge HOA",
	)

	out := buf.String()
	for _, expected := range []string{"Acme Lumber", "150000", "Maple Ridge HOA"} {
		if !strings.Contains(out, expected) {
			t.Errorf("Confidential field %q was scrubbed; should be preserved\nout=%s",
				expected, out)
		}
	}
}

// TestCorrelatingHandler_PreservesInternalCorrelationAttrs — the
// correlation-id trio (request_id, trace_id, span_id) and other
// Internal-class fields must always pass through clear so triage
// queries work.
func TestCorrelatingHandler_PreservesInternalCorrelationAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	ctx := ContextWithRequestID(context.Background(), "req-correlation-123")
	logger.InfoContext(ctx, "test", "event_type", "feed.card.actioned", "action", "approve_quote")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	if record["request_id"] != "req-correlation-123" {
		t.Errorf("request_id was scrubbed: %v", record["request_id"])
	}
	if record["event_type"] != "feed.card.actioned" {
		t.Errorf("event_type was scrubbed: %v", record["event_type"])
	}
	if record["action"] != "approve_quote" {
		t.Errorf("action was scrubbed: %v", record["action"])
	}
}

// TestCorrelatingHandler_ScrubsRestrictedInsideGroup — Group-typed
// attrs need recursive scrubbing; the catalog applies inside the
// group too. Without recursion, a logger.With(slog.Group("actor",
// "email", "alice@x")) bakes a leak.
func TestCorrelatingHandler_ScrubsRestrictedInsideGroup(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	logger.Info("nested",
		slog.Group("actor",
			slog.String("email", "alice@example.com"),
			slog.String("name", "Alice"),
			slog.String("user_id", "U-12345"), // unknown key → Internal default → pass through
		),
	)

	out := buf.String()
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("nested email leaked: %s", out)
	}
	if strings.Contains(out, "\"Alice\"") {
		t.Errorf("nested name leaked: %s", out)
	}
	if !strings.Contains(out, "U-12345") {
		t.Errorf("non-PII group member was scrubbed: %s", out)
	}
}

// TestCorrelatingHandler_ScrubsAttrsBakedViaWith — logger.With(...)
// bakes attrs into the inner handler at construction time (they
// don't flow through Handle's record). The wrapper's WithAttrs must
// scrub on the way through.
func TestCorrelatingHandler_ScrubsAttrsBakedViaWith(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	withPII := logger.With("email", "alice@example.com", "component", "agents")
	withPII.Info("baked attrs")

	out := buf.String()
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("With-baked email leaked: %s", out)
	}
	if !strings.Contains(out, "agents") {
		t.Errorf("non-PII baked attr was scrubbed: %s", out)
	}
}

// TestCorrelatingHandler_ScrubAndCorrelationCoexist — a single record
// carrying both PII attrs and active context (request_id + span)
// should land with PII redacted AND correlation ids stamped.
func TestCorrelatingHandler_ScrubAndCorrelationCoexist(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	tp := sdktrace.NewTracerProvider()
	tracer := tp.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test.op")
	defer span.End()
	ctx = ContextWithRequestID(ctx, "req-coex-1")

	logger.InfoContext(ctx, "user action",
		"email", "alice@example.com",
		"vendor_name", "Acme Lumber",
		"action", "approve_quote",
	)

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("decode: %v\n%s", err, buf.String())
	}
	// Correlation trio
	if record["request_id"] != "req-coex-1" {
		t.Errorf("request_id missing: %v", record)
	}
	if record["trace_id"] == nil {
		t.Errorf("trace_id missing: %v", record)
	}
	if record["span_id"] == nil {
		t.Errorf("span_id missing: %v", record)
	}
	// PII redacted, Confidential preserved, Internal preserved
	out := buf.String()
	if strings.Contains(out, "alice@example.com") {
		t.Errorf("email leaked: %s", out)
	}
	if record["vendor_name"] != "Acme Lumber" {
		t.Errorf("vendor_name was scrubbed: %v", record)
	}
	if record["action"] != "approve_quote" {
		t.Errorf("action was scrubbed: %v", record)
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

	ctx := ContextWithRequestID(context.Background(), "req-grp")
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
