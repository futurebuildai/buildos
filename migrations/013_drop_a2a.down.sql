-- Reverse of 013: recreate the A2A tables as they stood after
-- migrations 005 and 007. They come back empty — the application no
-- longer reads or writes them — but recreating keeps the schema
-- migration chain reversible for operators rolling back.

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

CREATE TABLE a2a_outbound_dlq (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    event_type      TEXT NOT NULL,
    target_url      TEXT NOT NULL,
    payload         JSONB NOT NULL,
    trace_id        TEXT,
    idempotency_key UUID,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_a2a_outbound_dlq_org_event ON a2a_outbound_dlq(org_id, event_type); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_a2a_outbound_dlq_created  ON a2a_outbound_dlq(created_at DESC); -- buildos:lock-ok: fresh table created in same migration
