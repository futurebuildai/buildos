package obs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/pii"
)

func quietForSentry() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestInitSentry_EmptyDSNIsNoop(t *testing.T) {
	flush, ok := InitSentry(SentryConfig{}, quietForSentry())
	if ok {
		t.Errorf("expected initialized=false for empty DSN")
	}
	if flush == nil {
		t.Fatal("flush should never be nil")
	}
	flush() // should be safe to call
}

func TestInitSentry_BadDSNReturnsFalse(t *testing.T) {
	flush, ok := InitSentry(SentryConfig{DSN: "not-a-valid-dsn"}, quietForSentry())
	if ok {
		t.Errorf("expected initialized=false for malformed DSN")
	}
	flush() // safe even on init failure
}

func TestCaptureError_NilErrorReturnsEmpty(t *testing.T) {
	id := CaptureError(context.Background(), nil, nil)
	if id != "" {
		t.Errorf("nil err should return empty event id; got %q", id)
	}
}

func TestCaptureUnless_IgnoresMatchingSentinel(t *testing.T) {
	sentinel := errors.New("ignore me")
	id := CaptureUnless(context.Background(), sentinel, nil, sentinel)
	if id != "" {
		t.Errorf("matching sentinel should be ignored; got id %q", id)
	}
}

func TestCaptureUnless_IgnoresWrappedSentinel(t *testing.T) {
	sentinel := errors.New("base")
	wrapped := errAs("wrap: %w", sentinel)
	id := CaptureUnless(context.Background(), wrapped, nil, sentinel)
	if id != "" {
		t.Errorf("wrapped sentinel should be ignored via errors.Is; got id %q", id)
	}
}

func TestCaptureUnless_NilErrShortCircuits(t *testing.T) {
	id := CaptureUnless(context.Background(), nil, nil, errors.New("never reached"))
	if id != "" {
		t.Errorf("nil err should return empty; got %q", id)
	}
}

// Smoke test: Sentry is uninitialized (no DSN), CaptureError should not
// panic. The hub falls back to the no-op default.
func TestCaptureError_UninitializedSDKDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CaptureError panicked with no SDK: %v", r)
		}
	}()
	ctx := brain.ContextWithRequestID(context.Background(), "req-test")
	_ = CaptureError(ctx, errors.New("test error"), map[string]string{"area": "smoke"})
}

func TestScrubSentryEvent_RedactsTagsContextsUserHeaders(t *testing.T) {
	// Build an event with values across every PII surface Sentry
	// SDK exposes. After scrubSentryEvent, only the
	// correlation-id-style fields should remain plaintext; PII
	// fields should be masked or sentinels.
	event := &sentry.Event{
		Tags: map[string]string{
			"email":      "alice@buildos.dev",
			"phone":      "+15551234",
			"request_id": "req-abc",
			"event_type": "test",
		},
		Contexts: map[string]sentry.Context{
			"caller": map[string]any{
				"first_name":  "Alice",
				"vendor_name": "Acme Corp",
				"trace_id":    "trace-xyz",
				"crew": []any{
					map[string]any{"name": "Bob", "phone": "+15550100"},
				},
			},
		},
		User: sentry.User{
			ID:        "user-id-1",
			Email:     "alice@buildos.dev",
			IPAddress: "10.0.0.1",
			Username:  "alice",
		},
		Request: &sentry.Request{
			Cookies: "session=abc123; csrf=xyz",
			Headers: map[string]string{
				"Authorization": "Bearer should-not-leak",
				"X-Request-ID":  "req-abc",
			},
			Data: `{"email":"alice@x.io","org_id":"abc"}`,
		},
	}

	scrubbed := scrubSentryEvent(event, pii.Restricted)

	// Tags
	if scrubbed.Tags["email"] != "[REDACTED]" {
		t.Errorf("Tags[email] = %q, want [REDACTED]", scrubbed.Tags["email"])
	}
	if scrubbed.Tags["phone"] != "[REDACTED]" {
		t.Errorf("Tags[phone] = %q, want [REDACTED]", scrubbed.Tags["phone"])
	}
	if scrubbed.Tags["request_id"] != "req-abc" {
		t.Errorf("Tags[request_id] altered: %q", scrubbed.Tags["request_id"])
	}
	if scrubbed.Tags["event_type"] != "test" {
		t.Errorf("Tags[event_type] altered: %q", scrubbed.Tags["event_type"])
	}

	// Contexts (nested)
	caller := scrubbed.Contexts["caller"]
	if caller["first_name"] != "[REDACTED]" {
		t.Errorf("Contexts.caller.first_name altered: %v", caller["first_name"])
	}
	if caller["vendor_name"] != "Acme Corp" {
		t.Errorf("Contexts.caller.vendor_name should pass at Restricted threshold; got %v", caller["vendor_name"])
	}
	if caller["trace_id"] != "trace-xyz" {
		t.Errorf("Contexts.caller.trace_id altered: %v", caller["trace_id"])
	}
	crew := caller["crew"].([]any)
	if entry := crew[0].(map[string]any); entry["name"] != "[REDACTED]" || entry["phone"] != "[REDACTED]" {
		t.Errorf("Contexts.caller.crew[0] not scrubbed: %+v", entry)
	}

	// User: ID kept, email/IP/username masked.
	if scrubbed.User.ID != "user-id-1" {
		t.Errorf("User.ID altered (kept for impact metrics): %q", scrubbed.User.ID)
	}
	if scrubbed.User.Email != "[REDACTED]" {
		t.Errorf("User.Email altered: %q", scrubbed.User.Email)
	}
	if scrubbed.User.IPAddress != "[REDACTED]" {
		t.Errorf("User.IPAddress altered: %q", scrubbed.User.IPAddress)
	}
	if scrubbed.User.Username != "[REDACTED]" {
		t.Errorf("User.Username altered: %q", scrubbed.User.Username)
	}

	// Request
	if scrubbed.Request.Cookies != "" {
		t.Errorf("Request.Cookies should be cleared, got %q", scrubbed.Request.Cookies)
	}
	if scrubbed.Request.Headers["Authorization"] != "[REDACTED]" {
		t.Errorf("Authorization header should be redacted, got %q", scrubbed.Request.Headers["Authorization"])
	}
	if scrubbed.Request.Headers["X-Request-ID"] != "req-abc" {
		t.Errorf("X-Request-ID header should pass through; got %q", scrubbed.Request.Headers["X-Request-ID"])
	}
	if !strings.Contains(scrubbed.Request.Data, "[REDACTED]") {
		t.Errorf("Request.Data not scrubbed: %q", scrubbed.Request.Data)
	}
}

func TestScrubSentryEvent_NilSafe(t *testing.T) {
	if got := scrubSentryEvent(nil, pii.Restricted); got != nil {
		t.Errorf("nil event should return nil; got %v", got)
	}
}

func TestScrubSentryEvent_EmptyEventNoOp(t *testing.T) {
	// Defensive: scrubbing an event with no PII-bearing fields
	// should not error or produce changes.
	event := &sentry.Event{Message: "hello world"}
	out := scrubSentryEvent(event, pii.Restricted)
	if out.Message != "hello world" {
		t.Errorf("non-PII fields should pass; got %q", out.Message)
	}
}

// errAs is a tiny helper so we can build a wrapped error without the
// fmt import at the top of the file.
func errAs(format string, err error) error {
	return wrappedErr{format: format, inner: err}
}

type wrappedErr struct {
	format string
	inner  error
}

func (w wrappedErr) Error() string { return w.format }
func (w wrappedErr) Unwrap() error { return w.inner }
