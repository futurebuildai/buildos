-- ============================================================
-- 008: Audit log — broad write-side capture for compliance + agentic monitoring
-- ============================================================
-- Every domain mutation that touches an org-owned resource lands a row
-- here from inside its tx. Reads + transient state changes (River
-- attempts, JWKS refresh, etc.) are NOT audited — those belong in
-- structured logs.
--
-- Schema is intentionally generic:
--   - resource_type + resource_id let agents query "history of THIS row"
--     without per-domain joins.
--   - before/after JSONB lets a replay show exactly what changed.
--   - metadata holds action-specific context (e.g. action_type for
--     feed_card.actioned, po_number for procurement_item.updated).
--   - request_id correlates to the same id stamped on responses + Brain
--     hops + structured logs (D-reqid).
--
-- user_sub is nullable — system actors (River cron jobs that audit
-- their own writes) leave it null and identify themselves via action.

CREATE TABLE audit_log (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    user_sub        TEXT,                        -- JWT `sub` claim; null for system actions
    action          TEXT NOT NULL,               -- e.g. "feed.card.actioned"
    resource_type   TEXT NOT NULL,               -- e.g. "feed_card"
    resource_id     UUID NOT NULL,               -- the row's id
    before_state    JSONB,                       -- prior state for updates; null for inserts
    after_state     JSONB,                       -- new state for inserts/updates; null for hard deletes
    metadata        JSONB,                       -- action-specific context, free-form
    request_id      TEXT,                        -- chi RequestID; null when out-of-request (cron)
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary lookup: "what happened in this org, recently".
CREATE INDEX idx_audit_log_org_occurred ON audit_log(org_id, occurred_at DESC); -- buildos:lock-ok: fresh table created in same migration

-- "Show me the history of this specific resource."
CREATE INDEX idx_audit_log_resource ON audit_log(resource_type, resource_id, occurred_at DESC); -- buildos:lock-ok: fresh table created in same migration

-- "What did this user do?"
CREATE INDEX idx_audit_log_user_sub ON audit_log(user_sub, occurred_at DESC) WHERE user_sub IS NOT NULL; -- buildos:lock-ok: fresh table created in same migration

-- "All approvals in the last 24h." Useful for agentic compliance scans.
CREATE INDEX idx_audit_log_action ON audit_log(action, occurred_at DESC); -- buildos:lock-ok: fresh table created in same migration
