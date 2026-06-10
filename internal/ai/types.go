// Package ai is BuildOS's native client for the Anthropic Messages API.
//
// It replaces the Brain "Maestro" AI gateway with direct calls to
// Anthropic. Every AI feature (daily briefing, intent classification,
// invoice extraction, procurement recommendations, tribunal review,
// schedule updates) is a single-shot, discriminated method on *Client.
//
// The package owns its own resilience (circuit breaker + exponential
// backoff retry) and is deliberately self-contained: it depends only on
// the Go standard library, otelhttp (already a project dependency), and
// google/uuid. It carries NO dependency on internal/obs (metrics are
// reported through a nil-safe MetricsObserver interface defined here) and
// NO billing / token / cost metadata (those were Maestro/Brain concerns).
//
// The Anthropic API key is never held by this package; callers supply a
// KeyResolver that resolves the per-org key at call time. When the
// resolver returns an empty key (or an error), methods return
// ErrUnconfigured.
package ai

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors returned by the AI client. Service-layer callers can
// use errors.Is to map these to domain-specific responses.
var (
	// ErrUnconfigured is returned when no Anthropic API key is
	// available for the org (the KeyResolver returned an empty string
	// or failed). Surface as a 503/feature-disabled in the caller's
	// API — the deployment simply has no AI configured.
	ErrUnconfigured = errors.New("ai: anthropic key unconfigured")

	// ErrTransient is returned when Anthropic responded with a 5xx (or
	// the transport failed) after the client exhausted its retry
	// budget. Surface as 502/503 in the caller's API.
	ErrTransient = errors.New("ai: upstream transient failure")

	// ErrCircuitOpen is returned when the client's circuit breaker is
	// open and refusing to forward calls. Surface as 503. Distinct
	// from ErrTransient so service-layer code can skip its own
	// fallback retry loop entirely while the breaker is tripped.
	// The concrete error is *CircuitOpenError (carrying a Retry-After
	// hint); errors.Is(err, ErrCircuitOpen) still matches it.
	ErrCircuitOpen = errors.New("ai: circuit breaker open")

	// ErrRateLimited is returned when Anthropic responded with HTTP 429
	// after the client exhausted its retry budget (the per-attempt
	// Retry-After hint was honored on the way). Surface as 429 in the
	// caller's API.
	ErrRateLimited = errors.New("ai: rate limited")
)

// DefaultOpenDuration mirrors the circuit breaker's default open window. Used as
// the Retry-After fallback when a breaker-open error carries no explicit hint.
const DefaultOpenDuration = 30 * time.Second

// CircuitOpenError is the concrete error returned when the breaker is open. It
// carries RetryAfter (the remaining open window) so the API layer can emit an
// accurate Retry-After. Is(ErrCircuitOpen) is true, so existing
// errors.Is(err, ErrCircuitOpen) checks keep working.
type CircuitOpenError struct {
	RetryAfter time.Duration
}

func (e *CircuitOpenError) Error() string { return ErrCircuitOpen.Error() }

func (e *CircuitOpenError) Is(target error) bool { return target == ErrCircuitOpen }

// HTTPError captures a non-2xx response from Anthropic in a typed way.
// Implements error so callers can switch on errors.As(err, &HTTPError{}).
//
// Type mirrors Anthropic's error envelope `error.type` discriminator
// (e.g. "invalid_request_error", "rate_limit_error", "overloaded_error").
type HTTPError struct {
	StatusCode int    `json:"status_code"`
	Type       string `json:"type"`    // Anthropic's error.type
	Message    string `json:"message"` // Anthropic's human-readable message
}

func (e *HTTPError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("ai: HTTP %d %s: %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("ai: HTTP %d", e.StatusCode)
}

// Is matches HTTPError against the sentinels above by status code so
// callers can write `if errors.Is(err, ai.ErrRateLimited)` without
// having to type-assert.
func (e *HTTPError) Is(target error) bool {
	switch target {
	case ErrRateLimited:
		return e.StatusCode == 429
	case ErrTransient:
		return e.StatusCode >= 500
	}
	return false
}

// RetryConfig controls the exponential-backoff policy used by the
// messages transport. Zero values are treated as "use default".
//
// Defaults mirror the Brain client's S1.5 tuning: 3 attempts with
// 200ms→800ms→3.2s inter-attempt delays. Multiplier=4 spaces probes far
// enough apart that a brief upstream hiccup is bridged without
// exhausting the budget on a single request.
type RetryConfig struct {
	MaxAttempts int     // default 3
	BaseDelayMs int     // default 200
	Multiplier  float64 // default 4.0
}

func (rc RetryConfig) withDefaults() RetryConfig {
	if rc.MaxAttempts <= 0 {
		rc.MaxAttempts = 3
	}
	if rc.BaseDelayMs <= 0 {
		rc.BaseDelayMs = 200
	}
	if rc.Multiplier <= 0 {
		rc.Multiplier = 4.0
	}
	return rc
}

// MetricsObserver is the consumer-side surface the AI client uses to
// report each completed call. Defined here so the ai package stays free
// of the prometheus/obs deps — and so callers without a metrics setup
// can pass nil.
//
// kind is the task name ("daily_briefing", "invoice_extract", …) or
// "messages" for a raw call; model is the model id dispatched; dur is
// the wall-clock of the attempt that produced outcome; err is non-nil
// when the call ultimately failed (nil on success).
type MetricsObserver interface {
	ObserveAICall(kind, model string, dur time.Duration, err error)
}

// KeyResolver resolves the Anthropic API key for an org at call time.
// The real implementation lives outside this package (e.g. reading from
// the secret source / per-fork config); tests use a stub.
//
// Returning an empty key (with a nil error) is treated identically to
// returning an error: the caller gets ErrUnconfigured and no HTTP round
// trip is made.
type KeyResolver interface {
	AnthropicKey(ctx context.Context, orgID string) (string, error)
}
