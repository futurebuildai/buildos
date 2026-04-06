package agents

import (
	"context"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/google/uuid"
)

// FeedWriter is the interface for creating feed cards from agents.
// Decoupled from *service.FeedService to allow testing and avoid import cycles.
type FeedWriter interface {
	WriteCard(ctx context.Context, card *models.FeedCard) error
}

// PgFeedWriter wraps service.FeedService to satisfy FeedWriter.
// The adapter pattern avoids coupling agents to the full service layer.
type PgFeedWriter struct {
	createCard func(ctx context.Context, card *models.FeedCard) (uuid.UUID, error)
}

// NewPgFeedWriter creates a FeedWriter from a CreateCard function.
// Usage: NewPgFeedWriter(feedSvc.CreateCard)
func NewPgFeedWriter(createCard func(ctx context.Context, card *models.FeedCard) (uuid.UUID, error)) *PgFeedWriter {
	return &PgFeedWriter{createCard: createCard}
}

// WriteCard creates a feed card, assigning the generated ID back to the card.
func (w *PgFeedWriter) WriteCard(ctx context.Context, card *models.FeedCard) error {
	id, err := w.createCard(ctx, card)
	if err != nil {
		return err
	}
	card.ID = id
	return nil
}
