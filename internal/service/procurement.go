package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/brain"
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

	// ErrMaestroUnavailable is returned by RecommendVendors when the
	// service was constructed without a MaestroProcurementRecommender
	// (e.g. the worker binary that only runs RecomputeStatuses). Lets
	// handlers surface a clean 503 rather than panicking on a nil call.
	ErrMaestroUnavailable = errors.New("procurement: maestro client not configured")
)

// MaestroProcurementRecommender is the consumer-side interface
// ProcurementService needs from the Brain client. Defined here so
// tests can substitute a fake without spinning up an HTTP server,
// and so ProcurementService doesn't transitively pin the entire
// brain.Client surface.
//
// Wraps the typed procurement_recommend Maestro task (ADR-001 D5)
// shipped in PR #14. CostMetadata on the response carries
// {run_id, tokens_used, cost_cents, currency_code} for billing.
type MaestroProcurementRecommender interface {
	ProcurementRecommend(ctx context.Context, req brain.ProcurementRecommendRequest) (*brain.ProcurementRecommendResponse, error)
}

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
	pool    *pgxpool.Pool
	store   *store.ProcurementStore
	maestro MaestroProcurementRecommender
	audit   AuditRecorder
}

// NewProcurementService creates a service bound to a pool + store.
// maestro may be nil — RecommendVendors then returns
// ErrMaestroUnavailable (worker-only deployments don't need it).
// audit may be nil; nil falls back to a no-op recorder.
func NewProcurementService(pool *pgxpool.Pool, items *store.ProcurementStore, maestro MaestroProcurementRecommender, audit AuditRecorder) *ProcurementService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ProcurementService{pool: pool, store: items, maestro: maestro, audit: audit}
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
				"project_id": in.ProjectID,
				"wbs_code":   got.WBSCode,
				"cost_cents": got.EstimatedCostCents,
				"currency":   got.EstimatedCostCurrencyCode,
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

// ProcurementRecommendationSet is the response shape from
// RecommendVendors: the persisted recommendation rows plus the
// cost-metadata block returned by Maestro (so callers can surface
// run_id, tokens_used, cost_cents to the operator without a second
// query).
type ProcurementRecommendationSet struct {
	Items        []models.ProcurementRecommendation
	RunID        uuid.UUID
	TokensUsed   int64
	CostCents    int64
	CurrencyCode string
}

// RecommendVendors asks Brain's Maestro `procurement_recommend` task
// for a ranked list of vendors for the given procurement item, then
// persists each recommendation row inside one tx alongside the audit
// entry. The whole batch shares Maestro's RunID so future evaluation
// can correlate "what did this Maestro call produce" with "what was
// actually ordered" once procurement_items.vendor_id is wired.
//
// Validates:
//   - callerOrgID + procurementItemID non-zero.
//   - The item belongs to callerOrgID (cross-org isolation).
//   - The service was constructed with a non-nil Maestro client.
//
// Maestro's float64 Confidence (0.0–1.0) is rounded * 100 into the
// SMALLINT confidence_pct column to match the codebase no-floats
// culture (CHECK constraint enforces 0..100).
//
// callerUserSub is recorded on the audit row.
func (s *ProcurementService) RecommendVendors(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, procurementItemID uuid.UUID) (ProcurementRecommendationSet, error) {
	if callerOrgID == uuid.Nil {
		return ProcurementRecommendationSet{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if procurementItemID == uuid.Nil {
		return ProcurementRecommendationSet{}, fmt.Errorf("%w: procurement_item_id is required", ErrInvalidInput)
	}
	if s.maestro == nil {
		return ProcurementRecommendationSet{}, ErrMaestroUnavailable
	}

	var result ProcurementRecommendationSet
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Ownership check + budget context. Returns
		// ErrProcurementItemNotFound on miss / cross-org.
		item, err := s.store.GetProcurementItem(ctx, tx, procurementItemID, callerOrgID)
		if err != nil {
			return err
		}

		// Maestro call rides outside the SQL round-trip but inside the
		// outer tx — if persistence fails we don't want a phantom
		// recommendation tracked only in Brain's ai_runs. The
		// trade-off is that a slow Maestro response holds a tx open
		// briefly; acceptable for an interactive recommend flow.
		resp, err := s.maestro.ProcurementRecommend(ctx, brain.ProcurementRecommendRequest{
			MaterialRequestID: item.ID,
			BudgetCents:       item.EstimatedCostCents,
			CurrencyCode:      item.EstimatedCostCurrencyCode,
		})
		if err != nil {
			return fmt.Errorf("maestro procurement_recommend: %w", err)
		}
		if resp == nil {
			return fmt.Errorf("maestro procurement_recommend: nil response")
		}

		recs := make([]models.ProcurementRecommendation, 0, len(resp.Recommendations))
		for _, r := range resp.Recommendations {
			// vendor_id is nullable: Maestro may recommend vendors
			// that don't exist in BuildOS's vendor table yet. Treat
			// uuid.Nil as "no canonical id".
			var vendorIDPtr *uuid.UUID
			if r.VendorID != uuid.Nil {
				v := r.VendorID
				vendorIDPtr = &v
			}
			var reasoningPtr *string
			if strings.TrimSpace(r.Reasoning) != "" {
				rp := r.Reasoning
				reasoningPtr = &rp
			}

			rec, err := s.store.CreateProcurementRecommendation(ctx, tx, store.CreateProcurementRecommendationParams{
				ProcurementItemID:          item.ID,
				OrgID:                      callerOrgID,
				RunID:                      resp.RunID,
				VendorID:                   vendorIDPtr,
				VendorName:                 r.VendorName,
				PredictedSpendCents:        r.PredictedSpendCents,
				PredictedSpendCurrencyCode: r.CurrencyCode,
				ConfidencePct:              confidenceToPct(r.Confidence),
				Reasoning:                  reasoningPtr,
			})
			if err != nil {
				return err
			}
			recs = append(recs, rec)
		}

		// One audit row for the whole batch — the resource is the
		// recommendation set (keyed by procurement_item_id; run_id
		// goes in metadata for replay). Per-row audit would create
		// 3-5 nearly-identical rows per Maestro call with no extra
		// signal.
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "procurement.recommendations.created",
			ResourceType: AuditResourceProcurementRecommendation,
			ResourceID:   item.ID,
			Metadata: marshalAudit(map[string]any{
				"run_id":               resp.RunID,
				"recommendation_count": len(recs),
				"tokens_used":          resp.TokensUsed,
				"cost_cents":           resp.CostCents,
				"currency_code":        resp.CurrencyCode,
			}),
		})

		result = ProcurementRecommendationSet{
			Items:        recs,
			RunID:        resp.RunID,
			TokensUsed:   resp.TokensUsed,
			CostCents:    resp.CostCents,
			CurrencyCode: resp.CurrencyCode,
		}
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrProcurementItemNotFound):
			return ProcurementRecommendationSet{}, ErrProcurementItemNotFound
		case errors.Is(err, store.ErrNotFound):
			return ProcurementRecommendationSet{}, ErrNotFound
		}
		return ProcurementRecommendationSet{}, fmt.Errorf("recommend vendors: %w", err)
	}
	return result, nil
}

// confidenceToPct converts Maestro's float64 confidence (0.0–1.0) to
// the SMALLINT 0..100 column the schema uses. Rounds half-up; clamps
// out-of-range inputs so a buggy Brain response can't violate the
// CHECK constraint and roll back the whole tx.
func confidenceToPct(c float64) int {
	if math.IsNaN(c) || c <= 0 {
		return 0
	}
	if c >= 1 {
		return 100
	}
	return int(math.Round(c * 100))
}
