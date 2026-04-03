-- Migration 007: Field sync tables for checkins and daily logs
-- Progress reports use existing task_progress table (migration 001)

-- ============================================================
-- 1. Field Checkins (GPS check-in records)
-- ============================================================
CREATE TABLE field_checkins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    latitude        DOUBLE PRECISION NOT NULL,
    longitude       DOUBLE PRECISION NOT NULL,
    idempotency_key TEXT UNIQUE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_field_checkins_user ON field_checkins(user_id);
CREATE INDEX idx_field_checkins_project ON field_checkins(project_id);

-- ============================================================
-- 2. Field Daily Logs
-- ============================================================
CREATE TABLE field_daily_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    log_date        DATE NOT NULL DEFAULT CURRENT_DATE,
    summary         TEXT NOT NULL,
    hours_worked    DOUBLE PRECISION NOT NULL DEFAULT 0,
    weather_notes   TEXT,
    safety_notes    TEXT,
    idempotency_key TEXT UNIQUE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_field_daily_logs_user ON field_daily_logs(user_id);
CREATE INDEX idx_field_daily_logs_project ON field_daily_logs(project_id);
CREATE INDEX idx_field_daily_logs_date ON field_daily_logs(log_date);
