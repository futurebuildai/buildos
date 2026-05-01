-- Migration 005: A2A inbound webhook log + idempotency dedup
--
-- Brain emits JWS-signed webhooks to BuildOS at /api/v1/a2a/webhook.
-- Each event carries an idempotency_key (UUID) generated on the Brain
-- side. We log every received event keyed on that UUID; the UNIQUE
-- constraint is the idempotency mechanism — `INSERT … ON CONFLICT DO
-- NOTHING` lets the receiver tell duplicates from new events without
-- a separate read.
--
-- payload is the raw event body for audit + replay. iss tracks the
-- emitter (currently always "fb-brain"; will diversify per ADR D4
-- cutover). trace_id correlates with Brain's outbound delivery log.

CREATE TABLE a2a_inbound_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key UUID NOT NULL UNIQUE,
    event_type      TEXT NOT NULL,
    trace_id        TEXT,
    iss             TEXT,
    payload         JSONB NOT NULL,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_a2a_inbound_event_type ON a2a_inbound_log(event_type); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_a2a_inbound_received_at ON a2a_inbound_log(received_at); -- buildos:lock-ok: fresh table created in same migration
