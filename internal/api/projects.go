package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ProjectServicer is the subset of *service.ProjectService consumed by
// ProjectHandler. The handler depends on the interface so unit tests
// can substitute a fake (matches SetupServicer / PipelineServicer).
type ProjectServicer interface {
	ListProjects(ctx context.Context, in service.ListProjectsInput) ([]models.Project, error)
	GetProject(ctx context.Context, orgID, projectID uuid.UUID) (models.Project, error)
	CreateProject(ctx context.Context, in service.CreateProjectInput) (models.Project, error)
	UpdateProject(ctx context.Context, in service.UpdateProjectInput) (models.Project, error)
}

// ProjectHandler handles /api/v1/projects/* endpoints. RBAC is enforced
// by the router (List/Get: any authed role; Create/Update: owner/admin).
type ProjectHandler struct {
	svc ProjectServicer
}

// NewProjectHandler binds the handler to a ProjectServicer.
func NewProjectHandler(svc ProjectServicer) *ProjectHandler { return &ProjectHandler{svc: svc} }

// ---------- request bodies ----------

type createProjectRequest struct {
	Name             string  `json:"name"`
	Address          *string `json:"address,omitempty"`
	PermitIssuedDate *string `json:"permit_issued_date,omitempty"`
	ProjectStartDate *string `json:"project_start_date,omitempty"`
	GSF              *int    `json:"gsf,omitempty"`
}

type updateProjectRequest struct {
	Name    *string `json:"name,omitempty"`
	Address *string `json:"address,omitempty"`
	Status  *string `json:"status,omitempty"`
	GSF     *int    `json:"gsf,omitempty"`
}

// ---------- GET /api/v1/projects ----------

// List returns a page of projects for the authenticated org.
// GET /api/v1/projects?status=&page=&per_page=
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	// Malformed page/per_page fall back to service defaults (0 → default).
	page, _ := strconv.Atoi(q.Get("page"))
	perPage, _ := strconv.Atoi(q.Get("per_page"))

	list, err := h.svc.ListProjects(r.Context(), service.ListProjectsInput{
		OrgID:   orgID,
		Status:  q.Get("status"),
		Page:    page,
		PerPage: perPage,
	})
	if err != nil {
		writeProjectError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"projects": nonNilProjects(list)})
}

// ---------- POST /api/v1/projects ----------

// Create creates a new project. Owner/admin only (router-gated).
// POST /api/v1/projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	var body createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	permitDate, err := parseOptionalDate(body.PermitIssuedDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "permit_issued_date must be YYYY-MM-DD or RFC3339")
		return
	}
	startDate, err := parseOptionalDate(body.ProjectStartDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "project_start_date must be YYYY-MM-DD or RFC3339")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	p, err := h.svc.CreateProject(r.Context(), service.CreateProjectInput{
		OrgID:            orgID,
		UserSub:          claims.Sub,
		Name:             body.Name,
		Address:          body.Address,
		PermitIssuedDate: permitDate,
		ProjectStartDate: startDate,
		GSF:              body.GSF,
	})
	if err != nil {
		writeProjectError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"project": p})
}

// ---------- GET /api/v1/projects/{projectID} ----------

// Get returns a single project by ID.
// GET /api/v1/projects/{projectID}
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	p, err := h.svc.GetProject(r.Context(), orgID, projectID)
	if err != nil {
		writeProjectError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"project": p})
}

// ---------- PUT /api/v1/projects/{projectID} ----------

// Update modifies an existing project. Owner/admin only (router-gated).
// PUT /api/v1/projects/{projectID}
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	var body updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	claims := mw.MustClaimsFromContext(r.Context())
	p, err := h.svc.UpdateProject(r.Context(), service.UpdateProjectInput{
		OrgID:     orgID,
		ProjectID: projectID,
		UserSub:   claims.Sub,
		Name:      body.Name,
		Address:   body.Address,
		Status:    body.Status,
		GSF:       body.GSF,
	})
	if err != nil {
		writeProjectError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"project": p})
}

// ---------- helpers ----------

// nonNilProjects returns an empty slice (not nil) so the wire list is a
// stable "[]" rather than null.
func nonNilProjects(s []models.Project) []models.Project {
	if s == nil {
		return []models.Project{}
	}
	return s
}

// writeProjectError maps service sentinels to HTTP responses.
//
//	ErrNotFound      → 404 NOT_FOUND
//	ErrInvalidInput  → 400 VALIDATION_ERROR
//	default          → 500 INTERNAL_ERROR (don't leak DB)
func writeProjectError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
