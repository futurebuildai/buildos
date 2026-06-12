package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/futurebuildai/buildos/internal/models"
)

// FieldStore manages task_progress, crew_checkins, daily_logs and
// supports the GET /field/sync read.
type FieldStore struct{}

// NewFieldStore creates a new FieldStore.
func NewFieldStore() *FieldStore { return &FieldStore{} }

// ErrIdempotencyConflict is returned by the three insert paths when
// the supplied idempotency_key already exists. The handler maps this
// to 409 — the mobile client treats this as a successful replay.
var ErrIdempotencyConflict = errors.New("field: idempotency key already used")

// ListAssignedTasksParams scopes a /field/sync task pull. Since may be
// zero — that means "first sync, give me everything currently open."
type ListAssignedTasksParams struct {
	UserID uuid.UUID
	OrgID  uuid.UUID
	Since  time.Time
}

// ListAssignedTasks returns project_tasks where the caller is in the
// assigned_crew array (or the project is in their org and unassigned).
// Filters out completed tasks. When Since is non-zero, only tasks
// whose updated_at is strictly greater than Since are returned —
// matches the mobile delta-sync semantics.
//
// Joining via projects(org_id) keeps cross-org tasks out of the result
// even if a stale assigned_crew array references the caller's user_id.
func (s *FieldStore) ListAssignedTasks(ctx context.Context, tx pgx.Tx, p ListAssignedTasksParams) ([]models.ProjectTask, error) {
	q := `
		SELECT t.id, t.project_id, t.wbs_code, t.name, t.duration_days,
		       t.early_start, t.early_finish, t.late_start, t.late_finish,
		       t.total_float, t.is_critical, t.status, t.percent_complete,
		       t.assigned_crew, t.created_at, t.updated_at
		FROM project_tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE p.org_id = $1
		  AND t.status <> 'completed'
		  AND $2::uuid = ANY(t.assigned_crew)`
	args := []any{p.OrgID, p.UserID}
	if !p.Since.IsZero() {
		args = append(args, p.Since)
		q += " AND t.updated_at > $3"
	}
	q += " ORDER BY t.early_start ASC NULLS LAST, t.created_at ASC"

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query field sync tasks: %w", err)
	}
	defer rows.Close()

	out := make([]models.ProjectTask, 0)
	for rows.Next() {
		var t models.ProjectTask
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.DurationDays,
			&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
			&t.TotalFloat, &t.IsCritical, &t.Status, &t.PercentComplete,
			&t.AssignedCrew, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan field sync task: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListAllocatedEquipmentParams scopes a /field/sync equipment pull. Today is
// the "as of" instant; only its calendar date (UTC) is used to decide which
// allocations are active.
type ListAllocatedEquipmentParams struct {
	UserID uuid.UUID
	OrgID  uuid.UUID
	Today  time.Time
}

// ListAllocatedEquipment returns the field-safe projection of fleet assets
// currently allocated to a project the caller is assigned to. The caller's
// project set is derived the same way the task pull derives visibility —
// projects where the caller is in assigned_crew on a NON-completed task — so a
// field worker sees only equipment on the sites they are actively working, not
// the org's whole fleet.
//
// This is a FULL-SET read (no Since/delta): relevance pivots on the allocation
// window and the asset status, neither of which is captured by a created_at
// delta, and neither table has updated_at. The caller refreshes its whole
// equipment cache each sync.
//
// Org isolation: equipment_allocations has no org_id, so it flows through the
// projects join on the allocation (p.org_id = $1) AND the org-scoped subquery
// (tp.org_id = $1) — defense in depth, mirroring ListAssignedTasks.
func (s *FieldStore) ListAllocatedEquipment(ctx context.Context, tx pgx.Tx, p ListAllocatedEquipmentParams) ([]models.FieldEquipment, error) {
	// Bind the date as an unambiguous 'YYYY-MM-DD' string so the [start,end)
	// window comparison doesn't depend on the DB session timezone.
	today := p.Today.UTC().Format("2006-01-02")
	q := `
		SELECT fa.id, fa.name, fa.asset_type, fa.serial_number, fa.status,
		       ea.start_date, ea.end_date
		FROM equipment_allocations ea
		JOIN fleet_assets fa ON fa.id = ea.asset_id
		JOIN projects p ON p.id = ea.project_id
		WHERE p.org_id = $1
		  AND fa.org_id = $1   -- the asset's own org too (defense in depth)
		  AND $3::date >= ea.start_date
		  AND $3::date <  ea.end_date
		  AND ea.project_id IN (
		    SELECT t.project_id
		    FROM project_tasks t
		    JOIN projects tp ON tp.id = t.project_id
		    WHERE tp.org_id = $1
		      AND t.status <> 'completed'
		      AND $2::uuid = ANY(t.assigned_crew)
		  )
		ORDER BY fa.name ASC, fa.id ASC`

	rows, err := tx.Query(ctx, q, p.OrgID, p.UserID, today)
	if err != nil {
		return nil, fmt.Errorf("query field sync equipment: %w", err)
	}
	defer rows.Close()

	out := make([]models.FieldEquipment, 0)
	for rows.Next() {
		var e models.FieldEquipment
		if err := rows.Scan(
			&e.ID, &e.Name, &e.AssetType, &e.SerialNumber, &e.Status,
			&e.StartDate, &e.EndDate,
		); err != nil {
			return nil, fmt.Errorf("scan field sync equipment: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReportProgressParams is the input for inserting a task_progress row.
type ReportProgressParams struct {
	TaskID          uuid.UUID
	ReportedBy      uuid.UUID
	PercentComplete int
	Notes           *string
	PhotoAssetID    *uuid.UUID
	GPSLat          *float64
	GPSLng          *float64
	ReportedVia     string
	IdempotencyKey  uuid.UUID
}

// ReportProgress inserts a task_progress row. UNIQUE(idempotency_key)
// dedup: a replayed insert returns ErrIdempotencyConflict, NOT a
// generic error — the handler maps it to 409.
func (s *FieldStore) ReportProgress(ctx context.Context, tx pgx.Tx, p ReportProgressParams) (models.TaskProgress, error) {
	var tp models.TaskProgress
	err := tx.QueryRow(ctx, `
		INSERT INTO task_progress (
			task_id, reported_by, percent_complete, notes, photo_asset_id,
			gps_lat, gps_lng, reported_via, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, task_id, reported_by, percent_complete, notes,
		          photo_asset_id, gps_lat, gps_lng, reported_via,
		          idempotency_key, reported_at`,
		p.TaskID, p.ReportedBy, p.PercentComplete, p.Notes, p.PhotoAssetID,
		p.GPSLat, p.GPSLng, p.ReportedVia, p.IdempotencyKey,
	).Scan(
		&tp.ID, &tp.TaskID, &tp.ReportedBy, &tp.PercentComplete, &tp.Notes,
		&tp.PhotoAssetID, &tp.GPSLat, &tp.GPSLng, &tp.ReportedVia,
		&tp.IdempotencyKey, &tp.ReportedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return models.TaskProgress{}, ErrIdempotencyConflict
		}
		return models.TaskProgress{}, fmt.Errorf("insert task_progress: %w", err)
	}
	return tp, nil
}

// CheckinParams is the input for inserting a crew_checkins row.
type CheckinParams struct {
	OrgID          uuid.UUID
	ProjectID      uuid.UUID
	ReportedBy     uuid.UUID
	CrewMembers    json.RawMessage // {worker_id, gps_lat, gps_lng}[]
	GPSLat         *float64
	GPSLng         *float64
	Notes          *string
	IdempotencyKey uuid.UUID
}

// Checkin inserts a crew_checkins row with idempotency dedup.
func (s *FieldStore) Checkin(ctx context.Context, tx pgx.Tx, p CheckinParams) (models.CrewCheckin, error) {
	crew := p.CrewMembers
	if len(crew) == 0 {
		crew = json.RawMessage(`[]`)
	}

	var c models.CrewCheckin
	err := tx.QueryRow(ctx, `
		INSERT INTO crew_checkins (
			org_id, project_id, reported_by, crew_members,
			gps_lat, gps_lng, notes, idempotency_key
		) VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)
		RETURNING id, org_id, project_id, reported_by, crew_members,
		          gps_lat, gps_lng, notes, idempotency_key, reported_at`,
		p.OrgID, p.ProjectID, p.ReportedBy, crew,
		p.GPSLat, p.GPSLng, p.Notes, p.IdempotencyKey,
	).Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.ReportedBy, &c.CrewMembers,
		&c.GPSLat, &c.GPSLng, &c.Notes, &c.IdempotencyKey, &c.ReportedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return models.CrewCheckin{}, ErrIdempotencyConflict
		}
		return models.CrewCheckin{}, fmt.Errorf("insert crew_checkin: %w", err)
	}
	return c, nil
}

// DailyLogParams is the input for inserting a daily_logs row.
type DailyLogParams struct {
	OrgID             uuid.UUID
	ProjectID         uuid.UUID
	ReportedBy        uuid.UUID
	LogDate           time.Time
	WeatherConditions *string
	WorkSummary       string
	SafetyIncidents   *string
	PhotoAssetIDs     []uuid.UUID
	IdempotencyKey    uuid.UUID
}

// DailyLog inserts a daily_logs row with idempotency dedup.
func (s *FieldStore) DailyLog(ctx context.Context, tx pgx.Tx, p DailyLogParams) (models.DailyLog, error) {
	var d models.DailyLog
	err := tx.QueryRow(ctx, `
		INSERT INTO daily_logs (
			org_id, project_id, reported_by, log_date,
			weather_conditions, work_summary, safety_incidents,
			photo_asset_ids, idempotency_key
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, org_id, project_id, reported_by, log_date,
		          weather_conditions, work_summary, safety_incidents,
		          photo_asset_ids, idempotency_key, reported_at`,
		p.OrgID, p.ProjectID, p.ReportedBy, p.LogDate,
		p.WeatherConditions, p.WorkSummary, p.SafetyIncidents,
		p.PhotoAssetIDs, p.IdempotencyKey,
	).Scan(
		&d.ID, &d.OrgID, &d.ProjectID, &d.ReportedBy, &d.LogDate,
		&d.WeatherConditions, &d.WorkSummary, &d.SafetyIncidents,
		&d.PhotoAssetIDs, &d.IdempotencyKey, &d.ReportedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return models.DailyLog{}, ErrIdempotencyConflict
		}
		return models.DailyLog{}, fmt.Errorf("insert daily_log: %w", err)
	}
	return d, nil
}

// AppendDailyLogPhotosParams scopes an append of photo asset ids to the daily
// log for a (project, date). The append is idempotent at the SQL level: the
// array union (de-dup) means re-running with already-linked ids is a no-op.
type AppendDailyLogPhotosParams struct {
	OrgID     uuid.UUID
	ProjectID uuid.UUID
	LogDate   time.Time
	AssetIDs  []uuid.UUID
	MaxPhotos int // hard cap on the resulting array length (0 = no cap)
}

// AppendDailyLogPhotos unions AssetIDs into the photo_asset_ids array of the
// daily_logs row for (org, project, date), de-duplicating and preserving order
// (existing ids first, then the new ones). It targets the MOST RECENT row for
// that day (matching DailyLogFieldsByProjectDate's ORDER BY reported_at DESC).
//
// Returns:
//   - ErrNotFound when no daily_logs row exists for that (project, date) — the
//     operator must have a log to attach photos to (the web flow creates one via
//     the field daily-log endpoint first if absent).
//   - ErrPhotoLimit when the union would exceed MaxPhotos.
//
// Org-scoped: a cross-org project never matches a row.
func (s *FieldStore) AppendDailyLogPhotos(ctx context.Context, tx pgx.Tx, p AppendDailyLogPhotosParams) (models.DailyLog, error) {
	d := p.LogDate.UTC().Format("2006-01-02")
	// Resolve the target row id first (most recent for the day), org-scoped.
	var logID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM daily_logs
		WHERE org_id = $1 AND project_id = $2 AND log_date = $3::date
		ORDER BY reported_at DESC
		LIMIT 1`,
		p.OrgID, p.ProjectID, d,
	).Scan(&logID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.DailyLog{}, ErrNotFound
		}
		return models.DailyLog{}, fmt.Errorf("locate daily log for append: %w", err)
	}

	// Union the arrays in SQL preserving "existing first, new after" ORDER and
	// de-duping. WITH ORDINALITY assigns each id a position; we keep the FIRST
	// occurrence (min ordinal) so an already-present id stays in its original
	// slot and the new ids append in input order. The cap guard rejects a union
	// that would exceed MaxPhotos.
	var dl models.DailyLog
	err = tx.QueryRow(ctx, `
		WITH src AS (
			SELECT u, ord FROM (
				SELECT u, ord FROM unnest(
					(SELECT COALESCE(photo_asset_ids, '{}'::uuid[]) FROM daily_logs WHERE id = $1)
				) WITH ORDINALITY AS e(u, ord)
				UNION ALL
				SELECT u, ord + 1000000 FROM unnest($2::uuid[]) WITH ORDINALITY AS n(u, ord)
			) z
		),
		ranked AS (
			SELECT u, MIN(ord) AS first_ord FROM src GROUP BY u
		),
		merged AS (
			SELECT ARRAY(SELECT u FROM ranked ORDER BY first_ord) AS new_ids
		)
		UPDATE daily_logs dl
		SET photo_asset_ids = merged.new_ids
		FROM merged
		WHERE dl.id = $1
		  AND ($3::int = 0 OR cardinality(merged.new_ids) <= $3)
		RETURNING dl.id, dl.org_id, dl.project_id, dl.reported_by, dl.log_date,
		          dl.weather_conditions, dl.work_summary, dl.safety_incidents,
		          dl.photo_asset_ids, dl.idempotency_key, dl.reported_at`,
		logID, p.AssetIDs, p.MaxPhotos,
	).Scan(
		&dl.ID, &dl.OrgID, &dl.ProjectID, &dl.ReportedBy, &dl.LogDate,
		&dl.WeatherConditions, &dl.WorkSummary, &dl.SafetyIncidents,
		&dl.PhotoAssetIDs, &dl.IdempotencyKey, &dl.ReportedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The UPDATE matched the row but the cap guard rejected it.
			return models.DailyLog{}, ErrPhotoLimit
		}
		return models.DailyLog{}, fmt.Errorf("append daily log photos: %w", err)
	}
	return dl, nil
}

// LookupUserIDBySubject resolves the `sub` JWT claim to a users.id UUID,
// scoped to the caller's org. After the standalone pivot, BuildOS mints its
// own tokens with sub = users.id (auth.go: issuer.Mint(u.ID.String(), ...)),
// so the subject IS the user id — the legacy oidc_subject column is NULL for
// every native (password-backed) user (migration 011). We therefore resolve
// by id directly; matching on oidc_subject would never hit for native auth and
// silently 404 the entire field write path. A non-UUID subject (impossible for
// a valid native token) and a missing row both map to ErrNotFound, which the
// service layer surfaces as a 401/404 (valid JWT but no matching user — e.g. a
// stale token after the user was removed).
func (s *FieldStore) LookupUserIDBySubject(ctx context.Context, tx pgx.Tx, subject string, orgID uuid.UUID) (uuid.UUID, error) {
	subjectID, err := uuid.Parse(subject)
	if err != nil {
		return uuid.Nil, ErrNotFound
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id FROM users WHERE id = $1 AND org_id = $2`,
		subjectID, orgID,
	).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("lookup user_id by subject: %w", err)
	}
	return id, nil
}

// VerifyTaskInOrg returns nil if a project_tasks row exists whose
// project_id belongs to orgID. Used to guard ReportProgress so a
// caller can't pin progress on tasks they don't own.
func (s *FieldStore) VerifyTaskInOrg(ctx context.Context, tx pgx.Tx, taskID, orgID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(
		  SELECT 1 FROM project_tasks t
		  JOIN projects p ON p.id = t.project_id
		  WHERE t.id = $1 AND p.org_id = $2
		)`,
		taskID, orgID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify task in org: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

// ProjectIDForTaskInOrg returns the project_id of a task that belongs to orgID,
// or ErrNotFound if the task does not exist in the org. It subsumes
// VerifyTaskInOrg (same org guard) and additionally yields the project_id so
// ReportProgress can validate a pinned photo against the task's project.
func (s *FieldStore) ProjectIDForTaskInOrg(ctx context.Context, tx pgx.Tx, taskID, orgID uuid.UUID) (uuid.UUID, error) {
	var projectID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT t.project_id FROM project_tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1 AND p.org_id = $2`,
		taskID, orgID,
	).Scan(&projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, fmt.Errorf("project id for task in org: %w", err)
	}
	return projectID, nil
}

// isUniqueViolation reports whether err is a Postgres SQLSTATE 23505
// (unique_violation). All three field-write inserts share this dedup
// path, so the helper lives here rather than duplicated three times.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

// isUniqueViolationOnConstraint reports whether err is a unique_violation (23505)
// raised specifically by the named constraint/index (Postgres reports a unique
// index's name in ConstraintName). Use it when a table has — or may later gain —
// more than one UNIQUE constraint and the caller must react to only one of them.
func isUniqueViolationOnConstraint(err error, name string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation && pgErr.ConstraintName == name
}
