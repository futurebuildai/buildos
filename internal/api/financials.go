package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// BudgetServicer is the subset of *service.BudgetService consumed by
// FinancialsHandler. Defined as an interface so handlers can be unit-
// tested against a mock without spinning up a database.
type BudgetServicer interface {
	GetProjectBudgets(ctx context.Context, projectID, callerOrgID uuid.UUID) ([]models.ProjectBudget, error)
	GetOrgFinancialsSummary(ctx context.Context, orgID uuid.UUID, currencyCode string) (service.FinancialsSummary, error)
	GetARAging(ctx context.Context, orgID uuid.UUID, currencyCode string) ([]models.ARAgingSnapshot, error)
	GetProjectFinancials(ctx context.Context, orgID uuid.UUID, currencyCode string) ([]models.ProjectFinancial, error)
	CreateInvoice(ctx context.Context, callerOrgID uuid.UUID, in service.CreateInvoiceInput) (models.Invoice, error)
	UpdateInvoice(ctx context.Context, callerOrgID uuid.UUID, in service.UpdateInvoiceInput) (models.Invoice, error)
}

// FinancialsHandler handles /api/v1/org/{orgID}/financials/* and
// /api/v1/projects/{projectID}/{budgets,invoices} endpoints.
type FinancialsHandler struct {
	svc BudgetServicer
}

// NewFinancialsHandler creates a handler bound to the given service.
func NewFinancialsHandler(svc BudgetServicer) *FinancialsHandler {
	return &FinancialsHandler{svc: svc}
}

// ---------- Org-scoped reads ----------

// Summary returns corporate budget rollups + AR aging for an org.
// GET /api/v1/org/{orgID}/financials/summary[?currency=USD]
func (h *FinancialsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	currency := r.URL.Query().Get("currency")
	summary, err := h.svc.GetOrgFinancialsSummary(r.Context(), orgID, currency)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, summary)
}

// ARAging returns latest AR aging snapshot per currency for an org.
// GET /api/v1/org/{orgID}/financials/ar-aging[?currency=USD]
func (h *FinancialsHandler) ARAging(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	currency := r.URL.Query().Get("currency")
	snapshots, err := h.svc.GetARAging(r.Context(), orgID, currency)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"snapshots": snapshots})
}

// ProjectFinancials returns per-project financial rollups for an org.
// GET /api/v1/org/{orgID}/financials/projects[?currency=USD]
func (h *FinancialsHandler) ProjectFinancials(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.requireOrgIDFromURL(w, r)
	if !ok {
		return
	}
	currency := r.URL.Query().Get("currency")
	projects, err := h.svc.GetProjectFinancials(r.Context(), orgID, currency)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"projects": projects})
}

// ---------- Project-scoped reads ----------

// ListBudgets returns budget rows for a project.
// GET /api/v1/projects/{projectID}/budgets
func (h *FinancialsHandler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := h.callerOrgID(w, r)
	if !ok {
		return
	}
	budgets, err := h.svc.GetProjectBudgets(r.Context(), projectID, callerOrg)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"budgets": budgets})
}

// ---------- Project-scoped writes ----------

type createInvoiceRequest struct {
	VendorName    string  `json:"vendor_name"`
	InvoiceNumber *string `json:"invoice_number,omitempty"`
	AmountCents   int64   `json:"amount_cents"`
	CurrencyCode  string  `json:"currency_code"`
	WBSCode       *string `json:"wbs_code,omitempty"`
	DueDate       *string `json:"due_date,omitempty"` // RFC3339 date or full timestamp
}

// CreateInvoice records a new invoice for a project.
// POST /api/v1/projects/{projectID}/invoices
func (h *FinancialsHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	callerOrg, ok := h.callerOrgID(w, r)
	if !ok {
		return
	}

	var body createInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	dueDate, err := parseOptionalDate(body.DueDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "due_date must be RFC3339")
		return
	}

	inv, err := h.svc.CreateInvoice(r.Context(), callerOrg, service.CreateInvoiceInput{
		ProjectID:     projectID,
		VendorName:    body.VendorName,
		InvoiceNumber: body.InvoiceNumber,
		AmountCents:   body.AmountCents,
		CurrencyCode:  body.CurrencyCode,
		WBSCode:       body.WBSCode,
		DueDate:       dueDate,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, map[string]any{"invoice": inv})
}

type updateInvoiceRequest struct {
	Status   *string `json:"status,omitempty"`
	PaidDate *string `json:"paid_date,omitempty"`
}

// UpdateInvoice modifies an existing invoice.
// PUT /api/v1/projects/{projectID}/invoices/{invoiceID}
func (h *FinancialsHandler) UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	projectID, ok := parseUUIDFromURL(w, r, "projectID")
	if !ok {
		return
	}
	invoiceID, ok := parseUUIDFromURL(w, r, "invoiceID")
	if !ok {
		return
	}
	callerOrg, ok := h.callerOrgID(w, r)
	if !ok {
		return
	}

	var body updateInvoiceRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}

	paidDate, err := parseOptionalDate(body.PaidDate)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "paid_date must be RFC3339")
		return
	}

	inv, err := h.svc.UpdateInvoice(r.Context(), callerOrg, service.UpdateInvoiceInput{
		InvoiceID: invoiceID,
		ProjectID: projectID,
		Status:    body.Status,
		PaidDate:  paidDate,
	})
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"invoice": inv})
}

// ---------- helpers ----------

// requireOrgIDFromURL extracts {orgID} from the URL and verifies it
// matches the authenticated caller's org_id claim. On mismatch or
// parse failure, writes the response and returns false.
func (h *FinancialsHandler) requireOrgIDFromURL(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	urlOrg, ok := parseUUIDFromURL(w, r, "orgID")
	if !ok {
		return uuid.Nil, false
	}
	callerOrg, ok := h.callerOrgID(w, r)
	if !ok {
		return uuid.Nil, false
	}
	if urlOrg != callerOrg {
		writeErrorResponse(w, r, http.StatusForbidden, "FORBIDDEN", "org_id mismatch")
		return uuid.Nil, false
	}
	return urlOrg, true
}

// callerOrgID returns the caller's org_id from JWT claims, parsed as UUID.
// Writes 401 on parse failure (claim is malformed).
func (h *FinancialsHandler) callerOrgID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	claims := mw.MustClaimsFromContext(r.Context())
	parsed, err := uuid.Parse(claims.OrgID)
	if err != nil {
		writeErrorResponse(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "invalid org_id claim")
		return uuid.Nil, false
	}
	return parsed, true
}

// writeServiceError maps a BudgetService sentinel error to an HTTP response
// per the API_CONTRACT.md error code conventions.
func (h *FinancialsHandler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		writeErrorResponse(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, service.ErrCrossCurrency):
		writeErrorResponse(w, r, http.StatusUnprocessableEntity, "CROSS_CURRENCY_ERROR", err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// parseUUIDFromURL extracts a chi URL param and parses it as UUID.
// On failure, writes 400 and returns false.
func parseUUIDFromURL(w http.ResponseWriter, r *http.Request, key string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, key)
	parsed, err := uuid.Parse(raw)
	if err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid "+key)
		return uuid.Nil, false
	}
	return parsed, true
}

// parseOptionalDate parses an optional RFC3339 date or timestamp string.
// Empty/nil input returns (nil, nil).
func parseOptionalDate(s *string) (*time.Time, error) {
	if s == nil || *s == "" {
		return nil, nil
	}
	// Accept both date-only (YYYY-MM-DD) and full RFC3339.
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, *s); err == nil {
			return &t, nil
		}
	}
	return nil, errors.New("unparseable date")
}
