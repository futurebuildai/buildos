package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/models/types"
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

// ListTasksParams filters a task listing. Empty Status / nil IsCritical
// means "no filter" on that dimension.
type ListTasksParams struct {
	ProjectID  uuid.UUID
	Status     string
	IsCritical *bool
}

// ListProjectTasks returns tasks for a project with optional status and
// is_critical filters. Distinct from GetProjectTasks (CPM internal use)
// which always returns the full set ordered for graph construction.
func (s *ScheduleStore) ListProjectTasks(ctx context.Context, tx pgx.Tx, params ListTasksParams) ([]models.ProjectTask, error) {
	// Static SQL with nullable filter args — same pattern as
	// PipelineStore.ListProspects, planner-friendly.
	var statusArg, criticalArg any
	if params.Status != "" {
		statusArg = params.Status
	}
	if params.IsCritical != nil {
		criticalArg = *params.IsCritical
	}

	rows, err := tx.Query(ctx, `
		SELECT id, project_id, wbs_code, name, duration_days,
		       early_start, early_finish, late_start, late_finish,
		       total_float, is_critical, status, percent_complete, assigned_crew,
		       created_at, updated_at
		FROM project_tasks
		WHERE project_id = $1
		  AND ($2::text IS NULL OR status = $2)
		  AND ($3::boolean IS NULL OR is_critical = $3)
		ORDER BY wbs_code`,
		params.ProjectID, statusArg, criticalArg)
	if err != nil {
		return nil, fmt.Errorf("query project_tasks: %w", err)
	}
	defer rows.Close()

	out := make([]models.ProjectTask, 0)
	for rows.Next() {
		var t models.ProjectTask
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.DurationDays,
			&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
			&t.TotalFloat, &t.IsCritical, &t.Status, &t.PercentComplete, &t.AssignedCrew,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project_task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// GetTaskInProject returns a single task scoped by (id, project_id).
// The service layer should already have verified project ownership.
// Returns ErrNotFound when the (task, project) pair doesn't match.
func (s *ScheduleStore) GetTaskInProject(ctx context.Context, tx pgx.Tx, taskID, projectID uuid.UUID) (models.ProjectTask, error) {
	var t models.ProjectTask
	err := tx.QueryRow(ctx, `
		SELECT id, project_id, wbs_code, name, duration_days,
		       early_start, early_finish, late_start, late_finish,
		       total_float, is_critical, status, percent_complete, assigned_crew,
		       created_at, updated_at
		FROM project_tasks
		WHERE id = $1 AND project_id = $2`, taskID, projectID,
	).Scan(
		&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.DurationDays,
		&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
		&t.TotalFloat, &t.IsCritical, &t.Status, &t.PercentComplete, &t.AssignedCrew,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ProjectTask{}, ErrNotFound
		}
		return models.ProjectTask{}, fmt.Errorf("get project_task: %w", err)
	}
	return t, nil
}

// UpdateTaskParams is the input for partial-updating a task. nil fields
// preserve existing values via COALESCE. AssignedCrew uses a pointer so
// callers can distinguish "no change" (nil) from "clear the crew"
// (non-nil pointer to empty slice).
type UpdateTaskParams struct {
	TaskID          uuid.UUID
	ProjectID       uuid.UUID
	PercentComplete *int
	Status          *string
	AssignedCrew    *[]uuid.UUID
}

// UpdateTask modifies a task's progress/status/crew. Returns ErrNotFound
// when no row matched. Does NOT trigger CPM recalculation — callers
// must POST .../schedule/recalculate when they want the critical path
// re-evaluated. (Future: enqueue DelayCascade automatically when status
// changes a critical task.)
func (s *ScheduleStore) UpdateTask(ctx context.Context, tx pgx.Tx, p UpdateTaskParams) (models.ProjectTask, error) {
	// AssignedCrew is a slice so we can't use COALESCE directly; pass nil
	// when the caller didn't supply one (CASE in SQL preserves the column).
	var crewArg any
	if p.AssignedCrew != nil {
		crewArg = *p.AssignedCrew
	}

	var t models.ProjectTask
	err := tx.QueryRow(ctx, `
		UPDATE project_tasks
		SET percent_complete = COALESCE($3, percent_complete),
		    status           = COALESCE($4, status),
		    assigned_crew    = CASE WHEN $5::uuid[] IS NULL THEN assigned_crew ELSE $5 END,
		    updated_at       = now()
		WHERE id = $1 AND project_id = $2
		RETURNING id, project_id, wbs_code, name, duration_days,
		          early_start, early_finish, late_start, late_finish,
		          total_float, is_critical, status, percent_complete, assigned_crew,
		          created_at, updated_at`,
		p.TaskID, p.ProjectID, p.PercentComplete, p.Status, crewArg,
	).Scan(
		&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.DurationDays,
		&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
		&t.TotalFloat, &t.IsCritical, &t.Status, &t.PercentComplete, &t.AssignedCrew,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ProjectTask{}, ErrNotFound
		}
		return models.ProjectTask{}, fmt.Errorf("update task: %w", err)
	}
	return t, nil
}
