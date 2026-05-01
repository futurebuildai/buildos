package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Sentinel errors specific to FleetService. ErrInvalidInput / ErrNotFound
// are reused from budget.go.
var (
	// ErrFleetAssetNotFound is returned when an asset lookup misses
	// (id + org mismatch). Mirrors the store-level sentinel.
	ErrFleetAssetNotFound = errors.New("fleet: asset not found")

	// ErrAllocationConflict surfaces a GiST exclusion-constraint
	// violation. Mapped to 409 by the handler.
	ErrAllocationConflict = errors.New("fleet: allocation conflicts with existing booking")
)

// CreateAssetInput is the validated input for CreateAsset.
type CreateAssetInput struct {
	Name         string
	AssetType    string
	SerialNumber *string
}

// AllocateAssetInput is the validated input for AllocateAsset. Dates
// form a half-open range [Start, End).
type AllocateAssetInput struct {
	AssetID   uuid.UUID
	ProjectID uuid.UUID
	StartDate time.Time
	EndDate   time.Time
}

// FleetService is the business-logic surface for fleet assets and
// allocations. Reads + writes flow through here.
type FleetService struct {
	pool  *pgxpool.Pool
	store *store.FleetStore
	audit AuditRecorder
}

// NewFleetService creates a service bound to a pool + store.
// audit may be nil; nil falls back to a no-op recorder.
func NewFleetService(pool *pgxpool.Pool, fleet *store.FleetStore, audit AuditRecorder) *FleetService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &FleetService{pool: pool, store: fleet, audit: audit}
}

// ListAssets returns all fleet assets for the caller's org.
func (s *FleetService) ListAssets(ctx context.Context, callerOrgID uuid.UUID, statusFilter []string) ([]models.FleetAsset, error) {
	if callerOrgID == uuid.Nil {
		return nil, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	for _, st := range statusFilter {
		if !models.IsValidFleetAssetStatus(st) {
			return nil, fmt.Errorf("%w: unknown fleet asset status %q", ErrInvalidInput, st)
		}
	}

	var assets []models.FleetAsset
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, err := s.store.ListAssets(ctx, tx, store.ListAssetsParams{
			OrgID:        callerOrgID,
			StatusFilter: statusFilter,
		})
		if err != nil {
			return err
		}
		assets = got
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list fleet assets: %w", err)
	}
	return assets, nil
}

// CreateAsset inserts a new asset. Validates non-empty name + asset_type.
// callerUserSub is recorded on the audit row.
func (s *FleetService) CreateAsset(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in CreateAssetInput) (models.FleetAsset, error) {
	if callerOrgID == uuid.Nil {
		return models.FleetAsset{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return models.FleetAsset{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	assetType := strings.TrimSpace(in.AssetType)
	if assetType == "" {
		return models.FleetAsset{}, fmt.Errorf("%w: asset_type is required", ErrInvalidInput)
	}
	var serial *string
	if in.SerialNumber != nil {
		t := strings.TrimSpace(*in.SerialNumber)
		if t == "" {
			serial = nil // treat empty string as no serial
		} else {
			serial = &t
		}
	}

	var asset models.FleetAsset
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.store.CreateAsset(ctx, tx, store.CreateAssetParams{
			OrgID:        callerOrgID,
			Name:         name,
			AssetType:    assetType,
			SerialNumber: serial,
		})
		if err != nil {
			return err
		}
		asset = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "fleet.asset.created",
			ResourceType: AuditResourceFleetAsset,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"asset_type": got.AssetType,
				"name":       got.Name,
			}),
		})
		return nil
	})
	if err != nil {
		return models.FleetAsset{}, fmt.Errorf("create fleet asset: %w", err)
	}
	return asset, nil
}

// AllocateAsset books an asset to a project for [start, end). Validates:
//   - end > start (single-day allocations should pass start, start+1).
//   - asset belongs to caller's org (cross-tenant isolation).
//   - project belongs to caller's org.
//
// On GiST exclusion conflict (overlapping range for same asset),
// returns ErrAllocationConflict. callerUserSub is recorded on the
// audit row.
func (s *FleetService) AllocateAsset(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in AllocateAssetInput) (models.EquipmentAllocation, error) {
	if callerOrgID == uuid.Nil {
		return models.EquipmentAllocation{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if in.AssetID == uuid.Nil || in.ProjectID == uuid.Nil {
		return models.EquipmentAllocation{}, fmt.Errorf("%w: asset_id and project_id are required", ErrInvalidInput)
	}
	if in.StartDate.IsZero() || in.EndDate.IsZero() {
		return models.EquipmentAllocation{}, fmt.Errorf("%w: start_date and end_date are required", ErrInvalidInput)
	}
	if !in.EndDate.After(in.StartDate) {
		return models.EquipmentAllocation{}, fmt.Errorf("%w: end_date must be after start_date", ErrInvalidInput)
	}

	var alloc models.EquipmentAllocation
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.store.VerifyAssetInOrg(ctx, tx, in.AssetID, callerOrgID); err != nil {
			return err
		}
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.AllocateAsset(ctx, tx, store.AllocateAssetParams{
			AssetID:   in.AssetID,
			ProjectID: in.ProjectID,
			StartDate: in.StartDate,
			EndDate:   in.EndDate,
		})
		if err != nil {
			return err
		}
		alloc = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "fleet.asset.allocated",
			ResourceType: AuditResourceEquipmentAlloc,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"asset_id":   got.AssetID,
				"project_id": got.ProjectID,
				"start_date": got.StartDate,
				"end_date":   got.EndDate,
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrFleetAssetNotFound):
			return models.EquipmentAllocation{}, ErrFleetAssetNotFound
		case errors.Is(err, store.ErrNotFound):
			return models.EquipmentAllocation{}, ErrNotFound
		case errors.Is(err, store.ErrAllocationConflict):
			return models.EquipmentAllocation{}, ErrAllocationConflict
		}
		return models.EquipmentAllocation{}, fmt.Errorf("allocate fleet asset: %w", err)
	}
	return alloc, nil
}
