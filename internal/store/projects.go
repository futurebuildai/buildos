package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// VerifyProjectInOrg returns nil if the project belongs to the given
// org, ErrNotFound otherwise. Shared by every per-domain store and
// service that guards project-scoped operations (financials, schedule,
// future fleet/HR allocations).
//
// Lives at package scope rather than as a method on a ProjectStore type
// because it has no state and every call site is the same shape. The
// PipelineStore's VerifyProspectInOrg sits next to its prospect data
// and remains a method — different table, different domain.
func VerifyProjectInOrg(ctx context.Context, tx pgx.Tx, projectID, orgID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1 AND org_id = $2)`,
		projectID, orgID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify project in org: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// ProjectStore provides raw SQL CRUD for the projects table. Stateless;
// every method takes a pgx.Tx so the service layer owns transaction
// scope (read-only tx for List/Get, read-write for Create/Update),
// matching FinancialsStore and the other per-domain stores.
type ProjectStore struct{}

// NewProjectStore creates a new ProjectStore.
func NewProjectStore() *ProjectStore { return &ProjectStore{} }

// projectColumns is the canonical column list + order. Shared by every
// query so scanProject stays in lockstep with the SELECT/RETURNING list.
// client_name/email/phone are the homeowner contact (Chunk D) — Restricted PII.
const projectColumns = `id, org_id, name, address, permit_issued_date,
	project_start_date, status, gsf, client_name, client_email, client_phone,
	created_at, updated_at`

// scanProject reads one projects row in projectColumns order. The
// nullable TEXT address column is scanned through a *string local so a
// SQL NULL maps to the model's empty string rather than erroring. The
// nullable client_* contact columns scan into the model's *string fields
// (NULL stays nil — omitted from JSON).
func scanProject(row pgx.Row) (models.Project, error) {
	var p models.Project
	var address *string
	if err := row.Scan(
		&p.ID, &p.OrgID, &p.Name, &address, &p.PermitIssuedDate,
		&p.ProjectStartDate, &p.Status, &p.GSF,
		&p.ClientName, &p.ClientEmail, &p.ClientPhone,
		&p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return models.Project{}, err
	}
	if address != nil {
		p.Address = *address
	}
	return p, nil
}

// ListByOrg returns projects for an org, newest first. status is an
// optional exact-match filter ("" = no filter). limit/offset paginate;
// the service layer is responsible for sane bounds.
func (s *ProjectStore) ListByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, status string, limit, offset int) ([]models.Project, error) {
	q := `SELECT ` + projectColumns + ` FROM projects WHERE org_id = $1`
	args := []any{orgID}
	if status != "" {
		q += ` AND status = $2`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query projects: %w", err)
	}
	defer rows.Close()

	out := make([]models.Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan projects: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ListActiveAcrossOrgsForSweep enumerates active projects across ALL orgs for
// the system-actor foresight sweep, keyset-paginated by id.
//
// !!! DELIBERATELY OMITS THE org_id FILTER !!!
// This is the ONE project read in the codebase that intentionally crosses org
// boundaries. It is the sanctioned ADR-002 exception (deployment = tenant, so
// the worker is the system actor — same posture as the corporate rollup). The
// isolation lint does NOT defend tenant scoping, so this is a review tripwire:
//
//	NEVER wire this behind an HTTP handler. It is worker-only.
//
// Keyset pagination (afterID, the previous page's last id) bounds memory; pass
// uuid.Nil to start (every real uuid sorts after the nil uuid). Ordered by id
// so the keyset cursor is stable.
func (s *ProjectStore) ListActiveAcrossOrgsForSweep(ctx context.Context, tx pgx.Tx, limit int, afterID uuid.UUID) ([]models.Project, error) {
	rows, err := tx.Query(ctx, `SELECT `+projectColumns+`
		FROM projects
		WHERE status = 'active' AND id > $1
		ORDER BY id
		LIMIT $2`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("query active projects for sweep: %w", err)
	}
	defer rows.Close()

	out := make([]models.Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan active projects for sweep: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetByID returns a single project scoped to its org. Maps no-row to
// ErrNotFound so cross-org access is indistinguishable from a missing
// project (no existence leak).
func (s *ProjectStore) GetByID(ctx context.Context, tx pgx.Tx, projectID, orgID uuid.UUID) (models.Project, error) {
	row := tx.QueryRow(ctx, `SELECT `+projectColumns+`
		FROM projects WHERE id = $1 AND org_id = $2`, projectID, orgID)
	p, err := scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Project{}, ErrNotFound
		}
		return models.Project{}, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

// CreateProjectParams is the insert payload. status is omitted — the
// projects.status column defaults to 'active' in the schema.
type CreateProjectParams struct {
	OrgID            uuid.UUID
	Name             string
	Address          *string
	PermitIssuedDate *time.Time
	ProjectStartDate *time.Time
	GSF              *int
}

// Create inserts a project and returns the persisted row (with the
// DB-applied id, status default, and timestamps).
func (s *ProjectStore) Create(ctx context.Context, tx pgx.Tx, p CreateProjectParams) (models.Project, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO projects (org_id, name, address, permit_issued_date, project_start_date, gsf)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+projectColumns,
		p.OrgID, p.Name, p.Address, p.PermitIssuedDate, p.ProjectStartDate, p.GSF)
	out, err := scanProject(row)
	if err != nil {
		return models.Project{}, fmt.Errorf("insert project: %w", err)
	}
	return out, nil
}

// UpdateProjectParams is the partial-update payload. Nil fields are
// left unchanged (COALESCE patch semantics). Scoped by ID + OrgID.
type UpdateProjectParams struct {
	ID      uuid.UUID
	OrgID   uuid.UUID
	Name    *string
	Address *string
	Status  *string
	GSF     *int
}

// Update applies a partial patch and returns the updated row. No-row
// (missing project or cross-org) maps to ErrNotFound.
func (s *ProjectStore) Update(ctx context.Context, tx pgx.Tx, p UpdateProjectParams) (models.Project, error) {
	row := tx.QueryRow(ctx, `
		UPDATE projects SET
			name    = COALESCE($3, name),
			address = COALESCE($4, address),
			status  = COALESCE($5, status),
			gsf     = COALESCE($6, gsf),
			updated_at = now()
		WHERE id = $1 AND org_id = $2
		RETURNING `+projectColumns,
		p.ID, p.OrgID, p.Name, p.Address, p.Status, p.GSF)
	out, err := scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Project{}, ErrNotFound
		}
		return models.Project{}, fmt.Errorf("update project: %w", err)
	}
	return out, nil
}
