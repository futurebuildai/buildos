-- migrations/014_invoice_ingestions.up.sql

-- Idempotency outbox for AI invoice ingestion. Isolates ingestion dedupe from
-- the invoices domain so the manual-entry path stays unconstrained.
CREATE TABLE invoice_ingestions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    idempotency_key UUID NOT NULL,
    invoice_id      UUID NOT NULL REFERENCES invoices(id),
    feed_card_id    UUID NOT NULL REFERENCES feed_cards(id),
    extracted_by    UUID NOT NULL,               -- users.id of the caller; no FK (cross-binary/author convention, see note)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, idempotency_key)         -- THE idempotency anchor
);

-- Distinguish AI-ingested invoices from manual entry in review.
ALTER TABLE invoices ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';  -- 'manual' | 'ai_ingest'
