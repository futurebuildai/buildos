package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// A2AStore manages the a2a_inbound_log table — the idempotency dedup
// surface for inbound webhooks from The Brain.
type A2AStore struct{}

// NewA2AStore creates a new A2AStore.
func NewA2AStore() *A2AStore { return &A2AStore{} }

// InsertInboundLogParams is the input for InsertInboundLog. Payload is
// the raw event body (post-JWS-verification) preserved for audit + replay.
type InsertInboundLogParams struct {
	IdempotencyKey uuid.UUID
	EventType      string
	TraceID        string
	Issuer         string
	Payload        json.RawMessage
}

// InsertInboundLog records an inbound webhook in the dedup log.
// Returns alreadyProcessed=true (no error) when the idempotency_key was
// already present — this is the "duplicate event" signal. Returns
// alreadyProcessed=false when the row was newly inserted.
//
// Implemented via INSERT ... ON CONFLICT DO NOTHING + RETURNING id —
// pgx returns ErrNoRows for the conflict path. Single statement, no
// race between dedup-check and insert.
func (s *A2AStore) InsertInboundLog(ctx context.Context, tx pgx.Tx, p InsertInboundLogParams) (alreadyProcessed bool, err error) {
	if len(p.Payload) == 0 {
		// Avoid storing SQL NULL — the column is NOT NULL.
		p.Payload = json.RawMessage(`null`)
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO a2a_inbound_log (idempotency_key, event_type, trace_id, iss, payload)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`,
		p.IdempotencyKey, p.EventType, p.TraceID, p.Issuer, p.Payload,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert a2a_inbound_log: %w", err)
	}
	return false, nil
}
