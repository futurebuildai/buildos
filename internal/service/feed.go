package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

var (
	ErrCardNotFound = errors.New("feed card not found or not active")
	ErrMissingTitle = errors.New("feed card title is required")
	ErrMissingOrgID = errors.New("org_id is required")
)

// FeedService provides business logic for feed card operations.
type FeedService struct {
	store *store.FeedStore
}

// NewFeedService creates a new FeedService.
func NewFeedService(s *store.FeedStore) *FeedService {
	return &FeedService{store: s}
}

// CreateCard creates a new feed card with validation.
func (svc *FeedService) CreateCard(ctx context.Context, card *models.FeedCard) (uuid.UUID, error) {
	if card.OrgID == uuid.Nil {
		return uuid.Nil, ErrMissingOrgID
	}
	if card.Title == "" {
		return uuid.Nil, ErrMissingTitle
	}
	if card.Status == "" {
		card.Status = models.FeedStatusActive
	}
	if card.Priority == "" {
		card.Priority = models.PriorityNormal
	}
	return svc.store.CreateFeedCard(ctx, card)
}

// ListCards returns feed cards for the given user/org with filters.
func (svc *FeedService) ListCards(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID, role string, filter models.FeedFilter) ([]models.FeedCard, int, error) {
	return svc.store.ListFeedCards(ctx, orgID, userID, role, filter)
}

// DismissCard marks a card as dismissed.
func (svc *FeedService) DismissCard(ctx context.Context, cardID uuid.UUID) error {
	err := svc.store.DismissFeedCard(ctx, cardID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCardNotFound, err)
	}
	return nil
}

// ActionCard marks a card as actioned.
func (svc *FeedService) ActionCard(ctx context.Context, cardID uuid.UUID) error {
	err := svc.store.ActionFeedCard(ctx, cardID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCardNotFound, err)
	}
	return nil
}
