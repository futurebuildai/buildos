package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// PipelineStore provides raw SQL queries for the pre-construction pipeline:
// prospects, estimates, permits.
//
// All methods take a pgx.Tx so callers control transaction scope. Reads
// from API handlers run in a short-lived ReadOnly tx; the Kanban→CPM
// transition (Sprint 3 PR 3) runs everything in one ReadWrite tx so
// project creation, prospect update, WBS hydration, and CPM enqueue all
// commit together or not at all.
type PipelineStore struct{}

// NewPipelineStore creates a new PipelineStore.
func NewPipelineStore() *PipelineStore { return &PipelineStore{} }

// ---------- Tenant guard ----------

// VerifyProspectInOrg returns nil if the prospect belongs to the given
// org, ErrNotFound otherwise. Service-layer methods that operate on a
// prospect (or any of its estimates/permits) must call this first.
func (s *PipelineStore) VerifyProspectInOrg(ctx context.Context, tx pgx.Tx, prospectID, orgID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM pre_construction_prospects WHERE id = $1 AND org_id = $2
		)`, prospectID, orgID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify prospect in org: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// ---------- Prospects ----------

// ListProspectsParams controls the scope of a prospect listing.
// Page is 1-based; PerPage is bounded by the caller (service layer).
type ListProspectsParams struct {
	OrgID   uuid.UUID
	Stage   string // optional; empty = all stages
	Page    int
	PerPage int
}

// ProspectsPage bundles a page of prospects with the total count for
// pagination UI. Total is computed in the same query (window function)
// so the count and the page are consistent — never a race where total
// disagrees with len(Prospects).
type ProspectsPage struct {
	Prospects  []models.Prospect
	Total      int
	Page       int
	PerPage    int
	TotalPages int
}

// ListProspects returns one page of prospects for an org, with optional
// stage filter. Ordered by most recent first.
func (s *PipelineStore) ListProspects(ctx context.Context, tx pgx.Tx, params ListProspectsParams) (ProspectsPage, error) {
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PerPage < 1 {
		params.PerPage = 50
	}
	offset := (params.Page - 1) * params.PerPage

	// $2 takes the optional stage filter as nullable text. Passing nil
	// causes the OR-IS-NULL branch to short-circuit to "no filter", so a
	// single static query handles both cases — the planner sees the same
	// shape every time.
	var stageArg any
	if params.Stage != "" {
		stageArg = params.Stage
	}
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, name, client_name, client_email, client_phone,
		       address, gsf, pipeline_stage, probability_pct, source, notes,
		       lost_reason, project_id, created_at, updated_at,
		       COUNT(*) OVER() AS total_count
		FROM pre_construction_prospects
		WHERE org_id = $1
		  AND ($2::text IS NULL OR pipeline_stage = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`,
		params.OrgID, stageArg, params.PerPage, offset)
	if err != nil {
		return ProspectsPage{}, fmt.Errorf("query prospects: %w", err)
	}
	defer rows.Close()

	out := ProspectsPage{
		Prospects: make([]models.Prospect, 0, params.PerPage),
		Page:      params.Page,
		PerPage:   params.PerPage,
	}
	for rows.Next() {
		var p models.Prospect
		var stage string
		var total int
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.ClientName, &p.ClientEmail, &p.ClientPhone,
			&p.Address, &p.GSF, &stage, &p.ProbabilityPct, &p.Source, &p.Notes,
			&p.LostReason, &p.ProjectID, &p.CreatedAt, &p.UpdatedAt,
			&total,
		); err != nil {
			return ProspectsPage{}, fmt.Errorf("scan prospect: %w", err)
		}
		p.PipelineStage = models.PipelineStage(stage)
		out.Prospects = append(out.Prospects, p)
		out.Total = total // identical across all rows
	}
	if err := rows.Err(); err != nil {
		return ProspectsPage{}, err
	}
	if out.PerPage > 0 {
		out.TotalPages = (out.Total + out.PerPage - 1) / out.PerPage
	}
	return out, nil
}

// GetProspect returns one prospect by id, scoped to an org. Returns
// ErrNotFound if the prospect doesn't exist or belongs to another org.
func (s *PipelineStore) GetProspect(ctx context.Context, tx pgx.Tx, prospectID, orgID uuid.UUID) (models.Prospect, error) {
	var p models.Prospect
	var stage string
	err := tx.QueryRow(ctx, `
		SELECT id, org_id, name, client_name, client_email, client_phone,
		       address, gsf, pipeline_stage, probability_pct, source, notes,
		       lost_reason, project_id, created_at, updated_at
		FROM pre_construction_prospects
		WHERE id = $1 AND org_id = $2`,
		prospectID, orgID,
	).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ClientName, &p.ClientEmail, &p.ClientPhone,
		&p.Address, &p.GSF, &stage, &p.ProbabilityPct, &p.Source, &p.Notes,
		&p.LostReason, &p.ProjectID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Prospect{}, ErrNotFound
		}
		return models.Prospect{}, fmt.Errorf("get prospect: %w", err)
	}
	p.PipelineStage = models.PipelineStage(stage)
	return p, nil
}

// ---------- Estimates ----------

// ListEstimatesForProspect returns every estimate for a prospect, newest
// version first. Caller should have already verified prospect ownership.
func (s *PipelineStore) ListEstimatesForProspect(ctx context.Context, tx pgx.Tx, prospectID uuid.UUID) ([]models.PipelineEstimate, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, prospect_id, version, total_estimated_cents, currency_code,
		       line_items, margin_pct, status, sent_at, created_at, updated_at
		FROM pre_construction_estimates
		WHERE prospect_id = $1
		ORDER BY version DESC`, prospectID)
	if err != nil {
		return nil, fmt.Errorf("query estimates: %w", err)
	}
	defer rows.Close()

	out := make([]models.PipelineEstimate, 0)
	for rows.Next() {
		var e models.PipelineEstimate
		if err := rows.Scan(
			&e.ID, &e.ProspectID, &e.Version, &e.TotalEstimatedCents, &e.CurrencyCode,
			&e.LineItems, &e.MarginPct, &e.Status, &e.SentAt, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan estimate: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------- Permits ----------

// ListPermitsForProspect returns every permit tracked for a prospect,
// ordered by submitted_date (or created_at when submitted_date is null).
// Caller should have already verified prospect ownership.
func (s *PipelineStore) ListPermitsForProspect(ctx context.Context, tx pgx.Tx, prospectID uuid.UUID) ([]models.Permit, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, prospect_id, permit_type, jurisdiction, application_number,
		       submitted_date, expected_issue_date, actual_issue_date,
		       fee_cents, fee_currency_code, status, notes, created_at, updated_at
		FROM pre_construction_permits
		WHERE prospect_id = $1
		ORDER BY COALESCE(submitted_date, created_at::DATE) DESC, created_at DESC`,
		prospectID)
	if err != nil {
		return nil, fmt.Errorf("query permits: %w", err)
	}
	defer rows.Close()

	out := make([]models.Permit, 0)
	for rows.Next() {
		var p models.Permit
		if err := rows.Scan(
			&p.ID, &p.ProspectID, &p.PermitType, &p.Jurisdiction, &p.ApplicationNumber,
			&p.SubmittedDate, &p.ExpectedIssueDate, &p.ActualIssueDate,
			&p.FeeCents, &p.FeeCurrencyCode, &p.Status, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan permit: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

