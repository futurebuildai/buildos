package models

import (
	"time"

	"github.com/google/uuid"
)

// ProjectBudget represents a budget entry per WBS phase within a project.
// All monetary values use the Composite Currency Pattern: *Cents int64 + CurrencyCode string.
// The DB constraint chk_budget_currency_match guarantees all currency codes match within a row.
type ProjectBudget struct {
	ID                         uuid.UUID `json:"id"`
	ProjectID                  uuid.UUID `json:"project_id"`
	WBSCode                    string    `json:"wbs_code"`
	PhaseName                  string    `json:"phase_name"`
	EstimatedCostCents         int64     `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode  string    `json:"estimated_cost_currency_code"`
	CommittedCostCents         int64     `json:"committed_cost_cents"`
	CommittedCostCurrencyCode  string    `json:"committed_cost_currency_code"`
	ActualCostCents            int64     `json:"actual_cost_cents"`
	ActualCostCurrencyCode     string    `json:"actual_cost_currency_code"`
	CreatedAt                  time.Time `json:"created_at"`
	UpdatedAt                  time.Time `json:"updated_at"`
}

// VarianceCents returns (estimated - actual) in cents.
// Caller must ensure currency codes match before calling.
func (b *ProjectBudget) VarianceCents() int64 {
	return b.EstimatedCostCents - b.ActualCostCents
}

// CorporateBudget represents a quarterly rollup across all projects in an org,
// grouped by currency_code.
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

// ARAgingSnapshot records accounts receivable buckets at a point in time,
// scoped to an org and currency.
type ARAgingSnapshot struct {
	ID                  uuid.UUID `json:"id"`
	OrgID               uuid.UUID `json:"org_id"`
	SnapshotDate        time.Time `json:"snapshot_date"`
	CurrencyCode        string    `json:"currency_code"`
	CurrentCents        int64     `json:"current_cents"`
	Days30Cents         int64     `json:"days_30_cents"`
	Days60Cents         int64     `json:"days_60_cents"`
	Days90PlusCents     int64     `json:"days_90_plus_cents"`
	TotalReceivableCents int64   `json:"total_receivable_cents"`
	CreatedAt           time.Time `json:"created_at"`
}

// Invoice represents a vendor invoice charged to a project.
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

// InvoiceStatus constants.
const (
	InvoiceStatusPending  = "pending"
	InvoiceStatusApproved = "approved"
	InvoiceStatusRejected = "rejected"
	InvoiceStatusPaid     = "paid"
)

// ProjectFinancialSummary is a computed view of a project's financial health.
type ProjectFinancialSummary struct {
	ProjectID          uuid.UUID `json:"project_id"`
	ProjectName        string    `json:"project_name"`
	CurrencyCode       string    `json:"currency_code"`
	TotalEstimatedCents int64   `json:"total_estimated_cents"`
	TotalCommittedCents int64   `json:"total_committed_cents"`
	TotalActualCents    int64   `json:"total_actual_cents"`
	VarianceCents       int64   `json:"variance_cents"`
	InvoiceCount        int     `json:"invoice_count"`
	PendingInvoiceCents int64   `json:"pending_invoice_cents"`
}
