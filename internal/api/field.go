package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// FieldHandler handles field sync endpoints.
type FieldHandler struct {
	svc *service.FieldSyncService
}

// NewFieldHandler creates a new FieldHandler.
func NewFieldHandler(svc *service.FieldSyncService) *FieldHandler {
	return &FieldHandler{svc: svc}
}

// Sync returns feed cards and tasks updated since the given timestamp.
// GET /api/v1/field/sync?since={RFC3339 timestamp}
func (h *FieldHandler) Sync(w http.ResponseWriter, r *http.Request) {
	claims, ok := mw.ClaimsFromContext(r.Context())
	if !ok {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing claims")
		return
	}

	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org_id in claims")
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid sub in claims")
		return
	}

	sinceStr := r.URL.Query().Get("since")
	var since time.Time
	if sinceStr != "" {
		since, err = time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid since timestamp (RFC3339)")
			return
		}
	} else {
		since = time.Now().Add(-24 * time.Hour) // Default: last 24 hours
	}

	payload, err := h.svc.Sync(r.Context(), orgID, userID, claims.Role, since)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusOK, payload)
}

// ReportProgress records a field progress update.
// POST /api/v1/field/progress
func (h *FieldHandler) ReportProgress(w http.ResponseWriter, r *http.Request) {
	claims, ok := mw.ClaimsFromContext(r.Context())
	if !ok {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing claims")
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid sub in claims")
		return
	}

	var body struct {
		ProjectID       string `json:"project_id"`
		TaskID          string `json:"task_id"`
		PercentComplete int    `json:"percent_complete"`
		Notes           string `json:"notes"`
		IdempotencyKey  string `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	projectID, _ := uuid.Parse(body.ProjectID)
	taskID, _ := uuid.Parse(body.TaskID)

	progress := &models.FieldProgress{
		ProjectID:       projectID,
		TaskID:          taskID,
		UserID:          userID,
		PercentComplete: body.PercentComplete,
		Notes:           body.Notes,
		IdempotencyKey:  body.IdempotencyKey,
	}

	id, err := h.svc.ReportProgress(r.Context(), progress)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateIdempotencyKey) {
			writeErrorResponse(w, r, http.StatusConflict, "DUPLICATE", "idempotency key already processed")
			return
		}
		if errors.Is(err, service.ErrMissingIdempotencyKey) {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id, "status": "recorded"})
}

// Checkin records a field worker location check-in.
// POST /api/v1/field/checkin
func (h *FieldHandler) Checkin(w http.ResponseWriter, r *http.Request) {
	claims, ok := mw.ClaimsFromContext(r.Context())
	if !ok {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing claims")
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid sub in claims")
		return
	}

	var body struct {
		ProjectID      string  `json:"project_id"`
		Latitude       float64 `json:"latitude"`
		Longitude      float64 `json:"longitude"`
		IdempotencyKey string  `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	projectID, _ := uuid.Parse(body.ProjectID)

	checkin := &models.FieldCheckin{
		UserID:         userID,
		ProjectID:      projectID,
		Latitude:       body.Latitude,
		Longitude:      body.Longitude,
		IdempotencyKey: body.IdempotencyKey,
	}

	id, err := h.svc.Checkin(r.Context(), checkin)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateIdempotencyKey) {
			writeErrorResponse(w, r, http.StatusConflict, "DUPLICATE", "idempotency key already processed")
			return
		}
		if errors.Is(err, service.ErrMissingIdempotencyKey) {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id, "status": "checked_in"})
}

// DailyLog records a field worker's daily summary.
// POST /api/v1/field/daily-log
func (h *FieldHandler) DailyLog(w http.ResponseWriter, r *http.Request) {
	claims, ok := mw.ClaimsFromContext(r.Context())
	if !ok {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "missing claims")
		return
	}

	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid sub in claims")
		return
	}

	var body struct {
		ProjectID      string  `json:"project_id"`
		LogDate        string  `json:"log_date"`
		Summary        string  `json:"summary"`
		HoursWorked    float64 `json:"hours_worked"`
		WeatherNotes   string  `json:"weather_notes"`
		SafetyNotes    string  `json:"safety_notes"`
		IdempotencyKey string  `json:"idempotency_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	projectID, _ := uuid.Parse(body.ProjectID)
	logDate, err := time.Parse("2006-01-02", body.LogDate)
	if err != nil {
		logDate = time.Now().UTC()
	}

	dl := &models.DailyLog{
		UserID:         userID,
		ProjectID:      projectID,
		LogDate:        logDate,
		Summary:        body.Summary,
		HoursWorked:    body.HoursWorked,
		WeatherNotes:   body.WeatherNotes,
		SafetyNotes:    body.SafetyNotes,
		IdempotencyKey: body.IdempotencyKey,
	}

	id, err := h.svc.DailyLog(r.Context(), dl)
	if err != nil {
		if errors.Is(err, service.ErrDuplicateIdempotencyKey) {
			writeErrorResponse(w, r, http.StatusConflict, "DUPLICATE", "idempotency key already processed")
			return
		}
		if errors.Is(err, service.ErrMissingIdempotencyKey) {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id, "status": "logged"})
}
