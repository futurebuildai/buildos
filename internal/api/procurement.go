package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ProcurementServicer is the subset of *service.ProcurementService
// consumed by ProcurementHandler. Defined consumer-side so the handler
// can be unit-tested without spinning up a database.
type ProcurementServicer interface {
	ListProcurement(ctx context.Context, projectID, callerOrgID uuid.UUID, statusFilter []string) ([]models.ProcurementItem, error)
	CreateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, in service.CreateProcurementItemInput) (models.ProcurementItem, error)
	UpdateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, in service.UpdateProcurementItemInput) (models.ProcurementItem, error)
}

// ProcurementHandler handles /api/v1/projects/{projectID}/procurement/* endpoints.
type ProcurementHandler struct {
	svc ProcurementServicer
}

// NewProcurementHandler creates a handler bound to the given service.
func NewProcurementHandler(svc ProcurementServicer) *ProcurementHandler {
	return &ProcurementHandler{svc: svc}
}

// List returns procurement items for a project.
//
// GET /api/v1/projects/{projectID}/procurement[?status=WARNING,CRITICAL]
func (h *ProcurementHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}
	statusFilter := splitCSVParam(r, "status")

	items, err := h.svc.ListProcurement(r.Context(), projectID, callerOrg, statusFilter)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

type createProcurementRequest struct {
	Name                      string     `json:"name"`
	WBSCode                   string     `json:"wbs_code"`
	EstimatedCostCents        int64      `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode string     `json:"estimated_cost_currency_code"`
	LeadTimeDays              int        `json:"lead_time_days"`
	WeatherBufferDays         int        `json:"weather_buffer_days"`
	VendorID                  *uuid.UUID `json:"vendor_id,omitempty"`
	NeedByDate                *string    `json:"need_by_date,omitempty"`
}

// Create adds a new procurement item.
//
// POST /api/v1/projects/{projectID}/procurement
func (h *ProcurementHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body createProcurementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	needBy, err := parseOptionalDate(body.NeedByDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "need_by_date must be RFC3339 or YYYY-MM-DD")
		return
	}

	item, err := h.svc.CreateProcurementItem(r.Context(), callerOrg, service.CreateProcurementItemInput{
		ProjectID:                 projectID,
		Name:                      body.Name,
		WBSCode:                   body.WBSCode,
		EstimatedCostCents:        body.EstimatedCostCents,
		EstimatedCostCurrencyCode: body.EstimatedCostCurrencyCode,
		LeadTimeDays:              body.LeadTimeDays,
		WeatherBufferDays:         body.WeatherBufferDays,
		VendorID:                  body.VendorID,
		NeedByDate:                needBy,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"item": item})
}

type updateProcurementRequest struct {
	Status    *string `json:"status,omitempty"`
	PONumber  *string `json:"po_number,omitempty"`
	OrderedAt *string `json:"ordered_at,omitempty"`
}

// Update modifies a procurement item.
//
// PUT /api/v1/projects/{projectID}/procurement/{itemID}
func (h *ProcurementHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	itemID, ok := parseUUIDFromURL(w, r, "itemID")
	if !ok {
		return
	}
	callerOrg, ok := callerOrgIDFromClaims(w, r)
	if !ok {
		return
	}

	var body updateProcurementRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	orderedAt, err := parseOptionalRFC3339(body.OrderedAt)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "ordered_at must be RFC3339")
		return
	}

	item, err := h.svc.UpdateProcurementItem(r.Context(), callerOrg, service.UpdateProcurementItemInput{
		ItemID:    itemID,
		ProjectID: projectID,
		Status:    body.Status,
		PONumber:  body.PONumber,
		OrderedAt: orderedAt,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"item": item})
}

// writeServiceError maps ProcurementService sentinels to HTTP responses.
func (h *ProcurementHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrProcurementItemNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "procurement item not found")
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", "project not found")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// parseOptionalRFC3339 differs from parseOptionalDate (financials.go) in
// that it ONLY accepts full RFC 3339 timestamps. Used for ordered_at,
// which carries a timestamp not just a date.
func parseOptionalRFC3339(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
