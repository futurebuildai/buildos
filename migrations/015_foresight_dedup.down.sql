-- migrations/015_foresight_dedup.down.sql
-- buildos:destructive: rollback of foresight risk-card dedup column + partial unique index
DROP INDEX IF EXISTS idx_feed_risk_dedup;
ALTER TABLE feed_cards DROP COLUMN subject_code;
