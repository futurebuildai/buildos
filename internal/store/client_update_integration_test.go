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

// TestClientUpdateStore_RoundTrip exercises the lifecycle against a real
// Postgres: Create (draft) -> UpdateDraft -> MarkSent, plus GetByID /
// ListByProject and cross-org isolation.
func TestClientUpdateStore_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewClientUpdateStore()
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "CU Org")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "CU Project")

	period := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	ai := "the original ai draft"

	var created models.ClientUpdate
	withTx(t, pool, func(tx pgx.Tx) error {
		c, err := s.Create(ctx, tx, CreateClientUpdateParams{
			OrgID:       orgID,
			ProjectID:   projID,
			PeriodStart: period,
			PeriodEnd:   period,
			AIDraft:     &ai,
			EditedBody:  ai,
			Subject:     "AI subject",
			CreatedBy:   userID,
		})
		if err != nil {
			return err
		}
		created = c
		return nil
	})
	if created.Status != models.ClientUpdateStatusDraft {
		t.Fatalf("status after create = %q, want draft", created.Status)
	}
	if created.PhotoAssetIDs == nil {
		t.Errorf("photo_asset_ids should be non-nil empty slice, got nil")
	}

	// UpdateDraft.
	photo := uuid.New()
	var edited models.ClientUpdate
	withTx(t, pool, func(tx pgx.Tx) error {
		c, err := s.UpdateDraft(ctx, tx, UpdateDraftParams{
			OrgID:         orgID,
			ID:            created.ID,
			Subject:       "edited subject",
			EditedBody:    "operator-edited body",
			PhotoAssetIDs: []uuid.UUID{photo},
		})
		edited = c
		return err
	})
	if edited.Subject != "edited subject" || edited.EditedBody != "operator-edited body" {
		t.Errorf("edit not applied: %+v", edited)
	}
	if len(edited.PhotoAssetIDs) != 1 || edited.PhotoAssetIDs[0] != photo {
		t.Errorf("photo ids not stored: %v", edited.PhotoAssetIDs)
	}

	// MarkSent snapshots the recipient.
	var sent models.ClientUpdate
	withTx(t, pool, func(tx pgx.Tx) error {
		c, err := s.MarkSent(ctx, tx, MarkSentParams{
			OrgID:          orgID,
			ID:             created.ID,
			RecipientEmail: "home@owner.example",
			SentBy:         userID,
		})
		sent = c
		return err
	})
	if sent.Status != models.ClientUpdateStatusSent {
		t.Errorf("status = %q, want sent", sent.Status)
	}
	if sent.RecipientEmail == nil || *sent.RecipientEmail != "home@owner.example" {
		t.Errorf("recipient snapshot = %v", sent.RecipientEmail)
	}
	if sent.SentAt == nil || sent.SentBy == nil {
		t.Errorf("sent_at/sent_by not set: %+v", sent)
	}

	// A sent row is not re-sendable (status <> 'sent' guard → ErrNotFound).
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := s.MarkSent(ctx, tx, MarkSentParams{OrgID: orgID, ID: created.ID, RecipientEmail: "x@y.z", SentBy: userID})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("re-send a sent row = %v, want ErrNotFound (status guard)", err)
		}
		return nil
	})

	// A sent row is not editable (status='draft' guard → ErrNotFound).
	withTx(t, pool, func(tx pgx.Tx) error {
		_, err := s.UpdateDraft(ctx, tx, UpdateDraftParams{OrgID: orgID, ID: created.ID, Subject: "x", EditedBody: "y"})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("edit a sent row = %v, want ErrNotFound (status guard)", err)
		}
		return nil
	})

	// ListByProject returns the row.
	withTx(t, pool, func(tx pgx.Tx) error {
		rows, err := s.ListByProject(ctx, tx, orgID, projID)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].ID != created.ID {
			t.Errorf("list = %+v", rows)
		}
		return nil
	})
}

// TestClientUpdateStore_CrossOrg404 proves org-scoping: another org's id resolves
// to ErrNotFound (no existence leak).
func TestClientUpdateStore_CrossOrg404(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewClientUpdateStore()
	ctx := context.Background()

	orgA, orgB := uuid.New(), uuid.New()
	userA := uuid.New()
	projA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")
	testdb.SeedUser(t, pool, userA, orgA)
	testdb.SeedProject(t, pool, projA, orgA, "PA")

	period := time.Now().UTC()
	var id uuid.UUID
	withTx(t, pool, func(tx pgx.Tx) error {
		c, err := s.Create(ctx, tx, CreateClientUpdateParams{
			OrgID: orgA, ProjectID: projA, PeriodStart: period, PeriodEnd: period,
			EditedBody: "b", Subject: "s", CreatedBy: userA,
		})
		id = c.ID
		return err
	})

	withTx(t, pool, func(tx pgx.Tx) error {
		if _, err := s.GetByID(ctx, tx, orgB, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("cross-org GetByID = %v, want ErrNotFound", err)
		}
		return nil
	})
}

// TestClientUpdateStore_StatusCheck proves the DB CHECK rejects an invalid
// status (defense beyond the service guards).
func TestClientUpdateStore_StatusCheck(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Org")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "P")

	_, err := pool.Exec(ctx, `
		INSERT INTO client_updates (org_id, project_id, period_start, period_end, status, created_by)
		VALUES ($1,$2,now()::date,now()::date,'bogus',$3)`, orgID, projID, userID)
	if err == nil {
		t.Fatal("expected CHECK violation on invalid status, got nil")
	}
}
