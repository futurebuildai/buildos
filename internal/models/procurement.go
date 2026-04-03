package models

import (
	"time"

	"github.com/google/uuid"
)

// ProcurementItem represents a material or equipment tracking entry.
// All monetary values use Composite Currency Pattern (BIGINT cents + currency_code).
type ProcurementItem struct {
	ID                       uuid.UUID  `json:"id"`
	OrgID                    uuid.UUID  `json:"org_id"`
	ProjectID                uuid.UUID  `json:"project_id"`
	Description              string     `json:"description"`
	EstimatedCostCents       int64      `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode string    `json:"estimated_cost_currency_code"`
	Status                   ProcurementStatus `json:"status"`
	MustOrderDate            *time.Time `json:"must_order_date,omitempty"`
	ExpectedDeliveryDate     *time.Time `json:"expected_delivery_date,omitempty"`
	SupplierName             string     `json:"supplier_name,omitempty"`
	SupplierContact          string     `json:"supplier_contact,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
}

// ProcurementStatus represents the lifecycle state of a procurement item.
type ProcurementStatus string

const (
	ProcurementPending   ProcurementStatus = "PENDING"
	ProcurementWarning   ProcurementStatus = "WARNING"
	ProcurementCritical  ProcurementStatus = "CRITICAL"
	ProcurementDelivered ProcurementStatus = "DELIVERED"
	ProcurementCancelled ProcurementStatus = "CANCELLED"
)

// ValidProcurementStatuses lists all allowed procurement states.
var ValidProcurementStatuses = map[ProcurementStatus]bool{
	ProcurementPending:   true,
	ProcurementWarning:   true,
	ProcurementCritical:  true,
	ProcurementDelivered: true,
	ProcurementCancelled: true,
}

// ProcurementCostSummary holds per-currency cost totals for a project.
type ProcurementCostSummary struct {
	CurrencyCode string `json:"currency_code"`
	TotalCents   int64  `json:"total_cents"`
	ItemCount    int    `json:"item_count"`
}
