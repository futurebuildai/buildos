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
	LinkPhotosToDailyLog(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time, assetIDs []uuid.UUID) (models.DailyLog, error)
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

// linkPhotosRequest is the POST .../daily-reports/{date}/photos body: the
// already-confirmed asset ids to associate with that day's daily log.
type linkPhotosRequest struct {
	AssetIDs []uuid.UUID `json:"asset_ids"`
}

// LinkPhotos associates confirmed assets with the daily log for a (project,
// date) — the operator web "Add photos" affordance on the daily-report view
// (Chunk B §3). Reuses the project's existing daily log for the day; 404s if no
// log exists (the operator must record a daily log first). RBAC minRole
// superintendent (matches the daily-reports operator surface).
//
// POST /api/v1/projects/{projectID}/daily-reports/{date}/photos
func (h *AssetHandler) LinkPhotos(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	day, ok := parseReportDateParam(w, r)
	if !ok {
		return
	}
	var body linkPhotosRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	dl, err := h.svc.LinkPhotosToDailyLog(r.Context(), orgID, claims.Sub, projectID, day, body.AssetIDs)
	if err != nil {
		writeAssetError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, dl)
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
	case errors.Is(err, service.ErrInvalidPhotoAsset):
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PHOTO_ASSET", "a photo asset is not a confirmed, org-owned photo for this project")
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
// flat routes (confirm, get) under /api/v1/assets/{id}; the operator "Add
// photos to a day's log" link route under the daily-reports subtree. All are
// minRole superintendent (the generic operator surface; the field-facing
// presign/confirm for field_worker mount separately in MountFieldRoutes).
// Caller must place this INSIDE the auth group.
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
	// Operator photo-link: associate confirmed assets with a (project, date)
	// daily log so the daily-report photo strip resolves them. Superintendent+
	// (same gate as the daily-report read surface).
	r.With(mw.RequireMinRole(mw.RoleSuperintendent)).
		Post("/api/v1/projects/{projectID}/daily-reports/{date}/photos", h.LinkPhotos)
}
