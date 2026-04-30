package models

import (
	"time"

	"github.com/google/uuid"
)

// Fleet asset statuses. Schema CHECK is advisory (free-text column);
// these are the only producers. The status flips via maintenance jobs
// later; for now CRUD only sets 'available'.
const (
	FleetAssetStatusAvailable   = "available"
	FleetAssetStatusUnavailable = "unavailable"
	FleetAssetStatusMaintenance = "maintenance"
)

// IsValidFleetAssetStatus reports whether s is one of the allowed
// values.
func IsValidFleetAssetStatus(s string) bool {
	switch s {
	case FleetAssetStatusAvailable, FleetAssetStatusUnavailable, FleetAssetStatusMaintenance:
		return true
	default:
		return false
	}
}

// FleetAsset mirrors a fleet_assets row. Each asset is org-scoped;
// allocations attach an asset to a project for a date range.
type FleetAsset struct {
	ID           uuid.UUID `json:"id"`
	OrgID        uuid.UUID `json:"org_id"`
	Name         string    `json:"name"`
	AssetType    string    `json:"asset_type"`
	SerialNumber *string   `json:"serial_number,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// EquipmentAllocation mirrors an equipment_allocations row. The
// [StartDate, EndDate) range is enforced by a GiST exclusion
// constraint — overlapping allocations for the same asset surface as
// SQLSTATE 23P01 and are mapped to ErrAllocationConflict in the store.
type EquipmentAllocation struct {
	ID        uuid.UUID `json:"id"`
	AssetID   uuid.UUID `json:"asset_id"`
	ProjectID uuid.UUID `json:"project_id"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	CreatedAt time.Time `json:"created_at"`
}
