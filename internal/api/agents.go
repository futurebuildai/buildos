package api

import (
	"context"
	"encoding/json"
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
	RecommendScheduleAdjustments(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID, dryRun bool) (service.ScheduleAdjustmentSet, error)
	ApplyScheduleAdjustments(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID, applies []service.ScheduleAdjustmentApply) (service.ScheduleApplyResult, error)
}

// AgentsHandler exposes BuildOS's AI-agent endpoints. Access is role-gated
// at the route level (the pro plan-tier gate was removed in ESC-002).
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
// Auth: any authenticated role (the caller's own briefing; the mobile app
// calls it on launch). The pro plan-tier gate was removed in ESC-002.
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

// RecommendScheduleAdjustments asks the native AI client to PROPOSE
// duration nudges for the project's task graph. PREVIEW-FIRST
// (ESC-AUX-01 — AI proposes, human commits):
//
// POST /api/v1/projects/{projectID}/schedule/recommend-adjustments?dry_run=true
//
//   - dry_run=true (the "Suggest adjustments" UI path): returns enriched
//     per-row proposals (wbs_code, name, old/new duration, rationale,
//     critical-path) and MUTATES NOTHING. The user then commits selected
//     rows via the sibling apply endpoint.
//   - dry_run omitted / false (legacy auto-apply): applies every in-range
//     numeric delta + re-runs CPM, writing a schedule.maestro_edit audit row.
//
// Role gate: superintendent or higher (CPM-affecting; matches the gate on
// /schedule/recalculate), applied at the route in router.go.
//
// Errors:
//
//   - 400 VALIDATION_ERROR: invalid project_id / project has no tasks /
//     missing required claim → ErrInvalidInput
//   - 404 NOT_FOUND: project not in caller's org → ErrNotFound
//   - 503 SERVICE_UNAVAILABLE: AgentsService constructed without the AI
//     adjuster or schedule trio (worker binary path); or no Anthropic key
//     configured → ai.ErrUnconfigured
//   - 429 RATE_LIMITED / 502 UPSTREAM_ERROR: AI provider transient
//
// On success: 200 with the full ScheduleAdjustmentSet. The legacy
// "apply succeeded; recalc deferred" case is mapped to 200 OK with the
// result body (the deltas were applied; only the CPM re-run was deferred).
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

	dryRun := r.URL.Query().Get("dry_run") == "true"

	result, err := h.svc.RecommendScheduleAdjustments(r.Context(), callerOrg, claims.Sub, projectID, dryRun)
	if err != nil {
		// "apply succeeded; recalc deferred: ..." carries a non-nil
		// result + wrapped error from the service (real path only).
		// Surface the applied deltas to the caller (200) — the recalc
		// lag resolves at next /schedule/recalculate.
		if !result.DryRun && result.AppliedDeltas > 0 {
			writeJSON(w, r, http.StatusOK, result)
			return
		}
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, result)
}

// applyAdjustmentRow is one user-selected duration change in the apply
// request body. WBSCode identifies the task; NewDurationDays is the
// duration to write.
type applyAdjustmentRow struct {
	WBSCode         string `json:"wbs_code"`
	NewDurationDays int    `json:"new_duration_days"`
}

// applyAdjustmentsRequest is the POST body for ApplyScheduleAdjustments.
type applyAdjustmentsRequest struct {
	Adjustments []applyAdjustmentRow `json:"adjustments"`
}

// ApplyScheduleAdjustments commits the user-selected duration changes from
// a dry-run preview in one tx and re-runs CPM (PREVIEW-FIRST, ESC-AUX-01).
//
// POST /api/v1/projects/{projectID}/schedule/adjustments/apply
// Body: { adjustments: [{ wbs_code, new_duration_days }] }
//
// Role gate: superintendent or higher (CPM-affecting; same gate as
// /recommend-adjustments and /recalculate), applied at the route.
//
// Errors:
//
//   - 400 VALIDATION_ERROR: empty body, unknown/duplicate wbs_code, or a
//     duration outside [1, 36500] → ErrInvalidInput
//   - 404 NOT_FOUND: project not in caller's org → ErrNotFound
//   - 503 SERVICE_UNAVAILABLE: schedule trio not wired (worker binary)
//
// On success: 200 with { applied_deltas, critical_recomputed }. The
// recalc-deferred case still returns 200 (deltas were applied).
func (h *AgentsHandler) ApplyScheduleAdjustments(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())

	var body applyAdjustmentsRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	if len(body.Adjustments) == 0 {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "at least one adjustment is required")
		return
	}

	applies := make([]service.ScheduleAdjustmentApply, 0, len(body.Adjustments))
	for _, a := range body.Adjustments {
		applies = append(applies, service.ScheduleAdjustmentApply{
			WBSCode:         a.WBSCode,
			NewDurationDays: a.NewDurationDays,
		})
	}

	result, err := h.svc.ApplyScheduleAdjustments(r.Context(), callerOrg, claims.Sub, projectID, applies)
	if err != nil {
		// recalc-deferred: deltas applied, CPM re-run deferred → 200.
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
//   - ai.ErrCircuitOpen (breaker tripped, transient)                → 503 + Retry-After
//   - ai.ErrTransient                                               → 502
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

	// agentic.ErrCapabilityDisabled — an admin deliberately turned the
	// capability off via the agent config registry (Phase 3a). This is a
	// configuration STATE, not an outage: 403 with a distinct code so clients
	// can tell "admin disabled this" apart from a 503 (key missing / AI down)
	// or an RBAC 403.
	if errors.Is(err, agentic.ErrCapabilityDisabled) {
		writeErrorResponse(w, r, http.StatusForbidden, "CAPABILITY_DISABLED", "this capability is disabled for your organization")
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
	// Circuit open: a transient self-protection state, distinct from a config
	// gap — surface 503 + a Retry-After equal to the remaining open window so
	// the client backs off and retries (the documented contract for
	// ai.ErrCircuitOpen). Must precede the ErrTransient leg.
	if errors.Is(err, ai.ErrCircuitOpen) {
		ra := ai.DefaultOpenDuration
		var coErr *ai.CircuitOpenError
		if errors.As(err, &coErr) && coErr.RetryAfter > 0 {
			ra = coErr.RetryAfter
		}
		writeErrorResponseRetry(w, r, http.StatusServiceUnavailable, "AI_CIRCUIT_OPEN", "AI provider temporarily unavailable (circuit open); retry after the indicated delay", ra)
		return
	}
	if errors.Is(err, ai.ErrTransient) {
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
