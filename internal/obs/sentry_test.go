package obs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/futurebuildai/buildos/internal/brain"
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
