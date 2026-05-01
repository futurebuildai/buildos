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

	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// Compile-time check that ProcurementService satisfies the worker
// package's interface. Catches signature drift at build time rather
// than at the first scheduled tick.
var _ worker.ProcurementChecker = (*ProcurementService)(nil)

// Sentinel errors specific to ProcurementService. ErrInvalidInput and
// ErrNotFound are reused from budget.go (the package-level sentinels
// shared across the service layer). ErrProcurementItemNotFound is
// kept distinct so handlers can surface a procurement-specific 404
// message if they want to.
var (
	// ErrProcurementItemNotFound is returned when an item lookup misses
	// (id + project_id + org_id mismatch). Mirrors the store sentinel.
	ErrProcurementItemNotFound = errors.New("procurement: item not found")
)

// CreateProcurementItemInput is the validated input for Create. The
// service performs cross-org isolation, currency validation, and
// non-negative checks before opening a transaction.
type CreateProcurementItemInput struct {
	ProjectID                 uuid.UUID
	Name                      string
	WBSCode                   string
	EstimatedCostCents        int64
	EstimatedCostCurrencyCode string
	LeadTimeDays              int
	WeatherBufferDays         int
	VendorID                  *uuid.UUID
	NeedByDate                *time.Time
}

// UpdateProcurementItemInput is the validated input for Update. All
// fields are optional pointers — nil means "leave unchanged". A status
// transition to ORDERED requires PONumber to be non-empty (the human
// user shouldn't mark something ordered without a PO).
type UpdateProcurementItemInput struct {
	ItemID    uuid.UUID
	ProjectID uuid.UUID
	Status    *string
	PONumber  *string
	OrderedAt *time.Time
}

// ProcurementService is the business-logic surface for procurement
// items. Reads + writes flow through here; A2A handlers create feed
// cards (not procurement rows), so this service is the only writer
// of procurement_items.
type ProcurementService struct {
	pool  *pgxpool.Pool
	store *store.ProcurementStore
	audit AuditRecorder
}

// NewProcurementService creates a service bound to a pool + store.
// audit may be nil; nil falls back to a no-op recorder.
func NewProcurementService(pool *pgxpool.Pool, items *store.ProcurementStore, audit AuditRecorder) *ProcurementService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ProcurementService{pool: pool, store: items, audit: audit}
}

// ListProcurement returns all items on a project visible to the caller's
// org. Cross-org access surfaces as ErrNotFound (we never leak existence
// across tenants).
func (s *ProcurementService) ListProcurement(ctx context.Context, projectID, callerOrgID uuid.UUID, statusFilter []string) ([]models.ProcurementItem, error) {
	if projectID == uuid.Nil || callerOrgID == uuid.Nil {
		return nil, fmt.Errorf("%w: project_id and caller org_id are required", ErrInvalidInput)
	}
	for _, st := range statusFilter {
		if !models.IsValidProcurementStatus(st) {
			return nil, fmt.Errorf("%w: unknown procurement status %q", ErrInvalidInput, st)
		}
	}

	var items []models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.ListProcurementItems(ctx, tx, store.ListProcurementItemsParams{
			ProjectID:    projectID,
			OrgID:        callerOrgID,
			StatusFilter: statusFilter,
		})
		if err != nil {
			return err
		}
		items = got
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("list procurement: %w", err)
	}
	return items, nil
}

// CreateProcurementItem inserts a new item with status='OK'. Validates:
//   - Name and WBSCode non-empty.
//   - EstimatedCostCents >= 0.
//   - LeadTimeDays >= 0, WeatherBufferDays >= 0.
//   - Currency code in the supported set (USD/CAD).
//   - ProjectID belongs to callerOrgID (cross-org isolation).
//
// callerUserSub is recorded on the audit row.
func (s *ProcurementService) CreateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in CreateProcurementItemInput) (models.ProcurementItem, error) {
	if callerOrgID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if in.ProjectID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Name) == "" {
		return models.ProcurementItem{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.WBSCode) == "" {
		return models.ProcurementItem{}, fmt.Errorf("%w: wbs_code is required", ErrInvalidInput)
	}
	if in.EstimatedCostCents < 0 {
		return models.ProcurementItem{}, fmt.Errorf("%w: estimated_cost_cents must be non-negative", ErrInvalidInput)
	}
	if in.LeadTimeDays < 0 || in.WeatherBufferDays < 0 {
		return models.ProcurementItem{}, fmt.Errorf("%w: lead_time_days and weather_buffer_days must be non-negative", ErrInvalidInput)
	}
	if err := currency.Validate(in.EstimatedCostCurrencyCode); err != nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.CreateProcurementItem(ctx, tx, store.CreateProcurementItemParams{
			ProjectID:                 in.ProjectID,
			OrgID:                     callerOrgID,
			Name:                      strings.TrimSpace(in.Name),
			WBSCode:                   strings.TrimSpace(in.WBSCode),
			EstimatedCostCents:        in.EstimatedCostCents,
			EstimatedCostCurrencyCode: in.EstimatedCostCurrencyCode,
			LeadTimeDays:              in.LeadTimeDays,
			WeatherBufferDays:         in.WeatherBufferDays,
			VendorID:                  in.VendorID,
			NeedByDate:                in.NeedByDate,
		})
		if err != nil {
			return err
		}
		item = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.item.created",
			ResourceType: AuditResourceProcurementItem,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"project_id":   in.ProjectID,
				"wbs_code":     got.WBSCode,
				"cost_cents":   got.EstimatedCostCents,
				"currency":     got.EstimatedCostCurrencyCode,
			}),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return models.ProcurementItem{}, ErrNotFound
		}
		return models.ProcurementItem{}, fmt.Errorf("create procurement item: %w", err)
	}
	return item, nil
}

// UpdateProcurementItem applies a partial update. Validates:
//   - At least one field is being changed.
//   - Status, if provided, is a valid value.
//   - Status transition to ORDERED requires non-empty PONumber.
//   - Item belongs to ProjectID belongs to callerOrgID.
//
// We don't enforce a strict status FSM (OK→WARNING→CRITICAL→ORDERED)
// because the agent overwrites time-based statuses on every tick — a
// brief stale-state read shouldn't block a human marking ORDERED.
//
// callerUserSub is recorded on the audit row.
func (s *ProcurementService) UpdateProcurementItem(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, in UpdateProcurementItemInput) (models.ProcurementItem, error) {
	if callerOrgID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if in.ItemID == uuid.Nil || in.ProjectID == uuid.Nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: item_id and project_id are required", ErrInvalidInput)
	}
	if in.Status == nil && in.PONumber == nil && in.OrderedAt == nil {
		return models.ProcurementItem{}, fmt.Errorf("%w: at least one updatable field is required", ErrInvalidInput)
	}
	if in.Status != nil {
		if !models.IsValidProcurementStatus(*in.Status) {
			return models.ProcurementItem{}, fmt.Errorf("%w: unknown procurement status %q", ErrInvalidInput, *in.Status)
		}
		if *in.Status == models.ProcurementStatusOrdered {
			if in.PONumber == nil || strings.TrimSpace(*in.PONumber) == "" {
				return models.ProcurementItem{}, fmt.Errorf("%w: po_number is required when transitioning to ORDERED", ErrInvalidInput)
			}
		}
	}

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.UpdateProcurementItem(ctx, tx, store.UpdateProcurementItemParams{
			ItemID:    in.ItemID,
			ProjectID: in.ProjectID,
			OrgID:     callerOrgID,
			Status:    in.Status,
			PONumber:  in.PONumber,
			OrderedAt: in.OrderedAt,
		})
		if err != nil {
			return err
		}
		item = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.item.updated",
			ResourceType: AuditResourceProcurementItem,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"status":     in.Status,
				"po_number":  in.PONumber,
				"ordered_at": in.OrderedAt,
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProcurementItemNotFound):
			return models.ProcurementItem{}, ErrProcurementItemNotFound
		case errors.Is(err, store.ErrNotFound):
			return models.ProcurementItem{}, ErrNotFound
		}
		return models.ProcurementItem{}, fmt.Errorf("update procurement item: %w", err)
	}
	return item, nil
}

// DefaultProcurementWarningWindowDays is the lead-time horizon at which
// an OK row flips to WARNING. Picked at 7 days based on the typical
// residential build cadence — far enough to react, close enough to be
// actionable. ProcurementCheckWorker scheduled daily; one full warning
// → critical → expedite cycle gives the contractor a ~7-day runway.
const DefaultProcurementWarningWindowDays = 7

// RecomputeStatuses runs the daily procurement health sweep, flipping
// every non-ORDERED row to OK / WARNING / CRITICAL per its
// must_order_date relative to today + the warning window. Returns the
// number of rows whose status actually changed (useful for
// observability — a healthy fleet should mostly produce zero or one
// per day; thousands of transitions in one tick is a smell).
//
// This is the worker-side entrypoint. The worker lives in
// internal/worker; this method satisfies its consumer-side interface.
func (s *ProcurementService) RecomputeStatuses(ctx context.Context) (int64, error) {
	var changed int64
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.store.RecomputeStatuses(ctx, tx, store.RecomputeStatusesParams{
			WarningWindowDays: DefaultProcurementWarningWindowDays,
		})
		if err != nil {
			return err
		}
		changed = got
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("recompute procurement statuses: %w", err)
	}
	return changed, nil
}
