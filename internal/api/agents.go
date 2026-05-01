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

	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "user not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
