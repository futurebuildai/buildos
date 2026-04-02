package api

import "net/http"

// FieldHandler handles /api/v1/field/* endpoints for Flutter mobile sync.
type FieldHandler struct{}

// Sync returns notifications and task updates since the given timestamp.
// GET /api/v1/field/sync
func (h *FieldHandler) Sync(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// ReportProgress records task progress from the field.
// POST /api/v1/field/progress
func (h *FieldHandler) ReportProgress(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Checkin records crew check-in at a project site.
// POST /api/v1/field/checkin
func (h *FieldHandler) Checkin(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// DailyLog records an end-of-day log from the field.
// POST /api/v1/field/daily-log
func (h *FieldHandler) DailyLog(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
