package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AuditStore manages audit_log inserts.
type AuditStore struct{}

// NewAuditStore creates a new AuditStore.
func NewAuditStore() *AuditStore { return &AuditStore{} }

// InsertAuditParams is the input for InsertAudit. UserSub may be empty
// (system actor); RequestID may be empty (out-of-request, e.g. cron).
// Before / After / Metadata may all be nil — they're nullable JSONB.
type InsertAuditParams struct {
	OrgID        uuid.UUID
	UserSub      string
	Action       string
	ResourceType string
	ResourceID   uuid.UUID
	Before       json.RawMessage
	After        json.RawMessage
	Metadata     json.RawMessage
	RequestID    string
}

// InsertAudit writes one audit row, normally inside the same tx as
// the mutation it describes — that way a rolled-back action also
// rolls back its audit row, and a successful action ALWAYS has its
// trail row.
func (s *AuditStore) InsertAudit(ctx context.Context, tx pgx.Tx, p InsertAuditParams) error {
	var (
		userSub   *string
		requestID *string
	)
	if p.UserSub != "" {
		userSub = &p.UserSub
	}
	if p.RequestID != "" {
		requestID = &p.RequestID
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_log (
			org_id, user_sub, action, resource_type, resource_id,
			before_state, after_state, metadata, request_id
		) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb, $9)`,
		p.OrgID, userSub, p.Action, p.ResourceType, p.ResourceID,
		nullableJSON(p.Before), nullableJSON(p.After), nullableJSON(p.Metadata), requestID,
	)
	if err != nil {
		return fmt.Errorf("insert audit_log: %w", err)
	}
	return nil
}

// nullableJSON returns nil for an empty RawMessage so the column lands
// as SQL NULL rather than the literal text "null". Two distinct
// states: "we have no info" vs "we have JSON null". The latter is
// vanishingly rare; treating empty as NULL matches the operator's
// expectation when reading the table.
func nullableJSON(m json.RawMessage) any {
	if len(m) == 0 {
		return nil
	}
	return []byte(m)
}
