-- 020_feedback.down.sql
-- buildos:destructive: drops the feedback table on rollback. Operator
-- feedback rows (and their triage state) are lost; no deterministic
-- engine state or financial data is affected. Harvested feedback lives
-- on as GitHub issues in buildos-operations, so the loop's durable
-- record survives a rollback.
DROP TABLE IF EXISTS feedback;
