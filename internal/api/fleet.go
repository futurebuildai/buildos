package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// FleetServicer is the subset of *service.FleetService consumed by
// FleetHandler.
type FleetServicer interface {
	ListAssets(ctx context.Context, callerOrgID uuid.UUID, statusFilter []string) ([]models.FleetAsset, error)
	CreateAsset(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.CreateAssetInput) (models.FleetAsset, error)
	AllocateAsset(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.AllocateAssetInput) (models.EquipmentAllocation, error)
}

// FleetHandler handles /api/v1/org/{orgID}/fleet/* endpoints.
type FleetHandler struct {
	svc FleetServicer
}

// NewFleetHandler creates a handler bound to the given service.
func NewFleetHandler(svc FleetServicer) *FleetHandler {
	return &FleetHandler{svc: svc}
}

// ListAssets returns fleet assets for the caller's org.
//
// GET /api/v1/org/{orgID}/fleet[?status=available,maintenance]
func (h *FleetHandler) ListAssets(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	statusFilter := splitCSVParam(r, "status")

	assets, err := h.svc.ListAssets(r.Context(), orgID, statusFilter)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"assets": assets})
}

type createAssetRequest struct {
	Name         string  `json:"name"`
	AssetType    string  `json:"asset_type"`
	SerialNumber *string `json:"serial_number,omitempty"`
}

// CreateAsset adds a new fleet asset. Owner/admin only (RBAC at route).
//
// POST /api/v1/org/{orgID}/fleet
func (h *FleetHandler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}

	var body createAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	claims := mw.MustClaimsFromContext(r.Context())
	asset, err := h.svc.CreateAsset(r.Context(), orgID, claims.Sub, service.CreateAssetInput{
		Name:         body.Name,
		AssetType:    body.AssetType,
		SerialNumber: body.SerialNumber,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"asset": asset})
}

type allocateAssetRequest struct {
	ProjectID uuid.UUID `json:"project_id"`
	StartDate string    `json:"start_date"`
	EndDate   string    `json:"end_date"`
}

// AllocateAsset books an asset to a project for [start, end).
//
// POST /api/v1/org/{orgID}/fleet/{assetID}/allocate
func (h *FleetHandler) AllocateAsset(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	assetID, ok := parseUUIDFromURL(w, r, "assetID")
	if !ok {
		return
	}

	var body allocateAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	start, err := parseRequiredDate(body.StartDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "start_date must be RFC3339 or YYYY-MM-DD")
		return
	}
	end, err := parseRequiredDate(body.EndDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "end_date must be RFC3339 or YYYY-MM-DD")
		return
	}

	claims := mw.MustClaimsFromContext(r.Context())
	alloc, err := h.svc.AllocateAsset(r.Context(), orgID, claims.Sub, service.AllocateAssetInput{
		AssetID:   assetID,
		ProjectID: body.ProjectID,
		StartDate: start,
		EndDate:   end,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"allocation": alloc})
}

// writeServiceError maps FleetService sentinels to HTTP responses.
func (h *FleetHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrFleetAssetNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "fleet asset not found")
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
	case errors.Is(err, service.ErrAllocationConflict):
		writeErrorResponse(w, r, http.StatusConflict, "CONFLICT", "allocation overlaps an existing booking for this asset")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// parseRequiredDate accepts RFC3339 or YYYY-MM-DD. Errors on empty.
func parseRequiredDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("date is required")
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("unparseable date")
}

// HRServicer is the subset of *service.HRService consumed by HRHandler.
type HRServicer interface {
	ListEmployees(ctx context.Context, callerOrgID uuid.UUID) ([]models.Employee, error)
	ListCertifications(ctx context.Context, callerOrgID, employeeID uuid.UUID) ([]models.Certification, error)
}

// HRHandler handles /api/v1/org/{orgID}/employees/* endpoints.
type HRHandler struct {
	svc HRServicer
}

// NewHRHandler creates a handler bound to the given service.
func NewHRHandler(svc HRServicer) *HRHandler {
	return &HRHandler{svc: svc}
}

// ListEmployees returns all employees for an org. Owner/admin only
// (RBAC enforced at the route).
//
// GET /api/v1/org/{orgID}/employees
func (h *HRHandler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	employees, err := h.svc.ListEmployees(r.Context(), orgID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"employees": employees})
}

// ListCertifications returns certifications for an employee.
//
// GET /api/v1/org/{orgID}/employees/{employeeID}/certifications
func (h *HRHandler) ListCertifications(w http.ResponseWriter, r *http.Request) {
	orgID, ok := requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	employeeID, ok := parseUUIDFromURL(w, r, "employeeID")
	if !ok {
		return
	}
	certs, err := h.svc.ListCertifications(r.Context(), orgID, employeeID)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"certifications": certs})
}

// writeServiceError maps HRService sentinels to HTTP responses.
func (h *HRHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrEmployeeNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "employee not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}
