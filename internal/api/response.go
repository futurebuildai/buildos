package api

import (
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// envelope wraps all API responses in the standard format.
type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
	Meta  meta      `json:"meta"`
}

type meta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

type apiError struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []fieldError `json:"details,omitempty"`
}

type fieldError struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Data: data,
		Meta: buildMeta(r),
	})
}

func writeErrorResponse(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeErr(w, r, status, code, message, 0)
}

// writeErrorResponseRetry is writeErrorResponse plus a Retry-After header
// (rounded up to whole seconds, min 1 per RFC 7231). Used for transient 429/503
// responses (rate limit, AI circuit-open) so clients back off correctly.
func writeErrorResponseRetry(w http.ResponseWriter, r *http.Request, status int, code, message string, retryAfter time.Duration) {
	writeErr(w, r, status, code, message, retryAfter)
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, message string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds(retryAfter)))
	}
	w.Header().Set("Content-Type", "application/json")
	// Observe every error response (Phase 4b-iii). Nil until the router wires
	// the metrics recorder; harmless when metrics are disabled.
	if obs := errResponseObserver; obs != nil {
		obs(code, statusClass(status))
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{
		Error: &apiError{Code: code, Message: message},
		Meta:  buildMeta(r),
	})
}

// errResponseObserver is set by NewRouter when metrics are enabled; it counts
// error responses by {code, status-class}. Package-level because the error
// writers are plain functions with no metrics handle.
var errResponseObserver func(code, statusClass string)

// retryAfterSeconds rounds d UP to whole seconds, min 1 (RFC 7231 delta-seconds).
func retryAfterSeconds(d time.Duration) int {
	s := int(math.Ceil(d.Seconds()))
	if s < 1 {
		s = 1
	}
	return s
}

// statusClass buckets an HTTP status into its class for bounded metric
// cardinality ("4xx"/"5xx"/...).
func statusClass(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	case status >= 300:
		return "3xx"
	case status >= 200:
		return "2xx"
	default:
		return "other"
	}
}

func buildMeta(r *http.Request) meta {
	reqID := chimw.GetReqID(r.Context())
	return meta{
		RequestID: reqID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
