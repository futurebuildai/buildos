-- Migration 005: Procurement items additional indexes
-- NOTE: procurement_items table already created in migration 002.
-- This migration adds supplemental indexes only.

CREATE INDEX IF NOT EXISTS idx_procurement_org ON procurement_items(org_id, status);
CREATE INDEX IF NOT EXISTS idx_procurement_order_date ON procurement_items(must_order_date) WHERE status IN ('OK', 'WARNING', 'CRITICAL');
