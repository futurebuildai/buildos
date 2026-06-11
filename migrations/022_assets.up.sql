-- 022_assets.up.sql
-- Chunk A (DAILY_REPORTS_CLIENT_UPDATES): the object-storage substrate. An
-- org/project-scoped registry of blobs held in the per-fork S3-compatible
-- (Cloudflare R2) object store. The bytes live in R2; this table tracks the
-- opaque object key, content-type, size, lifecycle status, and uploader so a
-- daily-log / client-update photo id resolves to a real, confirmed blob.
--
-- Lifecycle: 'pending' (presigned PUT issued, not yet confirmed) -> 'ready'
-- (client PUT confirmed) -> 'failed'. Daily-log linking (Chunk B) requires
-- 'ready'. Composite Currency Pattern N/A (no monetary columns; byte_size is a
-- size, not money — it does not match the linter's monetary regex).
--
-- PII: storage_key is INTERNAL (opaque; contains only org/project UUIDs, which
-- are Internal). content_type is Public. No emails/names/GPS columns. The image
-- CONTENT may carry GPS EXIF (Restricted) — neutralized by the EXIF-strip-on-
-- serve path (D-4), not by this row. No new pii.FieldClass entries needed.
--
-- uploaded_by is TEXT (the caller's JWT sub), matching the feedback/audit
-- user_sub convention (no users FK so rows survive user deletion and dev-header
-- subjects don't break inserts).

CREATE TABLE assets (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- nullable: org-level (non-project) assets are allowed.
    project_id    UUID REFERENCES projects(id) ON DELETE SET NULL,
    -- Opaque object key in the bucket. Convention:
    -- org/<org>/project/<proj>/<uuid>.<ext>
    storage_key   TEXT NOT NULL,
    content_type  TEXT NOT NULL,
    -- Size in bytes (NOT money — linter rule 1 monetary regex does not match).
    size_bytes    BIGINT NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'ready', 'failed')),
    -- Caller's JWT sub (TEXT, no users FK — see header).
    uploaded_by   TEXT NOT NULL DEFAULT '',
    -- sha256 of the bytes, set on confirm (dedup/integrity; optional v1).
    checksum_sha256 TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at  TIMESTAMPTZ
);

-- Plain CREATE INDEX (NOT CONCURRENTLY): the migrate runner wraps every
-- migration in a tx (cmd/migrate/main.go) and Postgres forbids CONCURRENTLY
-- inside a tx. Both indexes land on the assets table created above in this same
-- migration, so they build on an empty table — the brief ACCESS EXCLUSIVE lock
-- is a no-op (no rows, no concurrent writers). Matches migration 015's precedent.
--
-- Project gallery read path: list an org's project assets newest-first.
CREATE INDEX idx_assets_org_project ON assets (org_id, project_id, created_at DESC); -- buildos:lock-ok: index on table created in same migration (empty); brief lock is a no-op
-- One row per object key (guards a duplicate confirm / key collision).
CREATE UNIQUE INDEX idx_assets_storage_key ON assets (storage_key); -- buildos:lock-ok: index on table created in same migration (empty); brief lock is a no-op
