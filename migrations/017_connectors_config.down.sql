-- 017_connectors_config.down.sql
-- buildos:destructive: drops the connectors_config registry table on rollback.
-- Only per-org connector enable/config OVERRIDES are lost; connectors revert to
-- their default-OFF state. No operational data and no deterministic engine state
-- is affected.
DROP TABLE IF EXISTS connectors_config;
