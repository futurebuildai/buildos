package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// IntegrationsServicer is the subset of *service.VaultService consumed
// by IntegrationsHandler. The handler depends on the interface so unit
// tests can substitute a fake (matches the SetupServicer pattern).
type IntegrationsServicer interface {
	SetCredential(ctx context.Context, in service.SetCredentialInput) (models.IntegrationCredential, error)
	DeleteCredential(ctx context.Context, orgID uuid.UUID, provider, userSub string) error
	ListCredentials(ctx context.Context, orgID uuid.UUID) ([]models.IntegrationCredential, error)
	Capabilities(ctx context.Context, orgID uuid.UUID) (service.CapabilitiesResult, error)
}

// IntegrationsHandler exposes the /api/v1/integrations surface — the
// admin-gated encrypted BYOK credential vault (WS3). The vault stores
// per-org 3rd-party API keys (Anthropic, Resend, named vendors)
// AES-256-GCM encrypted; this handler only ever exposes metadata
// (provider, label, last4, is_active, timestamps), never secret bytes.
type IntegrationsHandler struct {
	svc IntegrationsServicer
}

// NewIntegrationsHandler binds the handler to an IntegrationsServicer.
func NewIntegrationsHandler(svc IntegrationsServicer) *IntegrationsHandler {
	return &IntegrationsHandler{svc: svc}
}

// integrationCredentialDTO is the wire shape for a stored credential.
// It deliberately carries only non-secret metadata — the model's
// Ciphertext/Nonce/KeyVersion are never surfaced.
type integrationCredentialDTO struct {
	ID        uuid.UUID `json:"id"`
	Provider  string    `json:"provider"`
	Label     string    `json:"label"`
	Last4     string    `json:"last4"`
	IsActive  bool      `json:"is_active"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func newIntegrationCredentialDTO(c models.IntegrationCredential) integrationCredentialDTO {
	return integrationCredentialDTO{
		ID:        c.ID,
		Provider:  c.Provider,
		Label:     c.Label,
		Last4:     c.Last4,
		IsActive:  c.IsActive,
		CreatedBy: c.CreatedBy,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// ---------- GET /integrations ----------

// List returns metadata for every credential in the caller's org.
// GET /api/v1/integrations — admin RBAC enforced by MountIntegrationRoutes.
func (h *IntegrationsHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	creds, err := h.svc.ListCredentials(r.Context(), orgID)
	if err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	dtos := make([]integrationCredentialDTO, 0, len(creds))
	for _, c := range creds {
		dtos = append(dtos, newIntegrationCredentialDTO(c))
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"integrations": dtos})
}

// ---------- PUT /integrations/{provider} ----------

type setCredentialRequest struct {
	Label string `json:"label"`
	Key   string `json:"key"`
}

// Set stores (or rotates) the active credential for a provider.
// PUT /api/v1/integrations/{provider} — admin RBAC enforced by router.
func (h *IntegrationsHandler) Set(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "provider")))
	if provider == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "provider is required")
		return
	}
	var body setCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if body.Key == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "key is required")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cred, err := h.svc.SetCredential(r.Context(), service.SetCredentialInput{
		OrgID:    orgID,
		Provider: provider,
		Label:    body.Label,
		Key:      body.Key,
		UserSub:  claims.Sub,
	})
	if err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"integration": newIntegrationCredentialDTO(cred)})
}

// ---------- DELETE /integrations/{provider} ----------

// Delete deactivates the active credential for a provider.
// DELETE /api/v1/integrations/{provider} — admin RBAC enforced by router.
func (h *IntegrationsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	provider := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "provider")))
	if provider == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "provider is required")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	if err := h.svc.DeleteCredential(r.Context(), orgID, provider, claims.Sub); err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- GET /capabilities ----------

// capabilityProviderDTO is the per-provider wire shape. Optional fields
// are omitted when the provider is unconfigured. Matches the frontend
// ProviderStatus type (web/src/types/models.ts).
type capabilityProviderDTO struct {
	Provider    string     `json:"provider"`
	Configured  bool       `json:"configured"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
}

// capabilitiesDTO is the GET /capabilities wire shape. Matches the
// frontend Capabilities type.
type capabilitiesDTO struct {
	AIConfigured      bool                    `json:"ai_configured"`
	EmailConfigured   bool                    `json:"email_configured"`
	StorageConfigured bool                    `json:"storage_configured"`
	Providers         []capabilityProviderDTO `json:"providers"`
}

// Capabilities reports which vault-backed features (AI, email) are live
// for the caller's org, derived from active-credential presence.
// GET /api/v1/capabilities — any authenticated role (every role's UI
// gates AI/email affordances on this).
func (h *IntegrationsHandler) Capabilities(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	caps, err := h.svc.Capabilities(r.Context(), orgID)
	if err != nil {
		writeIntegrationError(w, r, err)
		return
	}
	providers := make([]capabilityProviderDTO, 0, len(caps.Providers))
	for _, p := range caps.Providers {
		dto := capabilityProviderDTO{Provider: p.Provider, Configured: p.Configured}
		if p.Configured {
			dto.Fingerprint = p.Fingerprint
			dto.CreatedBy = p.CreatedBy
			if !p.CreatedAt.IsZero() {
				ts := p.CreatedAt
				dto.CreatedAt = &ts
			}
		}
		providers = append(providers, dto)
	}
	writeJSON(w, r, http.StatusOK, capabilitiesDTO{
		AIConfigured:      caps.AIConfigured,
		EmailConfigured:   caps.EmailConfigured,
		StorageConfigured: caps.StorageConfigured,
		Providers:         providers,
	})
}

// ---------- error mapping ----------

// writeIntegrationError maps service sentinel errors to HTTP responses.
//
//	ErrNotFound     → 404 NOT_FOUND
//	ErrInvalidInput → 400 VALIDATION_ERROR
//	default         → 500 INTERNAL_ERROR (don't leak DB internals)
func writeIntegrationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountIntegrationRoutes wires the vault under /api/v1/integrations.
// Kept in this file (rather than router.go) so adding/renaming routes
// touches a single file. The whole group is admin-gated here (mirroring
// MountSetupRoutes' use of mw.RequireMinRole). Caller is responsible for
// placing this group INSIDE the auth middleware group.
func MountIntegrationRoutes(r chi.Router, h *IntegrationsHandler) {
	r.Route("/api/v1/integrations", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleAdmin))
		r.Get("/", h.List)
		r.Put("/{provider}", h.Set)
		r.Delete("/{provider}", h.Delete)
	})
}

// MountCapabilitiesRoutes wires GET /api/v1/capabilities. Unlike the vault
// surface (admin-gated), this is auth-only — every role's UI gates its AI/email
// affordances on the capability flags, so all authenticated callers may read it.
// It never exposes secret bytes (only presence + non-secret last4 fingerprint).
// Caller is responsible for placing this INSIDE the auth middleware group.
func MountCapabilitiesRoutes(r chi.Router, h *IntegrationsHandler) {
	r.Get("/api/v1/capabilities", h.Capabilities)
}
