package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// Pipeline-specific sentinel errors. Handlers map to HTTP codes:
//
//	ErrInvalidTransition → 409 INVALID_TRANSITION
//	ErrTerminalStage     → 409 INVALID_TRANSITION (special-case message)
//	ErrNotImplemented    → 501 NOT_IMPLEMENTED (PERMIT_ISSUED until PR 3)
//
// Reuses ErrNotFound and ErrInvalidInput from budget.go (same service package).
var (
	ErrInvalidTransition = errors.New("pipeline: invalid stage transition")
	ErrTerminalStage     = errors.New("pipeline: source stage is terminal")
	ErrNotImplemented    = errors.New("pipeline: not yet implemented")
)

// PipelineService orchestrates pre-construction prospect operations:
// CRUD, stage transitions, estimate/permit attachments, and the atomic
// Kanban→CPM transition.
//
// The RiverClient is held to enqueue HydrateProject jobs from inside the
// transition tx (river.InsertTx) so the project insert + prospect
// update + WBS-hydration enqueue all commit together — no phantom jobs
// referencing rows that never made it.
type PipelineService struct {
	pool        *pgxpool.Pool
	store       *store.PipelineStore
	riverClient *river.Client[pgx.Tx]
	audit       AuditRecorder
}

// NewPipelineService creates a new PipelineService. riverClient may be
// nil for read-only deployments (no Kanban→CPM transitions); the service
// will return ErrNotImplemented when transition is attempted without one.
// audit may be nil; nil falls back to a no-op recorder.
func NewPipelineService(pool *pgxpool.Pool, ps *store.PipelineStore, riverClient *river.Client[pgx.Tx], audit AuditRecorder) *PipelineService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &PipelineService{pool: pool, store: ps, riverClient: riverClient, audit: audit}
}

// ---------- Reads ----------

// ListProspectsInput controls a paginated prospect listing.
type ListProspectsInput struct {
	OrgID   uuid.UUID
	Stage   string // optional; if non-empty must be a valid PipelineStage
	Page    int    // 1-based; defaults to 1 when <1
	PerPage int    // defaults to 50; clamped to [1,200]
}

// ListProspects returns one page of prospects with the total count.
func (s *PipelineService) ListProspects(ctx context.Context, in ListProspectsInput) (store.ProspectsPage, error) {
	if in.Stage != "" && models.PipelineStage(in.Stage).Probability() == 0 && models.PipelineStage(in.Stage) != models.StageLost {
		// Probability returns 0 for both Lost and unknown stages; we
		// distinguish by re-checking against the canonical Lost constant.
		return store.ProspectsPage{}, fmt.Errorf("%w: stage %q is not a valid PipelineStage", ErrInvalidInput, in.Stage)
	}
	if in.PerPage > 200 {
		in.PerPage = 200
	}

	var page store.ProspectsPage
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var qErr error
		page, qErr = s.store.ListProspects(ctx, tx, store.ListProspectsParams{
			OrgID:   in.OrgID,
			Stage:   in.Stage,
			Page:    in.Page,
			PerPage: in.PerPage,
		})
		return qErr
	})
	return page, err
}

// GetProspectWithDetails returns a prospect along with its estimates
// and permits in a single read tx so the snapshot is internally
// consistent.
func (s *PipelineService) GetProspectWithDetails(ctx context.Context, prospectID, callerOrgID uuid.UUID) (models.ProspectWithDetails, error) {
	var out models.ProspectWithDetails
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		prospect, err := s.store.GetProspect(ctx, tx, prospectID, callerOrgID)
		if err != nil {
			return err
		}
		estimates, err := s.store.ListEstimatesForProspect(ctx, tx, prospectID)
		if err != nil {
			return err
		}
		permits, err := s.store.ListPermitsForProspect(ctx, tx, prospectID)
		if err != nil {
			return err
		}
		out = models.ProspectWithDetails{
			Prospect:  prospect,
			Estimates: estimates,
			Permits:   permits,
		}
		return nil
	})
	if err != nil {
		return models.ProspectWithDetails{}, mapStoreError(err)
	}
	return out, nil
}

// ---------- Writes ----------

// CreateProspectInput is the service-layer input for adding a prospect.
type CreateProspectInput struct {
	OrgID       uuid.UUID
	Name        string
	ClientName  string
	ClientEmail *string
	ClientPhone *string
	Address     *string
	GSF         *int
	Source      *string
	Notes       *string
}

// CreateProspect inserts a prospect at stage LEAD.
func (s *PipelineService) CreateProspect(ctx context.Context, in CreateProspectInput) (models.Prospect, error) {
	if in.Name == "" {
		return models.Prospect{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if in.ClientName == "" {
		return models.Prospect{}, fmt.Errorf("%w: client_name is required", ErrInvalidInput)
	}
	if in.GSF != nil && *in.GSF <= 0 {
		return models.Prospect{}, fmt.Errorf("%w: gsf must be positive", ErrInvalidInput)
	}

	var prospect models.Prospect
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		prospect, qErr = s.store.CreateProspect(ctx, tx, store.CreateProspectParams{
			OrgID:       in.OrgID,
			Name:        in.Name,
			ClientName:  in.ClientName,
			ClientEmail: in.ClientEmail,
			ClientPhone: in.ClientPhone,
			Address:     in.Address,
			GSF:         in.GSF,
			Source:      in.Source,
			Notes:       in.Notes,
		})
		return qErr
	})
	if err != nil {
		return models.Prospect{}, err
	}
	return prospect, nil
}

// UpdateProspectInput is the service-layer input for partial-updating a
// prospect. Stage transitions are NOT done here — use AdvanceProspect /
// LoseProspect, which run the state-machine guard.
type UpdateProspectInput struct {
	ProspectID  uuid.UUID
	OrgID       uuid.UUID
	Name        *string
	ClientName  *string
	ClientEmail *string
	ClientPhone *string
	Address     *string
	GSF         *int
	Source      *string
	Notes       *string
}

// UpdateProspect modifies prospect details. Returns ErrNotFound when
// the prospect does not exist or belongs to another org.
func (s *PipelineService) UpdateProspect(ctx context.Context, in UpdateProspectInput) (models.Prospect, error) {
	if in.GSF != nil && *in.GSF <= 0 {
		return models.Prospect{}, fmt.Errorf("%w: gsf must be positive", ErrInvalidInput)
	}

	var prospect models.Prospect
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		prospect, qErr = s.store.UpdateProspect(ctx, tx, store.UpdateProspectParams{
			ProspectID:  in.ProspectID,
			OrgID:       in.OrgID,
			Name:        in.Name,
			ClientName:  in.ClientName,
			ClientEmail: in.ClientEmail,
			ClientPhone: in.ClientPhone,
			Address:     in.Address,
			GSF:         in.GSF,
			Source:      in.Source,
			Notes:       in.Notes,
		})
		return qErr
	})
	if err != nil {
		return models.Prospect{}, mapStoreError(err)
	}
	return prospect, nil
}

// AdvanceProspectInput is the service-layer input for stage advancement.
// PermitIssuedDate is required only when Target == PERMIT_ISSUED; the
// PERMIT_ISSUED transition is currently unimplemented (PR 3 work) and
// returns ErrNotImplemented even with a valid date.
type AdvanceProspectInput struct {
	ProspectID       uuid.UUID
	OrgID            uuid.UUID
	Target           models.PipelineStage
	PermitIssuedDate *time.Time
}

// AdvanceProspect validates the stage transition and runs it in a single
// transaction. Returns:
//
//	ErrNotFound          → prospect not visible to caller's org
//	ErrInvalidTransition → from→to is not a permitted transition
//	ErrTerminalStage     → source is PERMIT_ISSUED or LOST
//	ErrNotImplemented    → target is PERMIT_ISSUED (atomic CPM transition lands in PR 3)
//	ErrInvalidInput      → target is not a known PipelineStage
//
// callerUserSub is the OIDC subject of the user driving the transition;
// recorded on the audit row.
func (s *PipelineService) AdvanceProspect(ctx context.Context, callerUserSub string, in AdvanceProspectInput) (models.Prospect, error) {
	// Reject unknown targets up front.
	if in.Target != models.StageLost && in.Target.Probability() == 0 {
		return models.Prospect{}, fmt.Errorf("%w: target_stage %q is not a known PipelineStage", ErrInvalidInput, in.Target)
	}

	if in.Target == models.StagePermitIssued {
		return s.transitionToPermitIssued(ctx, callerUserSub, in)
	}

	var prospect models.Prospect
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		current, err := s.store.GetProspect(ctx, tx, in.ProspectID, in.OrgID)
		if err != nil {
			return err
		}
		if current.PipelineStage.IsTerminal() {
			return fmt.Errorf("%w: cannot advance from %s", ErrTerminalStage, current.PipelineStage)
		}
		if !models.CanTransition(current.PipelineStage, in.Target) {
			return fmt.Errorf("%w: %s → %s not permitted", ErrInvalidTransition, current.PipelineStage, in.Target)
		}

		updated, err := s.store.AdvanceStage(ctx, tx, in.ProspectID, in.OrgID, in.Target)
		if err != nil {
			return err
		}
		prospect = updated

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      callerUserSub,
			Action:       "pipeline.stage_transitioned",
			ResourceType: AuditResourceProspect,
			ResourceID:   in.ProspectID,
			Before: marshalAudit(map[string]any{
				"stage":           current.PipelineStage,
				"probability_pct": current.ProbabilityPct,
			}),
			After: marshalAudit(map[string]any{
				"stage":           updated.PipelineStage,
				"probability_pct": updated.ProbabilityPct,
			}),
			Metadata: marshalAudit(map[string]any{
				"prospect_name": updated.Name,
			}),
		})
		return nil
	})
	if err != nil {
		return models.Prospect{}, mapStoreError(err)
	}
	return prospect, nil
}

// transitionToPermitIssued runs the atomic Kanban→CPM transition.
// In a single transaction:
//
//  1. Load the prospect (verifies ownership + stage = PERMIT_APPLIED).
//  2. Insert a new row in projects, copying over name/address/gsf and
//     anchoring permit_issued_date as the project_start_date.
//  3. Update the prospect: stage=PERMIT_ISSUED, probability_pct=100,
//     project_id pointing at the new construction row.
//  4. Enqueue a HydrateProject River job — uses river.InsertTx so the
//     job row commits with the project + prospect writes. No phantom
//     jobs that reference rows that never made it.
//
// Async work (WBS hydration, initial CPM forward/backward pass) lands
// in the HydrateProject worker (Sprint 4+).
//
// Required preconditions enforced before any DB writes:
//   - in.PermitIssuedDate must be non-nil (the contract requires it).
//   - prospect.GSF must be non-nil (CPM scheduling requires it).
//   - prospect.PipelineStage must be PERMIT_APPLIED (CanTransition guard).
//
// If the RiverClient is nil (read-only deployments / partial wiring),
// returns ErrNotImplemented so callers see a clear failure rather than
// a silent missing-job-enqueue.
func (s *PipelineService) transitionToPermitIssued(ctx context.Context, callerUserSub string, in AdvanceProspectInput) (models.Prospect, error) {
	// Validate caller input before checking system state — keeps 400 vs
	// 501 ordering consistent with the rest of the API.
	if in.PermitIssuedDate == nil {
		return models.Prospect{}, fmt.Errorf("%w: permit_issued_date is required for PERMIT_ISSUED transition", ErrInvalidInput)
	}
	if s.riverClient == nil {
		return models.Prospect{}, fmt.Errorf("%w: RiverClient not wired; PERMIT_ISSUED transition unavailable", ErrNotImplemented)
	}

	var prospect models.Prospect
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		current, err := s.store.GetProspect(ctx, tx, in.ProspectID, in.OrgID)
		if err != nil {
			return err
		}
		if !models.CanTransition(current.PipelineStage, models.StagePermitIssued) {
			return fmt.Errorf("%w: %s → PERMIT_ISSUED not permitted", ErrInvalidTransition, current.PipelineStage)
		}
		if current.GSF == nil {
			return fmt.Errorf("%w: prospect.gsf is required before PERMIT_ISSUED transition (CPM needs square footage)", ErrInvalidInput)
		}

		projectID, err := s.store.CreateProjectFromProspect(ctx, tx, store.CreateProjectFromProspectParams{
			OrgID:            current.OrgID,
			Name:             current.Name,
			Address:          current.Address,
			GSF:              *current.GSF,
			PermitIssuedDate: *in.PermitIssuedDate,
		})
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}

		updated, err := s.store.MarkProspectPermitIssued(ctx, tx, in.ProspectID, in.OrgID, projectID)
		if err != nil {
			return fmt.Errorf("mark prospect permit_issued: %w", err)
		}

		if _, err := s.riverClient.InsertTx(ctx, tx, worker.HydrateProjectArgs{ProjectID: projectID}, nil); err != nil {
			return fmt.Errorf("enqueue hydrate_project: %w", err)
		}

		prospect = updated

		// Audit the Kanban→CPM promotion. Same tx, so any later
		// failure rolls the audit row back with the prospect/project
		// writes — no orphaned "promoted" records.
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      callerUserSub,
			Action:       "pipeline.stage_transitioned",
			ResourceType: AuditResourceProspect,
			ResourceID:   in.ProspectID,
			Before: marshalAudit(map[string]any{
				"stage":           current.PipelineStage,
				"probability_pct": current.ProbabilityPct,
			}),
			After: marshalAudit(map[string]any{
				"stage":           updated.PipelineStage,
				"probability_pct": updated.ProbabilityPct,
				"project_id":      projectID,
			}),
			Metadata: marshalAudit(map[string]any{
				"prospect_name":      updated.Name,
				"permit_issued_date": *in.PermitIssuedDate,
				"gsf":                *current.GSF,
			}),
		})
		return nil
	})
	if err != nil {
		return models.Prospect{}, mapStoreError(err)
	}
	return prospect, nil
}

// LoseProspectInput is the service-layer input for marking lost.
type LoseProspectInput struct {
	ProspectID uuid.UUID
	OrgID      uuid.UUID
	Reason     string
}

// ---------- Estimates ----------

// CreateEstimateInput is the service-layer input for creating an
// estimate. CurrencyCode is validated; total_estimated_cents is computed
// from line items so callers can't supply a number that disagrees with
// the persisted line-item sum.
type CreateEstimateInput struct {
	ProspectID   uuid.UUID
	OrgID        uuid.UUID
	CurrencyCode string
	LineItems    models.PipelineEstimateLineItems
	MarginPct    int
}

// CreateEstimate inserts a new estimate version under the given prospect.
// Validates currency, requires at least one line item, computes the
// total. Returns ErrNotFound if the prospect doesn't exist or belongs
// to another org.
func (s *PipelineService) CreateEstimate(ctx context.Context, in CreateEstimateInput) (models.PipelineEstimate, error) {
	if err := currency.Validate(in.CurrencyCode); err != nil {
		return models.PipelineEstimate{}, fmt.Errorf("%w: currency_code: %v", ErrInvalidInput, err)
	}
	if len(in.LineItems) == 0 {
		return models.PipelineEstimate{}, fmt.Errorf("%w: at least one line_item is required", ErrInvalidInput)
	}
	if in.MarginPct < 0 || in.MarginPct > 100 {
		return models.PipelineEstimate{}, fmt.Errorf("%w: margin_pct must be 0..100", ErrInvalidInput)
	}

	var total int64
	for i, item := range in.LineItems {
		if item.WBSCode == "" {
			return models.PipelineEstimate{}, fmt.Errorf("%w: line_items[%d].wbs_code is required", ErrInvalidInput, i)
		}
		if item.EstimatedCents < 0 {
			return models.PipelineEstimate{}, fmt.Errorf("%w: line_items[%d].estimated_cents must be >= 0", ErrInvalidInput, i)
		}
		total += item.EstimatedCents
	}

	itemsJSON, err := json.Marshal(in.LineItems)
	if err != nil {
		// Should never happen for the line-item type; treat as 500.
		return models.PipelineEstimate{}, fmt.Errorf("marshal line_items: %w", err)
	}

	var estimate models.PipelineEstimate
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.store.VerifyProspectInOrg(ctx, tx, in.ProspectID, in.OrgID); err != nil {
			return err
		}
		created, err := s.store.CreateEstimate(ctx, tx, store.CreateEstimateParams{
			ProspectID:          in.ProspectID,
			TotalEstimatedCents: total,
			CurrencyCode:        in.CurrencyCode,
			LineItemsJSON:       itemsJSON,
			MarginPct:           in.MarginPct,
		})
		if err != nil {
			return err
		}
		estimate = created
		return nil
	})
	if err != nil {
		return models.PipelineEstimate{}, mapStoreError(err)
	}
	return estimate, nil
}

// UpdateEstimateStatusInput is the service-layer input for changing an
// estimate's status. Both EstimateID and ProspectID are required so the
// service can verify the (estimate, prospect, org) chain in a single tx.
type UpdateEstimateStatusInput struct {
	EstimateID uuid.UUID
	ProspectID uuid.UUID
	OrgID      uuid.UUID
	NewStatus  string
}

// UpdateEstimateStatus changes only the status of an estimate. Validates
// the transition (CanTransitionEstimateStatus). Sent_at is auto-stamped
// by the store on the first transition to "sent".
func (s *PipelineService) UpdateEstimateStatus(ctx context.Context, in UpdateEstimateStatusInput) (models.PipelineEstimate, error) {
	if !models.IsValidEstimateStatus(in.NewStatus) {
		return models.PipelineEstimate{}, fmt.Errorf("%w: status %q is not one of {draft, sent, revised, accepted}", ErrInvalidInput, in.NewStatus)
	}

	var estimate models.PipelineEstimate
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.store.VerifyProspectInOrg(ctx, tx, in.ProspectID, in.OrgID); err != nil {
			return err
		}
		current, err := s.store.GetEstimate(ctx, tx, in.EstimateID, in.ProspectID)
		if err != nil {
			return err
		}
		if !models.CanTransitionEstimateStatus(current.Status, in.NewStatus) {
			return fmt.Errorf("%w: estimate %s → %s not permitted", ErrInvalidTransition, current.Status, in.NewStatus)
		}
		updated, err := s.store.UpdateEstimateStatus(ctx, tx, in.EstimateID, in.NewStatus)
		if err != nil {
			return err
		}
		estimate = updated
		return nil
	})
	if err != nil {
		return models.PipelineEstimate{}, mapStoreError(err)
	}
	return estimate, nil
}

// ---------- Permits ----------

// CreatePermitInput is the service-layer input for adding a permit.
type CreatePermitInput struct {
	ProspectID        uuid.UUID
	OrgID             uuid.UUID
	PermitType        string
	Jurisdiction      string
	ApplicationNumber *string
	SubmittedDate     *time.Time
	ExpectedIssueDate *time.Time
	FeeCents          int64
	FeeCurrencyCode   string
	Notes             *string
}

// CreatePermit inserts a permit at status "not_submitted".
func (s *PipelineService) CreatePermit(ctx context.Context, in CreatePermitInput) (models.Permit, error) {
	if in.PermitType == "" {
		return models.Permit{}, fmt.Errorf("%w: permit_type is required", ErrInvalidInput)
	}
	if in.Jurisdiction == "" {
		return models.Permit{}, fmt.Errorf("%w: jurisdiction is required", ErrInvalidInput)
	}
	if err := currency.Validate(in.FeeCurrencyCode); err != nil {
		return models.Permit{}, fmt.Errorf("%w: fee_currency_code: %v", ErrInvalidInput, err)
	}
	if in.FeeCents < 0 {
		return models.Permit{}, fmt.Errorf("%w: fee_cents must be >= 0", ErrInvalidInput)
	}

	var permit models.Permit
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.store.VerifyProspectInOrg(ctx, tx, in.ProspectID, in.OrgID); err != nil {
			return err
		}
		created, err := s.store.CreatePermit(ctx, tx, store.CreatePermitParams{
			ProspectID:        in.ProspectID,
			PermitType:        in.PermitType,
			Jurisdiction:      in.Jurisdiction,
			ApplicationNumber: in.ApplicationNumber,
			SubmittedDate:     in.SubmittedDate,
			ExpectedIssueDate: in.ExpectedIssueDate,
			FeeCents:          in.FeeCents,
			FeeCurrencyCode:   in.FeeCurrencyCode,
			Notes:             in.Notes,
		})
		if err != nil {
			return err
		}
		permit = created
		return nil
	})
	if err != nil {
		return models.Permit{}, mapStoreError(err)
	}
	return permit, nil
}

// UpdatePermitInput is the service-layer input for partial-updating a
// permit. fee_currency_code is intentionally not editable here — once
// recorded the currency is immutable.
type UpdatePermitInput struct {
	PermitID          uuid.UUID
	ProspectID        uuid.UUID
	OrgID             uuid.UUID
	ApplicationNumber *string
	SubmittedDate     *time.Time
	ExpectedIssueDate *time.Time
	ActualIssueDate   *time.Time
	FeeCents          *int64
	NewStatus         *string
	Notes             *string
}

// UpdatePermit modifies a permit. Validates the status transition if a
// new status is provided. fee_cents must be >= 0.
func (s *PipelineService) UpdatePermit(ctx context.Context, in UpdatePermitInput) (models.Permit, error) {
	if in.NewStatus != nil && !models.IsValidPermitStatus(*in.NewStatus) {
		return models.Permit{}, fmt.Errorf("%w: status %q is not one of {not_submitted, submitted, under_review, revisions_requested, approved, denied}", ErrInvalidInput, *in.NewStatus)
	}
	if in.FeeCents != nil && *in.FeeCents < 0 {
		return models.Permit{}, fmt.Errorf("%w: fee_cents must be >= 0", ErrInvalidInput)
	}

	var permit models.Permit
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.store.VerifyProspectInOrg(ctx, tx, in.ProspectID, in.OrgID); err != nil {
			return err
		}
		current, err := s.store.GetPermit(ctx, tx, in.PermitID, in.ProspectID)
		if err != nil {
			return err
		}
		if in.NewStatus != nil && !models.CanTransitionPermitStatus(current.Status, *in.NewStatus) {
			return fmt.Errorf("%w: permit %s → %s not permitted", ErrInvalidTransition, current.Status, *in.NewStatus)
		}
		updated, err := s.store.UpdatePermit(ctx, tx, store.UpdatePermitParams{
			PermitID:          in.PermitID,
			ApplicationNumber: in.ApplicationNumber,
			SubmittedDate:     in.SubmittedDate,
			ExpectedIssueDate: in.ExpectedIssueDate,
			ActualIssueDate:   in.ActualIssueDate,
			FeeCents:          in.FeeCents,
			Status:            in.NewStatus,
			Notes:             in.Notes,
		})
		if err != nil {
			return err
		}
		permit = updated
		return nil
	})
	if err != nil {
		return models.Permit{}, mapStoreError(err)
	}
	return permit, nil
}

// ---------- Analytics ----------

// GetPipelineAnalytics returns weighted-revenue and prospect-count
// totals per currency for the given org. Backed by a single SQL
// aggregation in a read-only tx — no caching layer yet (a future
// pipeline_analytics River job could write a precomputed cache table
// once the analytics surface is heavy enough to justify the hop).
func (s *PipelineService) GetPipelineAnalytics(ctx context.Context, orgID uuid.UUID) ([]models.PipelineAnalyticsRow, error) {
	var out []models.PipelineAnalyticsRow
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		var qErr error
		out, qErr = s.store.ListPipelineAnalytics(ctx, tx, orgID)
		return qErr
	})
	return out, err
}

// ---------- Lose ----------

// LoseProspect transitions a prospect to LOST. Reason is required.
// Returns ErrTerminalStage if the prospect is already in PERMIT_ISSUED
// (LOST is the failure terminal — once a prospect ships into CPM, marking
// it lost would orphan the project).
//
// callerUserSub is recorded on the audit row.
func (s *PipelineService) LoseProspect(ctx context.Context, callerUserSub string, in LoseProspectInput) (models.Prospect, error) {
	if in.Reason == "" {
		return models.Prospect{}, fmt.Errorf("%w: reason is required", ErrInvalidInput)
	}

	var prospect models.Prospect
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		current, err := s.store.GetProspect(ctx, tx, in.ProspectID, in.OrgID)
		if err != nil {
			return err
		}
		if current.PipelineStage == models.StagePermitIssued {
			return fmt.Errorf("%w: prospect already advanced to PERMIT_ISSUED; cannot mark lost", ErrTerminalStage)
		}
		// Already-lost prospects accept a reason rewrite (idempotent).

		updated, err := s.store.MarkLost(ctx, tx, in.ProspectID, in.OrgID, in.Reason)
		if err != nil {
			return err
		}
		prospect = updated

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      callerUserSub,
			Action:       "pipeline.stage_transitioned",
			ResourceType: AuditResourceProspect,
			ResourceID:   in.ProspectID,
			Before: marshalAudit(map[string]any{
				"stage":           current.PipelineStage,
				"probability_pct": current.ProbabilityPct,
			}),
			After: marshalAudit(map[string]any{
				"stage":           updated.PipelineStage,
				"probability_pct": updated.ProbabilityPct,
			}),
			// Reason can be free-form prose. The audit-JSONB scrub
			// (PR #10) masks any Restricted PII the user may have
			// included; vendor names / numeric figures stay readable.
			Metadata: marshalAudit(map[string]any{
				"reason":        in.Reason,
				"prospect_name": updated.Name,
			}),
		})
		return nil
	})
	if err != nil {
		return models.Prospect{}, mapStoreError(err)
	}
	return prospect, nil
}
