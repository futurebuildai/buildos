-- ============================================================
-- 007: A2A outbound dead-letter queue
-- ============================================================
-- River retries outbound A2A webhook dispatches with the same custom
-- backoff as the field notification queue (30s / 60s / 2m / 5m / 30m
-- / 1h). When the final attempt fails the worker writes here before
-- returning the error to River so the job lands as "discarded".
--
-- Operators replay manually (Phase D admin endpoint) or via a
-- one-shot script that picks rows ordered by created_at and re-emits.

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
