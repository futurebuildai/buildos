package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/agents"
	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/futurebuild/futurebuild-os/internal/store"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FieldHandler handles field sync endpoints.
type FieldHandler struct {
	svc      *service.FieldSyncService
	aiClient ai.Client     // optional: nil disables vision verification
	pool     *pgxpool.Pool // for vision store writes
}

// NewFieldHandler creates a new FieldHandler.
func NewFieldHandler(svc *service.FieldSyncService) *FieldHandler {
	return &FieldHandler{svc: svc}
}

// WithVision enables the vision verification endpoint.
func (h *FieldHandler) WithVision(aiClient ai.Client, pool *pgxpool.Pool) *FieldHandler {
	h.aiClient = aiClient
	h.pool = pool
	return h
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

// VerifyProgress uses Claude vision to verify construction progress from a photo.
// POST /api/v1/field/verify-progress
func (h *FieldHandler) VerifyProgress(w http.ResponseWriter, r *http.Request) {
	if h.aiClient == nil || h.pool == nil {
		writeErrorResponse(w, r, http.StatusServiceUnavailable, "FEATURE_DISABLED", "vision verification is not configured")
		return
	}

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

	var body struct {
		TaskID           string `json:"task_id"`
		PhotoURL         string `json:"photo_url"`
		ExpectedProgress int    `json:"expected_progress"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	taskID, err := uuid.Parse(body.TaskID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid task_id")
		return
	}

	if body.PhotoURL == "" {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "photo_url is required")
		return
	}

	if body.ExpectedProgress < 0 || body.ExpectedProgress > 100 {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "expected_progress must be 0-100")
		return
	}

	// Run vision verification
	agent := agents.NewVisionVerificationAgent(h.aiClient)
	result, err := agent.VerifyProgress(r.Context(), taskID, body.PhotoURL, body.ExpectedProgress)
	if err != nil {
		slog.Error("vision verification failed", "task_id", taskID, "error", err)
		writeErrorResponse(w, r, http.StatusInternalServerError, "AI_ERROR", "vision verification failed")
		return
	}

	// Persist the verification result
	issuesJSON, _ := json.Marshal(result.Issues)
	visionStore := store.NewVisionStore(h.pool)
	verificationID, err := visionStore.SaveVerification(r.Context(), &store.VisionVerification{
		OrgID:             orgID,
		TaskID:            taskID,
		PhotoURL:          body.PhotoURL,
		ExpectedProgress:  body.ExpectedProgress,
		EstimatedProgress: result.EstimatedProgress,
		Confidence:        result.Confidence,
		Notes:             result.Notes,
		Issues:            issuesJSON,
		RequiresReview:    result.RequiresReview,
	})
	if err != nil {
		slog.Error("failed to save vision verification", "task_id", taskID, "error", err)
		// Non-fatal: return the result even if persistence fails
	}

	writeJSON(w, r, http.StatusOK, map[string]any{
		"id":                 verificationID,
		"task_id":            result.TaskID,
		"estimated_progress": result.EstimatedProgress,
		"confidence":         result.Confidence,
		"notes":              result.Notes,
		"issues":             result.Issues,
		"requires_review":    result.RequiresReview,
	})
}
