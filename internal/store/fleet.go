package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// FleetStore provides raw SQL access for fleet operations.
type FleetStore struct {
	pool *pgxpool.Pool
}

// NewFleetStore creates a new FleetStore.
func NewFleetStore(pool *pgxpool.Pool) *FleetStore {
	return &FleetStore{pool: pool}
}

// CreateAsset inserts a new fleet asset.
func (s *FleetStore) CreateAsset(ctx context.Context, asset *models.FleetAsset) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO fleet_assets (org_id, name, asset_type, serial_number, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		asset.OrgID, asset.Name, asset.AssetType, asset.SerialNumber, asset.Status,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating fleet asset: %w", err)
	}
	return id, nil
}

// ListAssets returns all fleet assets for an org.
func (s *FleetStore) ListAssets(ctx context.Context, orgID uuid.UUID) ([]models.FleetAsset, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, name, asset_type, serial_number, status, created_at
		FROM fleet_assets WHERE org_id = $1
		ORDER BY name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing fleet assets: %w", err)
	}
	defer rows.Close()

	var assets []models.FleetAsset
	for rows.Next() {
		var a models.FleetAsset
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Name, &a.AssetType, &a.SerialNumber, &a.Status, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning fleet asset: %w", err)
		}
		assets = append(assets, a)
	}
	return assets, rows.Err()
}

// GetAsset returns a single fleet asset by ID.
func (s *FleetStore) GetAsset(ctx context.Context, assetID uuid.UUID) (*models.FleetAsset, error) {
	var a models.FleetAsset
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, name, asset_type, serial_number, status, created_at
		FROM fleet_assets WHERE id = $1`, assetID,
	).Scan(&a.ID, &a.OrgID, &a.Name, &a.AssetType, &a.SerialNumber, &a.Status, &a.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting fleet asset: %w", err)
	}
	return &a, nil
}

// AllocateAsset creates an equipment allocation.
// The GiST exclusion constraint prevents overlapping date ranges for the same asset.
func (s *FleetStore) AllocateAsset(ctx context.Context, alloc *models.EquipmentAllocation) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO equipment_allocations (asset_id, project_id, start_date, end_date)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		alloc.AssetID, alloc.ProjectID, alloc.StartDate, alloc.EndDate,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("allocating asset: %w", err)
	}
	return id, nil
}

// ListAllocations returns allocations for an asset.
func (s *FleetStore) ListAllocations(ctx context.Context, assetID uuid.UUID) ([]models.EquipmentAllocation, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, asset_id, project_id, start_date, end_date, created_at
		FROM equipment_allocations WHERE asset_id = $1
		ORDER BY start_date`, assetID)
	if err != nil {
		return nil, fmt.Errorf("listing allocations: %w", err)
	}
	defer rows.Close()

	var allocs []models.EquipmentAllocation
	for rows.Next() {
		var a models.EquipmentAllocation
		if err := rows.Scan(&a.ID, &a.AssetID, &a.ProjectID, &a.StartDate, &a.EndDate, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning allocation: %w", err)
		}
		allocs = append(allocs, a)
	}
	return allocs, rows.Err()
}

// ListActiveAllocations returns allocations overlapping the current date.
func (s *FleetStore) ListActiveAllocations(ctx context.Context, orgID uuid.UUID) ([]models.EquipmentAllocation, error) {
	now := time.Now().UTC()
	rows, err := s.pool.Query(ctx, `
		SELECT ea.id, ea.asset_id, ea.project_id, ea.start_date, ea.end_date, ea.created_at
		FROM equipment_allocations ea
		JOIN fleet_assets fa ON fa.id = ea.asset_id
		WHERE fa.org_id = $1
			AND ea.start_date <= $2 AND ea.end_date >= $2
		ORDER BY ea.start_date`, orgID, now)
	if err != nil {
		return nil, fmt.Errorf("listing active allocations: %w", err)
	}
	defer rows.Close()

	var allocs []models.EquipmentAllocation
	for rows.Next() {
		var a models.EquipmentAllocation
		if err := rows.Scan(&a.ID, &a.AssetID, &a.ProjectID, &a.StartDate, &a.EndDate, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning allocation: %w", err)
		}
		allocs = append(allocs, a)
	}
	return allocs, rows.Err()
}
