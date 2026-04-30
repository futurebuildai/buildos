-- ============================================================
-- 006: Field sync tables — crew check-ins + daily logs
-- ============================================================
-- Mobile clients post to /api/v1/field/{checkin,daily-log} from the
-- field with patchy connectivity. Each row carries a client-generated
-- idempotency_key (UUID v7) so the offline outbox can replay without
-- duplicating rows.

-- ============================================================
-- crew_checkins — crew arrival at a project site
-- ============================================================
CREATE TABLE crew_checkins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    reported_by     UUID NOT NULL REFERENCES users(id),
    crew_members    JSONB NOT NULL DEFAULT '[]'::jsonb,  -- [{worker_id, gps_lat, gps_lng}, ...]
    gps_lat         DOUBLE PRECISION,
    gps_lng         DOUBLE PRECISION,
    notes           TEXT,
    idempotency_key UUID NOT NULL UNIQUE,
    reported_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_crew_checkins_project ON crew_checkins(project_id);
CREATE INDEX idx_crew_checkins_reported_at ON crew_checkins(reported_at DESC);

-- ============================================================
-- daily_logs — end-of-day report from the site
-- ============================================================
CREATE TABLE daily_logs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    project_id          UUID NOT NULL REFERENCES projects(id),
    reported_by         UUID NOT NULL REFERENCES users(id),
    log_date            DATE NOT NULL,
    weather_conditions  TEXT,
    work_summary        TEXT NOT NULL,
    safety_incidents    TEXT,
    photo_asset_ids     UUID[],
    idempotency_key     UUID NOT NULL UNIQUE,
    reported_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_daily_logs_project_date ON daily_logs(project_id, log_date DESC);
