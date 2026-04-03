package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// FleetHandler handles fleet management endpoints.
type FleetHandler struct {
	fleetSvc *service.FleetService
}

// NewFleetHandler creates a new FleetHandler.
func NewFleetHandler(fleetSvc *service.FleetService) *FleetHandler {
	return &FleetHandler{fleetSvc: fleetSvc}
}

// ListAssets returns all fleet assets for an org.
// GET /api/v1/org/{orgID}/fleet
func (h *FleetHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org ID")
		return
	}

	assets, err := h.fleetSvc.ListAssets(r.Context(), orgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if assets == nil {
		assets = []models.FleetAsset{}
	}

	writeJSON(w, r, http.StatusOK, map[string]any{"assets": assets})
}

// CreateAsset registers a new fleet asset.
// POST /api/v1/org/{orgID}/fleet
func (h *FleetHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org ID")
		return
	}

	var body struct {
		Name         string `json:"name"`
		AssetType    string `json:"asset_type"`
		SerialNumber string `json:"serial_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	asset := &models.FleetAsset{
		OrgID:        orgID,
		Name:         body.Name,
		AssetType:    body.AssetType,
		SerialNumber: body.SerialNumber,
	}

	id, err := h.fleetSvc.CreateAsset(r.Context(), asset)
	if err != nil {
		if errors.Is(err, service.ErrMissingAssetName) || errors.Is(err, service.ErrMissingAssetType) {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id, "name": body.Name, "status": "available"})
}

// AllocateAsset allocates equipment to a project.
// POST /api/v1/org/{orgID}/fleet/{assetID}/allocate
func (h *FleetHandler) AllocateAsset(w http.ResponseWriter, r *http.Request) {
	assetID, err := uuid.Parse(chi.URLParam(r, "assetID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid asset ID")
		return
	}

	var body struct {
		ProjectID string `json:"project_id"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	projectID, err := uuid.Parse(body.ProjectID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid project_id")
		return
	}

	startDate, err := time.Parse("2006-01-02", body.StartDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid start_date format (YYYY-MM-DD)")
		return
	}

	endDate, err := time.Parse("2006-01-02", body.EndDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid end_date format (YYYY-MM-DD)")
		return
	}

	alloc := &models.EquipmentAllocation{
		AssetID:   assetID,
		ProjectID: projectID,
		StartDate: startDate,
		EndDate:   endDate,
	}

	id, err := h.fleetSvc.AllocateAsset(r.Context(), alloc)
	if err != nil {
		if errors.Is(err, service.ErrAllocationConflict) {
			writeErrorResponse(w, r, http.StatusConflict, "ALLOCATION_CONFLICT", err.Error())
			return
		}
		if errors.Is(err, service.ErrAssetNotFound) {
			writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		if errors.Is(err, service.ErrInvalidDateRange) {
			writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
			return
		}
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id, "asset_id": assetID, "status": "allocated"})
}

// HRHandler handles HR endpoints.
type HRHandler struct {
	hrSvc *service.HRService
}

// NewHRHandler creates a new HRHandler.
func NewHRHandler(hrSvc *service.HRService) *HRHandler {
	return &HRHandler{hrSvc: hrSvc}
}

// ListEmployees returns all employees for an org.
// GET /api/v1/org/{orgID}/employees
func (h *HRHandler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid org ID")
		return
	}

	employees, err := h.hrSvc.ListEmployees(r.Context(), orgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if employees == nil {
		employees = []models.Employee{}
	}

	writeJSON(w, r, http.StatusOK, map[string]any{"employees": employees})
}

// ListCertifications returns certifications for an employee.
// GET /api/v1/org/{orgID}/employees/{employeeID}/certifications
func (h *HRHandler) ListCertifications(w http.ResponseWriter, r *http.Request) {
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid employee ID")
		return
	}

	_ = mw.MustClaimsFromContext(r.Context()) // Auth check

	certs, err := h.hrSvc.ListCertifications(r.Context(), employeeID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if certs == nil {
		certs = []models.Certification{}
	}

	writeJSON(w, r, http.StatusOK, map[string]any{"certifications": certs})
}
