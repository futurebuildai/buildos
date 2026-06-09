-- 018_mcp_connectors.down.sql
-- buildos:destructive: drops the connector_tools cache table and the
-- connectors_config.kind column on rollback. Only per-org MCP instance tool
-- caches + the connector-kind discriminator are lost; no operational data and no
-- deterministic engine state is affected (cached tools are re-fetchable via a
-- connector refresh).
DROP TABLE IF EXISTS connector_tools;
ALTER TABLE connectors_config DROP COLUMN IF EXISTS kind;
