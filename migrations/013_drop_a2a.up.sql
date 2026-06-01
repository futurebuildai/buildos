-- Migration 013: drop A2A inbound/outbound infrastructure
--
-- buildos:destructive: BuildOS is now a self-contained standalone
-- deployment. The agent-to-agent (A2A) webhook surface — inbound
-- JWS-verified receipt from the Brain and the outbound dead-letter
-- queue — has been removed from the application (internal/a2a,
-- internal/a2asigner, the a2a_webhook_dispatch River job). These two
-- tables have no remaining readers or writers; dropping them removes
-- dead schema. The .down.sql recreates both empty so the migration is
-- reversible, but no code populates them.

DROP TABLE IF EXISTS a2a_outbound_dlq;
DROP TABLE IF EXISTS a2a_inbound_log;
