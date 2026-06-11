-- 021_seed_fork_zero_org.up.sql
-- Single-tenant fork bootstrap (ADR-002).
--
-- A fresh fork boots with an EMPTY organizations table — no migration
-- ever seeded one. That stalls onboarding before it can begin: the
-- boot-time bootstrap-token seeder (service.SeedBootstrapTokenIfNeeded)
-- attaches the one-shot first-owner claim token to "the first
-- onboarding-incomplete org", finds none, and silently no-ops — so
-- POST /api/v1/auth/claim always 401s and the SetupGate never opens.
-- The e2e harness hid this by INSERTing an org directly; real forks
-- (e.g. the first Railway deploy of buildos-fork0) had no equivalent.
--
-- Seed exactly one org, idempotently (only when the table is empty), so
-- a fresh deployment is claimable out of the box. The name/slug are
-- neutral placeholders; the owner sets the real company identity
-- (legal_name, region, calendar, …) in the onboarding wizard, and
-- onboarding_complete stays false until they finish. Per-customer forks
-- (builder #2+) re-seed via the fork playbook.
INSERT INTO organizations (name, slug)
SELECT 'BuildOS Fork', 'fork-zero'
WHERE NOT EXISTS (SELECT 1 FROM organizations);
