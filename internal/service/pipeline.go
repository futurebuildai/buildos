package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
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
// Kanban→CPM transition (Sprint 3 PR 3).
type PipelineService struct {
	pool  *pgxpool.Pool
	store *store.PipelineStore
}

// NewPipelineService creates a new PipelineService.
func NewPipelineService(pool *pgxpool.Pool, ps *store.PipelineStore) *PipelineService {
	return &PipelineService{pool: pool, store: ps}
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
func (s *PipelineService) AdvanceProspect(ctx context.Context, in AdvanceProspectInput) (models.Prospect, error) {
	// Reject unknown targets up front.
	if in.Target != models.StageLost && in.Target.Probability() == 0 {
		return models.Prospect{}, fmt.Errorf("%w: target_stage %q is not a known PipelineStage", ErrInvalidInput, in.Target)
	}

	if in.Target == models.StagePermitIssued {
		// Atomic Kanban→CPM lands in Sprint 3 PR 3. Returning here keeps
		// the wire contract honest — callers see a clear NOT_IMPLEMENTED
		// rather than silently transitioning the stage without creating
		// the construction Project.
		return models.Prospect{}, fmt.Errorf("%w: PERMIT_ISSUED transition (atomic Kanban→CPM) lands in Sprint 3 PR 3", ErrNotImplemented)
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

// LoseProspect transitions a prospect to LOST. Reason is required.
// Returns ErrTerminalStage if the prospect is already in PERMIT_ISSUED
// (LOST is the failure terminal — once a prospect ships into CPM, marking
// it lost would orphan the project).
func (s *PipelineService) LoseProspect(ctx context.Context, in LoseProspectInput) (models.Prospect, error) {
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
		return nil
	})
	if err != nil {
		return models.Prospect{}, mapStoreError(err)
	}
	return prospect, nil
}
