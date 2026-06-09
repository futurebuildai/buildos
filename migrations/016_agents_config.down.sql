-- 016_agents_config.down.sql
-- buildos:destructive: drops the agents_config registry table on rollback. Only
-- per-org agent enable/tune OVERRIDES are lost; capabilities revert to their
-- in-code catalog defaults (enabled-with-default), so no operational data and no
-- deterministic engine state is affected.
DROP TABLE IF EXISTS agents_config;
