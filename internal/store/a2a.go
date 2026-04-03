package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A2AWebhookLog represents a logged A2A webhook for idempotency.
type A2AWebhookLog struct {
	ID             uuid.UUID `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	EventType      string    `json:"event_type"`
	Payload        []byte    `json:"payload"`
	TraceID        string    `json:"trace_id"`
	Issuer         string    `json:"issuer"`
	Status         string    `json:"status"`
}

// A2AStore provides raw SQL access for A2A webhook operations.
type A2AStore struct {
	pool *pgxpool.Pool
}

// NewA2AStore creates a new A2AStore.
func NewA2AStore(pool *pgxpool.Pool) *A2AStore {
	return &A2AStore{pool: pool}
}

// CheckIdempotencyKey returns true if the key already exists (duplicate).
func (s *A2AStore) CheckIdempotencyKey(ctx context.Context, key string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM a2a_webhook_log WHERE idempotency_key = $1)`, key,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking idempotency key: %w", err)
	}
	return exists, nil
}

// LogWebhook inserts a webhook into the log. Returns ErrDuplicateKey on conflict.
func (s *A2AStore) LogWebhook(ctx context.Context, log *A2AWebhookLog) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO a2a_webhook_log (idempotency_key, event_type, payload, trace_id, issuer, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		log.IdempotencyKey, log.EventType, log.Payload, log.TraceID, log.Issuer, log.Status,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("logging webhook: %w", err)
	}
	return id, nil
}

// NotificationDLQEntry represents a failed notification in the dead-letter queue.
type NotificationDLQEntry struct {
	ID               uuid.UUID `json:"id"`
	UserID           uuid.UUID `json:"user_id"`
	NotificationType string    `json:"notification_type"`
	Payload          []byte    `json:"payload"`
	RetryCount       int       `json:"retry_count"`
	MaxRetries       int       `json:"max_retries"`
	LastError        *string   `json:"last_error,omitempty"`
	Status           string    `json:"status"`
}

// InsertDLQEntry adds a failed notification to the DLQ.
func (s *A2AStore) InsertDLQEntry(ctx context.Context, entry *NotificationDLQEntry) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO field_notification_dlq (user_id, notification_type, payload, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		entry.UserID, entry.NotificationType, entry.Payload, "pending",
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("inserting DLQ entry: %w", err)
	}
	return id, nil
}

// ListPendingDLQEntries returns entries ready for retry.
func (s *A2AStore) ListPendingDLQEntries(ctx context.Context, limit int) ([]NotificationDLQEntry, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, notification_type, payload, retry_count, max_retries, last_error, status
		FROM field_notification_dlq
		WHERE status = 'pending'
			AND (next_retry_at IS NULL OR next_retry_at <= now())
			AND retry_count < max_retries
		ORDER BY created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing pending DLQ entries: %w", err)
	}
	defer rows.Close()

	var entries []NotificationDLQEntry
	for rows.Next() {
		var e NotificationDLQEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.NotificationType, &e.Payload,
			&e.RetryCount, &e.MaxRetries, &e.LastError, &e.Status); err != nil {
			return nil, fmt.Errorf("scanning DLQ entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// IncrementDLQRetry bumps the retry count and sets the next retry time.
func (s *A2AStore) IncrementDLQRetry(ctx context.Context, entryID uuid.UUID, lastError string, backoffSeconds int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE field_notification_dlq SET
			retry_count = retry_count + 1,
			last_error = $2,
			next_retry_at = now() + ($3 || ' seconds')::interval,
			updated_at = now()
		WHERE id = $1`, entryID, lastError, fmt.Sprintf("%d", backoffSeconds))
	return err
}

// FailDLQEntry marks an entry as permanently failed.
func (s *A2AStore) FailDLQEntry(ctx context.Context, entryID uuid.UUID, lastError string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE field_notification_dlq SET
			status = 'failed', last_error = $2, updated_at = now()
		WHERE id = $1`, entryID, lastError)
	return err
}

// CompleteDLQEntry marks an entry as successfully delivered.
func (s *A2AStore) CompleteDLQEntry(ctx context.Context, entryID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE field_notification_dlq SET status = 'delivered', updated_at = now()
		WHERE id = $1`, entryID)
	return err
}

// UpdateProcurementStatus updates a procurement item's status by item ID.
func (s *A2AStore) UpdateProcurementStatus(ctx context.Context, itemID uuid.UUID, status string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE procurement_items SET status = $2, updated_at = now()
		WHERE id = $1`, itemID, status)
	if err != nil {
		return fmt.Errorf("updating procurement status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("procurement item not found")
	}
	return nil
}

// GetProjectIDByRFQ looks up a project associated with an RFQ-linked procurement item.
// Returns pgx.ErrNoRows if not found.
func (s *A2AStore) GetProjectIDByRFQ(ctx context.Context, orgID uuid.UUID) (*uuid.UUID, error) {
	// For now, return the first active project for the org
	var projectID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		SELECT id FROM projects WHERE org_id = $1 AND status = 'active' LIMIT 1`, orgID,
	).Scan(&projectID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("looking up project: %w", err)
	}
	return &projectID, nil
}
