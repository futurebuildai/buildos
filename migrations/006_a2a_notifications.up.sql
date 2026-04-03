-- Sprint 5: A2A webhook log (idempotency dedup) + notification DLQ enhancements

-- a2a_webhook_log tracks received webhooks for idempotency deduplication
CREATE TABLE IF NOT EXISTS a2a_webhook_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}',
    trace_id        TEXT,
    issuer          TEXT NOT NULL DEFAULT 'fb-brain',
    status          TEXT NOT NULL DEFAULT 'processed',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_a2a_webhook_log_created ON a2a_webhook_log (created_at DESC);

-- Enhance field_notification_dlq (created in migration 003) with retry tracking columns
ALTER TABLE field_notification_dlq ADD COLUMN IF NOT EXISTS max_retries INT NOT NULL DEFAULT 6;
ALTER TABLE field_notification_dlq ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ;
ALTER TABLE field_notification_dlq ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE field_notification_dlq ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE field_notification_dlq ALTER COLUMN payload SET DEFAULT '{}';

CREATE INDEX IF NOT EXISTS idx_notification_dlq_status ON field_notification_dlq (status, next_retry_at);
