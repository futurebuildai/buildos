package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/futurebuildai/buildos/internal/models"
)

// FleetStore manages fleet_assets + equipment_allocations.
type FleetStore struct{}

// NewFleetStore creates a new FleetStore.
func NewFleetStore() *FleetStore { return &FleetStore{} }

// ErrFleetAssetNotFound is returned when an asset lookup misses
// (id + org scope mismatch).
var ErrFleetAssetNotFound = errors.New("fleet_assets: not found")

// ErrAllocationConflict is returned when an INSERT into
// equipment_allocations violates the GiST exclusion constraint —
// i.e. the [start_date, end_date) range overlaps an existing
// allocation for the same asset. The handler maps this to a 409.
var ErrAllocationConflict = errors.New("equipment_allocations: range conflicts with existing allocation")

// CreateAssetParams is the input for inserting a fleet asset.
type CreateAssetParams struct {
	OrgID        uuid.UUID
	Name         string
	AssetType    string
	SerialNumber *string
}

// CreateAsset inserts a fleet asset with status='available'.
func (s *FleetStore) CreateAsset(ctx context.Context, tx pgx.Tx, p CreateAssetParams) (models.FleetAsset, error) {
	var a models.FleetAsset
	err := tx.QueryRow(ctx, `
		INSERT INTO fleet_assets (org_id, name, asset_type, serial_number)
		VALUES ($1, $2, $3, $4)
		RETURNING id, org_id, name, asset_type, serial_number, status, created_at`,
		p.OrgID, p.Name, p.AssetType, p.SerialNumber,
	).Scan(&a.ID, &a.OrgID, &a.Name, &a.AssetType, &a.SerialNumber, &a.Status, &a.CreatedAt)
	if err != nil {
		return models.FleetAsset{}, fmt.Errorf("insert fleet_asset: %w", err)
	}
	return a, nil
}

// ListAssetsParams controls a fleet listing.
type ListAssetsParams struct {
	OrgID        uuid.UUID
	StatusFilter []string // empty = no filter
}

// ListAssets returns all fleet_assets for an org, ordered newest first.
// Status filter is optional.
func (s *FleetStore) ListAssets(ctx context.Context, tx pgx.Tx, p ListAssetsParams) ([]models.FleetAsset, error) {
	args := []any{p.OrgID}
	q := `
		SELECT id, org_id, name, asset_type, serial_number, status, created_at
		FROM fleet_assets
		WHERE org_id = $1`
	if len(p.StatusFilter) > 0 {
		args = append(args, p.StatusFilter)
		q += " AND status = ANY($2)"
	}
	q += " ORDER BY created_at DESC"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query fleet_assets: %w", err)
	}
	defer rows.Close()

	out := make([]models.FleetAsset, 0)
	for rows.Next() {
		var a models.FleetAsset
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Name, &a.AssetType, &a.SerialNumber, &a.Status, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan fleet_asset: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// VerifyAssetInOrg returns nil if the asset belongs to the given org,
// ErrFleetAssetNotFound otherwise. Service layer guards allocation
// inserts with this so a sibling-org caller can't reference an asset
// they don't own.
func (s *FleetStore) VerifyAssetInOrg(ctx context.Context, tx pgx.Tx, assetID, orgID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM fleet_assets WHERE id = $1 AND org_id = $2)`,
		assetID, orgID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify fleet_asset in org: %w", err)
	}
	if !exists {
		return ErrFleetAssetNotFound
	}
	return nil
}

// AllocateAssetParams is the input for inserting an allocation.
// StartDate/EndDate form a half-open range [start, end).
type AllocateAssetParams struct {
	AssetID   uuid.UUID
	ProjectID uuid.UUID
	StartDate time.Time
	EndDate   time.Time
}

// AllocateAsset inserts a row in equipment_allocations. Overlapping
// ranges for the same asset trigger SQLSTATE 23P01 (exclusion
// violation), mapped here to ErrAllocationConflict.
func (s *FleetStore) AllocateAsset(ctx context.Context, tx pgx.Tx, p AllocateAssetParams) (models.EquipmentAllocation, error) {
	var alloc models.EquipmentAllocation
	err := tx.QueryRow(ctx, `
		INSERT INTO equipment_allocations (asset_id, project_id, start_date, end_date)
		VALUES ($1, $2, $3, $4)
		RETURNING id, asset_id, project_id, start_date, end_date, created_at`,
		p.AssetID, p.ProjectID, p.StartDate, p.EndDate,
	).Scan(&alloc.ID, &alloc.AssetID, &alloc.ProjectID, &alloc.StartDate, &alloc.EndDate, &alloc.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ExclusionViolation {
			return models.EquipmentAllocation{}, ErrAllocationConflict
		}
		return models.EquipmentAllocation{}, fmt.Errorf("insert equipment_allocation: %w", err)
	}
	return alloc, nil
}
