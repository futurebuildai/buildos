-- Migration 002: Financial tables — Composite Currency Pattern
-- All monetary values: amount_cents BIGINT + currency_code VARCHAR(3)
-- Cross-currency arithmetic is FORBIDDEN.

-- ============================================================
-- 1. Project Budgets (per WBS phase)
-- ============================================================
CREATE TABLE project_budgets (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id                      UUID NOT NULL REFERENCES projects(id),
    wbs_code                        TEXT NOT NULL,
    phase_name                      TEXT NOT NULL,
    estimated_cost_cents            BIGINT NOT NULL DEFAULT 0,
    estimated_cost_currency_code    VARCHAR(3) NOT NULL DEFAULT 'USD',
    committed_cost_cents            BIGINT NOT NULL DEFAULT 0,
    committed_cost_currency_code    VARCHAR(3) NOT NULL DEFAULT 'USD',
    actual_cost_cents               BIGINT NOT NULL DEFAULT 0,
    actual_cost_currency_code       VARCHAR(3) NOT NULL DEFAULT 'USD',
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, wbs_code),
    CONSTRAINT chk_budget_currency_match CHECK (
        estimated_cost_currency_code = committed_cost_currency_code
        AND committed_cost_currency_code = actual_cost_currency_code
    )
);

-- ============================================================
-- 2. Corporate Budget Rollups — grouped by currency_code
-- ============================================================
CREATE TABLE corporate_budgets (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                  UUID NOT NULL REFERENCES organizations(id),
    fiscal_year             INTEGER NOT NULL,
    quarter                 INTEGER NOT NULL,
    currency_code           VARCHAR(3) NOT NULL DEFAULT 'USD',
    total_estimated_cents   BIGINT NOT NULL DEFAULT 0,
    total_committed_cents   BIGINT NOT NULL DEFAULT 0,
    total_actual_cents      BIGINT NOT NULL DEFAULT 0,
    project_count           INTEGER NOT NULL DEFAULT 0,
    last_rollup_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, fiscal_year, quarter, currency_code)
);
CREATE INDEX idx_corp_budget_org ON corporate_budgets(org_id);

-- ============================================================
-- 3. AR Aging Snapshots — one row per currency per snapshot date
-- ============================================================
CREATE TABLE ar_aging_snapshots (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                  UUID NOT NULL REFERENCES organizations(id),
    snapshot_date           DATE NOT NULL,
    currency_code           VARCHAR(3) NOT NULL DEFAULT 'USD',
    current_cents           BIGINT NOT NULL DEFAULT 0,
    days_30_cents           BIGINT NOT NULL DEFAULT 0,
    days_60_cents           BIGINT NOT NULL DEFAULT 0,
    days_90_plus_cents      BIGINT NOT NULL DEFAULT 0,
    total_receivable_cents  BIGINT NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 4. Invoices
-- ============================================================
CREATE TABLE invoices (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    vendor_name         TEXT NOT NULL,
    invoice_number      TEXT,
    amount_cents        BIGINT NOT NULL,
    currency_code       VARCHAR(3) NOT NULL DEFAULT 'USD',
    wbs_code            TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, paid
    due_date            DATE,
    paid_date           DATE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================
-- 5. Procurement Items
-- ============================================================
CREATE TABLE procurement_items (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id                      UUID NOT NULL REFERENCES projects(id),
    org_id                          UUID NOT NULL REFERENCES organizations(id),
    name                            TEXT NOT NULL,
    wbs_code                        TEXT NOT NULL,
    estimated_cost_cents            BIGINT NOT NULL DEFAULT 0,
    estimated_cost_currency_code    VARCHAR(3) NOT NULL DEFAULT 'USD',
    lead_time_days                  INTEGER NOT NULL DEFAULT 0,
    weather_buffer_days             INTEGER NOT NULL DEFAULT 0,
    vendor_id                       UUID,
    need_by_date                    DATE,
    must_order_date                 DATE,          -- Computed: need_by - lead_time - weather_buffer
    status                          TEXT NOT NULL DEFAULT 'OK',  -- OK, WARNING, CRITICAL, ORDERED
    ordered_at                      TIMESTAMPTZ,
    po_number                       TEXT,
    status_changed_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_procurement_project ON procurement_items(project_id);
CREATE INDEX idx_procurement_status ON procurement_items(status);
