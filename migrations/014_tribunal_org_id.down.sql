-- Reverse of 014: Remove org_id from tribunal tables.
DROP INDEX IF EXISTS idx_shadow_execution_logs_org;
DROP INDEX IF EXISTS idx_tribunal_decisions_org;

ALTER TABLE shadow_execution_logs DROP COLUMN IF EXISTS org_id;
ALTER TABLE tribunal_decisions DROP COLUMN IF EXISTS org_id;
