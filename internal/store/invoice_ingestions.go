package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// InvoiceIngestionStore manages the invoice_ingestions outbox table
// (migration 014). The table isolates AI-ingestion idempotency from the
// unconstrained manual invoices path: its UNIQUE (project_id, idempotency_key)
// constraint is THE dedupe anchor for the Phase 2a ingestion pipeline.
type InvoiceIngestionStore struct{}

// NewInvoiceIngestionStore creates a new InvoiceIngestionStore.
func NewInvoiceIngestionStore() *InvoiceIngestionStore { return &InvoiceIngestionStore{} }

// InsertInvoiceIngestionParams is the input for InsertInvoiceIngestion. The
// InvoiceID and FeedCardID reference rows created earlier in the SAME tx.
type InsertInvoiceIngestionParams struct {
	ProjectID      uuid.UUID
	OrgID          uuid.UUID
	IdempotencyKey uuid.UUID
	InvoiceID      uuid.UUID // FK to the invoice created earlier in the SAME tx
	FeedCardID     uuid.UUID // FK to the review card created earlier in the SAME tx
	ExtractedBy    uuid.UUID // resolved users.id of the caller (sub)
}

// InsertInvoiceIngestion claims the (project_id, idempotency_key) dedupe slot by
// INSERT. On the UNIQUE violation (SQLSTATE 23505) it returns
// ErrIdempotencyConflict via the shared isUniqueViolation helper. It does NOT
// read anything back — the caller runs this as the LAST statement in the write
// tx, so a conflict rolls the whole tx back cleanly without poisoning it (no
// 25P02 "current transaction is aborted"). Mirrors the field.go semantics.
func (s *InvoiceIngestionStore) InsertInvoiceIngestion(ctx context.Context, tx pgx.Tx, p InsertInvoiceIngestionParams) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO invoice_ingestions (
			project_id, org_id, idempotency_key,
			invoice_id, feed_card_id, extracted_by
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ProjectID, p.OrgID, p.IdempotencyKey,
		p.InvoiceID, p.FeedCardID, p.ExtractedBy,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrIdempotencyConflict
		}
		return fmt.Errorf("insert invoice_ingestion: %w", err)
	}
	return nil
}
