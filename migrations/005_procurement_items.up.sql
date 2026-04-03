-- Migration 005: Procurement items table for material/equipment tracking

CREATE TABLE procurement_items (
    id                              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                          UUID NOT NULL REFERENCES organizations(id),
    project_id                      UUID NOT NULL REFERENCES projects(id),
    description                     TEXT NOT NULL,
    estimated_cost_cents            BIGINT NOT NULL DEFAULT 0,
    estimated_cost_currency_code    VARCHAR(3) NOT NULL DEFAULT 'USD',
    status                          TEXT NOT NULL DEFAULT 'PENDING',
    must_order_date                 TIMESTAMPTZ,
    expected_delivery_date          TIMESTAMPTZ,
    supplier_name                   TEXT,
    supplier_contact                TEXT,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_procurement_project ON procurement_items(project_id, status);
CREATE INDEX idx_procurement_org ON procurement_items(org_id, status);
CREATE INDEX idx_procurement_order_date ON procurement_items(must_order_date) WHERE status IN ('PENDING', 'WARNING');
