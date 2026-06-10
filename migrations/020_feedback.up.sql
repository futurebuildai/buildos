-- 020_feedback.up.sql
-- Phase 0b: native in-app feedback. Operators (any authenticated role —
-- field workers included) file bug/idea/friction reports from the web
-- console widget; admins triage them in-app; the buildos-operations
-- command center harvests them via GET /api/v1/admin/feedback and files
-- GitHub issues (the feedback → plan → spec → approve → execute loop).
--
-- user_sub is the caller's JWT subject as TEXT (matches the updated_by
-- convention in agents_config/connectors_config — no users FK, so rows
-- survive user deletion and dev-header subjects don't break inserts).
-- context is client-captured environment (route, role, app_version,
-- user_agent, viewport) — non-secret, free-form JSONB. message and
-- triage_note are Confidential in the internal/pii catalog.

CREATE TABLE feedback (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_sub    TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL
        CHECK (category IN ('bug', 'idea', 'friction', 'other')),
    message     TEXT NOT NULL
        CHECK (char_length(message) BETWEEN 1 AND 4000),
    context     JSONB NOT NULL DEFAULT '{}'::jsonb,
    status      TEXT NOT NULL DEFAULT 'new'
        CHECK (status IN ('new', 'triaged', 'planned', 'shipped', 'declined')),
    triage_note TEXT NOT NULL DEFAULT ''
        CHECK (char_length(triage_note) <= 4000),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The admin/harvest read path: list an org's feedback filtered by status.
CREATE INDEX idx_feedback_org_status ON feedback (org_id, status); -- buildos:lock-ok: fresh table created in same migration
