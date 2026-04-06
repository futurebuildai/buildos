CREATE TABLE IF NOT EXISTS vision_verifications (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL,
    task_id             UUID NOT NULL,
    photo_url           TEXT NOT NULL,
    expected_progress   INTEGER NOT NULL,
    estimated_progress  INTEGER NOT NULL,
    confidence          REAL NOT NULL,
    notes               TEXT,
    issues              JSONB,
    requires_review     BOOLEAN NOT NULL DEFAULT false,
    reviewed_by         TEXT,
    reviewed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_vision_task ON vision_verifications(task_id);
CREATE INDEX IF NOT EXISTS idx_vision_org ON vision_verifications(org_id, created_at);
CREATE INDEX IF NOT EXISTS idx_vision_review ON vision_verifications(requires_review) WHERE requires_review = true;
