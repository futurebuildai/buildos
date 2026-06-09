package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// AgentConfigServicer is the subset of *service.AgentConfigService consumed by
// AgentConfigHandler. The handler depends on the interface so unit tests can
// substitute a fake (matches IntegrationsServicer / SetupServicer).
type AgentConfigServicer interface {
	ListEffective(ctx context.Context, orgID uuid.UUID) ([]service.EffectiveAgentConfig, error)
	Set(ctx context.Context, in service.SetAgentConfigInput) (models.AgentConfig, error)
	Reset(ctx context.Context, orgID uuid.UUID, capability, userSub string) error
}

// AgentConfigHandler exposes the /api/v1/admin/agents surface — the admin-gated
// agent config registry (Phase 3a). It lets an operator enable/disable and tune
// each agentic capability per org, post-deploy, no redeploy. Mounted OFF the
// pro-tier /api/v1/agents tree so the kill-switch is reachable regardless of
// plan tier.
type AgentConfigHandler struct {
	svc AgentConfigServicer
}

// NewAgentConfigHandler binds the handler to an AgentConfigServicer.
func NewAgentConfigHandler(svc AgentConfigServicer) *AgentConfigHandler {
	return &AgentConfigHandler{svc: svc}
}

// setAgentConfigRequest is the PUT body. Full-document semantics: enabled is
// authoritative; an omitted/null config resets the capability's tuning to the
// catalog default.
type setAgentConfigRequest struct {
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// ---------- GET /api/v1/admin/agents ----------

// List returns every catalog capability with its effective config for the
// caller's org (default or override). Admin RBAC enforced by the route group.
func (h *AgentConfigHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	agents, err := h.svc.ListEffective(r.Context(), orgID)
	if err != nil {
		writeAgentConfigError(w, r, err)
		return
	}
	if agents == nil {
		agents = []service.EffectiveAgentConfig{}
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"agents": agents})
}

// ---------- PUT /api/v1/admin/agents/{capability} ----------

// Set upserts the override (enabled + config) for a capability. 404 if the
// capability is unknown to the catalog; 400 on a malformed config object.
func (h *AgentConfigHandler) Set(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	capability := chi.URLParam(r, "capability")
	claims := mw.MustClaimsFromContext(r.Context())

	var body setAgentConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	agent, err := h.svc.Set(r.Context(), service.SetAgentConfigInput{
		OrgID:      orgID,
		Capability: capability,
		Enabled:    body.Enabled,
		Config:     body.Config,
		UserSub:    claims.Sub,
	})
	if err != nil {
		writeAgentConfigError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"agent": agent})
}

// ---------- DELETE /api/v1/admin/agents/{capability} ----------

// Reset removes the override row for a capability (resetting to the catalog
// default). Idempotent 204 whether or not an override existed; 404 only when the
// capability is unknown to the catalog.
func (h *AgentConfigHandler) Reset(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	capability := chi.URLParam(r, "capability")
	claims := mw.MustClaimsFromContext(r.Context())

	if err := h.svc.Reset(r.Context(), orgID, capability, claims.Sub); err != nil {
		writeAgentConfigError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeAgentConfigError maps service sentinels to HTTP responses:
//   - service.ErrNotFound      -> 404 (capability unknown to the catalog)
//   - service.ErrInvalidInput  -> 400 (malformed config)
//   - anything else            -> 500 (opaque)
func writeAgentConfigError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountAgentConfigRoutes registers the admin agent config surface under
// /api/v1/admin/agents, admin-gated. Mounted inside the authenticated +
// SetupGate group by NewRouter (so it 403s SETUP_INCOMPLETE pre-onboarding).
func MountAgentConfigRoutes(r chi.Router, h *AgentConfigHandler) {
	r.Route("/api/v1/admin/agents", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleAdmin))
		r.Get("/", h.List)
		r.Put("/{capability}", h.Set)
		r.Delete("/{capability}", h.Reset)
	})
}
