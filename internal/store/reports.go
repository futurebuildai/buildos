package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// reports.go holds the DERIVED daily-report reads (Chunk C of
// DAILY_REPORTS_CLIENT_UPDATES). A "daily report" is NOT a table — it is
// assembled on read per (project, calendar_date) from daily_logs +
// crew_checkins + task_progress. Every method is org+project-scoped:
//
//   - daily_logs / crew_checkins carry org_id directly → filter by it.
//   - task_progress has NO org_id; it MUST join project_tasks → projects and
//     assert projects.org_id = $org (the same defense-in-depth join the field
//     sync reads use). A cross-org id resolves to an empty result, never
//     another org's rows.
//
// These reads are exposed as methods on FieldStore (the source tables are its
// domain); the service layer (ReportsService) owns the bucketing + photo
// resolution.

// dailyLogRow is the daily_logs projection used to build a report.
type dailyLogRow struct {
	ProjectID         uuid.UUID
	ReportedBy        uuid.UUID
	LogDate           time.Time
	WeatherConditions *string
	WorkSummary       string
	SafetyIncidents   *string
	PhotoAssetIDs     []uuid.UUID
	ReportedAt        time.Time
}

// ListDailyReportDates returns the distinct calendar dates that have ANY field
// activity for a project (a daily_logs row, a crew check-in, or task progress),
// newest first. Used to populate the operator date selector. Org-scoped: the
// daily_logs predicate filters org_id directly; the crew/progress unions are
// scoped through the project's org (a project can only be read here after
// VerifyProjectInOrg, but the queries are independently org-safe).
//
// `limit` bounds the result (the selector shows a recent window); pass <=0 for a
// sane default of 60.
func (s *FieldStore) ListDailyReportDates(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, limit int) ([]time.Time, error) {
	if limit <= 0 {
		limit = 60
	}
	// UNION the three sources' calendar dates (UTC), distinct, newest first.
	// task_progress dates are derived via the project_tasks→projects join so
	// org isolation holds for the org_id-less table.
	rows, err := tx.Query(ctx, `
		SELECT d FROM (
			SELECT log_date AS d
			FROM daily_logs
			WHERE org_id = $1 AND project_id = $2
			UNION
			SELECT (reported_at AT TIME ZONE 'UTC')::date AS d
			FROM crew_checkins
			WHERE org_id = $1 AND project_id = $2
			UNION
			SELECT (tp.reported_at AT TIME ZONE 'UTC')::date AS d
			FROM task_progress tp
			JOIN project_tasks t ON t.id = tp.task_id
			JOIN projects p ON p.id = t.project_id
			WHERE p.org_id = $1 AND t.project_id = $2
		) dates
		ORDER BY d DESC
		LIMIT $3`,
		orgID, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list daily report dates: %w", err)
	}
	defer rows.Close()

	out := make([]time.Time, 0)
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan report date: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetDailyLogByProjectDate returns the daily_logs row for a (project, date), or
// (zero, false, nil) when there is no log that day (the report can still be
// built from crew/progress alone). Org-scoped.
func (s *FieldStore) getDailyLogByProjectDate(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, day time.Time) (dailyLogRow, bool, error) {
	d := day.UTC().Format("2006-01-02")
	var r dailyLogRow
	err := tx.QueryRow(ctx, `
		SELECT project_id, reported_by, log_date, weather_conditions,
		       work_summary, safety_incidents, photo_asset_ids, reported_at
		FROM daily_logs
		WHERE org_id = $1 AND project_id = $2 AND log_date = $3::date
		ORDER BY reported_at DESC
		LIMIT 1`,
		orgID, projectID, d,
	).Scan(
		&r.ProjectID, &r.ReportedBy, &r.LogDate, &r.WeatherConditions,
		&r.WorkSummary, &r.SafetyIncidents, &r.PhotoAssetIDs, &r.ReportedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return dailyLogRow{}, false, nil
		}
		return dailyLogRow{}, false, fmt.Errorf("get daily log by date: %w", err)
	}
	return r, true, nil
}

// CrewCountByProjectDate returns the total crew-member count across all crew
// check-ins for a (project, date) — the sum of crew_members JSONB array lengths
// (the opaque crew shape is never surfaced; only the count). Org-scoped.
func (s *FieldStore) CrewCountByProjectDate(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, day time.Time) (int, error) {
	d := day.UTC().Format("2006-01-02")
	var count int
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(jsonb_array_length(
			CASE WHEN jsonb_typeof(crew_members) = 'array' THEN crew_members ELSE '[]'::jsonb END
		)), 0)::int
		FROM crew_checkins
		WHERE org_id = $1 AND project_id = $2
		  AND (reported_at AT TIME ZONE 'UTC')::date = $3::date`,
		orgID, projectID, d,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("crew count by date: %w", err)
	}
	return count, nil
}

// TaskProgressByProjectDate returns the per-task progress lines for a (project,
// date), joined through project_tasks→projects for org isolation (task_progress
// has no org_id). GPS coords and the reporter identity are NOT selected — those
// are Restricted and excluded at the read-model boundary. Newest first.
func (s *FieldStore) TaskProgressByProjectDate(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, day time.Time) ([]models.TaskProgressLine, error) {
	d := day.UTC().Format("2006-01-02")
	rows, err := tx.Query(ctx, `
		SELECT tp.task_id, t.wbs_code, t.name, tp.percent_complete, tp.notes, tp.reported_at
		FROM task_progress tp
		JOIN project_tasks t ON t.id = tp.task_id
		JOIN projects p ON p.id = t.project_id
		WHERE p.org_id = $1 AND t.project_id = $2
		  AND (tp.reported_at AT TIME ZONE 'UTC')::date = $3::date
		ORDER BY tp.reported_at DESC`,
		orgID, projectID, d)
	if err != nil {
		return nil, fmt.Errorf("task progress by date: %w", err)
	}
	defer rows.Close()

	out := make([]models.TaskProgressLine, 0)
	for rows.Next() {
		var l models.TaskProgressLine
		var notes *string
		if err := rows.Scan(&l.TaskID, &l.WBSCode, &l.Name, &l.PercentComplete, &notes, &l.ReportedAt); err != nil {
			return nil, fmt.Errorf("scan task progress line: %w", err)
		}
		if notes != nil {
			l.Notes = *notes
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// dailyLogRowFor exposes the daily-log lookup for the service layer (which lives
// in another package) without leaking the private struct. It returns the model
// fields the service needs. ok=false means no daily_logs row for that day.
func (s *FieldStore) DailyLogFieldsByProjectDate(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, day time.Time) (DailyLogFields, bool, error) {
	r, ok, err := s.getDailyLogByProjectDate(ctx, tx, orgID, projectID, day)
	if err != nil || !ok {
		return DailyLogFields{}, ok, err
	}
	f := DailyLogFields{
		ProjectID:     r.ProjectID,
		ReportedBy:    r.ReportedBy,
		LogDate:       r.LogDate,
		WorkSummary:   r.WorkSummary,
		PhotoAssetIDs: r.PhotoAssetIDs,
		ReportedAt:    r.ReportedAt,
	}
	if r.WeatherConditions != nil {
		f.WeatherConditions = *r.WeatherConditions
	}
	if r.SafetyIncidents != nil {
		f.SafetyIncidents = *r.SafetyIncidents
	}
	return f, true, nil
}

// DailyLogFields is the service-facing projection of a daily_logs row used to
// assemble a derived report. Pointers are flattened to values (a SQL NULL maps
// to the empty string).
type DailyLogFields struct {
	ProjectID         uuid.UUID
	ReportedBy        uuid.UUID
	LogDate           time.Time
	WeatherConditions string
	WorkSummary       string
	SafetyIncidents   string
	PhotoAssetIDs     []uuid.UUID
	ReportedAt        time.Time
}
