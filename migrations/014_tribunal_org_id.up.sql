-- 014: Add org_id to tribunal tables for multi-tenant data isolation.
-- tribunal_votes inherits org scope via decision_id FK.

ALTER TABLE tribunal_decisions
    ADD COLUMN org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

ALTER TABLE shadow_execution_logs
    ADD COLUMN org_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000';

-- Remove defaults after backfill (new rows must provide real org_id)
ALTER TABLE tribunal_decisions ALTER COLUMN org_id DROP DEFAULT;
ALTER TABLE shadow_execution_logs ALTER COLUMN org_id DROP DEFAULT;

-- Indexes for tenant-scoped queries
CREATE INDEX idx_tribunal_decisions_org ON tribunal_decisions(org_id);
CREATE INDEX idx_shadow_execution_logs_org ON shadow_execution_logs(org_id);
