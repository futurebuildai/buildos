-- Migration 003: Feed cards, communication logs, fleet, HR, field sync tables

-- ============================================================
-- 1. Feed Cards
-- ============================================================
CREATE TABLE feed_cards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    project_id      UUID REFERENCES projects(id),
    card_type       TEXT NOT NULL,     -- weather_alert, procurement, sub_confirmation, progress, etc.
    title           TEXT NOT NULL,
    body            TEXT,
    priority        TEXT NOT NULL DEFAULT 'normal',  -- critical, urgent, normal, low
    target_user_id  UUID REFERENCES users(id),
    target_role     TEXT,              -- Alternative to target_user: role-based targeting
    actions         JSONB,             -- [{label, action_type, payload}]
    status          TEXT NOT NULL DEFAULT 'active',  -- active, dismissed, actioned, expired
    actioned_at     TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_feed_org_status ON feed_cards(org_id, status); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_feed_target ON feed_cards(target_user_id); -- buildos:lock-ok: fresh table created in same migration

-- ============================================================
-- 2. Communication Logs (Sub Liaison)
-- ============================================================
CREATE TABLE communication_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    task_id         UUID NOT NULL REFERENCES project_tasks(id),
    contact_name    TEXT NOT NULL,
    contact_phone   TEXT,
    message_type    TEXT NOT NULL,      -- sms_confirmation, sms_reminder
    message_body    TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',  -- PENDING, SENT, DELIVERED, FAILED
    response_body   TEXT,
    response_parsed TEXT,               -- confirmed, delayed, unrecognized
    idempotency_key UUID UNIQUE,
    sent_at         TIMESTAMPTZ,
    response_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 3. Fleet Assets
-- ============================================================
CREATE TABLE fleet_assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    name            TEXT NOT NULL,
    asset_type      TEXT NOT NULL,      -- excavator, compactor, grader, crane
    serial_number   TEXT,
    status          TEXT NOT NULL DEFAULT 'available',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 4. Equipment Allocations (with exclusion constraint)
-- ============================================================
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE equipment_allocations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id        UUID NOT NULL REFERENCES fleet_assets(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    start_date      DATE NOT NULL,
    end_date        DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    EXCLUDE USING gist (asset_id WITH =, daterange(start_date, end_date) WITH &&)
);

-- ============================================================
-- 5. Employees
-- ============================================================
CREATE TABLE employees (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    user_id         UUID REFERENCES users(id),
    first_name      TEXT NOT NULL,
    last_name       TEXT NOT NULL,
    role            TEXT NOT NULL,
    phone           TEXT,
    hire_date       DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 6. Certifications
-- ============================================================
CREATE TABLE certifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id     UUID NOT NULL REFERENCES employees(id),
    cert_type       TEXT NOT NULL,      -- contractor_license, insurance, osha_10, etc.
    cert_number     TEXT,
    issued_date     DATE,
    expiry_date     DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_certs_expiry ON certifications(expiry_date); -- buildos:lock-ok: fresh table created in same migration

-- ============================================================
-- 7. Field Notification Dead Letter Queue
-- ============================================================
CREATE TABLE field_notification_dlq (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    notification_type TEXT NOT NULL,
    payload         JSONB NOT NULL,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 8. Weather Forecast Cache
-- ============================================================
CREATE TABLE weather_cache (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lat             DOUBLE PRECISION NOT NULL,
    lng             DOUBLE PRECISION NOT NULL,
    forecast_data   JSONB NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_weather_cache_location ON weather_cache(lat, lng); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_weather_cache_expiry ON weather_cache(expires_at); -- buildos:lock-ok: fresh table created in same migration
