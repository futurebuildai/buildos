package agents

import (
	"context"
	"fmt"
	"math"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgDriftRepository implements DriftRepository using PostgreSQL.
type PgDriftRepository struct {
	db *pgxpool.Pool
}

// NewPgDriftRepository creates a new PostgreSQL-backed drift repository.
func NewPgDriftRepository(db *pgxpool.Pool) *PgDriftRepository {
	return &PgDriftRepository{db: db}
}

// StreamCompletedTasksByProject fetches completed tasks with actual/predicted durations,
// grouped by project, and calls fn for each project batch.
func (r *PgDriftRepository) StreamCompletedTasksByProject(
	ctx context.Context,
	fn func(projectID, orgID uuid.UUID, tasks []CompletedTaskRow) error,
) error {
	rows, err := r.db.Query(ctx, `
		SELECT pt.id, pt.project_id, p.org_id,
			pt.calculated_duration,
			EXTRACT(EPOCH FROM (pt.actual_end - pt.actual_start)) / 86400.0 AS actual_days
		FROM project_tasks pt
		JOIN projects p ON p.id = pt.project_id
		WHERE pt.status = 'Completed'
			AND pt.actual_start IS NOT NULL
			AND pt.actual_end IS NOT NULL
			AND pt.calculated_duration > 0
		ORDER BY pt.project_id, pt.actual_end ASC
	`)
	if err != nil {
		return fmt.Errorf("drift repo: query: %w", err)
	}
	defer rows.Close()

	var (
		currentProjectID uuid.UUID
		currentOrgID     uuid.UUID
		batch            []CompletedTaskRow
	)

	flush := func() error {
		if len(batch) > 0 {
			if err := fn(currentProjectID, currentOrgID, batch); err != nil {
				return err
			}
			batch = nil
		}
		return nil
	}

	for rows.Next() {
		var row CompletedTaskRow
		var actualDays float64
		if err := rows.Scan(&row.TaskID, &row.ProjectID, &row.OrgID, &row.PredictedDuration, &actualDays); err != nil {
			return fmt.Errorf("drift repo: scan: %w", err)
		}
		row.ActualDurationDays = math.Max(actualDays, 0)

		if row.ProjectID != currentProjectID && len(batch) > 0 {
			if err := flush(); err != nil {
				return err
			}
		}
		currentProjectID = row.ProjectID
		currentOrgID = row.OrgID
		batch = append(batch, row)
	}

	if err := flush(); err != nil {
		return err
	}

	return rows.Err()
}

// Compile-time check
var _ DriftRepository = (*PgDriftRepository)(nil)
