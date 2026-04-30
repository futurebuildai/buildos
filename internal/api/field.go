package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// FieldServicer is the subset of *service.FieldService consumed by
// FieldHandler. Defined consumer-side so the handler stays free of a
// transitive db pool import.
type FieldServicer interface {
	Sync(ctx context.Context, opts service.SyncOptions) (models.FieldSyncResponse, error)
	ReportProgress(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject string, in service.ReportProgressInput) (models.TaskProgress, error)
	Checkin(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject string, in service.CheckinInput) (models.CrewCheckin, error)
	DailyLog(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject string, in service.DailyLogInput) (models.DailyLog, error)
}

// FieldHandler handles /api/v1/field/* endpoints for Flutter mobile sync.
type FieldHandler struct {
	svc FieldServicer
}

// NewFieldHandler creates a handler bound to the given service.
func NewFieldHandler(svc FieldServicer) *FieldHandler {
	return &FieldHandler{svc: svc}
}

// Sync returns the data the mobile client needs for offline-first
// operation: open tasks assigned to the caller, recent active feed
// cards, and the server timestamp the client should pass back as
// `?since=` on the next call.
//
// GET /api/v1/field/sync[?since=RFC3339]
func (h *FieldHandler) Sync(w http.ResponseWriter, r *http.Request) {
	claims, orgID, ok := claimsAndOrg(w, r)
	if !ok {
		return
	}

	var since time.Time
	if raw := r.URL.Query().Get("since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "since must be RFC3339")
			return
		}
		since = t
	}

	resp, err := h.svc.Sync(r.Context(), service.SyncOptions{
		CallerOrgID:       orgID,
		CallerOIDCSubject: claims.Sub,
		CallerRole:        claims.Role,
		Since:             since,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type reportProgressRequest struct {
	TaskID          uuid.UUID  `json:"task_id"`
	PercentComplete int        `json:"percent_complete"`
	Notes           *string    `json:"notes,omitempty"`
	PhotoAssetID    *uuid.UUID `json:"photo_asset_id,omitempty"`
	GPSLat          *float64   `json:"gps_lat,omitempty"`
	GPSLng          *float64   `json:"gps_lng,omitempty"`
	IdempotencyKey  uuid.UUID  `json:"idempotency_key"`
}

// ReportProgress records task progress from the field. Idempotent on
// idempotency_key — replayed posts return 409 (the mobile outbox
// treats 409 as "already accepted").
//
// POST /api/v1/field/progress
func (h *FieldHandler) ReportProgress(w http.ResponseWriter, r *http.Request) {
	claims, orgID, ok := claimsAndOrg(w, r)
	if !ok {
		return
	}

	var body reportProgressRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	row, err := h.svc.ReportProgress(r.Context(), orgID, claims.Sub, service.ReportProgressInput{
		TaskID:          body.TaskID,
		PercentComplete: body.PercentComplete,
		Notes:           body.Notes,
		PhotoAssetID:    body.PhotoAssetID,
		GPSLat:          body.GPSLat,
		GPSLng:          body.GPSLng,
		IdempotencyKey:  body.IdempotencyKey,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"progress": row})
}

type checkinRequest struct {
	ProjectID      uuid.UUID       `json:"project_id"`
	CrewMembers    json.RawMessage `json:"crew_members,omitempty"`
	GPSLat         *float64        `json:"gps_lat,omitempty"`
	GPSLng         *float64        `json:"gps_lng,omitempty"`
	Notes          *string         `json:"notes,omitempty"`
	IdempotencyKey uuid.UUID       `json:"idempotency_key"`
}

// Checkin records crew arrival at a project site.
//
// POST /api/v1/field/checkin
func (h *FieldHandler) Checkin(w http.ResponseWriter, r *http.Request) {
	claims, orgID, ok := claimsAndOrg(w, r)
	if !ok {
		return
	}

	var body checkinRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	row, err := h.svc.Checkin(r.Context(), orgID, claims.Sub, service.CheckinInput{
		ProjectID:      body.ProjectID,
		CrewMembers:    body.CrewMembers,
		GPSLat:         body.GPSLat,
		GPSLng:         body.GPSLng,
		Notes:          body.Notes,
		IdempotencyKey: body.IdempotencyKey,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"checkin": row})
}

type dailyLogRequest struct {
	ProjectID         uuid.UUID   `json:"project_id"`
	LogDate           string      `json:"log_date"`
	WeatherConditions *string     `json:"weather_conditions,omitempty"`
	WorkSummary       string      `json:"work_summary"`
	SafetyIncidents   *string     `json:"safety_incidents,omitempty"`
	PhotoAssetIDs     []uuid.UUID `json:"photo_asset_ids,omitempty"`
	IdempotencyKey    uuid.UUID   `json:"idempotency_key"`
}

// DailyLog records an end-of-day log from the field.
//
// POST /api/v1/field/daily-log
func (h *FieldHandler) DailyLog(w http.ResponseWriter, r *http.Request) {
	claims, orgID, ok := claimsAndOrg(w, r)
	if !ok {
		return
	}

	var body dailyLogRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	logDate, err := parseRequiredDate(body.LogDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "log_date must be RFC3339 or YYYY-MM-DD")
		return
	}

	row, err := h.svc.DailyLog(r.Context(), orgID, claims.Sub, service.DailyLogInput{
		ProjectID:         body.ProjectID,
		LogDate:           logDate,
		WeatherConditions: body.WeatherConditions,
		WorkSummary:       body.WorkSummary,
		SafetyIncidents:   body.SafetyIncidents,
		PhotoAssetIDs:     body.PhotoAssetIDs,
		IdempotencyKey:    body.IdempotencyKey,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"daily_log": row})
}

// writeServiceError maps FieldService sentinels to HTTP responses.
func (h *FieldHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrIdempotencyConflict):
		writeErrorResponse(w, r, http.StatusConflict, "CONFLICT", "idempotency key already used")
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "resource not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// claimsAndOrg extracts the JWT Claims + parses org_id. Centralized so
// every field handler does the same thing — every endpoint here is
// caller-scoped, never URL-scoped.
func claimsAndOrg(w http.ResponseWriter, r *http.Request) (mw.Claims, uuid.UUID, bool) {
	claims := mw.MustClaimsFromContext(r.Context())
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
		return mw.Claims{}, uuid.Nil, false
	}
	return claims, orgID, true
}
