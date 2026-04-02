package api

import "net/http"

// FleetHandler handles /api/v1/org/{orgID}/fleet/* endpoints.
type FleetHandler struct{}

// ListAssets returns all fleet assets for an org.
// GET /api/v1/org/{orgID}/fleet
func (h *FleetHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// CreateAsset adds a new fleet asset.
// POST /api/v1/org/{orgID}/fleet
func (h *FleetHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// AllocateAsset assigns a fleet asset to a project.
// POST /api/v1/org/{orgID}/fleet/{assetID}/allocate
func (h *FleetHandler) AllocateAsset(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// HRHandler handles /api/v1/org/{orgID}/employees/* endpoints.
type HRHandler struct{}

// ListEmployees returns all employees for an org.
// GET /api/v1/org/{orgID}/employees
func (h *HRHandler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// ListCertifications returns certifications for an employee.
// GET /api/v1/org/{orgID}/employees/{employeeID}/certifications
func (h *HRHandler) ListCertifications(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
