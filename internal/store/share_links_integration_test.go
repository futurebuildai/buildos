//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestShareLinkStore_RoundTrip exercises the lifecycle against a real Postgres:
// Create -> GetActiveByHash -> Revoke -> (revoked no longer active) plus GetByID,
// ListByClientUpdate, TouchView, and cross-org isolation.
func TestShareLinkStore_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	cuStore := NewClientUpdateStore()
	s := NewShareLinkStore()
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Share Org")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "Share Project")

	// A share link references a client_update; create + mark it sent.
	period := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	var cuID uuid.UUID
	withTx(t, pool, func(tx pgx.Tx) error {
		c, err := cuStore.Create(ctx, tx, CreateClientUpdateParams{
			OrgID: orgID, ProjectID: projID, PeriodStart: period, PeriodEnd: period,
			EditedBody: "body", Subject: "s", CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		cuID = c.ID
		return nil
	})
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := cuStore.MarkSent(ctx, tx, MarkSentParams{OrgID: orgID, ID: cuID, RecipientEmail: "h@o.example", SentBy: userID})
		return err
	})

	hash := "abc123hash"
	expires := time.Now().Add(30 * 24 * time.Hour)
	var created models.ShareLink
	withTx(t, pool, func(tx pgx.Tx) error {
		l, err := s.Create(ctx, tx, CreateShareLinkParams{
			OrgID: orgID, ClientUpdateID: cuID, TokenHash: hash, ExpiresAt: expires, CreatedBy: userID,
		})
		created = l
		return err
	})
	if created.ViewCount != 0 || created.RevokedAt != nil {
		t.Fatalf("fresh link: view_count=%d revoked=%v", created.ViewCount, created.RevokedAt)
	}

	// GetActiveByHash resolves it.
	withTx(t, pool, func(tx pgx.Tx) error {
		got, err := s.GetActiveByHash(ctx, tx, hash, time.Now())
		if err != nil {
			t.Fatalf("GetActiveByHash = %v", err)
		}
		if got.ID != created.ID || got.OrgID != orgID || got.ClientUpdateID != cuID {
			t.Fatalf("resolved wrong row: %+v", got)
		}
		return nil
	})

	// A bogus hash → ErrNotFound (uniform).
	withTx(t, pool, func(tx pgx.Tx) error {
		if _, err := s.GetActiveByHash(ctx, tx, "nope", time.Now()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("bogus hash = %v, want ErrNotFound", err)
		}
		return nil
	})

	// An expired link → ErrNotFound even with the right hash.
	withTx(t, pool, func(tx pgx.Tx) error {
		future := expires.Add(time.Hour)
		if _, err := s.GetActiveByHash(ctx, tx, hash, future); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expired link active = %v, want ErrNotFound", err)
		}
		return nil
	})

	// TouchView bumps the counter.
	withTx(t, pool, func(tx pgx.Tx) error {
		return s.TouchView(ctx, tx, created.ID, time.Now())
	})
	withTx(t, pool, func(tx pgx.Tx) error {
		got, err := s.GetByID(ctx, tx, orgID, created.ID)
		if err != nil {
			return err
		}
		if got.ViewCount != 1 || got.LastViewedAt == nil {
			t.Fatalf("after TouchView: view_count=%d last_viewed=%v", got.ViewCount, got.LastViewedAt)
		}
		return nil
	})

	// ListByClientUpdate returns it.
	withTx(t, pool, func(tx pgx.Tx) error {
		rows, err := s.ListByClientUpdate(ctx, tx, orgID, cuID)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].ID != created.ID {
			t.Fatalf("list = %+v", rows)
		}
		return nil
	})

	// Revoke flips revoked_at; afterward GetActiveByHash no longer resolves it.
	withTx(t, pool, func(tx pgx.Tx) error {
		l, err := s.Revoke(ctx, tx, orgID, created.ID, time.Now())
		if err != nil {
			return err
		}
		if l.RevokedAt == nil {
			t.Fatalf("revoke did not set revoked_at")
		}
		return nil
	})
	withTx(t, pool, func(tx pgx.Tx) error {
		if _, err := s.GetActiveByHash(ctx, tx, hash, time.Now()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoked link active = %v, want ErrNotFound", err)
		}
		return nil
	})

	// Re-revoke is a no-op (revoked_at IS NULL guard) → ErrNotFound.
	withTx(t, pool, func(tx pgx.Tx) error {
		if _, err := s.Revoke(ctx, tx, orgID, created.ID, time.Now()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("re-revoke = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestShareLinkStore_CrossOrg404 proves org-scoping on the operator surface:
// another org's id resolves to ErrNotFound on GetByID and Revoke.
func TestShareLinkStore_CrossOrg404(t *testing.T) {
	pool := testdb.NewPool(t)
	cuStore := NewClientUpdateStore()
	s := NewShareLinkStore()
	ctx := context.Background()

	orgA, orgB := uuid.New(), uuid.New()
	userA := uuid.New()
	projA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")
	testdb.SeedUser(t, pool, userA, orgA)
	testdb.SeedProject(t, pool, projA, orgA, "PA")

	period := time.Now().UTC()
	var cuID uuid.UUID
	withTx(t, pool, func(tx pgx.Tx) error {
		c, err := cuStore.Create(ctx, tx, CreateClientUpdateParams{
			OrgID: orgA, ProjectID: projA, PeriodStart: period, PeriodEnd: period,
			EditedBody: "b", Subject: "s", CreatedBy: userA,
		})
		cuID = c.ID
		return err
	})

	var linkID uuid.UUID
	withTx(t, pool, func(tx pgx.Tx) error {
		l, err := s.Create(ctx, tx, CreateShareLinkParams{
			OrgID: orgA, ClientUpdateID: cuID, TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour), CreatedBy: userA,
		})
		linkID = l.ID
		return err
	})

	withTx(t, pool, func(tx pgx.Tx) error {
		if _, err := s.GetByID(ctx, tx, orgB, linkID); !errors.Is(err, ErrNotFound) {
			t.Errorf("cross-org GetByID = %v, want ErrNotFound", err)
		}
		if _, err := s.Revoke(ctx, tx, orgB, linkID, time.Now()); !errors.Is(err, ErrNotFound) {
			t.Errorf("cross-org Revoke = %v, want ErrNotFound", err)
		}
		return nil
	})
}
