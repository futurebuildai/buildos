package models

import (
	"time"

	"github.com/google/uuid"
)

// FleetAsset represents an equipment asset.
// Matches fleet_assets table from migration 003.
type FleetAsset struct {
	ID           uuid.UUID `json:"id"`
	OrgID        uuid.UUID `json:"org_id"`
	Name         string    `json:"name"`
	AssetType    string    `json:"asset_type"`
	SerialNumber string    `json:"serial_number"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

// FleetAssetStatus constants.
const (
	AssetStatusAvailable   = "available"
	AssetStatusAllocated   = "allocated"
	AssetStatusMaintenance = "maintenance"
	AssetStatusRetired     = "retired"
)

// EquipmentAllocation represents a time-bounded equipment assignment.
// Matches equipment_allocations table from migration 003.
type EquipmentAllocation struct {
	ID        uuid.UUID `json:"id"`
	AssetID   uuid.UUID `json:"asset_id"`
	ProjectID uuid.UUID `json:"project_id"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	CreatedAt time.Time `json:"created_at"`
}
