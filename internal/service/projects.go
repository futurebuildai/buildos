package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Project audit-log resource type / action constants.
const (
	AuditResourceProject = "project"

	AuditActionProjectCreated = "project.created"
	AuditActionProjectUpdated = "project.updated"
)

// Project domain bounds. Status is a closed enum (matches the schema
// comment + API_CONTRACT). GSF range mirrors the documented residential
// envelope the DHSM duration model is calibrated for; enforced here
// because the column carries no CHECK constraint.
const (
	projectGSFMin = 1500
	projectGSFMax = 6000

	defaultProjectPageSize = 50
	maxProjectPageSize     = 200
)

// validProjectStatuses is the closed set accepted on create/update.
var validProjectStatuses = map[string]struct{}{
	"active":    {},
	"completed": {},
	"archived":  {},
}

// ProjectService owns project CRUD. Reads run under a short read-only
// tx; mutations follow the canonical one-tx-per-mutation + audit
// pattern (write + audit commit or roll back together).
type ProjectService struct {
	pool  *pgxpool.Pool
	store *store.ProjectStore
	audit AuditRecorder
}

// NewProjectService binds the service to a pool + store. audit may be
// nil; nil falls back to a no-op recorder.
func NewProjectService(pool *pgxpool.Pool, s *store.ProjectStore, audit AuditRecorder) *ProjectService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ProjectService{pool: pool, store: s, audit: audit}
}

// ---------- List ----------

// ListProjectsInput carries the optional status filter and pagination.
// Page is 1-based; zero/negative values fall back to defaults.
type ListProjectsInput struct {
	OrgID   uuid.UUID
	Status  string
	Page    int
	PerPage int
}

// ListProjects returns a page of projects for the org, newest first.
func (s *ProjectService) ListProjects(ctx context.Context, in ListProjectsInput) ([]models.Project, error) {
	if in.OrgID == uuid.Nil {
		return nil, fmt.Errorf("%w: org_id required", ErrInvalidInput)
	}
	status := strings.TrimSpace(in.Status)
	if status != "" {
		if _, ok := validProjectStatuses[status]; !ok {
			return nil, fmt.Errorf("%w: status must be one of {active, completed, archived}", ErrInvalidInput)
		}
	}
	limit, offset := paginate(in.Page, in.PerPage)

	var out []models.Project
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		list, err := s.store.ListByOrg(ctx, tx, in.OrgID, status, limit, offset)
		if err != nil {
			return err
		}
		out = list
		return nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return out, nil
}

// paginate normalizes a 1-based page + per_page into LIMIT/OFFSET,
// clamping per_page to [1, maxProjectPageSize] and page to >= 1.
func paginate(page, perPage int) (limit, offset int) {
	if perPage <= 0 {
		perPage = defaultProjectPageSize
	}
	if perPage > maxProjectPageSize {
		perPage = maxProjectPageSize
	}
	if page < 1 {
		page = 1
	}
	return perPage, (page - 1) * perPage
}

// ---------- Get ----------

// GetProject returns a single project scoped to the caller's org.
func (s *ProjectService) GetProject(ctx context.Context, orgID, projectID uuid.UUID) (models.Project, error) {
	if orgID == uuid.Nil || projectID == uuid.Nil {
		return models.Project{}, fmt.Errorf("%w: org_id and project_id required", ErrInvalidInput)
	}
	var out models.Project
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		p, err := s.store.GetByID(ctx, tx, projectID, orgID)
		if err != nil {
			return err
		}
		out = p
		return nil
	})
	if err != nil {
		return models.Project{}, mapStoreError(err)
	}
	return out, nil
}

// ---------- Create ----------

// CreateProjectInput is the POST /projects payload. Status is not
// settable on create — it defaults to 'active'. Address empty/blank is
// normalized to NULL.
type CreateProjectInput struct {
	OrgID            uuid.UUID
	UserSub          string
	Name             string
	Address          *string
	PermitIssuedDate *time.Time
	ProjectStartDate *time.Time
	GSF              *int
}

// CreateProject inserts a project after validating name + GSF range.
func (s *ProjectService) CreateProject(ctx context.Context, in CreateProjectInput) (models.Project, error) {
	if in.OrgID == uuid.Nil {
		return models.Project{}, fmt.Errorf("%w: org_id required", ErrInvalidInput)
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return models.Project{}, fmt.Errorf("%w: name required", ErrInvalidInput)
	}
	if err := validateGSF(in.GSF); err != nil {
		return models.Project{}, err
	}

	var out models.Project
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p, err := s.store.Create(ctx, tx, store.CreateProjectParams{
			OrgID:            in.OrgID,
			Name:             in.Name,
			Address:          normalizeOptionalString(in.Address),
			PermitIssuedDate: in.PermitIssuedDate,
			ProjectStartDate: in.ProjectStartDate,
			GSF:              in.GSF,
		})
		if err != nil {
			return err
		}
		out = p
		after, _ := json.Marshal(p)
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       AuditActionProjectCreated,
			ResourceType: AuditResourceProject,
			ResourceID:   p.ID,
			After:        after,
		})
		return nil
	})
	if err != nil {
		return models.Project{}, mapStoreError(err)
	}
	return out, nil
}

// ---------- Update ----------

// UpdateProjectInput is the PUT /projects/{id} partial patch. Nil
// fields are left unchanged. Dates are not editable here (per the API
// contract); use create to set them.
type UpdateProjectInput struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	UserSub   string
	Name      *string
	Address   *string
	Status    *string
	GSF       *int
}

// UpdateProject applies a partial patch. Requires at least one field
// and validates status + GSF when present.
func (s *ProjectService) UpdateProject(ctx context.Context, in UpdateProjectInput) (models.Project, error) {
	if in.OrgID == uuid.Nil || in.ProjectID == uuid.Nil {
		return models.Project{}, fmt.Errorf("%w: org_id and project_id required", ErrInvalidInput)
	}
	if in.Name == nil && in.Address == nil && in.Status == nil && in.GSF == nil {
		return models.Project{}, fmt.Errorf("%w: at least one field required", ErrInvalidInput)
	}
	if in.Name != nil {
		trimmed := strings.TrimSpace(*in.Name)
		if trimmed == "" {
			return models.Project{}, fmt.Errorf("%w: name must not be blank", ErrInvalidInput)
		}
		in.Name = &trimmed
	}
	if in.Status != nil {
		if _, ok := validProjectStatuses[*in.Status]; !ok {
			return models.Project{}, fmt.Errorf("%w: status must be one of {active, completed, archived}", ErrInvalidInput)
		}
	}
	if err := validateGSF(in.GSF); err != nil {
		return models.Project{}, err
	}

	var out models.Project
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		before, err := s.store.GetByID(ctx, tx, in.ProjectID, in.OrgID)
		if err != nil {
			return err
		}
		p, err := s.store.Update(ctx, tx, store.UpdateProjectParams{
			ID:      in.ProjectID,
			OrgID:   in.OrgID,
			Name:    in.Name,
			Address: in.Address,
			Status:  in.Status,
			GSF:     in.GSF,
		})
		if err != nil {
			return err
		}
		out = p
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(p)
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       AuditActionProjectUpdated,
			ResourceType: AuditResourceProject,
			ResourceID:   p.ID,
			Before:       beforeJSON,
			After:        afterJSON,
		})
		return nil
	})
	if err != nil {
		return models.Project{}, mapStoreError(err)
	}
	return out, nil
}

// validateGSF enforces the documented residential envelope when a GSF
// is supplied. nil (unset) is always valid — GSF is optional.
func validateGSF(gsf *int) error {
	if gsf == nil {
		return nil
	}
	if *gsf < projectGSFMin || *gsf > projectGSFMax {
		return fmt.Errorf("%w: gsf must be between %d and %d", ErrInvalidInput, projectGSFMin, projectGSFMax)
	}
	return nil
}

// normalizeOptionalString trims an optional string and collapses an
// empty/blank value to nil so it stores as SQL NULL rather than "".
func normalizeOptionalString(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
