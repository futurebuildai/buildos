package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

var (
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrMissingIdempotencyKey   = errors.New("idempotency key is required")
)

// FieldSyncService provides business logic for field sync operations.
type FieldSyncService struct {
	store *store.FieldSyncStore
}

// NewFieldSyncService creates a new FieldSyncService.
func NewFieldSyncService(s *store.FieldSyncStore) *FieldSyncService {
	return &FieldSyncService{store: s}
}

// Sync returns a payload with feed cards and tasks updated since the given timestamp.
func (svc *FieldSyncService) Sync(ctx context.Context, orgID uuid.UUID, userID uuid.UUID, role string, since time.Time) (*models.SyncPayload, error) {
	return svc.store.GetSyncPayload(ctx, orgID, userID, role, since)
}

// ReportProgress records a field progress update with idempotency.
func (svc *FieldSyncService) ReportProgress(ctx context.Context, p *models.FieldProgress) (uuid.UUID, error) {
	if p.IdempotencyKey == "" {
		return uuid.Nil, ErrMissingIdempotencyKey
	}

	dup, err := svc.store.CheckIdempotencyKey(ctx, p.IdempotencyKey)
	if err != nil {
		return uuid.Nil, err
	}
	if dup {
		return uuid.Nil, ErrDuplicateIdempotencyKey
	}

	return svc.store.SaveProgress(ctx, p)
}

// Checkin records a field worker check-in with idempotency.
func (svc *FieldSyncService) Checkin(ctx context.Context, c *models.FieldCheckin) (uuid.UUID, error) {
	if c.IdempotencyKey == "" {
		return uuid.Nil, ErrMissingIdempotencyKey
	}

	dup, err := svc.store.CheckIdempotencyKey(ctx, c.IdempotencyKey)
	if err != nil {
		return uuid.Nil, err
	}
	if dup {
		return uuid.Nil, ErrDuplicateIdempotencyKey
	}

	return svc.store.SaveCheckin(ctx, c)
}

// DailyLog records a field daily log with idempotency.
func (svc *FieldSyncService) DailyLog(ctx context.Context, dl *models.DailyLog) (uuid.UUID, error) {
	if dl.IdempotencyKey == "" {
		return uuid.Nil, ErrMissingIdempotencyKey
	}

	dup, err := svc.store.CheckIdempotencyKey(ctx, dl.IdempotencyKey)
	if err != nil {
		return uuid.Nil, err
	}
	if dup {
		return uuid.Nil, ErrDuplicateIdempotencyKey
	}

	return svc.store.SaveDailyLog(ctx, dl)
}
