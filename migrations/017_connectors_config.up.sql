-- 017_connectors_config.up.sql
-- Phase 3b-i: per-org, admin-editable configuration for the integration
-- CONNECTORS (built-in in-process connectors now; vault-backed MCP connectors in
-- 3b-ii). One row per (org_id, connector_name). Unlike agents_config (Phase 3a),
-- connectors are DEFAULT-OFF: absence of a row means the connector is DISABLED;
-- an admin must explicitly enable one (opt-in). config holds non-secret,
-- forward-compatible settings (3b-ii: endpoint, allowed-tools); credentials
-- belong in the encrypted vault (integration_credentials), NEVER here.

CREATE TABLE connectors_config (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    connector_name TEXT NOT NULL,                       -- matches a built-in connectors.Connector Name()
    enabled        BOOLEAN NOT NULL DEFAULT false,      -- DEFAULT-OFF (opt-in; opposite of agents_config)
    config         JSONB NOT NULL DEFAULT '{}'::jsonb,  -- non-secret connector settings; NEVER credentials (vault)
    updated_by     TEXT NOT NULL DEFAULT '',            -- OIDC subject of the last writer
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, connector_name)                     -- one row per (org, connector); enables ON CONFLICT upsert
);

CREATE INDEX idx_connectors_config_org ON connectors_config (org_id); -- buildos:lock-ok: fresh table created in same migration
