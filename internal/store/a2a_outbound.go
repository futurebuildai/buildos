package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// A2AOutboundStore manages a2a_outbound_dlq.
type A2AOutboundStore struct{}

// NewA2AOutboundStore creates a new A2AOutboundStore.
func NewA2AOutboundStore() *A2AOutboundStore { return &A2AOutboundStore{} }

// InsertOutboundDLQParams is the input for InsertOutboundDLQ. Payload
// must be valid JSON; the column is JSONB. IdempotencyKey is optional
// (some outbound events have no client-supplied dedup key).
type InsertOutboundDLQParams struct {
	OrgID          uuid.UUID
	EventType      string
	TargetURL      string
	Payload        json.RawMessage
	TraceID        string
	IdempotencyKey *uuid.UUID
	RetryCount     int
	LastError      string
}

// InsertOutboundDLQ records a discarded outbound dispatch. Returns
// the persisted row id. The org_id has a FK to organizations(id),
// so callers must reference an existing org.
func (s *A2AOutboundStore) InsertOutboundDLQ(ctx context.Context, tx pgx.Tx, p InsertOutboundDLQParams) (uuid.UUID, error) {
	if len(p.Payload) == 0 {
		p.Payload = json.RawMessage(`{}`)
	}
	var lastErr *string
	if p.LastError != "" {
		lastErr = &p.LastError
	}
	var traceID *string
	if p.TraceID != "" {
		traceID = &p.TraceID
	}

	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO a2a_outbound_dlq (
			org_id, event_type, target_url, payload, trace_id,
			idempotency_key, retry_count, last_error
		) VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)
		RETURNING id`,
		p.OrgID, p.EventType, p.TargetURL, p.Payload, traceID,
		p.IdempotencyKey, p.RetryCount, lastErr,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert a2a_outbound_dlq: %w", err)
	}
	return id, nil
}
