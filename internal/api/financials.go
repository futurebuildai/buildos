package api

import "net/http"

// FinancialsHandler handles /api/v1/org/{orgID}/financials/* endpoints.
type FinancialsHandler struct{}

// Summary returns corporate budget + AR aging for an org.
// GET /api/v1/org/{orgID}/financials/summary
func (h *FinancialsHandler) Summary(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// ARAging returns AR aging snapshots for an org.
// GET /api/v1/org/{orgID}/financials/ar-aging
func (h *FinancialsHandler) ARAging(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// ProjectFinancials returns financial summary per project.
// GET /api/v1/org/{orgID}/financials/projects
func (h *FinancialsHandler) ProjectFinancials(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// ListBudgets returns budgets for a specific project.
// GET /api/v1/projects/{projectID}/budgets
func (h *FinancialsHandler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// CreateInvoice records a new invoice for a project.
// POST /api/v1/projects/{projectID}/invoices
func (h *FinancialsHandler) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}

// UpdateInvoice modifies an existing invoice.
// PUT /api/v1/projects/{projectID}/invoices/{invoiceID}
func (h *FinancialsHandler) UpdateInvoice(w http.ResponseWriter, r *http.Request) {
	writeNotImplemented(w, r)
}
