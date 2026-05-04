package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/pii"
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
//
// Before/After/Metadata JSONB payloads are scrubbed of Restricted-class
// PII (emails, phones, names, GPS coords, OIDC subjects, IPs) before
// persistence — the audit log is read by compliance reviewers and
// support staff and must not leak PII even within the fork. Caller-
// known business-sensitive values (vendor names, *_cents amounts —
// Confidential class) are preserved so the audit trail retains its
// investigative value.
//
// The scrub is idempotent: calling InsertAudit with already-scrubbed
// JSON is a no-op on those fields.
//
// Caveat: pii.ScrubJSON returns the input unchanged on JSON parse
// failure (per package design — better to ship a maybe-leaky audit
// than drop the row). Malformed-JSON payloads will reach the row
// unscrubbed; callers MUST construct valid JSON before calling.
func (s *AuditStore) InsertAudit(ctx context.Context, tx pgx.Tx, p InsertAuditParams) error {
	p = scrubAuditPayloads(p)
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

// scrubAuditPayloads returns a copy of p with Before/After/Metadata
// scrubbed of Restricted-class PII via pii.ScrubJSON. Other fields
// (OrgID, UserSub, Action, etc.) are passed through unchanged — the
// scrub targets dynamic JSONB blobs only. UserSub itself is a JWT
// subject (Restricted in the catalog) but is intentionally retained
// in the audit table column for accountability tracing; the JSONB
// scrub catches any incidental copy of it inside Before/After/Metadata.
func scrubAuditPayloads(p InsertAuditParams) InsertAuditParams {
	p.Before = pii.ScrubJSON(p.Before, pii.Restricted)
	p.After = pii.ScrubJSON(p.After, pii.Restricted)
	p.Metadata = pii.ScrubJSON(p.Metadata, pii.Restricted)
	return p
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
