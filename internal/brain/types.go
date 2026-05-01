package brain

import (
	"encoding/json"
	"errors"
	"fmt"
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
type RetryConfig struct {
	MaxAttempts int     // default 3
	BaseDelayMs int     // default 100
	Multiplier  float64 // default 2.0
}

func (rc RetryConfig) withDefaults() RetryConfig {
	if rc.MaxAttempts <= 0 {
		rc.MaxAttempts = 3
	}
	if rc.BaseDelayMs <= 0 {
		rc.BaseDelayMs = 100
	}
	if rc.Multiplier <= 0 {
		rc.Multiplier = 2.0
	}
	return rc
}
