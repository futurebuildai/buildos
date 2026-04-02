package api

import "net/http"

// ProcurementHandler handles /api/v1/projects/{projectID}/procurement/* endpoints.
type ProcurementHandler struct{}

// List returns procurement items for a project.
// GET /api/v1/projects/{projectID}/procurement
func (h *ProcurementHandler) List(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Create adds a new procurement item.
// POST /api/v1/projects/{projectID}/procurement
func (h *ProcurementHandler) Create(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Update modifies a procurement item.
// PUT /api/v1/projects/{projectID}/procurement/{itemID}
func (h *ProcurementHandler) Update(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
