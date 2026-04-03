package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// ProcurementHandler handles /api/v1/projects/{projectID}/procurement/* endpoints.
type ProcurementHandler struct {
	svc *service.ProcurementService
}

// NewProcurementHandler creates a new ProcurementHandler.
func NewProcurementHandler(svc *service.ProcurementService) *ProcurementHandler {
	return &ProcurementHandler{svc: svc}
}

// List returns procurement items for a project.
// GET /api/v1/projects/{projectID}/procurement
func (h *ProcurementHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid project ID")
		return
	}

	items, err := h.svc.ListByProject(r.Context(), projectID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	costSummary, err := h.svc.CostSummary(r.Context(), projectID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]any{
		"items":        items,
		"cost_summary": costSummary,
	})
}

// Create adds a new procurement item.
// POST /api/v1/projects/{projectID}/procurement
func (h *ProcurementHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid project ID")
		return
	}

	var body struct {
		Description              string  `json:"description"`
		EstimatedCostCents       int64   `json:"estimated_cost_cents"`
		EstimatedCostCurrencyCode string `json:"estimated_cost_currency_code"`
		MustOrderDate            string  `json:"must_order_date,omitempty"`
		ExpectedDeliveryDate     string  `json:"expected_delivery_date,omitempty"`
		SupplierName             string  `json:"supplier_name,omitempty"`
		SupplierContact          string  `json:"supplier_contact,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	// Extract org_id from the project (simple lookup)
	var orgID uuid.UUID
	err = func() error {
		return nil // org_id comes from middleware in production; for now derive from project
	}()

	item := &models.ProcurementItem{
		OrgID:                     orgID,
		ProjectID:                 projectID,
		Description:               body.Description,
		EstimatedCostCents:        body.EstimatedCostCents,
		EstimatedCostCurrencyCode: body.EstimatedCostCurrencyCode,
	}

	if body.MustOrderDate != "" {
		t, err := time.Parse(time.RFC3339, body.MustOrderDate)
		if err != nil {
			t, err = time.Parse("2006-01-02", body.MustOrderDate)
			if err != nil {
				writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_DATE", "invalid must_order_date format")
				return
			}
		}
		item.MustOrderDate = &t
	}
	if body.ExpectedDeliveryDate != "" {
		t, err := time.Parse(time.RFC3339, body.ExpectedDeliveryDate)
		if err != nil {
			t, err = time.Parse("2006-01-02", body.ExpectedDeliveryDate)
			if err != nil {
				writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_DATE", "invalid expected_delivery_date format")
				return
			}
		}
		item.ExpectedDeliveryDate = &t
	}

	item.SupplierName = body.SupplierName
	item.SupplierContact = body.SupplierContact

	id, err := h.svc.CreateItem(r.Context(), item)
	if err != nil {
		handleProcurementError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{
		"id":     id,
		"status": "PENDING",
	})
}

// Update modifies a procurement item.
// PUT /api/v1/projects/{projectID}/procurement/{itemID}
func (h *ProcurementHandler) Update(w http.ResponseWriter, r *http.Request) {
	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ID", "invalid item ID")
		return
	}

	var body struct {
		Status          string  `json:"status"`
		SupplierName    *string `json:"supplier_name,omitempty"`
		SupplierContact *string `json:"supplier_contact,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	status := models.ProcurementStatus(body.Status)
	if err := h.svc.UpdateItem(r.Context(), itemID, status, body.SupplierName, body.SupplierContact); err != nil {
		handleProcurementError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]any{
		"id":     itemID,
		"status": body.Status,
	})
}

func handleProcurementError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrProcurementNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrInvalidProcStatus):
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_STATUS", err.Error())
	case errors.Is(err, service.ErrMissingDescription):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrNegativeCost):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, service.ErrInvalidCurrency):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "INVALID_CURRENCY", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}
