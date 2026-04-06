package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// FeedStore provides raw SQL access to feed_cards and communication_logs.
type FeedStore struct {
	pool *pgxpool.Pool
}

// NewFeedStore creates a new FeedStore.
func NewFeedStore(pool *pgxpool.Pool) *FeedStore {
	return &FeedStore{pool: pool}
}

// CreateFeedCard inserts a new feed card.
func (s *FeedStore) CreateFeedCard(ctx context.Context, card *models.FeedCard) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO feed_cards (
			org_id, project_id, card_type, title, body,
			priority, target_user_id, target_role, actions, status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`,
		card.OrgID, card.ProjectID, card.CardType, card.Title, card.Body,
		card.Priority, card.TargetUserID, card.TargetRole, card.Actions,
		card.Status, card.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating feed card: %w", err)
	}
	return id, nil
}

// ListFeedCards returns feed cards for a user in an org, with optional filters.
func (s *FeedStore) ListFeedCards(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, role string, filter models.FeedFilter) ([]models.FeedCard, int, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Build query with optional filters
	query := `
		SELECT id, org_id, project_id, card_type, title, body,
			priority, target_user_id, target_role, actions, status,
			actioned_at, expires_at, created_at
		FROM feed_cards
		WHERE org_id = $1
			AND status != 'expired'
			AND (target_user_id = $2 OR target_role = $3 OR (target_user_id IS NULL AND target_role IS NULL))`

	args := []any{orgID, userID, role}
	argIdx := 4

	if filter.Priority != "" {
		query += fmt.Sprintf(" AND priority = $%d", argIdx)
		args = append(args, filter.Priority)
		argIdx++
	}
	if filter.Status != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, filter.Status)
		argIdx++
	}

	// Count query
	countQuery := `SELECT COUNT(*) FROM feed_cards WHERE org_id = $1
		AND status != 'expired'
		AND (target_user_id = $2 OR target_role = $3 OR (target_user_id IS NULL AND target_role IS NULL))`
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, orgID, userID, role).Scan(&total); err != nil {
		slog.Warn("failed to query feed card count", "error", err, "org_id", orgID)
	}

	query += fmt.Sprintf(" ORDER BY CASE priority WHEN 'critical' THEN 0 WHEN 'urgent' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END, created_at DESC LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing feed cards: %w", err)
	}
	defer rows.Close()

	var cards []models.FeedCard
	for rows.Next() {
		var c models.FeedCard
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body,
			&c.Priority, &c.TargetUserID, &c.TargetRole, &c.Actions, &c.Status,
			&c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning feed card: %w", err)
		}
		cards = append(cards, c)
	}
	return cards, total, rows.Err()
}

// GetFeedCard returns a single feed card by ID.
func (s *FeedStore) GetFeedCard(ctx context.Context, cardID uuid.UUID) (*models.FeedCard, error) {
	var c models.FeedCard
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, project_id, card_type, title, body,
			priority, target_user_id, target_role, actions, status,
			actioned_at, expires_at, created_at
		FROM feed_cards WHERE id = $1`, cardID,
	).Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body,
		&c.Priority, &c.TargetUserID, &c.TargetRole, &c.Actions, &c.Status,
		&c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting feed card: %w", err)
	}
	return &c, nil
}

// DismissFeedCard marks a card as dismissed.
func (s *FeedStore) DismissFeedCard(ctx context.Context, cardID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE feed_cards SET status = 'dismissed'
		WHERE id = $1 AND status = 'active'`, cardID)
	if err != nil {
		return fmt.Errorf("dismissing feed card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("card not found or already dismissed")
	}
	return nil
}

// ActionFeedCard marks a card as actioned and records the timestamp.
func (s *FeedStore) ActionFeedCard(ctx context.Context, cardID uuid.UUID) error {
	now := time.Now().UTC()
	tag, err := s.pool.Exec(ctx, `
		UPDATE feed_cards SET status = 'actioned', actioned_at = $2
		WHERE id = $1 AND status = 'active'`, cardID, now)
	if err != nil {
		return fmt.Errorf("actioning feed card: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("card not found or not active")
	}
	return nil
}
