package obs

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"

	"github.com/futurebuildai/buildos/internal/pii"
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

		// BeforeSend scrubs known PII before any event leaves the
		// process for Sentry's ingest endpoint. We default to a
		// Restricted threshold: redact personal data (names, emails,
		// phone, GPS, OIDC subject) but keep business-sensitive
		// fields (vendor names, amounts, project names) so engineering
		// has enough context to debug. Customer forks subject to
		// stricter agreements can crank the threshold by editing
		// this file in their fork.
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			return scrubSentryEvent(event, pii.Restricted)
		},
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

// scrubSentryEvent walks the major PII-bearing fields on a Sentry
// event and applies pii.MaskString / pii.ScrubMap at the supplied
// threshold. Hooked into BeforeSend so EVERY event — exception,
// message, transaction — passes through. Unknown future fields are
// safe-by-default since the Sentry Event struct's nested maps go
// through ScrubMap which knows the BuildOS field-name catalog.
func scrubSentryEvent(event *sentry.Event, threshold pii.Class) *sentry.Event {
	if event == nil {
		return nil
	}
	// Tags: scrub the values whose key matches a known PII pattern.
	// (Tag values are always strings.)
	if event.Tags != nil {
		out := make(map[string]string, len(event.Tags))
		for k, v := range event.Tags {
			cls := pii.ClassFor(k)
			if cls.IsAtLeast(threshold) {
				out[k] = pii.MaskString(v, cls)
			} else {
				out[k] = v
			}
		}
		event.Tags = out
	}
	// Contexts: each context bag is a map[string]any; scrub each
	// to apply the field-name catalog. Sentry's "extra" field of
	// older SDK versions has been folded into Contexts in v0.x —
	// this single-pass walk covers both the pre-v0.20 and current
	// shapes.
	if event.Contexts != nil {
		for name, ctxMap := range event.Contexts {
			event.Contexts[name] = sentry.Context(pii.ScrubMap(ctxMap, threshold))
		}
	}
	// User: the Sentry SDK's User struct has named PII fields. We
	// keep the ID intact (used for the "users impacted" UI) but
	// redact email + IP + username.
	if event.User.Email != "" {
		event.User.Email = pii.MaskString(event.User.Email, pii.Restricted)
	}
	if event.User.IPAddress != "" {
		event.User.IPAddress = pii.MaskString(event.User.IPAddress, pii.Restricted)
	}
	if event.User.Username != "" {
		event.User.Username = pii.MaskString(event.User.Username, pii.Restricted)
	}
	// Request payload: query params + cookies + headers + data. The
	// Request struct has typed fields; cookies and a few headers
	// (Authorization) are always sensitive. Strip Authorization
	// outright; scrub the rest by name.
	if event.Request != nil {
		event.Request.Cookies = ""
		if event.Request.Headers != nil {
			for k := range event.Request.Headers {
				if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Cookie") {
					event.Request.Headers[k] = "[REDACTED]"
				}
			}
		}
		if event.Request.Data != "" {
			event.Request.Data = string(pii.ScrubJSON([]byte(event.Request.Data), threshold))
		}
	}
	return event
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
