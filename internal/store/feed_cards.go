package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// FeedCardsStore provides read+write access to the feed_cards table.
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

// ListFeedCardsParams controls a feed listing query.
//
// Targeting model: a card is visible to the caller if EITHER its
// target_user_id equals the caller's user id OR its target_role matches
// CallerRole. After the standalone pivot the JWT `sub` (CallerOIDCSubject)
// IS the users.id (auth mints sub = u.ID.String()), so target_user_id is
// compared directly against the parsed subject — the legacy oidc_subject
// column is NULL for native users and must not be used. Cross-org isolation
// is enforced by OrgID — never null. Status defaults to {"active"}; pass a
// non-empty list to override.
type ListFeedCardsParams struct {
	OrgID             uuid.UUID
	CallerOIDCSubject string
	CallerRole        string
	StatusFilter      []string // empty = ["active"]
	PriorityFilter    []string // empty = no filter
	Limit             int      // capped at 200, default 50
	Offset            int
}

// ListFeedCardsResult bundles the page rows with the pre-pagination
// total so the handler can render pagination metadata.
type ListFeedCardsResult struct {
	Cards []models.FeedCard
	Total int
}

// ListFeedCards returns a page of feed cards visible to the caller plus
// the total matching count (for pagination). Order: priority bucket
// first (critical → urgent → normal → low), then created_at DESC, so
// the most-pressing freshest cards land at the top.
func (s *FeedCardsStore) ListFeedCards(ctx context.Context, tx pgx.Tx, p ListFeedCardsParams) (ListFeedCardsResult, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	statuses := p.StatusFilter
	if len(statuses) == 0 {
		statuses = []string{"active"}
	}

	// The native JWT `sub` is the users.id; a malformed/empty subject
	// (never produced by a valid native token) resolves to uuid.Nil so it
	// matches no real target_user_id while role-targeted cards still show.
	callerID, perr := uuid.Parse(p.CallerOIDCSubject)
	if perr != nil {
		callerID = uuid.Nil
	}

	// Build the WHERE clause incrementally — one args slice shared by
	// the count query and the page query so we don't risk arg drift.
	args := []any{p.OrgID, callerID, p.CallerRole, statuses}
	where := `
		WHERE org_id = $1
		  AND (
		    target_user_id = $2
		    OR target_role = $3
		  )
		  AND status = ANY($4)`
	if len(p.PriorityFilter) > 0 {
		args = append(args, p.PriorityFilter)
		where += fmt.Sprintf(" AND priority = ANY($%d)", len(args))
	}

	var total int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM feed_cards`+where, args...).Scan(&total); err != nil {
		return ListFeedCardsResult{}, fmt.Errorf("count feed_cards: %w", err)
	}
	if total == 0 {
		return ListFeedCardsResult{Cards: []models.FeedCard{}, Total: 0}, nil
	}

	// Priority ordering: assign integer ranks via a CASE so SQL ordering
	// matches the documented severity hierarchy regardless of locale.
	limitPos := len(args) + 1
	offsetPos := len(args) + 2
	pageArgs := append(append([]any{}, args...), p.Limit, p.Offset)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT id, org_id, project_id, card_type, title, body, priority,
		       target_user_id, target_role, actions,
		       status, actioned_at, expires_at, created_at
		FROM feed_cards%s
		ORDER BY
		  CASE priority
		    WHEN 'critical' THEN 1
		    WHEN 'urgent'   THEN 2
		    WHEN 'normal'   THEN 3
		    WHEN 'low'      THEN 4
		    ELSE 5
		  END ASC,
		  created_at DESC
		LIMIT $%d OFFSET $%d`, where, limitPos, offsetPos),
		pageArgs...,
	)
	if err != nil {
		return ListFeedCardsResult{}, fmt.Errorf("query feed_cards: %w", err)
	}
	defer rows.Close()

	out := make([]models.FeedCard, 0, p.Limit)
	for rows.Next() {
		var c models.FeedCard
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body, &c.Priority,
			&c.TargetUserID, &c.TargetRole, &c.Actions,
			&c.Status, &c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
		); err != nil {
			return ListFeedCardsResult{}, fmt.Errorf("scan feed_card: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return ListFeedCardsResult{}, fmt.Errorf("iterate feed_cards: %w", err)
	}
	return ListFeedCardsResult{Cards: out, Total: total}, nil
}

// ErrFeedCardNotFound is returned by single-row reads/updates when no
// row matches (id + orgID). The service layer maps this to a 404.
var ErrFeedCardNotFound = errors.New("feed_cards: not found")

// GetFeedCard returns a single card scoped to the caller's org.
// Returns ErrFeedCardNotFound if the row doesn't exist or belongs to a
// different org (we don't distinguish — never leak existence across
// tenants).
func (s *FeedCardsStore) GetFeedCard(ctx context.Context, tx pgx.Tx, cardID, orgID uuid.UUID) (models.FeedCard, error) {
	var c models.FeedCard
	err := tx.QueryRow(ctx, `
		SELECT id, org_id, project_id, card_type, title, body, priority,
		       target_user_id, target_role, actions,
		       status, actioned_at, expires_at, created_at
		FROM feed_cards
		WHERE id = $1 AND org_id = $2`,
		cardID, orgID,
	).Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body, &c.Priority,
		&c.TargetUserID, &c.TargetRole, &c.Actions,
		&c.Status, &c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.FeedCard{}, ErrFeedCardNotFound
		}
		return models.FeedCard{}, fmt.Errorf("get feed_card: %w", err)
	}
	return c, nil
}

// DismissFeedCard transitions a card to status='dismissed'. Idempotent:
// dismissing an already-dismissed card returns the row unchanged. Any
// other terminal status (actioned/expired) is left alone — the API
// contract only allows dismiss from active.
func (s *FeedCardsStore) DismissFeedCard(ctx context.Context, tx pgx.Tx, cardID, orgID uuid.UUID) (models.FeedCard, error) {
	var c models.FeedCard
	err := tx.QueryRow(ctx, `
		UPDATE feed_cards
		SET status = 'dismissed'
		WHERE id = $1 AND org_id = $2 AND status IN ('active', 'dismissed')
		RETURNING id, org_id, project_id, card_type, title, body, priority,
		          target_user_id, target_role, actions,
		          status, actioned_at, expires_at, created_at`,
		cardID, orgID,
	).Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body, &c.Priority,
		&c.TargetUserID, &c.TargetRole, &c.Actions,
		&c.Status, &c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.FeedCard{}, ErrFeedCardNotFound
		}
		return models.FeedCard{}, fmt.Errorf("dismiss feed_card: %w", err)
	}
	return c, nil
}

// ActionFeedCard transitions a card to status='actioned' and stamps
// actioned_at. Only allowed from status='active' — dismissed/actioned/
// expired rows return ErrFeedCardNotFound (treated as "no actionable
// row exists for this id").
func (s *FeedCardsStore) ActionFeedCard(ctx context.Context, tx pgx.Tx, cardID, orgID uuid.UUID) (models.FeedCard, error) {
	var c models.FeedCard
	err := tx.QueryRow(ctx, `
		UPDATE feed_cards
		SET status = 'actioned', actioned_at = now()
		WHERE id = $1 AND org_id = $2 AND status = 'active'
		RETURNING id, org_id, project_id, card_type, title, body, priority,
		          target_user_id, target_role, actions,
		          status, actioned_at, expires_at, created_at`,
		cardID, orgID,
	).Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.CardType, &c.Title, &c.Body, &c.Priority,
		&c.TargetUserID, &c.TargetRole, &c.Actions,
		&c.Status, &c.ActionedAt, &c.ExpiresAt, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.FeedCard{}, ErrFeedCardNotFound
		}
		return models.FeedCard{}, fmt.Errorf("action feed_card: %w", err)
	}
	return c, nil
}
