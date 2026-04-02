package api

import "net/http"

// ScheduleHandler handles /api/v1/projects/{projectID}/schedule/* and tasks/* endpoints.
type ScheduleHandler struct{}

// Recalculate triggers a full CPM recalculation for a project.
// POST /api/v1/projects/{projectID}/schedule/recalculate
func (h *ScheduleHandler) Recalculate(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Gantt returns the Gantt chart data for a project.
// GET /api/v1/projects/{projectID}/schedule/gantt
func (h *ScheduleHandler) Gantt(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// ListTasks returns tasks for a project.
// GET /api/v1/projects/{projectID}/tasks
func (h *ScheduleHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// UpdateTask modifies a project task.
// PUT /api/v1/projects/{projectID}/tasks/{taskID}
func (h *ScheduleHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
