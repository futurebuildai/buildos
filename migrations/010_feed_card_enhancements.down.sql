-- Migration 010 down: Remove agent enhancements from feed_cards

DROP TABLE IF EXISTS agent_pending_actions;

DROP INDEX IF EXISTS idx_feed_cards_agent_source;
DROP INDEX IF EXISTS idx_feed_cards_expires;
DROP INDEX IF EXISTS idx_feed_cards_task;

ALTER TABLE feed_cards DROP COLUMN IF EXISTS headline;
ALTER TABLE feed_cards DROP COLUMN IF EXISTS consequence;
ALTER TABLE feed_cards DROP COLUMN IF EXISTS horizon;
ALTER TABLE feed_cards DROP COLUMN IF EXISTS agent_source;
ALTER TABLE feed_cards DROP COLUMN IF EXISTS deadline;
ALTER TABLE feed_cards DROP COLUMN IF EXISTS engine_data;
ALTER TABLE feed_cards DROP COLUMN IF EXISTS task_id;
