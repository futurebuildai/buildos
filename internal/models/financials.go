package models

import (
	"time"

	"github.com/google/uuid"
)

// ProjectBudget is one budget row per WBS phase of a project. The three
// monetary pairs (estimated, committed, actual) MUST share the same
// currency_code; this is enforced by the chk_budget_currency_match CHECK
// constraint in migration 002.
type ProjectBudget struct {
	ID                        uuid.UUID `json:"id"`
	ProjectID                 uuid.UUID `json:"project_id"`
	WBSCode                   string    `json:"wbs_code"`
	PhaseName                 string    `json:"phase_name"`
	EstimatedCostCents        int64     `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode string    `json:"estimated_cost_currency_code"`
	CommittedCostCents        int64     `json:"committed_cost_cents"`
	CommittedCostCurrencyCode string    `json:"committed_cost_currency_code"`
	ActualCostCents           int64     `json:"actual_cost_cents"`
	ActualCostCurrencyCode    string    `json:"actual_cost_currency_code"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// CorporateBudget is one rollup row per (org, fiscal_year, quarter,
// currency_code). Aggregation is always grouped by currency; never summed
// across currencies. Produced by the corporate_rollup River job.
type CorporateBudget struct {
	ID                  uuid.UUID `json:"id"`
	OrgID               uuid.UUID `json:"org_id"`
	FiscalYear          int       `json:"fiscal_year"`
	Quarter             int       `json:"quarter"`
	CurrencyCode        string    `json:"currency_code"`
	TotalEstimatedCents int64     `json:"total_estimated_cents"`
	TotalCommittedCents int64     `json:"total_committed_cents"`
	TotalActualCents    int64     `json:"total_actual_cents"`
	ProjectCount        int       `json:"project_count"`
	LastRollupAt        time.Time `json:"last_rollup_at"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// ARAgingSnapshot is one AR aging row per (org, snapshot_date,
// currency_code). The financials API returns the most recent snapshot
// per currency.
type ARAgingSnapshot struct {
	ID                   uuid.UUID `json:"id"`
	OrgID                uuid.UUID `json:"org_id"`
	SnapshotDate         time.Time `json:"snapshot_date"`
	CurrencyCode         string    `json:"currency_code"`
	CurrentCents         int64     `json:"current_cents"`
	Days30Cents          int64     `json:"days_30_cents"`
	Days60Cents          int64     `json:"days_60_cents"`
	Days90PlusCents      int64     `json:"days_90_plus_cents"`
	TotalReceivableCents int64     `json:"total_receivable_cents"`
	CreatedAt            time.Time `json:"created_at"`
}

// Invoice represents a vendor invoice for a project.
type Invoice struct {
	ID            uuid.UUID  `json:"id"`
	ProjectID     uuid.UUID  `json:"project_id"`
	OrgID         uuid.UUID  `json:"org_id"`
	VendorName    string     `json:"vendor_name"`
	InvoiceNumber *string    `json:"invoice_number,omitempty"`
	AmountCents   int64      `json:"amount_cents"`
	CurrencyCode  string     `json:"currency_code"`
	WBSCode       *string    `json:"wbs_code,omitempty"`
	Status        string     `json:"status"`
	DueDate       *time.Time `json:"due_date,omitempty"`
	PaidDate      *time.Time `json:"paid_date,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

// ProjectFinancial is a derived per-project aggregation: one row per
// (project, currency_code) summing across all WBS phases. Returned by
// the GET /api/v1/org/{orgID}/financials/projects endpoint. Not stored
// directly; computed on demand by the FinancialsStore.
type ProjectFinancial struct {
	ProjectID           uuid.UUID `json:"project_id"`
	ProjectName         string    `json:"project_name"`
	CurrencyCode        string    `json:"currency_code"`
	TotalEstimatedCents int64     `json:"total_estimated_cents"`
	TotalCommittedCents int64     `json:"total_committed_cents"`
	TotalActualCents    int64     `json:"total_actual_cents"`
	PhaseCount          int       `json:"phase_count"`
}

// Invoice statuses. Match the migration's default and CHECK-able values.
const (
	InvoiceStatusPending  = "pending"
	InvoiceStatusApproved = "approved"
	InvoiceStatusRejected = "rejected"
	InvoiceStatusPaid     = "paid"
)

// IsValidInvoiceStatus reports whether a string is one of the allowed
// status values. The service layer should call this when accepting status
// updates from API requests.
func IsValidInvoiceStatus(s string) bool {
	switch s {
	case InvoiceStatusPending, InvoiceStatusApproved, InvoiceStatusRejected, InvoiceStatusPaid:
		return true
	default:
		return false
	}
}
