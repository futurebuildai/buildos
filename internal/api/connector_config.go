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

// ConnectorServicer is the subset of *service.ConnectorService consumed by
// ConnectorHandler. The handler depends on the interface so unit tests can
// substitute a fake (matches AgentConfigServicer / IntegrationsServicer).
type ConnectorServicer interface {
	ListEffective(ctx context.Context, orgID uuid.UUID) ([]service.EffectiveConnector, error)
	Set(ctx context.Context, in service.SetConnectorInput) (models.ConnectorConfig, error)
	Reset(ctx context.Context, orgID uuid.UUID, connectorName, userSub string) error
}

// ConnectorHandler exposes /api/v1/admin/connectors — the admin-gated integration
// connector registry (Phase 3b). It lets an operator enable/disable and configure
// each connector per org, post-deploy, no redeploy. Connectors are DEFAULT-OFF.
// Mounted OFF the pro-tier /api/v1/agents tree so the kill-switch is reachable
// regardless of plan tier.
type ConnectorHandler struct {
	svc ConnectorServicer
}

// NewConnectorHandler binds the handler to a ConnectorServicer.
func NewConnectorHandler(svc ConnectorServicer) *ConnectorHandler {
	return &ConnectorHandler{svc: svc}
}

// setConnectorRequest is the PUT body. Full-document semantics: enabled is
// authoritative; an omitted/null config resets to the empty object.
type setConnectorRequest struct {
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config,omitempty"`
}

// ---------- GET /api/v1/admin/connectors ----------

// List returns every built-in connector with its effective config for the
// caller's org (default-OFF, or an override). Admin RBAC enforced by the route.
func (h *ConnectorHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	conns, err := h.svc.ListEffective(r.Context(), orgID)
	if err != nil {
		writeConnectorError(w, r, err)
		return
	}
	if conns == nil {
		conns = []service.EffectiveConnector{}
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"connectors": conns})
}

// ---------- PUT /api/v1/admin/connectors/{connector} ----------

// Set upserts the override (enabled + config) for a connector. 404 if the
// connector is unknown to the catalog; 400 on a malformed config object.
func (h *ConnectorHandler) Set(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	connector := chi.URLParam(r, "connector")
	claims := mw.MustClaimsFromContext(r.Context())

	var body setConnectorRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	conn, err := h.svc.Set(r.Context(), service.SetConnectorInput{
		OrgID:         orgID,
		ConnectorName: connector,
		Enabled:       body.Enabled,
		Config:        body.Config,
		UserSub:       claims.Sub,
	})
	if err != nil {
		writeConnectorError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"connector": conn})
}

// ---------- DELETE /api/v1/admin/connectors/{connector} ----------

// Reset removes the override row for a connector (resetting to default-OFF).
// Idempotent 204 whether or not an override existed; 404 only when the connector
// is unknown to the catalog.
func (h *ConnectorHandler) Reset(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	connector := chi.URLParam(r, "connector")
	claims := mw.MustClaimsFromContext(r.Context())

	if err := h.svc.Reset(r.Context(), orgID, connector, claims.Sub); err != nil {
		writeConnectorError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeConnectorError maps service sentinels to HTTP responses:
//   - service.ErrNotFound      -> 404 (connector unknown to the catalog)
//   - service.ErrInvalidInput  -> 400 (malformed config)
//   - anything else            -> 500 (opaque)
func writeConnectorError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountConnectorRoutes registers the admin connector surface under
// /api/v1/admin/connectors, admin-gated. Mounted inside the authenticated +
// SetupGate group by NewRouter (so it 403s SETUP_INCOMPLETE pre-onboarding).
func MountConnectorRoutes(r chi.Router, h *ConnectorHandler) {
	r.Route("/api/v1/admin/connectors", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleAdmin))
		r.Get("/", h.List)
		r.Put("/{connector}", h.Set)
		r.Delete("/{connector}", h.Reset)
	})
}
