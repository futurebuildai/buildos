-- 024_client_updates.up.sql
-- Chunk D (DAILY_REPORTS_CLIENT_UPDATES): the client-update lifecycle table.
-- The human-in-the-loop composer: an AI draft (Chunk C's ClientProgressUpdate)
-- is persisted as a draft, the operator EDITS the client-safe subject/body and
-- curates which photos the homeowner sees, then explicitly SENDS — the row
-- flips to 'sent' and the email goes out via the existing Resend mailer
-- post-commit. NEVER auto-sent.
--
-- Lifecycle: 'draft' -> 'sent' (mailer accepted) | 'failed' (mailer rejected /
-- unconfigured). 'failed' carries send_error so the operator KNOWS it didn't go
-- out (the one place that diverges from the auth-reset best-effort posture). A
-- failed update is re-sendable (the send path resets it to draft semantics on
-- retry by allowing a send when status != 'sent').
--
-- period_start/period_end bound the reporting window the draft summarizes
-- (Chunk C produces a single-day draft today, so they may be equal). subject /
-- edited_body are what the homeowner receives; ai_draft preserves the original
-- AI text for audit/diff. photo_asset_ids is the operator-curated subset of the
-- period's confirmed daily-log photos (a redaction control) — resolved to
-- signed/proxied URLs at render, validated 'ready'+org+project on edit.
--
-- PII: recipient_email is Restricted — snapshot at send, NEVER logged and NEVER
-- in audit metadata (audit references client_update.id + project_id only).
-- subject / edited_body / ai_draft are Confidential (client-facing prose).
-- pii.FieldClass gains recipient_email -> Restricted in this chunk.
--
-- Composite Currency Pattern N/A: no monetary columns (no _cents / cost / etc.).

CREATE TABLE client_updates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'sent', 'failed')),
    -- The original AI draft (Chunk C). Preserved for audit/diff; the operator
    -- edits edited_body, not this.
    ai_draft        TEXT,
    -- The operator-edited, client-safe body that is actually sent.
    edited_body     TEXT NOT NULL DEFAULT '',
    subject         TEXT NOT NULL DEFAULT '',
    -- Snapshot of the homeowner address at send. Restricted; NEVER logged.
    recipient_email TEXT,
    -- Operator-curated subset of the period's confirmed daily-log photos
    -- (a redaction control: the operator chooses what the homeowner sees).
    photo_asset_ids UUID[] NOT NULL DEFAULT '{}',
    created_by      UUID NOT NULL REFERENCES users(id),
    sent_by         UUID REFERENCES users(id),
    sent_at         TIMESTAMPTZ,
    -- Set when a send fails (mailer unconfigured / rejected) so the operator
    -- knows it did not go out. Never carries the recipient address.
    send_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Plain CREATE INDEX (NOT CONCURRENTLY): the migrate runner wraps every
-- migration in a tx (cmd/migrate/main.go) and Postgres forbids CONCURRENTLY
-- inside a tx. Both indexes land on the client_updates table created above in
-- this same migration, so they build on an empty table — the brief ACCESS
-- EXCLUSIVE lock is a no-op (no rows, no concurrent writers). Matches 022's
-- precedent.
--
-- Project history read path: list a project's client updates newest-first.
CREATE INDEX idx_client_updates_project ON client_updates (project_id, created_at DESC); -- buildos:lock-ok: index on table created in same migration (empty); brief lock is a no-op
-- Org-wide status read path (portfolio history, draft/sent filtering).
CREATE INDEX idx_client_updates_org_status ON client_updates (org_id, status, created_at DESC); -- buildos:lock-ok: index on table created in same migration (empty); brief lock is a no-op
