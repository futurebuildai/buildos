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

func TestAuditStore_InsertAudit_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewAuditStore()
	ctx := context.Background()

	orgID := uuid.New()
	resourceID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")

	wantBefore := json.RawMessage(`{"status":"active","percent_complete":0}`)
	wantAfter := json.RawMessage(`{"status":"actioned","percent_complete":50}`)
	wantMeta := json.RawMessage(`{"action_type":"approve_quote"}`)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return s.InsertAudit(ctx, tx, InsertAuditParams{
			OrgID:        orgID,
			UserSub:      "user-sub-123",
			Action:       "feed.card.actioned",
			ResourceType: "feed_card",
			ResourceID:   resourceID,
			Before:       wantBefore,
			After:        wantAfter,
			Metadata:     wantMeta,
			RequestID:    "req-abc-456",
		})
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Read back and verify all fields landed correctly.
	var (
		gotUserSub *string
		gotAction  string
		gotResType string
		gotResID   uuid.UUID
		gotBefore  []byte
		gotAfter   []byte
		gotMeta    []byte
		gotReqID   *string
	)
	err = pool.QueryRow(ctx, `
		SELECT user_sub, action, resource_type, resource_id,
		       before_state, after_state, metadata, request_id
		FROM audit_log
		WHERE org_id = $1`, orgID).Scan(
		&gotUserSub, &gotAction, &gotResType, &gotResID,
		&gotBefore, &gotAfter, &gotMeta, &gotReqID,
	)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if gotUserSub == nil || *gotUserSub != "user-sub-123" {
		t.Errorf("user_sub = %v, want user-sub-123", gotUserSub)
	}
	if gotAction != "feed.card.actioned" {
		t.Errorf("action = %q", gotAction)
	}
	if gotResType != "feed_card" {
		t.Errorf("resource_type = %q", gotResType)
	}
	if gotResID != resourceID {
		t.Errorf("resource_id = %s, want %s", gotResID, resourceID)
	}
	if gotReqID == nil || *gotReqID != "req-abc-456" {
		t.Errorf("request_id = %v", gotReqID)
	}
	// JSONB round-trip — Postgres may reorder keys/strip whitespace.
	for label, gotBytes := range map[string][]byte{
		"before":   gotBefore,
		"after":    gotAfter,
		"metadata": gotMeta,
	} {
		if len(gotBytes) == 0 {
			t.Errorf("%s landed empty", label)
		}
		// Confirm valid JSON by re-parsing.
		var any map[string]any
		if err := json.Unmarshal(gotBytes, &any); err != nil {
			t.Errorf("%s not valid JSON: %v", label, err)
		}
	}
}

func TestAuditStore_InsertAudit_NullableFields(t *testing.T) {
	// user_sub, request_id, before, after, metadata all support NULL.
	// Confirms the empty-input → SQL NULL conversion lives end-to-end.
	pool := testdb.NewPool(t)
	s := NewAuditStore()
	ctx := context.Background()

	orgID := uuid.New()
	resourceID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return s.InsertAudit(ctx, tx, InsertAuditParams{
			OrgID:        orgID,
			Action:       "system.cron.ran",
			ResourceType: "system",
			ResourceID:   resourceID,
			// UserSub, Before, After, Metadata, RequestID all empty.
		})
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	var (
		userSub, reqID          *string
		before, after, metadata []byte
	)
	err = pool.QueryRow(ctx, `
		SELECT user_sub, request_id, before_state, after_state, metadata
		FROM audit_log WHERE org_id = $1`, orgID).Scan(
		&userSub, &reqID, &before, &after, &metadata)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if userSub != nil {
		t.Errorf("user_sub should be NULL when empty; got %v", *userSub)
	}
	if reqID != nil {
		t.Errorf("request_id should be NULL when empty; got %v", *reqID)
	}
	if before != nil || after != nil || metadata != nil {
		t.Errorf("JSONB columns should be NULL when empty; got before=%v after=%v meta=%v",
			before, after, metadata)
	}
}
