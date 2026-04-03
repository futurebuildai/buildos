-- Sprint 5: A2A webhook log (idempotency dedup) + notification DLQ

-- a2a_webhook_log tracks received webhooks for idempotency deduplication
CREATE TABLE a2a_webhook_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key TEXT NOT NULL UNIQUE,
    event_type      TEXT NOT NULL,
    payload         JSONB NOT NULL DEFAULT '{}',
    trace_id        TEXT,
    issuer          TEXT NOT NULL DEFAULT 'fb-brain',
    status          TEXT NOT NULL DEFAULT 'processed',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_a2a_webhook_log_created ON a2a_webhook_log (created_at DESC);

-- field_notification_dlq: dead-letter queue for failed push notifications
CREATE TABLE field_notification_dlq (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users(id),
    notification_type TEXT NOT NULL,
    payload           JSONB NOT NULL DEFAULT '{}',
    retry_count       INT NOT NULL DEFAULT 0,
    max_retries       INT NOT NULL DEFAULT 6,
    last_error        TEXT,
    next_retry_at     TIMESTAMPTZ,
    status            TEXT NOT NULL DEFAULT 'pending',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_dlq_status ON field_notification_dlq (status, next_retry_at);
