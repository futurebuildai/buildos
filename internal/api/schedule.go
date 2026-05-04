package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/physics"
	"github.com/futurebuildai/buildos/internal/service"
)

// ScheduleServicer is the subset of *service.ScheduleService consumed by
// ScheduleHandler. Defined as an interface here for testability.
type ScheduleServicer interface {
	RecalculateSchedule(ctx context.Context, projectID, callerOrgID uuid.UUID) (*physics.CPMResult, time.Duration, error)
	GetGantt(ctx context.Context, projectID, callerOrgID uuid.UUID) (service.GanttView, error)
	ListProjectTasks(ctx context.Context, in service.ListProjectTasksInput) ([]models.ProjectTask, error)
	UpdateTask(ctx context.Context, in service.UpdateTaskInput) (models.ProjectTask, error)
}

// ScheduleHandler handles /api/v1/projects/{projectID}/schedule/* and
// /tasks/* endpoints.
type ScheduleHandler struct {
	svc ScheduleServicer
}

// NewScheduleHandler creates a handler bound to the given service.
func NewScheduleHandler(svc ScheduleServicer) *ScheduleHandler {
	return &ScheduleHandler{svc: svc}
}

// Recalculate triggers a full CPM recalculation for a project.
// POST /api/v1/projects/{projectID}/schedule/recalculate
// NFR: <800ms end-to-end, <200ms physics computation (80-task graph).
func (h *ScheduleHandler) Recalculate(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	cpm, took, err := h.svc.RecalculateSchedule(r.Context(), projectID, callerOrg)
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"cpm_result":       cpm,
		"recalculation_ms": took.Milliseconds(),
	})
}

// Gantt returns the stored Gantt-shaped view for a project.
// GET /api/v1/projects/{projectID}/schedule/gantt
func (h *ScheduleHandler) Gantt(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	view, err := h.svc.GetGantt(r.Context(), projectID, callerOrg)
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, view)
}

// ListTasks returns tasks for a project with optional filters.
// GET /api/v1/projects/{projectID}/tasks[?status=pending&is_critical=true]
func (h *ScheduleHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var critical *bool
	if v := r.URL.Query().Get("is_critical"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "is_critical must be true or false")
			return
		}
		critical = &parsed
	}
	tasks, err := h.svc.ListProjectTasks(r.Context(), service.ListProjectTasksInput{
		ProjectID:  projectID,
		OrgID:      callerOrg,
		Status:     r.URL.Query().Get("status"),
		IsCritical: critical,
	})
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"tasks": tasks})
}

type updateTaskRequest struct {
	PercentComplete *int         `json:"percent_complete,omitempty"`
	Status          *string      `json:"status,omitempty"`
	AssignedCrew    *[]uuid.UUID `json:"assigned_crew,omitempty"` // pointer distinguishes absent vs []
}

// UpdateTask modifies a project task.
// PUT /api/v1/projects/{projectID}/tasks/{taskID}
func (h *ScheduleHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	taskID, ok := parseUUIDFromURL(w, r, "taskID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body updateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	task, err := h.svc.UpdateTask(r.Context(), service.UpdateTaskInput{
		TaskID:          taskID,
		ProjectID:       projectID,
		OrgID:           callerOrg,
		PercentComplete: body.PercentComplete,
		Status:          body.Status,
		AssignedCrew:    body.AssignedCrew,
	})
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"task": task})
}

// writeScheduleError maps service errors to HTTP responses. ScheduleService
// reuses the budget service's error sentinels (ErrNotFound, ErrInvalidInput).
func writeScheduleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
