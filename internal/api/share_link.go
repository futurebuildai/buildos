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

// ShareLinkServicer is the consumer-side interface ShareLinkHandler needs for
// the AUTHENTICATED operator surface (create/list/revoke). The public-resolution
// methods are consumed by PublicShareHandler, not here.
type ShareLinkServicer interface {
	CreateShareLink(ctx context.Context, orgID uuid.UUID, userSub string, clientUpdateID uuid.UUID, ttl time.Duration) (service.IssuedShareLink, error)
	RevokeShareLink(ctx context.Context, orgID uuid.UUID, userSub string, linkID uuid.UUID) (models.ShareLink, error)
	ListShareLinks(ctx context.Context, orgID, clientUpdateID uuid.UUID) ([]models.ShareLink, error)
}

// ShareLinkHandler exposes the owner/admin share-link controls on a SENT client
// update (Chunk E): create a public link (cleartext URL shown ONCE), list links
// (no cleartext), revoke a link. RBAC owner/admin is enforced in
// MountShareLinkRoutes (external comms = owner/admin trust — §9-1).
type ShareLinkHandler struct {
	svc ShareLinkServicer
	// publicBaseURL is the deployment's public origin (e.g. https://acme.example).
	// Used to build the full /p/<cleartext> URL returned ONCE at create. Empty =>
	// the handler returns a relative /p/<cleartext> path (the operator prepends
	// the host) rather than guessing.
	publicBaseURL string
}

// NewShareLinkHandler binds the handler to a servicer + the public base URL.
func NewShareLinkHandler(svc ShareLinkServicer, publicBaseURL string) *ShareLinkHandler {
	return &ShareLinkHandler{svc: svc, publicBaseURL: strings.TrimRight(publicBaseURL, "/")}
}

// shareLinkView is the operator-side wire shape. It drops TokenHash (already
// json:"-" on the model) and adds a derived status so the UI can label
// active/expired/revoked without re-deriving from timestamps.
type shareLinkView struct {
	ID             uuid.UUID  `json:"id"`
	ClientUpdateID uuid.UUID  `json:"client_update_id"`
	Status         string     `json:"status"` // active | revoked | expired
	ExpiresAt      time.Time  `json:"expires_at"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	LastViewedAt   *time.Time `json:"last_viewed_at,omitempty"`
	ViewCount      int64      `json:"view_count"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toShareLinkView(l models.ShareLink, now time.Time) shareLinkView {
	status := "active"
	switch {
	case l.RevokedAt != nil:
		status = "revoked"
	case !now.Before(l.ExpiresAt):
		status = "expired"
	}
	return shareLinkView{
		ID:             l.ID,
		ClientUpdateID: l.ClientUpdateID,
		Status:         status,
		ExpiresAt:      l.ExpiresAt,
		RevokedAt:      l.RevokedAt,
		LastViewedAt:   l.LastViewedAt,
		ViewCount:      l.ViewCount,
		CreatedAt:      l.CreatedAt,
	}
}

// createShareLinkRequest is the POST .../share-links body. ttl_days is optional;
// 0/absent => the service default (30 days). The service caps it at 365 days.
type createShareLinkRequest struct {
	TTLDays int `json:"ttl_days"`
}

// createShareLinkResponse carries the one-time cleartext URL + the link view.
// `url` is the value the operator copies and emails to the homeowner; it is
// shown ONCE (the cleartext is never stored and never returned again).
type createShareLinkResponse struct {
	URL  string        `json:"url"`
	Link shareLinkView `json:"link"`
}

// Create mints a public share link for a sent client update.
// POST /api/v1/client-updates/{id}/share-links — owner/admin.
func (h *ShareLinkHandler) Create(w http.ResponseWriter, r *http.Request) {
	clientUpdateID, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body createShareLinkRequest
	// An empty body is valid (use the default TTL); only a malformed body is an
	// error.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	var ttl time.Duration
	if body.TTLDays > 0 {
		ttl = time.Duration(body.TTLDays) * 24 * time.Hour
	}
	claims := mw.MustClaimsFromContext(r.Context())
	issued, err := h.svc.CreateShareLink(r.Context(), orgID, claims.Sub, clientUpdateID, ttl)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, createShareLinkResponse{
		URL:  h.buildURL(issued.Cleartext),
		Link: toShareLinkView(issued.Link, time.Now()),
	})
}

// List returns a client update's share links (no cleartext).
// GET /api/v1/client-updates/{id}/share-links — owner/admin.
func (h *ShareLinkHandler) List(w http.ResponseWriter, r *http.Request) {
	clientUpdateID, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	links, err := h.svc.ListShareLinks(r.Context(), orgID, clientUpdateID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	now := time.Now()
	views := make([]shareLinkView, 0, len(links))
	for _, l := range links {
		views = append(views, toShareLinkView(l, now))
	}
	writeJSON(w, r, http.StatusOK, views)
}

// Revoke revokes a share link. After revoke GET /p/{token} returns a uniform 404.
// DELETE /api/v1/share-links/{linkID} — owner/admin.
func (h *ShareLinkHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	linkID, ok := parseUUIDFromURL(w, r, "linkID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	if _, err := h.svc.RevokeShareLink(r.Context(), orgID, claims.Sub, linkID); err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// buildURL composes the one-time public URL from the configured base + the
// cleartext token. With no base configured it returns the relative path.
func (h *ShareLinkHandler) buildURL(cleartext string) string {
	if h.publicBaseURL == "" {
		return "/p/" + cleartext
	}
	return h.publicBaseURL + "/p/" + cleartext
}

// writeServiceError maps ShareLinkService sentinels to HTTP responses.
func (h *ShareLinkHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrShareLinkNotSent):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "UPDATE_NOT_SENT", "a public link can only be created for a client update that has been sent")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountShareLinkRoutes wires the AUTHENTICATED owner/admin share-link surface.
// Caller must place this INSIDE the auth group. Two subtrees:
//   - /api/v1/client-updates/{id}/share-links  (create, list)
//   - /api/v1/share-links/{linkID}             (revoke)
//
// RBAC owner/admin (external comms trust — §9-1), enforced here.
func MountShareLinkRoutes(r chi.Router, h *ShareLinkHandler) {
	r.Route("/api/v1/client-updates/{id}/share-links", func(r chi.Router) {
		r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
		r.Post("/", h.Create)
		r.Get("/", h.List)
	})
	r.Route("/api/v1/share-links/{linkID}", func(r chi.Router) {
		r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
		r.Delete("/", h.Revoke)
	})
}
