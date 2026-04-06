-- Migration 012: Bid leveling analysis storage
-- Stores Claude-powered bid comparison results for procurement decisions.

CREATE TABLE IF NOT EXISTS bid_analyses (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL,
    project_id      UUID NOT NULL,
    item_id         UUID,
    bid_count       INTEGER NOT NULL,
    bids_data       JSONB NOT NULL,
    analysis        JSONB NOT NULL,
    recommendation  TEXT,
    confidence      REAL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bid_analyses_org ON bid_analyses(org_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bid_analyses_project ON bid_analyses(project_id);
