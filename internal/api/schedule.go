package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/physics"
	"github.com/futurebuildai/buildos/internal/service"
)

// ScheduleServicer is the subset of *service.ScheduleService consumed by
// ScheduleHandler. Defined as an interface here for testability.
type ScheduleServicer interface {
	RecalculateSchedule(ctx context.Context, projectID, callerOrgID uuid.UUID, callerUserSub string) (*physics.CPMResult, time.Duration, error)
	GetGantt(ctx context.Context, projectID, callerOrgID uuid.UUID) (service.GanttView, error)
	ListProjectTasks(ctx context.Context, in service.ListProjectTasksInput) ([]models.ProjectTask, error)
	UpdateTask(ctx context.Context, in service.UpdateTaskInput) (models.ProjectTask, error)
	ImportSchedule(ctx context.Context, projectID, callerOrgID uuid.UUID, callerUserSub string, in service.ImportScheduleInput) (*service.ImportScheduleResult, error)
	CreateTask(ctx context.Context, in service.CreateTaskInput) (models.ProjectTask, error)
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
	claims := mw.MustClaimsFromContext(r.Context())
	cpm, took, err := h.svc.RecalculateSchedule(r.Context(), projectID, callerOrg, claims.Sub)
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"cpm_result":       cpm,
		"recalculation_ms": took.Milliseconds(),
	})
}

// importTaskRequest mirrors a tasks[] element. CPM-output columns are not
// accepted (ignored if present). Pointers distinguish "omitted" (apply
// default) from an explicit value.
type importTaskRequest struct {
	WBSCode         string      `json:"wbs_code"`
	Name            string      `json:"name"`
	DurationDays    int         `json:"duration_days"`
	Status          string      `json:"status,omitempty"`
	PercentComplete *int        `json:"percent_complete,omitempty"`
	AssignedCrew    []uuid.UUID `json:"assigned_crew,omitempty"`
}

// importDependencyRequest is one wbs_code-keyed dependency (mirrors
// models.WBSTemplateDep's string-code shape — the client doesn't know
// server-assigned task UUIDs).
type importDependencyRequest struct {
	PredecessorCode string `json:"predecessor_code"`
	SuccessorCode   string `json:"successor_code"`
	DependencyType  string `json:"dependency_type,omitempty"`
	LagDays         int    `json:"lag_days,omitempty"`
}

type importScheduleRequest struct {
	Tasks        []importTaskRequest       `json:"tasks"`
	Dependencies []importDependencyRequest `json:"dependencies,omitempty"`
	// Recalculate defaults to TRUE — the keystone goal is a populated Gantt.
	// A pointer distinguishes omitted (→ true) from an explicit false.
	Recalculate *bool `json:"recalculate,omitempty"`
}

// Import authors a whole task graph (tasks + dependencies) atomically, then
// auto-recalcs CPM (default) so the Gantt is populated in the same request.
// POST /api/v1/projects/{projectID}/schedule/import
func (h *ScheduleHandler) Import(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body importScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	tasks := make([]service.ImportTaskInput, 0, len(body.Tasks))
	for _, t := range body.Tasks {
		pct := 0
		if t.PercentComplete != nil {
			pct = *t.PercentComplete
		}
		tasks = append(tasks, service.ImportTaskInput{
			WBSCode:         t.WBSCode,
			Name:            t.Name,
			DurationDays:    t.DurationDays,
			Status:          t.Status,
			PercentComplete: pct,
			AssignedCrew:    t.AssignedCrew,
		})
	}
	deps := make([]service.ImportDependencyInput, 0, len(body.Dependencies))
	for _, d := range body.Dependencies {
		deps = append(deps, service.ImportDependencyInput{
			PredecessorCode: d.PredecessorCode,
			SuccessorCode:   d.SuccessorCode,
			DependencyType:  d.DependencyType,
			LagDays:         d.LagDays,
		})
	}
	recalc := true
	if body.Recalculate != nil {
		recalc = *body.Recalculate
	}

	claims := mw.MustClaimsFromContext(r.Context())
	res, err := h.svc.ImportSchedule(r.Context(), projectID, callerOrg, claims.Sub, service.ImportScheduleInput{
		Tasks:        tasks,
		Dependencies: deps,
		Recalculate:  recalc,
	})
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{
		"tasks":            res.Tasks,
		"dependency_count": res.DependencyCount,
		"cpm_result":       res.CPMResult,
		"recalculation_ms": res.RecalcDuration.Milliseconds(),
	})
}

type createTaskRequest struct {
	WBSCode         string      `json:"wbs_code"`
	Name            string      `json:"name"`
	DurationDays    int         `json:"duration_days"`
	Status          string      `json:"status,omitempty"`
	PercentComplete *int        `json:"percent_complete,omitempty"`
	AssignedCrew    []uuid.UUID `json:"assigned_crew,omitempty"`
}

// CreateTask adds a single task (no deps, no auto-recalc).
// POST /api/v1/projects/{projectID}/tasks
func (h *ScheduleHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body createTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	pct := 0
	if body.PercentComplete != nil {
		pct = *body.PercentComplete
	}
	claims := mw.MustClaimsFromContext(r.Context())
	task, err := h.svc.CreateTask(r.Context(), service.CreateTaskInput{
		ProjectID:       projectID,
		OrgID:           callerOrg,
		CallerUserSub:   claims.Sub,
		WBSCode:         body.WBSCode,
		Name:            body.Name,
		DurationDays:    body.DurationDays,
		Status:          body.Status,
		PercentComplete: pct,
		AssignedCrew:    body.AssignedCrew,
	})
	if err != nil {
		writeScheduleError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"task": task})
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
