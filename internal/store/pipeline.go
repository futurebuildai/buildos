package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// PipelineStore provides raw SQL access to pre-construction pipeline tables.
type PipelineStore struct {
	pool *pgxpool.Pool
}

// NewPipelineStore creates a new PipelineStore.
func NewPipelineStore(pool *pgxpool.Pool) *PipelineStore {
	return &PipelineStore{pool: pool}
}

// --- Prospects ---

// ListProspects returns all prospects for an org, ordered by creation date (newest first).
func (s *PipelineStore) ListProspects(ctx context.Context, orgID uuid.UUID) ([]models.Prospect, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, name, client_name, client_email, client_phone,
			address, gsf, pipeline_stage, probability_pct, source, notes,
			lost_reason, project_id, created_at, updated_at
		FROM pre_construction_prospects
		WHERE org_id = $1
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing prospects: %w", err)
	}
	defer rows.Close()

	return collectProspects(rows)
}

// GetProspect returns a single prospect by ID.
func (s *PipelineStore) GetProspect(ctx context.Context, prospectID uuid.UUID) (*models.Prospect, error) {
	var p models.Prospect
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, name, client_name, client_email, client_phone,
			address, gsf, pipeline_stage, probability_pct, source, notes,
			lost_reason, project_id, created_at, updated_at
		FROM pre_construction_prospects
		WHERE id = $1`, prospectID,
	).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ClientName, &p.ClientEmail, &p.ClientPhone,
		&p.Address, &p.GSF, &p.PipelineStage, &p.ProbabilityPct, &p.Source, &p.Notes,
		&p.LostReason, &p.ProjectID, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting prospect: %w", err)
	}
	return &p, nil
}

// CreateProspect inserts a new prospect and returns its ID.
func (s *PipelineStore) CreateProspect(ctx context.Context, p *models.Prospect) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pre_construction_prospects (
			org_id, name, client_name, client_email, client_phone,
			address, gsf, pipeline_stage, probability_pct, source, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		p.OrgID, p.Name, p.ClientName, p.ClientEmail, p.ClientPhone,
		p.Address, p.GSF, p.PipelineStage, p.ProbabilityPct, p.Source, p.Notes,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating prospect: %w", err)
	}
	return id, nil
}

// UpdateProspect updates mutable fields on a prospect.
func (s *PipelineStore) UpdateProspect(ctx context.Context, p *models.Prospect) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pre_construction_prospects SET
			name = $2, client_name = $3, client_email = $4, client_phone = $5,
			address = $6, gsf = $7, source = $8, notes = $9, updated_at = now()
		WHERE id = $1`,
		p.ID, p.Name, p.ClientName, p.ClientEmail, p.ClientPhone,
		p.Address, p.GSF, p.Source, p.Notes,
	)
	if err != nil {
		return fmt.Errorf("updating prospect: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// AdvanceProspect sets the pipeline stage, probability, and updated_at.
func (s *PipelineStore) AdvanceProspect(ctx context.Context, prospectID uuid.UUID, stage models.PipelineStage, probability int) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pre_construction_prospects
		SET pipeline_stage = $2, probability_pct = $3, updated_at = now()
		WHERE id = $1`,
		prospectID, stage, probability,
	)
	if err != nil {
		return fmt.Errorf("advancing prospect: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// LoseProspect marks a prospect as lost with a reason.
func (s *PipelineStore) LoseProspect(ctx context.Context, prospectID uuid.UUID, reason string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pre_construction_prospects
		SET pipeline_stage = 'LOST', probability_pct = 0, lost_reason = $2, updated_at = now()
		WHERE id = $1`,
		prospectID, reason,
	)
	if err != nil {
		return fmt.Errorf("losing prospect: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SetProspectProject links a prospect to a project (on PERMIT_ISSUED transition).
func (s *PipelineStore) SetProspectProject(ctx context.Context, prospectID, projectID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE pre_construction_prospects
		SET project_id = $2, updated_at = now()
		WHERE id = $1`,
		prospectID, projectID,
	)
	if err != nil {
		return fmt.Errorf("setting prospect project: %w", err)
	}
	return nil
}

// --- Estimates ---

// ListEstimatesByProspect returns all estimates for a prospect.
func (s *PipelineStore) ListEstimatesByProspect(ctx context.Context, prospectID uuid.UUID) ([]models.PipelineEstimate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, prospect_id, version, total_estimated_cents, currency_code,
			line_items, margin_pct, status, sent_at, created_at, updated_at
		FROM pre_construction_estimates
		WHERE prospect_id = $1
		ORDER BY version DESC`, prospectID)
	if err != nil {
		return nil, fmt.Errorf("listing estimates: %w", err)
	}
	defer rows.Close()

	return collectEstimates(rows)
}

// CreateEstimate inserts a new estimate.
func (s *PipelineStore) CreateEstimate(ctx context.Context, e *models.PipelineEstimate) (uuid.UUID, error) {
	// Auto-increment version
	var maxVersion int
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM pre_construction_estimates WHERE prospect_id = $1`,
		e.ProspectID,
	).Scan(&maxVersion)

	lineItems := e.LineItems
	if lineItems == nil {
		lineItems = json.RawMessage(`[]`)
	}

	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pre_construction_estimates (
			prospect_id, version, total_estimated_cents, currency_code,
			line_items, margin_pct, status, sent_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		e.ProspectID, maxVersion+1, e.TotalEstimatedCents, e.CurrencyCode,
		lineItems, e.MarginPct, e.Status, e.SentAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating estimate: %w", err)
	}
	return id, nil
}

// UpdateEstimate modifies an existing estimate.
func (s *PipelineStore) UpdateEstimate(ctx context.Context, e *models.PipelineEstimate) error {
	lineItems := e.LineItems
	if lineItems == nil {
		lineItems = json.RawMessage(`[]`)
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE pre_construction_estimates SET
			total_estimated_cents = $2, currency_code = $3, line_items = $4,
			margin_pct = $5, status = $6, sent_at = $7, updated_at = now()
		WHERE id = $1`,
		e.ID, e.TotalEstimatedCents, e.CurrencyCode, lineItems,
		e.MarginPct, e.Status, e.SentAt,
	)
	if err != nil {
		return fmt.Errorf("updating estimate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// LatestAcceptedEstimate returns the newest accepted estimate for a prospect.
func (s *PipelineStore) LatestAcceptedEstimate(ctx context.Context, prospectID uuid.UUID) (*models.PipelineEstimate, error) {
	var e models.PipelineEstimate
	err := s.pool.QueryRow(ctx, `
		SELECT id, prospect_id, version, total_estimated_cents, currency_code,
			line_items, margin_pct, status, sent_at, created_at, updated_at
		FROM pre_construction_estimates
		WHERE prospect_id = $1 AND status = 'accepted'
		ORDER BY version DESC
		LIMIT 1`, prospectID,
	).Scan(
		&e.ID, &e.ProspectID, &e.Version, &e.TotalEstimatedCents, &e.CurrencyCode,
		&e.LineItems, &e.MarginPct, &e.Status, &e.SentAt, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting latest accepted estimate: %w", err)
	}
	return &e, nil
}

// --- Permits ---

// ListPermitsByProspect returns all permits for a prospect.
func (s *PipelineStore) ListPermitsByProspect(ctx context.Context, prospectID uuid.UUID) ([]models.Permit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, prospect_id, permit_type, jurisdiction, application_number,
			submitted_date, expected_issue_date, actual_issue_date,
			fee_cents, fee_currency_code, status, notes, created_at, updated_at
		FROM pre_construction_permits
		WHERE prospect_id = $1
		ORDER BY created_at DESC`, prospectID)
	if err != nil {
		return nil, fmt.Errorf("listing permits: %w", err)
	}
	defer rows.Close()

	return collectPermits(rows)
}

// CreatePermit inserts a new permit.
func (s *PipelineStore) CreatePermit(ctx context.Context, p *models.Permit) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO pre_construction_permits (
			prospect_id, permit_type, jurisdiction, application_number,
			submitted_date, expected_issue_date, actual_issue_date,
			fee_cents, fee_currency_code, status, notes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		p.ProspectID, p.PermitType, p.Jurisdiction, p.ApplicationNumber,
		p.SubmittedDate, p.ExpectedIssueDate, p.ActualIssueDate,
		p.FeeCents, p.FeeCurrencyCode, p.Status, p.Notes,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating permit: %w", err)
	}
	return id, nil
}

// UpdatePermit modifies an existing permit.
func (s *PipelineStore) UpdatePermit(ctx context.Context, p *models.Permit) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE pre_construction_permits SET
			permit_type = $2, jurisdiction = $3, application_number = $4,
			submitted_date = $5, expected_issue_date = $6, actual_issue_date = $7,
			fee_cents = $8, fee_currency_code = $9, status = $10, notes = $11,
			updated_at = now()
		WHERE id = $1`,
		p.ID, p.PermitType, p.Jurisdiction, p.ApplicationNumber,
		p.SubmittedDate, p.ExpectedIssueDate, p.ActualIssueDate,
		p.FeeCents, p.FeeCurrencyCode, p.Status, p.Notes,
	)
	if err != nil {
		return fmt.Errorf("updating permit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// --- Analytics ---

// PipelineAnalyticsRow is the raw result of the pipeline analytics query.
type PipelineAnalyticsRow struct {
	CurrencyCode        string
	Stage               models.PipelineStage
	Count               int
	TotalEstimatedCents int64
}

// PipelineAnalyticsData returns per-stage prospect counts and weighted estimate totals, grouped by currency.
func (s *PipelineStore) PipelineAnalyticsData(ctx context.Context, orgID uuid.UUID) ([]PipelineAnalyticsRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			COALESCE(e.currency_code, 'USD') AS currency_code,
			p.pipeline_stage,
			COUNT(DISTINCT p.id) AS prospect_count,
			COALESCE(SUM(e.total_estimated_cents), 0) AS total_estimated_cents
		FROM pre_construction_prospects p
		LEFT JOIN pre_construction_estimates e ON e.prospect_id = p.id AND e.status = 'accepted'
		WHERE p.org_id = $1 AND p.pipeline_stage != 'LOST'
		GROUP BY e.currency_code, p.pipeline_stage
		ORDER BY e.currency_code, p.pipeline_stage`, orgID)
	if err != nil {
		return nil, fmt.Errorf("querying pipeline analytics: %w", err)
	}
	defer rows.Close()

	var result []PipelineAnalyticsRow
	for rows.Next() {
		var r PipelineAnalyticsRow
		if err := rows.Scan(&r.CurrencyCode, &r.Stage, &r.Count, &r.TotalEstimatedCents); err != nil {
			return nil, fmt.Errorf("scanning analytics row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// --- Transaction support ---

// BeginTx starts a database transaction.
func (s *PipelineStore) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}

// CreateProjectInTx inserts a project row within a transaction (for Kanban→CPM transition).
func (s *PipelineStore) CreateProjectInTx(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, name, address string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, address, status)
		VALUES ($1, $2, $3, 'active')
		RETURNING id`,
		orgID, name, address,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating project in tx: %w", err)
	}
	return id, nil
}

// AdvanceProspectInTx sets the stage within a transaction.
func (s *PipelineStore) AdvanceProspectInTx(ctx context.Context, tx pgx.Tx, prospectID uuid.UUID, stage models.PipelineStage, probability int) error {
	_, err := tx.Exec(ctx, `
		UPDATE pre_construction_prospects
		SET pipeline_stage = $2, probability_pct = $3, updated_at = now()
		WHERE id = $1`,
		prospectID, stage, probability,
	)
	return err
}

// SetProspectProjectInTx links a prospect to a project within a transaction.
func (s *PipelineStore) SetProspectProjectInTx(ctx context.Context, tx pgx.Tx, prospectID, projectID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE pre_construction_prospects
		SET project_id = $2, updated_at = now()
		WHERE id = $1`,
		prospectID, projectID,
	)
	return err
}

// --- helpers ---

func collectProspects(rows pgx.Rows) ([]models.Prospect, error) {
	var prospects []models.Prospect
	for rows.Next() {
		var p models.Prospect
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.ClientName, &p.ClientEmail, &p.ClientPhone,
			&p.Address, &p.GSF, &p.PipelineStage, &p.ProbabilityPct, &p.Source, &p.Notes,
			&p.LostReason, &p.ProjectID, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning prospect: %w", err)
		}
		prospects = append(prospects, p)
	}
	return prospects, rows.Err()
}

func collectEstimates(rows pgx.Rows) ([]models.PipelineEstimate, error) {
	var estimates []models.PipelineEstimate
	for rows.Next() {
		var e models.PipelineEstimate
		if err := rows.Scan(
			&e.ID, &e.ProspectID, &e.Version, &e.TotalEstimatedCents, &e.CurrencyCode,
			&e.LineItems, &e.MarginPct, &e.Status, &e.SentAt, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning estimate: %w", err)
		}
		estimates = append(estimates, e)
	}
	return estimates, rows.Err()
}

func collectPermits(rows pgx.Rows) ([]models.Permit, error) {
	var permits []models.Permit
	for rows.Next() {
		var p models.Permit
		if err := rows.Scan(
			&p.ID, &p.ProspectID, &p.PermitType, &p.Jurisdiction, &p.ApplicationNumber,
			&p.SubmittedDate, &p.ExpectedIssueDate, &p.ActualIssueDate,
			&p.FeeCents, &p.FeeCurrencyCode, &p.Status, &p.Notes, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning permit: %w", err)
		}
		permits = append(permits, p)
	}
	return permits, rows.Err()
}
