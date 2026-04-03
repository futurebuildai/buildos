package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// FinancialsHandler handles /api/v1/org/{orgID}/financials/* and project-level financial endpoints.
type FinancialsHandler struct {
	budgetSvc    *service.BudgetService
	corporateSvc *service.CorporateFinancialsService
}

// NewFinancialsHandler creates a handler with service dependencies.
func NewFinancialsHandler(budgetSvc *service.BudgetService, corporateSvc *service.CorporateFinancialsService) *FinancialsHandler {
	return &FinancialsHandler{
		budgetSvc:    budgetSvc,
		corporateSvc: corporateSvc,
	}
}

// Summary returns corporate budget + AR aging for an org.
// GET /api/v1/org/{orgID}/financials/summary
func (h *FinancialsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseOrgID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", err.Error())
		return
	}

	cc := currencyFromQuery(r)
	summary, err := h.corporateSvc.Summary(r.Context(), orgID, cc)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, summary)
}

// ARAging returns AR aging snapshots for an org.
// GET /api/v1/org/{orgID}/financials/ar-aging
func (h *FinancialsHandler) ARAging(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseOrgID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", err.Error())
		return
	}

	cc := currencyFromQuery(r)
	if cc == "" {
		cc = "USD"
	}
	if !service.SupportedCurrencies[cc] {
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "INVALID_CURRENCY", "only USD and CAD are supported")
		return
	}

	// Use the corporate service's store to get the latest snapshot
	summary, err := h.corporateSvc.Summary(r.Context(), orgID, cc)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, summary.LatestARAging)
}

// ProjectFinancials returns financial summary per project.
// GET /api/v1/org/{orgID}/financials/projects
func (h *FinancialsHandler) ProjectFinancials(w http.ResponseWriter, r *http.Request) {
	orgID, err := parseOrgID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", err.Error())
		return
	}

	cc := currencyFromQuery(r)
	summaries, err := h.corporateSvc.ProjectFinancials(r.Context(), orgID, cc)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, summaries)
}

// ListBudgets returns budgets for a specific project.
// GET /api/v1/projects/{projectID}/budgets
func (h *FinancialsHandler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", err.Error())
		return
	}

	cc := currencyFromQuery(r)
	budgets, err := h.budgetSvc.ListBudgets(r.Context(), projectID, cc)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, budgets)
}

// createInvoiceRequest is the request body for CreateInvoice.
type createInvoiceRequest struct {
	VendorName    string  `json:"vendor_name"`
	InvoiceNumber *string `json:"invoice_number,omitempty"`
	AmountCents   int64   `json:"amount_cents"`
	CurrencyCode  string  `json:"currency_code"`
	WBSCode       *string `json:"wbs_code,omitempty"`
	DueDate       *string `json:"due_date,omitempty"`
}

// CreateInvoice records a new invoice for a project.
// POST /api/v1/projects/{projectID}/invoices
func (h *FinancialsHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseProjectID(r)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", err.Error())
		return
	}

	claims := mw.MustClaimsFromContext(r.Context())
	orgID, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ORG_ID", "org_id in token is not a valid UUID")
		return
	}

	var req createInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	inv := &models.Invoice{
		ProjectID:     projectID,
		OrgID:         orgID,
		VendorName:    req.VendorName,
		InvoiceNumber: req.InvoiceNumber,
		AmountCents:   req.AmountCents,
		CurrencyCode:  req.CurrencyCode,
		WBSCode:       req.WBSCode,
		Status:        models.InvoiceStatusPending,
	}

	id, err := h.budgetSvc.CreateInvoice(r.Context(), inv)
	if err != nil {
		handleServiceError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusCreated, map[string]any{"id": id})
}

// updateInvoiceRequest is the request body for UpdateInvoice.
type updateInvoiceRequest struct {
	Action string `json:"action"` // approve, reject, pay
}

// UpdateInvoice modifies an existing invoice.
// PUT /api/v1/projects/{projectID}/invoices/{invoiceID}
func (h *FinancialsHandler) UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	invoiceID, err := uuid.Parse(chi.URLParam(r, "invoiceID"))
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_INVOICE_ID", "invalid invoice ID")
		return
	}

	var req updateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_BODY", "invalid JSON body")
		return
	}

	switch req.Action {
	case "approve":
		err = h.budgetSvc.ApproveInvoice(r.Context(), invoiceID)
	case "reject":
		err = h.budgetSvc.RejectInvoice(r.Context(), invoiceID)
	case "pay":
		err = h.budgetSvc.PayInvoice(r.Context(), invoiceID)
	default:
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_ACTION", "action must be approve, reject, or pay")
		return
	}

	if err != nil {
		handleServiceError(w, r, err)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{"status": "updated"})
}

// --- helpers ---

func parseOrgID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "orgID"))
}

func parseProjectID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "projectID"))
}

func currencyFromQuery(r *http.Request) string {
	cc := r.URL.Query().Get("currency")
	if cc == "" {
		return "USD"
	}
	return cc
}

func handleServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrCrossCurrency):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "CROSS_CURRENCY_ERROR", err.Error())
	case errors.Is(err, service.ErrInvalidCurrency):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "INVALID_CURRENCY", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred")
	}
}
