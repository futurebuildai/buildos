package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/models/types"
)

// ScheduleStore provides raw SQL queries for CPM schedule data.
type ScheduleStore struct{}

// NewScheduleStore creates a new ScheduleStore.
func NewScheduleStore() *ScheduleStore {
	return &ScheduleStore{}
}

// GetProjectTasks returns all tasks for a project, ordered by WBS code.
func (s *ScheduleStore) GetProjectTasks(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) ([]models.ProjectTask, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, project_id, wbs_code, name, duration_days,
			   early_start, early_finish, late_start, late_finish,
			   total_float, is_critical, status, percent_complete, assigned_crew,
			   created_at, updated_at
		FROM project_tasks
		WHERE project_id = $1
		ORDER BY wbs_code`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.ProjectTask
	for rows.Next() {
		var t models.ProjectTask
		err := rows.Scan(
			&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.DurationDays,
			&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
			&t.TotalFloat, &t.IsCritical, &t.Status, &t.PercentComplete, &t.AssignedCrew,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

// GetProjectDependencies returns all task dependencies for a project.
func (s *ScheduleStore) GetProjectDependencies(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) ([]models.TaskDependency, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, project_id, predecessor_id, successor_id, dependency_type, lag_days
		FROM task_dependencies
		WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []models.TaskDependency
	for rows.Next() {
		var d models.TaskDependency
		var depType string
		err := rows.Scan(&d.ID, &d.ProjectID, &d.PredecessorID, &d.SuccessorID, &depType, &d.LagDays)
		if err != nil {
			return nil, err
		}
		d.DependencyType = types.DependencyType(depType)
		deps = append(deps, d)
	}

	return deps, rows.Err()
}

// GetProjectStartDate returns the project start date for CPM root tasks.
func (s *ScheduleStore) GetProjectStartDate(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) (time.Time, error) {
	var startDate *time.Time
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(project_start_date, permit_issued_date, created_at)
		FROM projects
		WHERE id = $1`, projectID).Scan(&startDate)
	if err != nil {
		return time.Time{}, err
	}
	if startDate == nil {
		return time.Now(), nil
	}
	return *startDate, nil
}

// UpdateSchedule persists CPM-computed schedule results for all tasks.
func (s *ScheduleStore) UpdateSchedule(ctx context.Context, tx pgx.Tx, tasks []models.ProjectTask, scheduleResults map[uuid.UUID]ScheduleResult) error {
	for _, task := range tasks {
		result, ok := scheduleResults[task.ID]
		if !ok {
			continue
		}

		_, err := tx.Exec(ctx, `
			UPDATE project_tasks
			SET early_start = $2,
				early_finish = $3,
				late_start = $4,
				late_finish = $5,
				total_float = $6,
				is_critical = $7,
				updated_at = now()
			WHERE id = $1`,
			task.ID,
			result.EarlyStart,
			result.EarlyFinish,
			result.LateStart,
			result.LateFinish,
			result.TotalFloat,
			result.IsCritical,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// ScheduleResult maps physics engine output to DB-persistable fields.
type ScheduleResult struct {
	EarlyStart  time.Time
	EarlyFinish time.Time
	LateStart   time.Time
	LateFinish  time.Time
	TotalFloat  int
	IsCritical  bool
}
