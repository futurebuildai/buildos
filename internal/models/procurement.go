package models

import (
	"time"

	"github.com/google/uuid"
)

// Procurement item statuses. Match the `status` column in
// procurement_items (free-text in SQL — these are the only producers).
//
//	OK       — must_order_date is comfortably in the future.
//	WARNING  — must_order_date is within the warning horizon (e.g., 7d).
//	CRITICAL — must_order_date has passed; order now or schedule slips.
//	ORDERED  — purchase order placed; ordered_at + po_number populated.
//
// Transitions are linear OK → WARNING → CRITICAL → ORDERED. The agent
// (ProcurementCheckWorker, Phase A3+) computes the time-based statuses
// daily; humans transition to ORDERED via the API.
const (
	ProcurementStatusOK       = "OK"
	ProcurementStatusWarning  = "WARNING"
	ProcurementStatusCritical = "CRITICAL"
	ProcurementStatusOrdered  = "ORDERED"
)

// IsValidProcurementStatus reports whether s is one of the allowed
// status values.
func IsValidProcurementStatus(s string) bool {
	switch s {
	case ProcurementStatusOK, ProcurementStatusWarning, ProcurementStatusCritical, ProcurementStatusOrdered:
		return true
	default:
		return false
	}
}

// ProcurementItem mirrors the procurement_items row. Cost is stored as
// the Composite Currency pair (cents + currency_code). MustOrderDate
// is computed and persisted by the SQL layer (or set on insert by the
// service when need_by_date + lead_time + weather_buffer are known).
type ProcurementItem struct {
	ID                        uuid.UUID  `json:"id"`
	ProjectID                 uuid.UUID  `json:"project_id"`
	OrgID                     uuid.UUID  `json:"org_id"`
	Name                      string     `json:"name"`
	WBSCode                   string     `json:"wbs_code"`
	EstimatedCostCents        int64      `json:"estimated_cost_cents"`
	EstimatedCostCurrencyCode string     `json:"estimated_cost_currency_code"`
	LeadTimeDays              int        `json:"lead_time_days"`
	WeatherBufferDays         int        `json:"weather_buffer_days"`
	VendorID                  *uuid.UUID `json:"vendor_id,omitempty"`
	NeedByDate                *time.Time `json:"need_by_date,omitempty"`
	MustOrderDate             *time.Time `json:"must_order_date,omitempty"`
	Status                    string     `json:"status"`
	OrderedAt                 *time.Time `json:"ordered_at,omitempty"`
	PONumber                  *string    `json:"po_number,omitempty"`
	StatusChangedAt           time.Time  `json:"status_changed_at"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
}
