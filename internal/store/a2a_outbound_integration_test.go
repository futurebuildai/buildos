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

func TestA2AOutboundStore_InsertOutboundDLQ_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewA2AOutboundStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")

	idem := uuid.New()
	wantPayload := json.RawMessage(`{"card_id":"abc","action_type":"approve_quote"}`)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		id, err := s.InsertOutboundDLQ(ctx, tx, InsertOutboundDLQParams{
			OrgID:          orgID,
			EventType:      "buildos.feed_card_actioned",
			TargetURL:      "https://brain.example/api/v1/a2a/webhook",
			Payload:        wantPayload,
			TraceID:        "trace-1",
			IdempotencyKey: &idem,
			RetryCount:     6,
			LastError:      "Brain: HTTP 502",
		})
		if err != nil {
			return err
		}
		if id == uuid.Nil {
			t.Error("returned id is nil")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Verify the row landed with the expected fields.
	var (
		gotEvent     string
		gotURL       string
		gotPayload   json.RawMessage
		gotTrace     *string
		gotIdem      *uuid.UUID
		gotRetry     int
		gotLastError *string
	)
	err = pool.QueryRow(ctx, `
		SELECT event_type, target_url, payload, trace_id, idempotency_key, retry_count, last_error
		FROM a2a_outbound_dlq WHERE org_id = $1`, orgID).
		Scan(&gotEvent, &gotURL, &gotPayload, &gotTrace, &gotIdem, &gotRetry, &gotLastError)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotEvent != "buildos.feed_card_actioned" {
		t.Errorf("event_type = %q", gotEvent)
	}
	if gotIdem == nil || *gotIdem != idem {
		t.Errorf("idempotency_key = %v, want %s", gotIdem, idem)
	}
	if gotRetry != 6 {
		t.Errorf("retry_count = %d", gotRetry)
	}
	if gotLastError == nil || *gotLastError != "Brain: HTTP 502" {
		t.Errorf("last_error = %v", gotLastError)
	}
	if gotTrace == nil || *gotTrace != "trace-1" {
		t.Errorf("trace_id = %v", gotTrace)
	}
}

func TestA2AOutboundStore_InsertOutboundDLQ_EmptyPayloadDefaultsToObject(t *testing.T) {
	// JSONB NOT NULL constraint must never be hit when callers pass
	// an empty payload — the store substitutes "{}" so the row lands
	// regardless.
	pool := testdb.NewPool(t)
	s := NewA2AOutboundStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.InsertOutboundDLQ(ctx, tx, InsertOutboundDLQParams{
			OrgID:     orgID,
			EventType: "x",
			TargetURL: "https://x.example",
			Payload:   nil, // empty
		})
		return err
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var got json.RawMessage
	err = pool.QueryRow(ctx, `SELECT payload FROM a2a_outbound_dlq WHERE org_id = $1`, orgID).Scan(&got)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "{}" {
		t.Errorf("payload = %s, want {}", got)
	}
}
