-- migrations/015_foresight_dedup.up.sql
-- Foresight risk-card dedup: anchor an active/dismissed risk card to its subject
-- (WBS / "total") so the daily foresight sweep skips instead of spamming a new
-- card each run. subject_code is NOT NULL DEFAULT '' so the partial-index
-- card_type predicate (not nullability) excludes non-risk cards, and the unique
-- index has no NULL-distinctness hole.
ALTER TABLE feed_cards ADD COLUMN subject_code VARCHAR(50) NOT NULL DEFAULT '';

-- Plain CREATE INDEX (NOT CONCURRENTLY): the migrate runner wraps every migration
-- in a tx (cmd/migrate/main.go:119-136) and Postgres forbids CONCURRENTLY inside
-- a tx. Brief ACCESS EXCLUSIVE on the small per-fork feed_cards table is acceptable.
CREATE UNIQUE INDEX idx_feed_risk_dedup -- buildos:lock-ok: partial index on small per-fork feed_cards table; brief lock at deploy acceptable
  ON feed_cards (project_id, card_type, subject_code)
  WHERE status IN ('active', 'dismissed')
    AND card_type IN ('procurement_criticality', 'schedule_slip', 'budget_burn');
