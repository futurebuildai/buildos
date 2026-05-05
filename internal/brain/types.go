package brain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Sentinel errors returned by the Brain client. Service-layer callers
// can use errors.Is to map these to domain-specific responses.
var (
	// ErrUnauthenticated is returned when the request reaches Brain
	// without a valid token (e.g., context missing one).
	ErrUnauthenticated = errors.New("brain: unauthenticated")

	// ErrNotFound is returned when Brain responds with HTTP 404. The
	// HTTPError it wraps has the original code/message for inspection.
	ErrNotFound = errors.New("brain: resource not found")

	// ErrTransient is returned when Brain responded with a 5xx after
	// the client exhausted its retry budget. Surface as 502/503 in the
	// caller's API.
	ErrTransient = errors.New("brain: upstream transient failure")

	// ErrCircuitOpen is returned when the brain client's circuit
	// breaker is open and refusing to forward calls. Surface as 503
	// (service unavailable) in the caller's API. Distinct from
	// ErrTransient so service-layer code can choose to skip the
	// fallback retry loop entirely while the breaker is tripped.
	ErrCircuitOpen = errors.New("brain: circuit breaker open")
)

// HTTPError captures a non-2xx response from Brain in a typed way.
// Implements error so callers can switch on errors.As(err, &HTTPError{}).
type HTTPError struct {
	StatusCode int    `json:"status_code"`
	Code       string `json:"code"`    // Brain's error envelope code (e.g. "INVALID_BODY")
	Message    string `json:"message"` // Brain's human-readable message
	RequestID  string `json:"request_id"`
}

func (e *HTTPError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("brain: HTTP %d %s: %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("brain: HTTP %d", e.StatusCode)
}

// Is matches HTTPError against the sentinels above by status code so
// callers can write `if errors.Is(err, brain.ErrNotFound)` without
// having to type-assert.
func (e *HTTPError) Is(target error) bool {
	switch target {
	case ErrUnauthenticated:
		return e.StatusCode == 401
	case ErrNotFound:
		return e.StatusCode == 404
	case ErrTransient:
		return e.StatusCode >= 500
	}
	return false
}

// envelope mirrors Brain's standard response envelope:
// {"data": ..., "error": ..., "meta": {...}}. The client unwraps `data`
// before returning to callers.
type envelope struct {
	Data  json.RawMessage `json:"data,omitempty"`
	Error *errorBody      `json:"error,omitempty"`
	Meta  *metaBlock      `json:"meta,omitempty"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type metaBlock struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

// RetryConfig controls the exponential-backoff policy used by doRequest.
// Zero values are treated as "use default".
//
// Defaults are tuned to S1.5 spec: 3 attempts with 200ms→800ms→3.2s
// inter-attempt delays. Multiplier=4 spaces probes far enough apart
// that a brain restart (typical 1–3s) is bridged without exhausting
// the budget on a single request.
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

// Timeouts configures per-method context deadlines. Each sub-client
// wraps the caller's ctx with WithTimeout(ctx, <timeout>) before
// invoking doRequest. Zero values fall back to defaults.
//
// Why per-method rather than a single client-wide timeout: Maestro
// LLM round-trips are long (5–30s typical, 60s tail) while billing
// reads are sub-second; binding both to a single ceiling either
// strangles long calls or wastes user time on short ones that have
// hung.
type Timeouts struct {
	// Maestro applies to MaestroClient methods (chat, sessions). Default 30s.
	Maestro time.Duration
	// Billing applies to BillingClient methods (usage). Default 5s.
	Billing time.Duration
}

func (t Timeouts) withDefaults() Timeouts {
	if t.Maestro <= 0 {
		t.Maestro = 30 * time.Second
	}
	if t.Billing <= 0 {
		t.Billing = 5 * time.Second
	}
	return t
}
