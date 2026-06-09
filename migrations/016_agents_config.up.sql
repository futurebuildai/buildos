-- 016_agents_config.up.sql
-- Phase 3a: DB-backed, per-org, admin-editable configuration for the agentic
-- capabilities (delay_cascade, foresight, experience). One row per
-- (org_id, capability) encodes an OVERRIDE of the in-code catalog default;
-- absence of a row means "enabled with the catalog default" (no seeding needed).
--
-- config holds capability-specific tuning (a JSON object). It must NEVER hold
-- secrets — credentials belong in the encrypted vault (integration_credentials),
-- not here.

CREATE TABLE agents_config (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    capability  TEXT NOT NULL,                       -- matches an in-code agentic.Capability key
    enabled     BOOLEAN NOT NULL DEFAULT true,
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,  -- capability-specific tuning; NEVER secrets
    updated_by  TEXT NOT NULL DEFAULT '',            -- OIDC subject of the last writer
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, capability)                      -- one row per (org, capability); enables ON CONFLICT upsert
);

-- Org-scoped lookups (resolver Resolve / admin List). The UNIQUE(org_id,
-- capability) constraint above already serves the single-row resolver read; this
-- index covers the per-org list scan.
CREATE INDEX idx_agents_config_org ON agents_config (org_id); -- buildos:lock-ok: fresh table created in same migration
