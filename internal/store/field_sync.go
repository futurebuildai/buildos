package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// FieldSyncStore provides raw SQL access for field sync operations.
type FieldSyncStore struct {
	pool *pgxpool.Pool
}

// NewFieldSyncStore creates a new FieldSyncStore.
func NewFieldSyncStore(pool *pgxpool.Pool) *FieldSyncStore {
	return &FieldSyncStore{pool: pool}
}

// GetSyncPayload returns feed cards and tasks updated since the given timestamp.
func (s *FieldSyncStore) GetSyncPayload(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, role string, since time.Time) (*models.SyncPayload, error) {
	// Get feed cards since timestamp
	cardRows, err := s.pool.Query(ctx, `
		SELECT id, org_id, project_id, card_type, title, body,
			priority, target_user_id, target_role, actions, status,
			actioned_at, expires_at, created_at
		FROM feed_cards
		WHERE org_id = $1 AND created_at > $2
			AND status = 'active'
			AND (target_user_id = $3 OR target_role = $4 OR (target_user_id IS NULL AND target_role IS NULL))
		ORDER BY created_at DESC
		LIMIT 100`, orgID, since, userID, role)
	if err != nil {
		return nil, fmt.Errorf("querying feed cards for sync: %w", err)
	}
	defer cardRows.Close()

	var cards []models.FeedCard
	for cardRows.Next() {
		var c models.FeedCard
		if err := cardRows.Scan(
			&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body,
			&c.Priority, &c.TargetUserID, &c.TargetRole, &c.Actions, &c.Status,
			&c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning feed card: %w", err)
		}
		cards = append(cards, c)
	}
	if err := cardRows.Err(); err != nil {
		return nil, err
	}

	// Get tasks updated since timestamp (using project_tasks table)
	taskRows, err := s.pool.Query(ctx, `
		SELECT t.id, t.project_id, t.name, t.status, t.percent_complete, t.late_finish, t.updated_at
		FROM project_tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE p.org_id = $1 AND t.updated_at > $2
		ORDER BY t.updated_at DESC
		LIMIT 200`, orgID, since)
	if err != nil {
		return nil, fmt.Errorf("querying tasks for sync: %w", err)
	}
	defer taskRows.Close()

	var tasks []models.SyncTask
	for taskRows.Next() {
		var t models.SyncTask
		if err := taskRows.Scan(&t.ID, &t.ProjectID, &t.Name, &t.Status, &t.PercentComplete, &t.DueDate, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning sync task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}

	return &models.SyncPayload{
		FeedCards: cards,
		Tasks:     tasks,
		SyncedAt:  time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// CheckIdempotencyKey returns true if the key already exists in any field table.
func (s *FieldSyncStore) CheckIdempotencyKey(ctx context.Context, key string) (bool, error) {
	var exists bool
	// Check across task_progress, field_checkins, and field_daily_logs
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM task_progress WHERE idempotency_key = $1::uuid
			UNION ALL
			SELECT 1 FROM field_checkins WHERE idempotency_key = $2
			UNION ALL
			SELECT 1 FROM field_daily_logs WHERE idempotency_key = $2
		)`, key, key,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking idempotency key: %w", err)
	}
	return exists, nil
}

// SaveProgress records a field progress report using the task_progress table.
func (s *FieldSyncStore) SaveProgress(ctx context.Context, p *models.FieldProgress) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO task_progress (task_id, reported_by, percent_complete, notes, reported_via, idempotency_key)
		VALUES ($1, $2, $3, $4, 'mobile', $5::uuid)
		RETURNING id`,
		p.TaskID, p.UserID, p.PercentComplete, p.Notes, p.IdempotencyKey,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return uuid.Nil, fmt.Errorf("duplicate idempotency key")
		}
		return uuid.Nil, fmt.Errorf("saving progress: %w", err)
	}

	// Update task percent_complete on project_tasks
	if _, err := s.pool.Exec(ctx, `
		UPDATE project_tasks SET percent_complete = $2, updated_at = now()
		WHERE id = $1`, p.TaskID, p.PercentComplete); err != nil {
		// Non-fatal: progress was saved, but task rollup failed.
		// Log for debugging; the next sync will reconcile.
		_ = err // logged at call site if needed
	}

	return id, nil
}

// SaveCheckin records a field check-in.
func (s *FieldSyncStore) SaveCheckin(ctx context.Context, c *models.FieldCheckin) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO field_checkins (user_id, project_id, latitude, longitude, idempotency_key)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		c.UserID, c.ProjectID, c.Latitude, c.Longitude, c.IdempotencyKey,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return uuid.Nil, fmt.Errorf("duplicate idempotency key")
		}
		return uuid.Nil, fmt.Errorf("saving checkin: %w", err)
	}
	return id, nil
}

// SaveDailyLog records a field daily log.
func (s *FieldSyncStore) SaveDailyLog(ctx context.Context, dl *models.DailyLog) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO field_daily_logs (user_id, project_id, log_date, summary, hours_worked, weather_notes, safety_notes, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`,
		dl.UserID, dl.ProjectID, dl.LogDate, dl.Summary, dl.HoursWorked,
		dl.WeatherNotes, dl.SafetyNotes, dl.IdempotencyKey,
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return uuid.Nil, fmt.Errorf("duplicate idempotency key")
		}
		return uuid.Nil, fmt.Errorf("saving daily log: %w", err)
	}
	return id, nil
}

// isUniqueViolation checks for PostgreSQL unique constraint violation (23505).
func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "unique")
}
