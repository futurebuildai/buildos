package api

import "net/http"

// PipelineHandler handles /api/v1/org/{orgID}/pipeline/* endpoints.
type PipelineHandler struct{}

// ListProspects returns all pipeline prospects for an org.
// GET /api/v1/org/{orgID}/pipeline/prospects
func (h *PipelineHandler) ListProspects(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// CreateProspect adds a new prospect to the pipeline.
// POST /api/v1/org/{orgID}/pipeline/prospects
func (h *PipelineHandler) CreateProspect(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// GetProspect returns a prospect with its estimates and permits.
// GET /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
func (h *PipelineHandler) GetProspect(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// UpdateProspect modifies prospect details.
// PUT /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
func (h *PipelineHandler) UpdateProspect(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// AdvanceProspect transitions a prospect to the next pipeline stage.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/advance
func (h *PipelineHandler) AdvanceProspect(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// LoseProspect marks a prospect as lost.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/lose
func (h *PipelineHandler) LoseProspect(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// CreateEstimate adds a new estimate for a prospect.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/estimates
func (h *PipelineHandler) CreateEstimate(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// UpdateEstimate modifies an existing estimate.
// PUT /api/v1/org/{orgID}/pipeline/estimates/{estimateID}
func (h *PipelineHandler) UpdateEstimate(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// CreatePermit adds a permit for a prospect.
// POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/permits
func (h *PipelineHandler) CreatePermit(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// UpdatePermit modifies an existing permit.
// PUT /api/v1/org/{orgID}/pipeline/permits/{permitID}
func (h *PipelineHandler) UpdatePermit(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// Analytics returns weighted pipeline revenue grouped by currency.
// GET /api/v1/org/{orgID}/pipeline/analytics
func (h *PipelineHandler) Analytics(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
