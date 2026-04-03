package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

var (
	ErrAssetNotFound       = errors.New("fleet asset not found")
	ErrAllocationConflict  = errors.New("equipment allocation conflict: overlapping date range")
	ErrInvalidDateRange    = errors.New("end_date must be after start_date")
	ErrMissingAssetName    = errors.New("asset name is required")
	ErrMissingAssetType    = errors.New("asset type is required")
)

// FleetService provides business logic for fleet management.
type FleetService struct {
	store *store.FleetStore
}

// NewFleetService creates a new FleetService.
func NewFleetService(s *store.FleetStore) *FleetService {
	return &FleetService{store: s}
}

// CreateAsset creates a new fleet asset with validation.
func (svc *FleetService) CreateAsset(ctx context.Context, asset *models.FleetAsset) (uuid.UUID, error) {
	if asset.Name == "" {
		return uuid.Nil, ErrMissingAssetName
	}
	if asset.AssetType == "" {
		return uuid.Nil, ErrMissingAssetType
	}
	if asset.Status == "" {
		asset.Status = models.AssetStatusAvailable
	}
	return svc.store.CreateAsset(ctx, asset)
}

// ListAssets returns all fleet assets for an org.
func (svc *FleetService) ListAssets(ctx context.Context, orgID uuid.UUID) ([]models.FleetAsset, error) {
	return svc.store.ListAssets(ctx, orgID)
}

// AllocateAsset allocates equipment to a project for a date range.
// The database GiST exclusion constraint prevents overlapping allocations.
func (svc *FleetService) AllocateAsset(ctx context.Context, alloc *models.EquipmentAllocation) (uuid.UUID, error) {
	if !alloc.EndDate.After(alloc.StartDate) {
		return uuid.Nil, ErrInvalidDateRange
	}

	// Verify asset exists
	_, err := svc.store.GetAsset(ctx, alloc.AssetID)
	if err != nil {
		return uuid.Nil, ErrAssetNotFound
	}

	id, err := svc.store.AllocateAsset(ctx, alloc)
	if err != nil {
		// GiST exclusion constraint violation
		if isExclusionViolation(err) {
			return uuid.Nil, ErrAllocationConflict
		}
		return uuid.Nil, err
	}

	return id, nil
}

// ListAllocations returns allocations for an asset.
func (svc *FleetService) ListAllocations(ctx context.Context, assetID uuid.UUID) ([]models.EquipmentAllocation, error) {
	return svc.store.ListAllocations(ctx, assetID)
}

func isExclusionViolation(err error) bool {
	// PostgreSQL exclusion constraint violation code: 23P01
	return strings.Contains(err.Error(), "23P01") || strings.Contains(err.Error(), "exclusion")
}

// FleetSummary returns fleet utilization stats.
type FleetSummary struct {
	TotalAssets     int `json:"total_assets"`
	Available       int `json:"available"`
	Allocated       int `json:"allocated"`
	InMaintenance   int `json:"in_maintenance"`
}

// Summary returns fleet utilization statistics.
func (svc *FleetService) Summary(ctx context.Context, orgID uuid.UUID) (*FleetSummary, error) {
	assets, err := svc.store.ListAssets(ctx, orgID)
	if err != nil {
		return nil, err
	}

	summary := &FleetSummary{TotalAssets: len(assets)}
	now := time.Now().UTC()
	allocations, _ := svc.store.ListActiveAllocations(ctx, orgID)
	allocatedIDs := make(map[uuid.UUID]bool)
	for _, a := range allocations {
		if a.StartDate.Before(now) && a.EndDate.After(now) {
			allocatedIDs[a.AssetID] = true
		}
	}

	for _, a := range assets {
		switch {
		case a.Status == models.AssetStatusMaintenance:
			summary.InMaintenance++
		case allocatedIDs[a.ID]:
			summary.Allocated++
		default:
			summary.Available++
		}
	}

	return summary, nil
}
