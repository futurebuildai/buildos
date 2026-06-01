-- Migration 011: Native email/password authentication
--
-- BuildOS now owns identity directly instead of delegating to The Brain's
-- OIDC provider. Users authenticate with email + password (argon2id), and
-- BuildOS mints/validates its own RS256 JWTs. This migration:
--   * adds password_hash + last_login_at to users
--   * makes oidc_subject nullable (legacy column; native users have none)
--   * enforces one email per org (case-insensitive)
--   * adds refresh-token and password-reset-token tables (opaque, hashed)

-- ------------------------------------------------------------
-- 1. users: native credential columns
-- ------------------------------------------------------------
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_hash TEXT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login_at TIMESTAMPTZ;

-- oidc_subject was NOT NULL UNIQUE for Brain-issued identities. Native users
-- have no OIDC subject, so the column becomes nullable. The original UNIQUE
-- constraint still permits many NULLs (NULLs are distinct in Postgres unique
-- indexes), so it is retained as-is for any residual Brain-linked rows.
ALTER TABLE users ALTER COLUMN oidc_subject DROP NOT NULL;

-- One email address per org, case-insensitive. Native login looks users up by
-- (org_id, lower(email)); this index both enforces uniqueness and backs that
-- lookup.
CREATE UNIQUE INDEX users_org_email_uidx ON users (org_id, lower(email)); -- buildos:lock-ok: backs unique email-per-org; tables are small at onboarding time

-- ------------------------------------------------------------
-- 2. auth_refresh_tokens: opaque, sha256-hashed refresh tokens
-- ------------------------------------------------------------
-- Cleartext is a 32-byte CSPRNG value shown once to the client; only the
-- sha256 hash is stored. Rotation revokes the old row and issues a new one.
CREATE TABLE auth_refresh_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    issued_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ
);
-- Active-token lookups filter on revoked_at IS NULL; a partial index keeps the
-- working set small as revoked rows accumulate.
CREATE INDEX auth_refresh_tokens_user_active_idx ON auth_refresh_tokens (user_id) WHERE revoked_at IS NULL; -- buildos:lock-ok: fresh table created in same migration

-- ------------------------------------------------------------
-- 3. auth_password_reset_tokens: single-use, hashed reset tokens
-- ------------------------------------------------------------
-- Same hashing discipline as refresh tokens. redeemed_at flips on use; a token
-- is valid only while redeemed_at IS NULL and now() < expires_at.
CREATE TABLE auth_password_reset_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    issued_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    redeemed_at TIMESTAMPTZ
);
CREATE INDEX auth_password_reset_tokens_user_active_idx ON auth_password_reset_tokens (user_id) WHERE redeemed_at IS NULL; -- buildos:lock-ok: fresh table created in same migration
