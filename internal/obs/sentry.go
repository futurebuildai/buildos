package obs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
)

// SentryConfig is the subset of operator-controlled Sentry parameters
// the obs package needs. Mirrors the cfg fields in internal/config so
// the cmd/* entrypoints can pass through without a hand-curated
// translation step.
type SentryConfig struct {
	DSN              string
	Environment      string
	Release          string
	TracesSampleRate float64
}

// InitSentry boots the Sentry SDK in process. Returns a Flush function
// to call at shutdown (drains the in-memory queue with a 2s budget) and
// a bool reporting whether Sentry actually initialized. An empty DSN is
// not an error — many environments (local dev, CI) deliberately don't
// have one; we return Init=false so callers can skip wiring the
// middleware on those environments.
//
// Why a flush function instead of init-only: panic captures are async
// (Sentry posts to its ingest endpoint). Without a flush at shutdown,
// the last events before SIGTERM never leave the process. The 2s budget
// matches what the SDK uses internally and is well below k8s's typical
// terminationGracePeriod.
func InitSentry(cfg SentryConfig, logger *slog.Logger) (flush func(), initialized bool) {
	if cfg.DSN == "" {
		logger.Info("sentry: DSN unset, skipping initialization")
		return func() {}, false
	}
	if cfg.Environment == "" {
		cfg.Environment = "dev"
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		TracesSampleRate: cfg.TracesSampleRate,
		// AttachStacktrace=true so non-panic CaptureException calls
		// also carry a stack — much cheaper to debug than just an
		// error message string.
		AttachStacktrace: true,
		// EnableTracing is implicit when TracesSampleRate > 0.
	})
	if err != nil {
		logger.Error("sentry: init failed; continuing without",
			"error", err,
			"environment", cfg.Environment)
		return func() {}, false
	}

	logger.Info("sentry initialized",
		"environment", cfg.Environment,
		"release", cfg.Release,
		"traces_rate", cfg.TracesSampleRate)

	return func() {
		// 2s flush budget aligns with the worker's River.Stop budget
		// and the server's HTTP shutdown deadline.
		sentry.Flush(2 * time.Second)
	}, true
}

// SentryHTTPMiddleware wraps the chi router stack so panics propagated
// up by chi.Recoverer are captured. Mounts:
//   - HTTP request hub (per-request scope) so CaptureException calls
//     in handlers tag the right request.
//   - Repanic = true so chi.Recoverer still gets the panic and writes
//     the 500 response. (If we set repanic=false, Sentry would
//     swallow the panic and the client would hang.)
func SentryHTTPMiddleware() func(http.Handler) http.Handler {
	handler := sentryhttp.New(sentryhttp.Options{
		Repanic:         true,
		WaitForDelivery: false,
		Timeout:         3 * time.Second,
	})
	return func(next http.Handler) http.Handler {
		return handler.Handle(next)
	}
}

// CaptureError reports a non-panic error to Sentry, attaching common
// tags (request_id from ctx + any caller-supplied tags). Returns the
// event ID so callers can include it in error responses.
//
// Non-blocking: Sentry's CaptureException posts to a buffered channel.
// A full channel drops the event silently rather than blocking the
// hot path.
//
// Pass nil tags for the common case. Use the variadic form when the
// caller wants extra context: tags["org_id"] = orgID.String(), etc.
func CaptureError(ctx context.Context, err error, tags map[string]string) string {
	if err == nil {
		return ""
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		// Tag with request_id so Sentry events correlate to the same
		// id stamped on logs + Brain hops.
		if reqID := requestIDFromContext(ctx); reqID != "" {
			scope.SetTag("request_id", reqID)
		}
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		hub.CaptureException(err)
	})
	// Returning the latest event id from the hub — useful for support
	// flows ("send me your error id and we'll look it up").
	if id := hub.LastEventID(); id != "" {
		return string(id)
	}
	return ""
}

// CaptureMessage reports a non-error event (e.g. a notable log line
// the operator wants in Sentry). level should be one of
// sentry.LevelDebug / Info / Warning / Error / Fatal.
func CaptureMessage(ctx context.Context, level sentry.Level, message string, tags map[string]string) {
	if message == "" {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(level)
		if reqID := requestIDFromContext(ctx); reqID != "" {
			scope.SetTag("request_id", reqID)
		}
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		hub.CaptureMessage(message)
	})
}

// CaptureUnless is a convenience wrapper that only reports `err` when
// it is NOT one of the supplied "ignore" sentinels. Pattern: a
// caller's expected errors (NotFound, ValidationError) shouldn't pollute
// Sentry, but everything else should.
//
// Usage:
//
//	obs.CaptureUnless(ctx, err, nil, service.ErrNotFound, service.ErrInvalidInput)
func CaptureUnless(ctx context.Context, err error, tags map[string]string, ignore ...error) string {
	if err == nil {
		return ""
	}
	for _, ig := range ignore {
		if errors.Is(err, ig) {
			return ""
		}
	}
	return CaptureError(ctx, err, tags)
}
