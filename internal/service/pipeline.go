package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// Pipeline service errors.
var (
	ErrInvalidTransition = errors.New("INVALID_TRANSITION: cannot advance to the requested stage")
	ErrProspectLost      = errors.New("PROSPECT_LOST: lost prospects cannot be modified")
	ErrAlreadyIssued     = errors.New("ALREADY_ISSUED: prospect already has a project")
	ErrMissingName       = errors.New("VALIDATION: name is required")
	ErrMissingClientName = errors.New("VALIDATION: client_name is required")
)

// PipelineService provides business logic for the pre-construction pipeline.
type PipelineService struct {
	store *store.PipelineStore
}

// NewPipelineService creates a new PipelineService.
func NewPipelineService(s *store.PipelineStore) *PipelineService {
	return &PipelineService{store: s}
}

// ListProspects returns all prospects for an org.
func (svc *PipelineService) ListProspects(ctx context.Context, orgID uuid.UUID) ([]models.Prospect, error) {
	return svc.store.ListProspects(ctx, orgID)
}

// GetProspectDetail returns a prospect with its estimates and permits.
func (svc *PipelineService) GetProspectDetail(ctx context.Context, prospectID uuid.UUID) (*models.ProspectDetail, error) {
	prospect, err := svc.store.GetProspect(ctx, prospectID)
	if err != nil {
		return nil, err
	}

	estimates, err := svc.store.ListEstimatesByProspect(ctx, prospectID)
	if err != nil {
		return nil, err
	}

	permits, err := svc.store.ListPermitsByProspect(ctx, prospectID)
	if err != nil {
		return nil, err
	}

	return &models.ProspectDetail{
		Prospect:  *prospect,
		Estimates: estimates,
		Permits:   permits,
	}, nil
}

// CreateProspect validates and creates a new prospect.
func (svc *PipelineService) CreateProspect(ctx context.Context, p *models.Prospect) (uuid.UUID, error) {
	if p.Name == "" {
		return uuid.Nil, ErrMissingName
	}
	if p.ClientName == "" {
		return uuid.Nil, ErrMissingClientName
	}

	// Defaults
	if p.PipelineStage == "" {
		p.PipelineStage = models.StageLead
	}
	p.ProbabilityPct = models.StageProbability[p.PipelineStage]

	return svc.store.CreateProspect(ctx, p)
}

// UpdateProspect validates and updates a prospect's mutable fields.
func (svc *PipelineService) UpdateProspect(ctx context.Context, p *models.Prospect) error {
	existing, err := svc.store.GetProspect(ctx, p.ID)
	if err != nil {
		return err
	}
	if existing.PipelineStage == models.StageLost {
		return ErrProspectLost
	}
	if p.Name == "" {
		return ErrMissingName
	}
	if p.ClientName == "" {
		return ErrMissingClientName
	}
	return svc.store.UpdateProspect(ctx, p)
}

// AdvanceProspect transitions a prospect to the next pipeline stage.
// Stage transitions must be sequential: LEAD→QUALIFIED→ESTIMATE_SENT→...→PERMIT_ISSUED.
// PERMIT_ISSUED triggers the atomic Kanban→CPM transition.
func (svc *PipelineService) AdvanceProspect(ctx context.Context, prospectID uuid.UUID) (*models.Prospect, error) {
	prospect, err := svc.store.GetProspect(ctx, prospectID)
	if err != nil {
		return nil, err
	}

	if prospect.PipelineStage == models.StageLost {
		return nil, ErrProspectLost
	}
	if prospect.ProjectID != nil {
		return nil, ErrAlreadyIssued
	}

	nextStage, err := nextPipelineStage(prospect.PipelineStage)
	if err != nil {
		return nil, err
	}

	// PERMIT_ISSUED is special — triggers Kanban→CPM transition
	if nextStage == models.StagePermitIssued {
		return svc.transitionToConstruction(ctx, prospect)
	}

	probability := models.StageProbability[nextStage]
	if err := svc.store.AdvanceProspect(ctx, prospectID, nextStage, probability); err != nil {
		return nil, err
	}

	prospect.PipelineStage = nextStage
	prospect.ProbabilityPct = probability
	return prospect, nil
}

// transitionToConstruction performs the atomic Kanban→CPM transition:
// 1. Create a project from the prospect
// 2. Advance prospect to PERMIT_ISSUED
// 3. Link prospect to project
// All within a single database transaction.
func (svc *PipelineService) transitionToConstruction(ctx context.Context, prospect *models.Prospect) (*models.Prospect, error) {
	tx, err := svc.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	address := ""
	if prospect.Address != nil {
		address = *prospect.Address
	}

	// 1. Create the project
	projectID, err := svc.store.CreateProjectInTx(ctx, tx, prospect.OrgID, prospect.Name, address)
	if err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}

	// 2. Advance to PERMIT_ISSUED
	probability := models.StageProbability[models.StagePermitIssued]
	if err := svc.store.AdvanceProspectInTx(ctx, tx, prospect.ID, models.StagePermitIssued, probability); err != nil {
		return nil, fmt.Errorf("advancing prospect: %w", err)
	}

	// 3. Link prospect to project
	if err := svc.store.SetProspectProjectInTx(ctx, tx, prospect.ID, projectID); err != nil {
		return nil, fmt.Errorf("linking prospect to project: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	prospect.PipelineStage = models.StagePermitIssued
	prospect.ProbabilityPct = probability
	prospect.ProjectID = &projectID
	return prospect, nil
}

// LoseProspect marks a prospect as lost with a reason.
func (svc *PipelineService) LoseProspect(ctx context.Context, prospectID uuid.UUID, reason string) error {
	prospect, err := svc.store.GetProspect(ctx, prospectID)
	if err != nil {
		return err
	}
	if prospect.PipelineStage == models.StageLost {
		return ErrProspectLost
	}
	if prospect.PipelineStage == models.StagePermitIssued {
		return fmt.Errorf("cannot lose a prospect that has already been issued a permit")
	}
	return svc.store.LoseProspect(ctx, prospectID, reason)
}

// --- Estimates ---

// CreateEstimate validates and creates a new estimate for a prospect.
func (svc *PipelineService) CreateEstimate(ctx context.Context, e *models.PipelineEstimate) (uuid.UUID, error) {
	if !SupportedCurrencies[e.CurrencyCode] {
		return uuid.Nil, ErrInvalidCurrency
	}
	if e.Status == "" {
		e.Status = models.EstimateStatusDraft
	}
	if e.MarginPct == 0 {
		e.MarginPct = 15 // default 15%
	}
	return svc.store.CreateEstimate(ctx, e)
}

// UpdateEstimate validates and updates an existing estimate.
func (svc *PipelineService) UpdateEstimate(ctx context.Context, e *models.PipelineEstimate) error {
	if !SupportedCurrencies[e.CurrencyCode] {
		return ErrInvalidCurrency
	}
	return svc.store.UpdateEstimate(ctx, e)
}

// --- Permits ---

// CreatePermit validates and creates a new permit for a prospect.
func (svc *PipelineService) CreatePermit(ctx context.Context, p *models.Permit) (uuid.UUID, error) {
	if !SupportedCurrencies[p.FeeCurrencyCode] {
		return uuid.Nil, ErrInvalidCurrency
	}
	if p.PermitType == "" {
		return uuid.Nil, fmt.Errorf("permit_type is required")
	}
	if p.Jurisdiction == "" {
		return uuid.Nil, fmt.Errorf("jurisdiction is required")
	}
	if p.Status == "" {
		p.Status = models.PermitStatusNotSubmitted
	}
	return svc.store.CreatePermit(ctx, p)
}

// UpdatePermit validates and updates an existing permit.
func (svc *PipelineService) UpdatePermit(ctx context.Context, p *models.Permit) error {
	if !SupportedCurrencies[p.FeeCurrencyCode] {
		return ErrInvalidCurrency
	}
	return svc.store.UpdatePermit(ctx, p)
}

// --- Analytics ---

// Analytics computes weighted revenue forecast grouped by currency.
func (svc *PipelineService) Analytics(ctx context.Context, orgID uuid.UUID) ([]models.PipelineAnalytics, error) {
	rows, err := svc.store.PipelineAnalyticsData(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// Group by currency
	byCurrency := make(map[string]*models.PipelineAnalytics)
	for _, row := range rows {
		analytics, ok := byCurrency[row.CurrencyCode]
		if !ok {
			analytics = &models.PipelineAnalytics{
				CurrencyCode: row.CurrencyCode,
			}
			byCurrency[row.CurrencyCode] = analytics
		}

		probability := models.StageProbability[row.Stage]
		weightedCents := row.TotalEstimatedCents * int64(probability) / 100

		analytics.TotalProspects += row.Count
		analytics.WeightedRevenueCents += weightedCents
		analytics.ByStage = append(analytics.ByStage, models.StageAnalytics{
			Stage:               row.Stage,
			Count:               row.Count,
			TotalEstimatedCents: row.TotalEstimatedCents,
			WeightedCents:       weightedCents,
		})
	}

	result := make([]models.PipelineAnalytics, 0, len(byCurrency))
	for _, a := range byCurrency {
		result = append(result, *a)
	}
	return result, nil
}

// --- stage validation helpers ---

// nextPipelineStage returns the next valid stage in the pipeline sequence.
func nextPipelineStage(current models.PipelineStage) (models.PipelineStage, error) {
	for i, stage := range models.StageOrder {
		if stage == current {
			if i+1 >= len(models.StageOrder) {
				return "", fmt.Errorf("%w: already at final stage %s", ErrInvalidTransition, current)
			}
			return models.StageOrder[i+1], nil
		}
	}
	return "", fmt.Errorf("%w: unknown stage %s", ErrInvalidTransition, current)
}

// ValidateStageTransition checks if moving from one stage to another is valid.
func ValidateStageTransition(from, to models.PipelineStage) error {
	next, err := nextPipelineStage(from)
	if err != nil {
		return err
	}
	if next != to {
		return fmt.Errorf("%w: cannot go from %s to %s, next valid stage is %s", ErrInvalidTransition, from, to, next)
	}
	return nil
}
