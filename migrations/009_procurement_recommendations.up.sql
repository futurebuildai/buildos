-- ============================================================
-- 009: Procurement recommendations — Maestro vendor-recommendation persistence (S3 Session 7.1)
-- ============================================================
-- Persists the output of brain.MaestroClient.ProcurementRecommend so:
--   1. The audit log + this table together let humans replay why a
--      given vendor was recommended (run_id ties to Brain's ai_runs).
--   2. Future evaluation can compare Maestro recommendations against
--      what was actually ordered (procurement_items.vendor_id later)
--      to measure recommendation quality.
--
-- Each row is one (vendor, item) recommendation; a single Maestro call
-- typically produces 3-5 rows that all share run_id.
--
-- Design notes:
--   * predicted_spend_cents + predicted_spend_currency_code is the
--     standard Composite Currency pair (CHECK enforces USD/CAD).
--   * confidence_pct is SMALLINT 0..100 instead of a float — keeps
--     SQL aggregations exact and matches the codebase no-floats culture.
--     Maestro's float64 Confidence is rounded * 100 in the store layer.
--   * vendor_id is nullable because Maestro may recommend a vendor
--     that isn't yet in BuildOS's vendor table; vendor_name is always
--     populated for human-readable rendering.
--   * No FK on vendor_id (vendors table not yet modeled in this repo;
--     when it lands, follow up with ALTER TABLE ADD CONSTRAINT in a
--     later migration).
--   * org_id is denormalized so per-org compliance/audit sweeps can
--     index without joining procurement_items — matches the pattern
--     established by procurement_items + project_budgets.

CREATE TABLE procurement_recommendations (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    procurement_item_id             UUID NOT NULL REFERENCES procurement_items(id) ON DELETE CASCADE,
    org_id                          UUID NOT NULL REFERENCES organizations(id),
    run_id                          UUID NOT NULL,                          -- Maestro RunID; correlates to ai_runs in Brain
    vendor_id                       UUID,                                    -- nullable; vendors table not yet modeled
    vendor_name                     TEXT NOT NULL,
    predicted_spend_cents           BIGINT NOT NULL,
    predicted_spend_currency_code   VARCHAR(3) NOT NULL DEFAULT 'USD',
    confidence_pct                  SMALLINT NOT NULL,                      -- 0..100; Maestro Confidence float * 100, rounded
    reasoning                       TEXT,                                    -- optional Maestro explanation
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT procurement_recommendations_currency_check
        CHECK (predicted_spend_currency_code IN ('USD', 'CAD')),
    CONSTRAINT procurement_recommendations_confidence_range
        CHECK (confidence_pct >= 0 AND confidence_pct <= 100),
    CONSTRAINT procurement_recommendations_spend_nonneg
        CHECK (predicted_spend_cents >= 0)
);

-- "Latest recommendations for this procurement item" — the dominant query.
CREATE INDEX idx_procurement_recommendations_item ON procurement_recommendations(procurement_item_id, created_at DESC); -- buildos:lock-ok: fresh table created in same migration

-- "All recommendations from one Maestro run" — for replay + quality eval.
CREATE INDEX idx_procurement_recommendations_run ON procurement_recommendations(run_id); -- buildos:lock-ok: fresh table created in same migration

-- "Per-org sweep" — compliance/audit dashboards.
CREATE INDEX idx_procurement_recommendations_org_created ON procurement_recommendations(org_id, created_at DESC); -- buildos:lock-ok: fresh table created in same migration
