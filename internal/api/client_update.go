package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ClientUpdateServicer is the consumer-side interface ClientUpdateHandler needs.
// Defined here so the handler stays free of a transitive db-pool import and
// tests substitute a fake.
type ClientUpdateServicer interface {
	CreateDraft(ctx context.Context, orgID uuid.UUID, userSub string, in service.CreateDraftInput) (models.ClientUpdate, error)
	UpdateDraft(ctx context.Context, orgID uuid.UUID, userSub string, in service.UpdateDraftInput) (models.ClientUpdate, error)
	SendClientUpdate(ctx context.Context, orgID uuid.UUID, userSub string, id uuid.UUID) (models.ClientUpdate, error)
	ListByProject(ctx context.Context, orgID, projectID uuid.UUID) ([]models.ClientUpdate, error)
	Get(ctx context.Context, orgID, id uuid.UUID) (models.ClientUpdate, error)
}

// ClientUpdateHandler exposes the human-in-the-loop client-update composer
// (Chunk D): create draft (from a date's AI draft), edit, send via Resend, and
// list history. RBAC owner/admin (external comms = owner/admin trust — §9-1),
// enforced in MountClientUpdateRoutes. recipient_email is never serialized
// (models.ClientUpdate has json:"-" on it).
type ClientUpdateHandler struct {
	svc ClientUpdateServicer
}

// NewClientUpdateHandler binds the handler to a ClientUpdateServicer.
func NewClientUpdateHandler(svc ClientUpdateServicer) *ClientUpdateHandler {
	return &ClientUpdateHandler{svc: svc}
}

// createDraftRequest is the POST .../client-updates body. report_date is the
// (project, date) whose AI draft seeds this update (Chunk C drafts a single
// day). period_start/period_end are accepted as aliases; report_date wins.
type createDraftRequest struct {
	ReportDate  string `json:"report_date"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
}

// Create generates an AI draft for the date and persists it as a draft.
// POST /api/v1/projects/{projectID}/client-updates — owner/admin.
func (h *ClientUpdateHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body createDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	dateStr := body.ReportDate
	if dateStr == "" {
		dateStr = body.PeriodStart
	}
	if dateStr == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "report_date (YYYY-MM-DD) is required")
		return
	}
	day, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "report_date must be YYYY-MM-DD")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cu, err := h.svc.CreateDraft(r.Context(), orgID, claims.Sub, service.CreateDraftInput{
		ProjectID:  projectID,
		ReportDate: day,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, cu)
}

// List returns a project's client-update history newest-first.
// GET /api/v1/projects/{projectID}/client-updates — owner/admin.
func (h *ClientUpdateHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	updates, err := h.svc.ListByProject(r.Context(), orgID, projectID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if updates == nil {
		updates = []models.ClientUpdate{}
	}
	writeJSON(w, r, http.StatusOK, updates)
}

// Get returns one client update, org-scoped.
// GET /api/v1/client-updates/{id} — owner/admin.
func (h *ClientUpdateHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	cu, err := h.svc.Get(r.Context(), orgID, id)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, cu)
}

// updateDraftRequest is the PATCH .../client-updates/{id} body.
type updateDraftRequest struct {
	Subject       string      `json:"subject"`
	EditedBody    string      `json:"edited_body"`
	PhotoAssetIDs []uuid.UUID `json:"photo_asset_ids"`
}

// Update applies the operator edit to a draft.
// PATCH /api/v1/client-updates/{id} — owner/admin. 409 ALREADY_SENT on a sent row.
func (h *ClientUpdateHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body updateDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cu, err := h.svc.UpdateDraft(r.Context(), orgID, claims.Sub, service.UpdateDraftInput{
		ID:            id,
		Subject:       body.Subject,
		EditedBody:    body.EditedBody,
		PhotoAssetIDs: body.PhotoAssetIDs,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, cu)
}

// Send is the human-pressed send via Resend.
// POST /api/v1/client-updates/{id}/send — owner/admin.
// 422 NO_CLIENT_CONTACT, 422 MAILER_UNCONFIGURED, 409 ALREADY_SENT.
func (h *ClientUpdateHandler) Send(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	cu, err := h.svc.SendClientUpdate(r.Context(), orgID, claims.Sub, id)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, cu)
}

// writeServiceError maps ClientUpdateService sentinels to HTTP responses. The
// send failures (NO_CLIENT_CONTACT / MAILER_UNCONFIGURED) are 422 with a
// distinct code so the operator UI surfaces "it did not send" clearly — these
// are NOT swallowed.
func (h *ClientUpdateHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrClientUpdateAIUnavailable):
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "AI is not available to draft a client update")
	case errors.Is(err, service.ErrNoClientContact):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "NO_CLIENT_CONTACT", "the project has no client email; add a homeowner contact before sending")
	case errors.Is(err, service.ErrMailerUnconfigured):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "MAILER_UNCONFIGURED", "email is not configured (no Resend API key); the update was NOT sent")
	case errors.Is(err, service.ErrClientUpdateSendFailed):
		writeErrorResponse(w, r, http.StatusBadGateway, "SEND_FAILED", "the email provider rejected the send; the update was NOT sent")
	case errors.Is(err, service.ErrAlreadySent):
		writeErrorResponse(w, r, http.StatusConflict, "ALREADY_SENT", "this client update has already been sent")
	case errors.Is(err, service.ErrInvalidPhotoAsset):
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PHOTO_ASSET", "a selected photo is not a confirmed photo for this project")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountClientUpdateRoutes wires the client-update endpoints. ALL routes are
// owner/admin (external comms trust — §9-1). The project-subtree routes
// (create draft, list history) live under /api/v1/projects/{projectID}/
// client-updates; the flat routes (get, edit, send) under /api/v1/
// client-updates/{id}. Caller must place this INSIDE the auth group.
func MountClientUpdateRoutes(r chi.Router, h *ClientUpdateHandler) {
	r.Route("/api/v1/projects/{projectID}/client-updates", func(r chi.Router) {
		r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
		r.Post("/", h.Create)
		r.Get("/", h.List)
	})
	r.Route("/api/v1/client-updates/{id}", func(r chi.Router) {
		r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))
		r.Get("/", h.Get)
		r.Patch("/", h.Update)
		r.Post("/send", h.Send)
	})
}
