-- Migration 001: Core tables (organizations, users, projects, tasks, dependencies, progress)
-- FutureBuild OS — Sprint 0 Walking Skeleton

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================
-- 1. Organizations (multi-tenant root)
-- ============================================================
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    plan_tier   TEXT NOT NULL DEFAULT 'free',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 2. Users (identity from FB-Brain OIDC)
-- ============================================================
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_subject    TEXT NOT NULL UNIQUE,  -- FB-Brain OIDC sub claim
    org_id          UUID NOT NULL REFERENCES organizations(id),
    email           TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'field_worker',  -- owner, admin, superintendent, field_worker
    locale          TEXT NOT NULL DEFAULT 'en-US',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_oidc_subject ON users(oidc_subject); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_users_org_id ON users(org_id); -- buildos:lock-ok: fresh table created in same migration

-- ============================================================
-- 3. Projects
-- ============================================================
CREATE TABLE projects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    name                TEXT NOT NULL,
    address             TEXT,
    permit_issued_date  DATE,
    project_start_date  DATE,
    status              TEXT NOT NULL DEFAULT 'active',  -- active, completed, archived
    gsf                 INTEGER,  -- Gross Square Footage (1500-6000)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_projects_org_id ON projects(org_id); -- buildos:lock-ok: fresh table created in same migration

-- ============================================================
-- 4. Project Tasks (WBS-based)
-- ============================================================
CREATE TABLE project_tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    wbs_code        TEXT NOT NULL,         -- e.g., "9.2"
    name            TEXT NOT NULL,
    duration_days   INTEGER NOT NULL,
    early_start     TIMESTAMPTZ,           -- Computed by CPM ForwardPass
    early_finish    TIMESTAMPTZ,
    late_start      TIMESTAMPTZ,           -- Computed by CPM BackwardPass
    late_finish     TIMESTAMPTZ,
    total_float     INTEGER,               -- In working days
    is_critical     BOOLEAN DEFAULT false,
    status          TEXT NOT NULL DEFAULT 'pending',
    percent_complete INTEGER NOT NULL DEFAULT 0,
    assigned_crew   UUID[],                -- Array of user IDs
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, wbs_code)
);
CREATE INDEX idx_tasks_project ON project_tasks(project_id); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_tasks_status ON project_tasks(status); -- buildos:lock-ok: fresh table created in same migration

-- ============================================================
-- 5. Task Dependencies (4 types: FS, FF, SS, SF)
-- ============================================================
CREATE TABLE task_dependencies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    predecessor_id  UUID NOT NULL REFERENCES project_tasks(id),
    successor_id    UUID NOT NULL REFERENCES project_tasks(id),
    dependency_type TEXT NOT NULL DEFAULT 'FS',  -- FS, FF, SS, SF
    lag_days        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(predecessor_id, successor_id)
);

-- ============================================================
-- 6. Task Progress (field reports)
-- ============================================================
CREATE TABLE task_progress (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         UUID NOT NULL REFERENCES project_tasks(id),
    reported_by     UUID NOT NULL REFERENCES users(id),
    percent_complete INTEGER NOT NULL,
    notes           TEXT,
    photo_asset_id  UUID,
    gps_lat         DOUBLE PRECISION,
    gps_lng         DOUBLE PRECISION,
    reported_via    TEXT NOT NULL DEFAULT 'web',  -- web, mobile
    idempotency_key UUID UNIQUE,  -- For offline dedup
    reported_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_progress_task ON task_progress(task_id); -- buildos:lock-ok: fresh table created in same migration
CREATE INDEX idx_progress_idempotency ON task_progress(idempotency_key); -- buildos:lock-ok: fresh table created in same migration
