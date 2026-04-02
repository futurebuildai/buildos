package api

import "net/http"

// ProjectHandler handles /api/v1/projects/* endpoints.
type ProjectHandler struct{}

// List returns all projects for the authenticated org.
// GET /api/v1/projects
func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Create creates a new project.
// POST /api/v1/projects
func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Get returns a single project by ID.
// GET /api/v1/projects/{projectID}
func (h *ProjectHandler) Get(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Update modifies an existing project.
// PUT /api/v1/projects/{projectID}
func (h *ProjectHandler) Update(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
