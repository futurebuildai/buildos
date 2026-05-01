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

func TestA2AStore_InsertInboundLog_FirstAndDuplicate(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewA2AStore()
	ctx := context.Background()

	key := uuid.New()
	params := InsertInboundLogParams{
		IdempotencyKey: key,
		EventType:      "review_material_quote",
		TraceID:        "trace-123",
		Issuer:         "fb-brain",
		Payload:        json.RawMessage(`{"vendor":"Acme"}`),
	}

	// First insert: alreadyProcessed=false.
	var firstAlready bool
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var e error
		firstAlready, e = s.InsertInboundLog(ctx, tx, params)
		return e
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if firstAlready {
		t.Error("first insert should return alreadyProcessed=false")
	}

	// Second insert with same idempotency_key: alreadyProcessed=true.
	var secondAlready bool
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var e error
		secondAlready, e = s.InsertInboundLog(ctx, tx, params)
		return e
	})
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if !secondAlready {
		t.Error("second insert should return alreadyProcessed=true (duplicate idempotency_key)")
	}

	// Verify only ONE row exists.
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM a2a_inbound_log WHERE idempotency_key = $1`, key).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row, got %d", count)
	}
}

func TestA2AStore_InsertInboundLog_EmptyPayloadOK(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewA2AStore()
	ctx := context.Background()

	// Empty payload is normalized to JSONB null inside InsertInboundLog;
	// the column is NOT NULL so without normalization this would fail.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.InsertInboundLog(ctx, tx, InsertInboundLogParams{
			IdempotencyKey: uuid.New(),
			EventType:      "create_feed_card",
			Payload:        nil, // empty
		})
		return err
	})
	if err != nil {
		t.Errorf("insert with empty payload should succeed (normalized to JSON null): %v", err)
	}
}

func TestFeedCardsStore_CreateFeedCard_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedCardsStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	role := "owner"
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		card, err := s.CreateFeedCard(ctx, tx, CreateFeedCardParams{
			OrgID:      orgID,
			CardType:   "procurement.material_quote",
			Title:      "Review material quote from Acme",
			Body:       "1500.00 USD",
			Priority:   "urgent",
			TargetRole: &role,
		})
		if err != nil {
			return err
		}
		if card.OrgID != orgID {
			t.Errorf("org_id = %s, want %s", card.OrgID, orgID)
		}
		if card.Status != "active" {
			t.Errorf("status = %q, want active", card.Status)
		}
		if card.TargetRole == nil || *card.TargetRole != "owner" {
			t.Errorf("target_role = %v, want owner", card.TargetRole)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
