-- ============================================================
-- 012: Integration credentials — encrypted BYOK vault (WS3)
-- ============================================================
-- The architectural pivot makes BuildOS self-contained: there is no
-- external "Brain" service holding 3rd-party API keys in its Hub vault.
-- Customers bring their own keys (Anthropic for AI, Resend for email,
-- and named vendors like Gable / LocalBlue) and store them locally,
-- AES-256-GCM encrypted via internal/cryptobox.
--
-- Design notes:
--   * Every row is org_id-scoped (the single-tenant fork invariant per
--     ADR-002 — leaves room for the future co-op variant).
--   * The secret key never lands in cleartext: ciphertext + nonce +
--     key_version are the only persisted forms of the key material.
--     The cryptobox master key lives in the secret source, never here.
--   * last4 is the last 4 chars of the cleartext key, kept for UI
--     display only ("sk-…ab12"). It is NOT secret — 4 chars of a
--     high-entropy key reveal nothing useful.
--   * Exactly one ACTIVE credential per (org_id, provider): the partial
--     unique index enforces it. Rotating a key = deactivate the old
--     row, insert a new active row, atomically in one tx.
--   * No monetary columns — the Composite Currency Pattern does not
--     apply to this migration.

CREATE TABLE integration_credentials (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider     TEXT NOT NULL,                  -- e.g. 'anthropic', 'resend', 'gable', 'localblue'
    label        TEXT NOT NULL DEFAULT '',       -- operator-friendly name
    ciphertext   BYTEA NOT NULL,                 -- AES-256-GCM ciphertext of the cleartext key
    nonce        BYTEA NOT NULL,                 -- per-row 96-bit GCM nonce
    key_version  INT NOT NULL,                   -- cryptobox master-key version that sealed this row
    last4        TEXT NOT NULL DEFAULT '',        -- last 4 chars of the cleartext key (UI display only)
    is_active    BOOLEAN NOT NULL DEFAULT true,
    created_by   TEXT NOT NULL DEFAULT '',       -- user sub (OIDC subject) that set the credential
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only ONE active credential per (org_id, provider). The resolver path
-- (AnthropicKey / ResendKey) looks up the single active row.
CREATE UNIQUE INDEX integration_credentials_active_uidx ON integration_credentials (org_id, provider) WHERE is_active; -- buildos:lock-ok: fresh table, index lands in same migration

-- Plain lookup index for ListByOrg + per-provider scans (active + inactive).
CREATE INDEX integration_credentials_org_provider_idx ON integration_credentials (org_id, provider); -- buildos:lock-ok: fresh table, index lands in same migration
