package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/ai"
	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/service"
)

// AgentsServicer is the consumer-side interface AgentsHandler needs.
type AgentsServicer interface {
	GenerateDailyBriefing(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject, callerRole string) (service.DailyBriefing, error)
	RecommendScheduleAdjustments(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID) (service.ScheduleAdjustmentSet, error)
}

// AgentsHandler exposes BuildOS's AI-agent endpoints. Each handler is
// gated on a minimum plan tier at the route level.
type AgentsHandler struct {
	svc AgentsServicer
}

// NewAgentsHandler creates a handler bound to the given service.
func NewAgentsHandler(svc AgentsServicer) *AgentsHandler {
	return &AgentsHandler{svc: svc}
}

// DailyBriefing returns a Maestro-generated morning briefing for the
// authenticated caller. Synchronous endpoint — the mobile app calls
// this on app launch.
//
// POST /api/v1/agents/daily-briefing
//
// Plan gate: pro tier or higher (applied at the route in router.go).
func (h *AgentsHandler) DailyBriefing(w http.ResponseWriter, r *http.Request) {
	claims := mw.MustClaimsFromContext(r.Context())
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
		return
	}

	briefing, err := h.svc.GenerateDailyBriefing(r.Context(), orgID, claims.Sub, claims.Role)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"briefing": briefing})
}

// RecommendScheduleAdjustments asks the native AI client to suggest
// duration nudges for the project's task graph, applies them through
// ScheduleStore.UpdateTask, and re-runs CPM physics so the critical
// path / floats stay coherent. The audit trail (one batch
// "schedule.maestro_edit" row + the recalc's own row) is written by
// the service.
//
// POST /api/v1/projects/{projectID}/schedule/recommend-adjustments
//
// Role gate: superintendent or higher (CPM-affecting; matches the
// gate on /schedule/recalculate). Plan-tier gate: pro tier or higher
// (AI consumes the org's metered Anthropic key). Both applied at the
// route in router.go.
//
// Errors:
//
//   - 400 VALIDATION_ERROR: invalid project_id / project has no tasks /
//     missing required claim → ErrInvalidInput
//   - 404 NOT_FOUND: project not in caller's org → ErrNotFound
//   - 503 SERVICE_UNAVAILABLE: AgentsService constructed without the
//     AI adjuster or schedule trio (worker binary path) →
//     ErrAgentsAIUnavailable / ErrAgentsScheduleServiceUnavailable; or
//     no Anthropic key configured for the org → ai.ErrUnconfigured
//   - 429 RATE_LIMITED: AI provider rate limited
//   - 502 UPSTREAM_ERROR: AI provider transient / 5xx
//
// On success: 200 with the full ScheduleAdjustmentSet (adjustments,
// applied_deltas, skipped_rationale_only). The "apply succeeded; recalc deferred"
// case from the service is mapped to 200 OK with the result body —
// returning 5xx would mislead the caller into thinking the deltas
// weren't applied (they were; only the CPM re-run was deferred).
// Future: surface a "recalc_deferred" boolean on the response so the
// frontend can hint at it; for now the next /schedule/recalculate
// catches up.
func (h *AgentsHandler) RecommendScheduleAdjustments(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())

	result, err := h.svc.RecommendScheduleAdjustments(r.Context(), callerOrg, claims.Sub, projectID)
	if err != nil {
		// "apply succeeded; recalc deferred: ..." carries a non-nil
		// result + wrapped error from the service. Surface the
		// applied deltas to the caller (200) — the recalc lag will
		// resolve at next /schedule/recalculate. We detect this by
		// the presence of applied deltas on the result (the only path
		// that defers a recalc).
		if result.AppliedDeltas > 0 {
			writeJSON(w, r, http.StatusOK, result)
			return
		}
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// writeServiceError maps AgentsService sentinels + native AI errors to
// HTTP responses. It delegates to the shared free function
// writeAIServiceError so the AssistantHandler reuses the identical map.
func (h *AgentsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	writeAIServiceError(w, r, err)
}

// writeAIServiceError maps native-AI + agentic + AgentsService sentinels to
// HTTP responses. Shared by AgentsHandler.writeServiceError and
// AssistantHandler.Converse so both AI surfaces soft-fail identically:
//
//   - ai.ErrUnconfigured / agentic.ErrAssistantUnavailable          → 503
//   - *ai.HTTPError{401} (bad stored key) / *ai.HTTPError{>=500}     → 503 / 502
//   - ai.ErrRateLimited                                             → 429
//   - ai.ErrTransient / ai.ErrCircuitOpen                           → 502
//   - service.ErrAgentsAIUnavailable / ScheduleServiceUnavailable   → 503
//   - service.ErrNotFound                                           → 404
//   - service.ErrInvalidInput                                       → 400
//   - anything else                                                 → 500 (opaque)
func writeAIServiceError(w http.ResponseWriter, r *http.Request, err error) {
	// agentic.ErrAssistantUnavailable (no AI client wired / no Anthropic key
	// resolved for the org) — a configuration gap, not an outage: 503 so the
	// operator knows to set a key in the vault.
	if errors.Is(err, agentic.ErrAssistantUnavailable) {
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "assistant is not available")
		return
	}

	// Native AI errors next — they wrap inside service errors.
	//
	// ErrUnconfigured (no Anthropic key set for the org) is a
	// configuration gap, not an outage: surface 503 so the operator
	// knows to set a key in the vault. Rate-limit / transient / circuit
	// map to the usual upstream codes.
	if errors.Is(err, ai.ErrUnconfigured) {
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "AI is not configured for this organization")
		return
	}
	if errors.Is(err, ai.ErrRateLimited) {
		writeErrorResponse(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "AI provider rate limited")
		return
	}
	if errors.Is(err, ai.ErrTransient) || errors.Is(err, ai.ErrCircuitOpen) {
		writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "AI provider transient error")
		return
	}
	var httpErr *ai.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusUnauthorized:
			// 401 from Anthropic means the stored key is bad — an
			// operator-fixable configuration problem, not a caller
			// auth failure. Surface 503 so the client doesn't treat
			// it as its own token being rejected.
			writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "AI provider rejected the configured key")
			return
		case httpErr.StatusCode >= 500:
			writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "AI provider upstream unavailable")
			return
		}
	}

	// AgentsService nil-dep sentinels — RecommendScheduleAdjustments
	// returns these when the service was constructed without a
	// ScheduleAdjuster / ScheduleService (the worker binary). Map to
	// 503 so callers know to retry against a server binary rather than
	// treating it as a permanent input error.
	if errors.Is(err, service.ErrAgentsAIUnavailable) || errors.Is(err, service.ErrAgentsScheduleServiceUnavailable) {
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "agent flow not available on this binary")
		return
	}

	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
