package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// NotificationsStore manages the field_notification_dlq table.
//
// Writes: when a River notification job exhausts its retry budget, the
// worker records the discard here. Reads: ops endpoints (Sprint 6+)
// surface the DLQ for visibility.
type NotificationsStore struct{}

// NewNotificationsStore creates a new NotificationsStore.
func NewNotificationsStore() *NotificationsStore { return &NotificationsStore{} }

// InsertDLQEntryParams is the input for InsertDLQEntry. Payload must be
// valid JSON (e.g. `{"to":"+1...","body":"..."}` for SMS) — the column
// is JSONB. Empty payload is rejected by the NOT NULL constraint, so
// callers must always supply something even if it's `{}`.
type InsertDLQEntryParams struct {
	UserID           uuid.UUID
	NotificationType string
	Payload          json.RawMessage
	RetryCount       int
	LastError        string
}

// InsertDLQEntry records a discarded notification job. Returns the
// persisted row. The user_id has a FK to users(id), so callers must
// reference an existing user.
func (s *NotificationsStore) InsertDLQEntry(ctx context.Context, tx pgx.Tx, p InsertDLQEntryParams) (models.FieldNotificationDLQEntry, error) {
	if len(p.Payload) == 0 {
		// JSONB NOT NULL — at minimum store an empty object so we never
		// hit the constraint with a zero-value caller.
		p.Payload = json.RawMessage(`{}`)
	}

	var entry models.FieldNotificationDLQEntry
	var lastErr *string
	if p.LastError != "" {
		lastErr = &p.LastError
	}
	err := tx.QueryRow(ctx, `
		INSERT INTO field_notification_dlq (
			user_id, notification_type, payload, retry_count, last_error
		) VALUES ($1, $2, $3::jsonb, $4, $5)
		RETURNING id, user_id, notification_type, payload, retry_count, last_error, created_at`,
		p.UserID, p.NotificationType, p.Payload, p.RetryCount, lastErr,
	).Scan(
		&entry.ID, &entry.UserID, &entry.NotificationType, &entry.Payload,
		&entry.RetryCount, &entry.LastError, &entry.CreatedAt,
	)
	if err != nil {
		return models.FieldNotificationDLQEntry{}, fmt.Errorf("insert field_notification_dlq: %w", err)
	}
	return entry, nil
}

// ListDLQParams controls a DLQ listing. UserID filter is optional;
// Limit defaults to 100 (capped at 1000).
type ListDLQParams struct {
	UserID *uuid.UUID
	Limit  int
}

// ListDLQ returns DLQ entries newest first.
func (s *NotificationsStore) ListDLQ(ctx context.Context, tx pgx.Tx, p ListDLQParams) ([]models.FieldNotificationDLQEntry, error) {
	if p.Limit <= 0 {
		p.Limit = 100
	}
	if p.Limit > 1000 {
		p.Limit = 1000
	}

	var userArg any
	if p.UserID != nil {
		userArg = *p.UserID
	}
	rows, err := tx.Query(ctx, `
		SELECT id, user_id, notification_type, payload, retry_count, last_error, created_at
		FROM field_notification_dlq
		WHERE ($1::uuid IS NULL OR user_id = $1)
		ORDER BY created_at DESC
		LIMIT $2`,
		userArg, p.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query field_notification_dlq: %w", err)
	}
	defer rows.Close()

	out := make([]models.FieldNotificationDLQEntry, 0)
	for rows.Next() {
		var e models.FieldNotificationDLQEntry
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.NotificationType, &e.Payload,
			&e.RetryCount, &e.LastError, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan field_notification_dlq: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
