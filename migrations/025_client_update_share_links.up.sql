-- 025_client_update_share_links.up.sql
-- Chunk E (DAILY_REPORTS_CLIENT_UPDATES): the public-share token table — the
-- FIRST surface outside the everything-behind-auth invariant.
--
-- A homeowner who received a client update can also be given an unauthenticated,
-- token-gated, read-only progress page at GET /p/{token}. This table holds the
-- share tokens. Security model mirrors setup_bootstrap_tokens EXACTLY:
--
--   * 32-byte CSPRNG cleartext, base64url (43 chars), shown ONCE to the operator
--     at create (it becomes the URL they send). The cleartext is NEVER stored.
--   * Only the sha256 hash lands here (token_hash). The unique index on it
--     supports a direct lookup at resolution time.
--   * Resolution filters expires_at > now() AND revoked_at IS NULL, and returns
--     a UNIFORM not-found on any failure (missing / expired / revoked / mismatch)
--     so an attacker probing /p/{token} learns nothing — enumeration defense.
--
-- Differences from the bootstrap token: a share link is EXPIRABLE (default 30
-- days — a homeowner link wants a longer but bounded life, §9-6/D-6) and
-- operator-REVOCABLE at any time (revoked_at). It is NOT one-shot — a homeowner
-- may reload the page repeatedly; last_viewed_at / view_count are best-effort
-- view telemetry with NO PII.
--
-- ON DELETE CASCADE on client_update_id: deleting the client update (or its
-- project/org chain) drops the link, so a stale token can never resolve to a
-- vanished update.
--
-- PII: token_hash is the hash of a credential — treat like a secret, NEVER log.
-- No emails / names / amounts on this table. Composite Currency Pattern N/A.

CREATE TABLE client_update_share_links (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    client_update_id UUID NOT NULL REFERENCES client_updates(id) ON DELETE CASCADE,
    -- sha256 of the 32-byte CSPRNG cleartext, hex-encoded (matches the
    -- bootstrap-token hashing). The cleartext is NEVER stored.
    token_hash       TEXT NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    -- Operator revoke. NULL = active. A revoked link resolves to a uniform 404.
    revoked_at       TIMESTAMPTZ,
    created_by       UUID NOT NULL REFERENCES users(id),
    -- Best-effort view telemetry. No PII (no IP, no UA).
    last_viewed_at   TIMESTAMPTZ,
    view_count       BIGINT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Plain CREATE INDEX (NOT CONCURRENTLY): the migrate runner wraps every
-- migration in a tx (cmd/migrate/main.go) and Postgres forbids CONCURRENTLY
-- inside a tx. Both indexes land on the table created above in this same
-- migration, so they build on an empty table — the brief ACCESS EXCLUSIVE lock
-- is a no-op (no rows, no concurrent writers). Matches 022/024's precedent.
--
-- Resolution path: direct unique lookup by token hash on every /p/{token} hit.
CREATE UNIQUE INDEX idx_share_links_token_hash ON client_update_share_links (token_hash); -- buildos:lock-ok: unique index on table created in same migration (empty); brief lock is a no-op
-- Operator list path: a client update's links newest-first (active/expired/revoked).
CREATE INDEX idx_share_links_update ON client_update_share_links (client_update_id, created_at DESC); -- buildos:lock-ok: index on table created in same migration (empty); brief lock is a no-op
