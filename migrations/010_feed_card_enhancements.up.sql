-- Migration 010: Feed card agent enhancements + pending actions table
-- Adds columns for Claude-powered agent output and human-in-the-loop approval.

-- Add agent-related columns to feed_cards
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS headline TEXT;
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS consequence TEXT;
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS horizon TEXT;
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS agent_source TEXT;
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS deadline TIMESTAMPTZ;
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS engine_data JSONB;
ALTER TABLE feed_cards ADD COLUMN IF NOT EXISTS task_id UUID;

CREATE INDEX IF NOT EXISTS idx_feed_cards_agent_source ON feed_cards(agent_source) WHERE agent_source IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_feed_cards_expires ON feed_cards(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_feed_cards_task ON feed_cards(task_id) WHERE task_id IS NOT NULL;

-- Agent pending actions table for human-in-the-loop approval
CREATE TABLE IF NOT EXISTS agent_pending_actions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL,
    card_id      UUID REFERENCES feed_cards(id) ON DELETE CASCADE,
    agent_source TEXT NOT NULL,
    action_type  TEXT NOT NULL,
    action_data  JSONB NOT NULL,
    status       TEXT NOT NULL DEFAULT 'pending',
    resolved_by  TEXT,
    resolved_at  TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_agent_actions_org ON agent_pending_actions(org_id, status);
CREATE INDEX IF NOT EXISTS idx_agent_actions_card ON agent_pending_actions(card_id);
