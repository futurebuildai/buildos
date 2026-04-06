-- Migration 013: Notification outbox for field notification delivery
-- Stores outbound notifications awaiting delivery to field users.
-- The FieldNotificationRetryWorker processes this table with exponential backoff.

CREATE TABLE IF NOT EXISTS notification_outbox (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL,
    org_id          UUID NOT NULL,
    notification_type TEXT NOT NULL,       -- push, sms, email
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    payload         JSONB,                 -- Additional structured data for the notification
    status          TEXT NOT NULL DEFAULT 'pending',  -- pending, sent, failed
    retry_count     INTEGER NOT NULL DEFAULT 0,
    max_retries     INTEGER NOT NULL DEFAULT 6,
    last_error      TEXT,
    next_retry_at   TIMESTAMPTZ,
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notification_outbox_pending
    ON notification_outbox(status, next_retry_at)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_notification_outbox_user
    ON notification_outbox(user_id, created_at DESC);
