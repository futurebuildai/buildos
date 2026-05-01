package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/futurebuildai/buildos/internal/brain"
)

// BillingClient is the consumer-side surface BillingHandler needs.
// Defined here so the handler stays free of internal/brain at the
// type level — the dependency is only on the methods, which lets
// tests substitute a fake without touching HTTP.
type BillingClient interface {
	GetUsageSummary(ctx context.Context, r brain.UsageRange) (*brain.UsageSummary, error)
	GetDailyUsage(ctx context.Context, r brain.UsageRange) (*brain.DailyUsageResponse, error)
}

// BillingHandler exposes The Brain's billing endpoints to BuildOS
// callers. The Bearer token is plumbed via context (auth middleware
// stashes it), so the handler just forwards request scope and lets
// Brain own the org isolation.
type BillingHandler struct {
	client BillingClient
}

// NewBillingHandler creates a handler bound to the given Brain client.
func NewBillingHandler(client BillingClient) *BillingHandler {
	return &BillingHandler{client: client}
}

// Usage returns aggregated AI-token usage for the caller's org over a
// window (defaults to current calendar month if start/end omitted).
//
// GET /api/v1/billing/usage[?start=RFC3339&end=RFC3339]
func (h *BillingHandler) Usage(w http.ResponseWriter, r *http.Request) {
	rng, ok := parseUsageRange(w, r)
	if !ok {
		return
	}

	resp, err := h.client.GetUsageSummary(r.Context(), rng)
	if err != nil {
		writeBrainError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"usage": resp})
}

// DailyUsage returns per-day usage rows over a window for chart
// rendering.
//
// GET /api/v1/billing/usage/daily[?start=RFC3339&end=RFC3339]
func (h *BillingHandler) DailyUsage(w http.ResponseWriter, r *http.Request) {
	rng, ok := parseUsageRange(w, r)
	if !ok {
		return
	}

	resp, err := h.client.GetDailyUsage(r.Context(), rng)
	if err != nil {
		writeBrainError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"daily": resp})
}

// parseUsageRange reads ?start=&end= as RFC3339 timestamps. Empty/
// missing means "let Brain pick a default" (current month). Returns
// false after writing a 400 if either timestamp is malformed.
func parseUsageRange(w http.ResponseWriter, r *http.Request) (brain.UsageRange, bool) {
	var rng brain.UsageRange
	q := r.URL.Query()
	if s := q.Get("start"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "start must be RFC3339")
			return brain.UsageRange{}, false
		}
		rng.Start = t
	}
	if s := q.Get("end"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "end must be RFC3339")
			return brain.UsageRange{}, false
		}
		rng.End = t
	}
	if !rng.Start.IsZero() && !rng.End.IsZero() && !rng.End.After(rng.Start) {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "end must be after start")
		return brain.UsageRange{}, false
	}
	return rng, true
}

// writeBrainError maps a brain.HTTPError or sentinel back to an
// appropriate HTTP response. Brain errors should ALWAYS be reflected
// to the caller — these are pass-through reads. Two reflected status
// classes:
//
//	401 → the caller's token was rejected by Brain. Surface 401 so the
//	      frontend can prompt for re-auth.
//	5xx → Brain is degraded. Surface 502 so the caller knows it's
//	      upstream, not BuildOS.
//
// Anything else lands as 500 with the original error message logged
// (but not surfaced to avoid leaking internal detail).
func writeBrainError(w http.ResponseWriter, r *http.Request, err error) {
	var httpErr *brain.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusUnauthorized:
			writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "brain rejected token")
			return
		case httpErr.StatusCode == http.StatusForbidden:
			writeErrorResponse(w, r, http.StatusForbidden, "FORBIDDEN", "brain access denied")
			return
		case httpErr.StatusCode == http.StatusNotFound:
			writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "brain resource not found")
			return
		case httpErr.StatusCode >= 500:
			writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "brain upstream unavailable")
			return
		case httpErr.StatusCode >= 400:
			writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", httpErr.Message)
			return
		}
	}
	if errors.Is(err, brain.ErrUnauthenticated) {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing brain token")
		return
	}
	if errors.Is(err, brain.ErrTransient) {
		writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "brain upstream transient error")
		return
	}
	writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
}
