package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/brain"
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

// RecommendScheduleAdjustments asks Brain Maestro to suggest duration
// nudges for the project's task graph, applies them through
// ScheduleStore.UpdateTask, and re-runs CPM physics so the critical
// path / floats stay coherent. The audit trail (one batch
// "schedule.maestro_edit" row + the recalc's own row) is written by
// the service.
//
// POST /api/v1/projects/{projectID}/schedule/recommend-adjustments
//
// Role gate: superintendent or higher (CPM-affecting; matches the
// gate on /schedule/recalculate). Plan-tier gate: pro tier or higher
// (Maestro consumes metered AI tokens). Both applied at the route in
// router.go.
//
// Errors:
//
//   - 400 VALIDATION_ERROR: invalid project_id / project has no tasks /
//     missing required claim → ErrInvalidInput
//   - 404 NOT_FOUND: project not in caller's org → ErrNotFound
//   - 503 SERVICE_UNAVAILABLE: AgentsService constructed without the
//     Maestro adjuster or schedule trio (worker binary path) →
//     ErrAgentsMaestroUnavailable / ErrAgentsScheduleServiceUnavailable
//   - 502 UPSTREAM_ERROR: Brain transient / 5xx
//   - 401 UNAUTHORIZED: Brain rejected token
//
// On success: 200 with the full ScheduleAdjustmentSet (run_id,
// tokens_used, cost_cents, currency_code, adjustments, applied_deltas,
// skipped_rationale_only). The "apply succeeded; recalc deferred"
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
		// presence of an Adjustments slice on the result.
		if result.RunID != uuid.Nil {
			writeJSON(w, r, http.StatusOK, result)
			return
		}
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// writeServiceError maps AgentsService sentinels + Brain errors to
// HTTP responses.
func (h *AgentsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	// Brain errors first — they wrap inside service errors.
	var httpErr *brain.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusUnauthorized:
			writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "brain rejected token")
			return
		case httpErr.StatusCode >= 500:
			writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "brain upstream unavailable")
			return
		}
	}
	if errors.Is(err, brain.ErrTransient) {
		writeErrorResponse(w, r, http.StatusBadGateway, "UPSTREAM_ERROR", "brain upstream transient error")
		return
	}
	if errors.Is(err, brain.ErrUnauthenticated) {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing brain token")
		return
	}

	// AgentsService nil-dep sentinels — RecommendScheduleAdjustments
	// returns these when the service was constructed without a
	// MaestroScheduleAdjuster / ScheduleService (the worker binary).
	// Map to 503 so callers know to retry against a server binary
	// rather than treating it as a permanent input error.
	if errors.Is(err, service.ErrAgentsMaestroUnavailable) || errors.Is(err, service.ErrAgentsScheduleServiceUnavailable) {
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
