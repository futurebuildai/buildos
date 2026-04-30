//go:build integration

package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

func TestNotificationsStore_InsertDLQEntry_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewNotificationsStore()
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedUser(t, pool, userID, orgID)

	wantPayload := json.RawMessage(`{"to":"+15551234","body":"Re: tomorrow's site visit"}`)
	wantErr := "Twilio: queue full"

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.InsertDLQEntry(ctx, tx, InsertDLQEntryParams{
			UserID:           userID,
			NotificationType: "sms",
			Payload:          wantPayload,
			RetryCount:       6,
			LastError:        wantErr,
		})
		if err != nil {
			return err
		}
		if got.UserID != userID {
			t.Errorf("user_id round-trip: got %s, want %s", got.UserID, userID)
		}
		if got.NotificationType != "sms" {
			t.Errorf("notification_type = %q", got.NotificationType)
		}
		if got.RetryCount != 6 {
			t.Errorf("retry_count = %d", got.RetryCount)
		}
		if got.LastError == nil || *got.LastError != wantErr {
			t.Errorf("last_error = %v, want %q", got.LastError, wantErr)
		}
		if got.CreatedAt.IsZero() {
			t.Error("created_at should default to now()")
		}
		// JSONB round-trip — Postgres may reformat keys/whitespace, so
		// compare as parsed values rather than raw bytes.
		var gotMap, wantMap map[string]any
		if err := json.Unmarshal(got.Payload, &gotMap); err != nil {
			t.Errorf("payload unmarshal: %v", err)
		}
		if err := json.Unmarshal(wantPayload, &wantMap); err != nil {
			t.Fatalf("seed payload unmarshal: %v", err)
		}
		if gotMap["to"] != wantMap["to"] || gotMap["body"] != wantMap["body"] {
			t.Errorf("payload round-trip mismatch: got %v, want %v", gotMap, wantMap)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestNotificationsStore_InsertDLQEntry_NilLastError(t *testing.T) {
	// LastError == "" should land as SQL NULL (the column is nullable).
	pool := testdb.NewPool(t)
	s := NewNotificationsStore()
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedUser(t, pool, userID, orgID)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.InsertDLQEntry(ctx, tx, InsertDLQEntryParams{
			UserID:           userID,
			NotificationType: "push",
			Payload:          json.RawMessage(`{}`),
			RetryCount:       6,
			// LastError intentionally empty.
		})
		if err != nil {
			return err
		}
		if got.LastError != nil {
			t.Errorf("last_error should be NULL when LastError == \"\"; got %q", *got.LastError)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func TestNotificationsStore_ListDLQ_FiltersAndOrdering(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewNotificationsStore()
	ctx := context.Background()

	orgID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedUser(t, pool, userA, orgID)
	testdb.SeedUser(t, pool, userB, orgID)

	// Insert 3 entries: 2 for userA, 1 for userB.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, uid := range []uuid.UUID{userA, userB, userA} {
			_, err := s.InsertDLQEntry(ctx, tx, InsertDLQEntryParams{
				UserID:           uid,
				NotificationType: "sms",
				Payload:          json.RawMessage(`{"body":"msg"}`),
				RetryCount:       6,
				LastError:        "transient",
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("no filter returns all", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListDLQ(ctx, tx, ListDLQParams{})
			if err != nil {
				return err
			}
			if len(rows) != 3 {
				t.Errorf("got %d rows, want 3", len(rows))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("user_id filter narrows", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListDLQ(ctx, tx, ListDLQParams{UserID: &userA})
			if err != nil {
				return err
			}
			if len(rows) != 2 {
				t.Errorf("user A: got %d rows, want 2", len(rows))
			}
			for _, r := range rows {
				if r.UserID != userA {
					t.Errorf("filter leak: row userID = %s, want %s", r.UserID, userA)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ordering is newest first", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListDLQ(ctx, tx, ListDLQParams{})
			if err != nil {
				return err
			}
			for i := 1; i < len(rows); i++ {
				if rows[i-1].CreatedAt.Before(rows[i].CreatedAt) {
					t.Errorf("rows not in DESC created_at order at index %d", i)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
