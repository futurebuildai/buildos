-- 018_mcp_connectors.up.sql
-- Phase 3b-ii: the MCP connector type. Connectors are now either a built-in
-- in-process type (kind='builtin', e.g. 'reference') or a runtime-created MCP
-- server INSTANCE (kind='mcp', connector_name is an operator-chosen instance
-- name, config carries the https endpoint). A separate connector_tools table
-- caches the tools fetched from an MCP server's tools/list (operator-driven
-- refresh) — kept OUT of connectors_config.config because the cached tool
-- names/descriptions/schemas are attacker-influenced (rendered into the model's
-- tools[]) and have a different trust + lifecycle than operator-authored config.

ALTER TABLE connectors_config ADD COLUMN kind TEXT NOT NULL DEFAULT 'builtin'; -- 'builtin' | 'mcp'

CREATE TABLE connector_tools (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    connector_name TEXT NOT NULL,                       -- the mcp instance name
    tool_name      TEXT NOT NULL,                       -- the REMOTE (un-namespaced) tool name
    description    TEXT NOT NULL DEFAULT '',            -- bounded at refresh time
    input_schema   JSONB NOT NULL DEFAULT '{}'::jsonb,  -- a JSON object, bounded at refresh time
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, connector_name, tool_name)
);

CREATE INDEX idx_connector_tools_org_conn ON connector_tools (org_id, connector_name); -- buildos:lock-ok: fresh table created in same migration
