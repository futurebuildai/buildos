-- Migration 004: Pre-Construction Pipeline tables (CRM Kanban, estimates, permits)
-- Composite Currency Pattern enforced on all monetary columns.

-- ============================================================
-- 1. Pre-Construction Prospects (CRM Kanban)
-- ============================================================
CREATE TABLE pre_construction_prospects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    name                TEXT NOT NULL,
    client_name         TEXT NOT NULL,
    client_email        TEXT,
    client_phone        TEXT,
    address             TEXT,
    gsf                 INTEGER,  -- Gross Square Footage estimate
    pipeline_stage      TEXT NOT NULL DEFAULT 'LEAD',
    -- LEAD (10%) -> QUALIFIED (25%) -> ESTIMATE_SENT (50%)
    -- -> VERBAL_COMMITMENT (75%) -> PERMIT_APPLIED (85%) -> PERMIT_ISSUED (100%)
    probability_pct     INTEGER NOT NULL DEFAULT 10,
    source              TEXT,     -- referral, website, repeat_client
    notes               TEXT,
    lost_reason         TEXT,     -- Only set if stage = LOST
    project_id          UUID REFERENCES projects(id),  -- Set on PERMIT_ISSUED transition
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_prospects_org ON pre_construction_prospects(org_id);
CREATE INDEX idx_prospects_stage ON pre_construction_prospects(pipeline_stage);

-- ============================================================
-- 2. Pre-Construction Estimates (preliminary, pre-permit)
-- ============================================================
CREATE TABLE pre_construction_estimates (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id             UUID NOT NULL REFERENCES pre_construction_prospects(id),
    version                 INTEGER NOT NULL DEFAULT 1,
    total_estimated_cents   BIGINT NOT NULL DEFAULT 0,
    currency_code           VARCHAR(3) NOT NULL DEFAULT 'USD',
    line_items              JSONB NOT NULL DEFAULT '[]',
    -- [{wbs_code, description, estimated_cents, unit, quantity}]
    margin_pct              INTEGER NOT NULL DEFAULT 15,
    status                  TEXT NOT NULL DEFAULT 'draft', -- draft, sent, revised, accepted
    sent_at                 TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_estimates_prospect ON pre_construction_estimates(prospect_id);

-- ============================================================
-- 3. Pre-Construction Permits (municipal tracking)
-- ============================================================
CREATE TABLE pre_construction_permits (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id         UUID NOT NULL REFERENCES pre_construction_prospects(id),
    permit_type         TEXT NOT NULL,  -- building, electrical, plumbing, mechanical
    jurisdiction        TEXT NOT NULL,
    application_number  TEXT,
    submitted_date      DATE,
    expected_issue_date DATE,
    actual_issue_date   DATE,
    fee_cents           BIGINT NOT NULL DEFAULT 0,
    fee_currency_code   VARCHAR(3) NOT NULL DEFAULT 'USD',
    status              TEXT NOT NULL DEFAULT 'not_submitted',
    -- not_submitted, submitted, under_review, revisions_requested, approved, denied
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_permits_prospect ON pre_construction_permits(prospect_id);
CREATE INDEX idx_permits_status ON pre_construction_permits(status);
