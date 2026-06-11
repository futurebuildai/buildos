package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// AssetServicer is the subset of *service.AssetService consumed by
// AssetHandler. Defined consumer-side so the handler stays free of a transitive
// db-pool import and tests can substitute a fake.
type AssetServicer interface {
	RequestUpload(ctx context.Context, orgID uuid.UUID, userSub string, in service.RequestUploadInput) (service.RequestUploadResult, error)
	ConfirmUpload(ctx context.Context, orgID uuid.UUID, userSub string, assetID uuid.UUID, checksum *string) (models.Asset, error)
	GetAsset(ctx context.Context, orgID, assetID uuid.UUID) (models.Asset, error)
	SignedGetURL(ctx context.Context, orgID, assetID uuid.UUID, ttl time.Duration) (string, error)
	ServeAsset(ctx context.Context, orgID, assetID uuid.UUID) (io.ReadCloser, string, error)
	ListProjectAssets(ctx context.Context, orgID, projectID uuid.UUID, readyOnly bool) ([]models.Asset, error)
}

// AssetHandler exposes the object-storage substrate endpoints (Chunk A): a
// presigned-PUT request, a confirm, a signed-GET, and a project gallery list.
// The blob bytes flow DIRECT to/from R2 via presigned URLs — they never transit
// the Go server on the happy path. The raw storage key is never serialized
// (models.Asset has json:"-" on StorageKey).
type AssetHandler struct {
	svc AssetServicer
}

// NewAssetHandler binds the handler to an AssetServicer.
func NewAssetHandler(svc AssetServicer) *AssetHandler {
	return &AssetHandler{svc: svc}
}

// presignPutRequest is the POST .../assets/presign-put body.
type presignPutRequest struct {
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
	Filename    string `json:"filename,omitempty"` // hint only; unused server-side v1
}

// presignPutResponse is the 201 data payload: the asset id the client confirms
// later, the URL it PUTs bytes to, the headers it must echo, and the expiry.
type presignPutResponse struct {
	AssetID       uuid.UUID         `json:"asset_id"`
	UploadURL     string            `json:"upload_url"`
	SignedHeaders map[string]string `json:"signed_headers"`
	ExpiresAt     time.Time         `json:"expires_at"`
}

// PresignPut creates a pending asset row and returns a presigned PUT URL.
// POST /api/v1/projects/{projectID}/assets/presign-put — minRole superintendent.
func (h *AssetHandler) PresignPut(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body presignPutRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	pid := projectID
	res, err := h.svc.RequestUpload(r.Context(), orgID, claims.Sub, service.RequestUploadInput{
		ProjectID:   &pid,
		ContentType: body.ContentType,
		SizeBytes:   body.ByteSize,
	})
	if err != nil {
		writeAssetError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, presignPutResponse{
		AssetID:       res.Asset.ID,
		UploadURL:     res.UploadURL,
		SignedHeaders: res.SignedHeaders,
		ExpiresAt:     res.ExpiresAt,
	})
}

type confirmRequest struct {
	ChecksumSHA256 string `json:"checksum_sha256,omitempty"`
}

// Confirm transitions a pending asset to ready after the client's PUT.
// POST /api/v1/assets/{id}/confirm — minRole superintendent.
func (h *AssetHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	assetID, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body confirmRequest
	// Body is optional; tolerate an empty/absent body.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	claims := mw.MustClaimsFromContext(r.Context())
	var checksum *string
	if body.ChecksumSHA256 != "" {
		checksum = &body.ChecksumSHA256
	}
	asset, err := h.svc.ConfirmUpload(r.Context(), orgID, claims.Sub, assetID, checksum)
	if err != nil {
		writeAssetError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, asset)
}

// Get returns a short-lived presigned GET URL for a ready asset (302 redirect),
// org-scoped. GET /api/v1/assets/{id} — minRole superintendent.
func (h *AssetHandler) Get(w http.ResponseWriter, r *http.Request) {
	assetID, ok := parseUUIDFromURL(w, r, "id")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	url, err := h.svc.SignedGetURL(r.Context(), orgID, assetID, 0)
	if err != nil {
		writeAssetError(w, r, err)
		return
	}
	// 302 to the short-lived signed URL. The R2 host appears only in this
	// redirect, which is delivered over the authenticated, same-origin response
	// — not embedded in client HTML. The same-origin EXIF-stripping proxy
	// (ServeAsset) is the path the public page (Chunk E) uses instead.
	http.Redirect(w, r, url, http.StatusFound)
}

// List returns a project's ready assets (gallery), org-scoped.
// GET /api/v1/projects/{projectID}/assets — minRole superintendent.
func (h *AssetHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	assets, err := h.svc.ListProjectAssets(r.Context(), orgID, projectID, true)
	if err != nil {
		writeAssetError(w, r, err)
		return
	}
	if assets == nil {
		assets = []models.Asset{}
	}
	writeJSON(w, r, http.StatusOK, assets)
}

// writeAssetError maps AssetService sentinels to HTTP responses.
//
//	ErrStorageUnavailable → 503 STORAGE_UNAVAILABLE
//	ErrInvalidInput       → 400 VALIDATION_ERROR
//	ErrNotFound           → 404 NOT_FOUND (uniform on cross-org)
//	default               → 500 INTERNAL_ERROR
func writeAssetError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrStorageUnavailable):
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "object storage is not configured")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountAssetRoutes wires the asset endpoints. The project-subtree routes
// (presign-put, list) live under /api/v1/projects/{projectID}/assets; the
// flat routes (confirm, get) under /api/v1/assets/{id}. All are minRole
// superintendent (the generic operator surface; the field-facing variant for
// field_worker lands in Chunk B). Caller must place this INSIDE the auth group.
func MountAssetRoutes(r chi.Router, h *AssetHandler) {
	r.Route("/api/v1/projects/{projectID}/assets", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleSuperintendent))
		r.Post("/presign-put", h.PresignPut)
		r.Get("/", h.List)
	})
	r.Route("/api/v1/assets/{id}", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleSuperintendent))
		r.Post("/confirm", h.Confirm)
		r.Get("/", h.Get)
	})
}
