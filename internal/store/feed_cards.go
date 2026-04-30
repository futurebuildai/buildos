package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// FeedCardsStore provides write access to the feed_cards table. Reads
// land alongside the rest of the feed API in Sprint 5.
type FeedCardsStore struct{}

// NewFeedCardsStore creates a new FeedCardsStore.
func NewFeedCardsStore() *FeedCardsStore { return &FeedCardsStore{} }

// CreateFeedCardParams is the input for inserting a feed card. Either
// TargetUserID or TargetRole must be set; service layer enforces this.
// Actions is pre-marshalled JSON ready to land in the JSONB column.
type CreateFeedCardParams struct {
	OrgID        uuid.UUID
	ProjectID    *uuid.UUID
	CardType     string
	Title        string
	Body         string
	Priority     string
	TargetUserID *uuid.UUID
	TargetRole   *string
	Actions      json.RawMessage
}

// CreateFeedCard inserts a feed card with status='active'. The card_type
// + priority strings are persisted as-is — schema CHECKs are advisory
// (free-text columns); service layer validates with IsValidFeedPriority.
func (s *FeedCardsStore) CreateFeedCard(ctx context.Context, tx pgx.Tx, p CreateFeedCardParams) (models.FeedCard, error) {
	// Empty actions becomes JSON `null`; normalize to a JSONB null literal
	// so the column never holds an SQL-level NULL when callers passed an
	// explicit empty list. Cleaner downstream: jsonb_array_elements works.
	actions := p.Actions
	if len(actions) == 0 {
		actions = json.RawMessage(`null`)
	}

	var card models.FeedCard
	err := tx.QueryRow(ctx, `
		INSERT INTO feed_cards (
			org_id, project_id, card_type, title, body, priority,
			target_user_id, target_role, actions
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb)
		RETURNING id, org_id, project_id, card_type, title, body, priority,
		          target_user_id, target_role, actions,
		          status, actioned_at, expires_at, created_at`,
		p.OrgID, p.ProjectID, p.CardType, p.Title, p.Body, p.Priority,
		p.TargetUserID, p.TargetRole, actions,
	).Scan(
		&card.ID, &card.OrgID, &card.ProjectID, &card.CardType,
		&card.Title, &card.Body, &card.Priority,
		&card.TargetUserID, &card.TargetRole, &card.Actions,
		&card.Status, &card.ActionedAt, &card.ExpiresAt, &card.CreatedAt,
	)
	if err != nil {
		return models.FeedCard{}, fmt.Errorf("insert feed_card: %w", err)
	}
	return card, nil
}
