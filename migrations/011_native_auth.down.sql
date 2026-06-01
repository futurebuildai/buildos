-- Migration 011 (down): revert native email/password authentication.

DROP TABLE IF EXISTS auth_password_reset_tokens;
DROP TABLE IF EXISTS auth_refresh_tokens;

DROP INDEX IF EXISTS users_org_email_uidx;

-- Restore the NOT NULL constraint on oidc_subject. This only succeeds if no
-- native (oidc_subject IS NULL) users remain; that is the intended safety
-- behavior for a rollback to the Brain-only identity model.
ALTER TABLE users ALTER COLUMN oidc_subject SET NOT NULL;

ALTER TABLE users DROP COLUMN IF EXISTS last_login_at;
ALTER TABLE users DROP COLUMN IF EXISTS password_hash;
