package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Sentinel errors specific to FeedService. Generic ErrInvalidInput is
// shared with the rest of the service layer (defined in budget.go).
var (
	// ErrFeedCardNotFound is returned when a card lookup fails. The
	// handler maps this to 404. Mirrors the store-level sentinel but
	// kept independent so handlers don't need to import store.
	ErrFeedCardNotFound = errors.New("feed: card not found")
)

// MaxFeedActionPayloadBytes caps the JSON body the API will accept on
// /feed/{id}/action. The actual side-effect dispatch is wired in a
// later sprint; for now we record the payload on the actioned-card
// audit log only. The cap is generous (8 KiB) but bounded so a
// runaway client can't write multi-MB blobs to logs.
const MaxFeedActionPayloadBytes = 8 * 1024

// FeedListOptions narrows a feed listing query at the service layer.
// CallerOrgID is the JWT-validated org; CallerOIDCSubject is the JWT
// `sub` claim used to resolve the user row inside the SQL query.
type FeedListOptions struct {
	CallerOrgID       uuid.UUID
	CallerOIDCSubject string
	CallerRole        string
	StatusFilter      []string
	PriorityFilter    []string
	Page              int
	PerPage           int
}

// FeedListResult mirrors the store result so the handler doesn't have
// to import the store package.
type FeedListResult struct {
	Cards []models.FeedCard
	Total int
}

// FeedActionInput captures the body of POST /feed/{id}/action. The
// payload is opaque JSON the action dispatcher will parse later — we
// store it on the audit log unchanged.
type FeedActionInput struct {
	ActionType string
	Payload    json.RawMessage
}

// FeedService handles list/dismiss/action operations against feed
// cards. Writes (insert) flow through A2AService; this service is the
// read+transition surface.
type FeedService struct {
	pool   *pgxpool.Pool
	store  *store.FeedCardsStore
	logger *slog.Logger
}

// NewFeedService creates a service bound to a pool + store + logger.
func NewFeedService(pool *pgxpool.Pool, cards *store.FeedCardsStore, logger *slog.Logger) *FeedService {
	return &FeedService{pool: pool, store: cards, logger: logger}
}

// ListFeed returns a page of cards visible to the caller. RBAC
// (target_user_id resolved by oidc_subject OR target_role match) is
// enforced inside the SQL — never rely on an in-memory filter.
func (s *FeedService) ListFeed(ctx context.Context, opts FeedListOptions) (FeedListResult, error) {
	if opts.CallerOrgID == uuid.Nil {
		return FeedListResult{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if opts.CallerOIDCSubject == "" {
		return FeedListResult{}, fmt.Errorf("%w: caller oidc subject is required", ErrInvalidInput)
	}
	if opts.CallerRole == "" {
		return FeedListResult{}, fmt.Errorf("%w: caller role is required", ErrInvalidInput)
	}
	for _, st := range opts.StatusFilter {
		if !isValidFeedStatus(st) {
			return FeedListResult{}, fmt.Errorf("%w: unknown status %q", ErrInvalidInput, st)
		}
	}
	for _, pr := range opts.PriorityFilter {
		if !models.IsValidFeedPriority(pr) {
			return FeedListResult{}, fmt.Errorf("%w: unknown priority %q", ErrInvalidInput, pr)
		}
	}

	page, perPage := opts.Page, opts.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	if perPage > 200 {
		perPage = 200
	}

	var res store.ListFeedCardsResult
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		out, err := s.store.ListFeedCards(ctx, tx, store.ListFeedCardsParams{
			OrgID:             opts.CallerOrgID,
			CallerOIDCSubject: opts.CallerOIDCSubject,
			CallerRole:        opts.CallerRole,
			StatusFilter:      opts.StatusFilter,
			PriorityFilter:    opts.PriorityFilter,
			Limit:             perPage,
			Offset:            (page - 1) * perPage,
		})
		if err != nil {
			return err
		}
		res = out
		return nil
	})
	if err != nil {
		return FeedListResult{}, fmt.Errorf("list feed: %w", err)
	}
	return FeedListResult{Cards: res.Cards, Total: res.Total}, nil
}

// DismissCard transitions a card to status='dismissed'. Cross-org reads
// are blocked at the SQL level (id + org_id composite filter); a
// missing row from a sibling org surfaces as ErrFeedCardNotFound,
// matching the contract's "no leakage across tenants" rule.
func (s *FeedService) DismissCard(ctx context.Context, callerOrgID, cardID uuid.UUID) (models.FeedCard, error) {
	if callerOrgID == uuid.Nil || cardID == uuid.Nil {
		return models.FeedCard{}, fmt.Errorf("%w: org_id and card_id are required", ErrInvalidInput)
	}
	var out models.FeedCard
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, err := s.store.DismissFeedCard(ctx, tx, cardID, callerOrgID)
		if err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrFeedCardNotFound) {
			return models.FeedCard{}, ErrFeedCardNotFound
		}
		return models.FeedCard{}, fmt.Errorf("dismiss card: %w", err)
	}
	return out, nil
}

// ActionCard transitions a card to status='actioned' and records the
// action_type/payload on the audit log. Side-effect dispatch (e.g.,
// "approve_quote" → outbound A2A call to The Brain) lands in a later
// sprint — that work is intentionally NOT in this service.
func (s *FeedService) ActionCard(ctx context.Context, callerOrgID, cardID uuid.UUID, in FeedActionInput) (models.FeedCard, error) {
	if callerOrgID == uuid.Nil || cardID == uuid.Nil {
		return models.FeedCard{}, fmt.Errorf("%w: org_id and card_id are required", ErrInvalidInput)
	}
	if in.ActionType == "" {
		return models.FeedCard{}, fmt.Errorf("%w: action_type is required", ErrInvalidInput)
	}
	if len(in.Payload) > MaxFeedActionPayloadBytes {
		return models.FeedCard{}, fmt.Errorf("%w: action payload exceeds %d bytes", ErrInvalidInput, MaxFeedActionPayloadBytes)
	}

	var out models.FeedCard
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, err := s.store.ActionFeedCard(ctx, tx, cardID, callerOrgID)
		if err != nil {
			return err
		}
		out = c
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrFeedCardNotFound) {
			return models.FeedCard{}, ErrFeedCardNotFound
		}
		return models.FeedCard{}, fmt.Errorf("action card: %w", err)
	}

	// Audit log — once SubLiaisonAgent / outbound A2A dispatch lands
	// this becomes the dispatch trigger. For now it's a structured log
	// record for ops to grep against.
	s.logger.InfoContext(ctx, "feed.card.actioned",
		"card_id", out.ID,
		"org_id", out.OrgID,
		"card_type", out.CardType,
		"action_type", in.ActionType,
		"payload_bytes", len(in.Payload),
	)
	return out, nil
}

// isValidFeedStatus reports whether s is one of the four allowed
// status values from the migration 003 CHECK constraint.
func isValidFeedStatus(s string) bool {
	switch s {
	case "active", "dismissed", "actioned", "expired":
		return true
	default:
		return false
	}
}
