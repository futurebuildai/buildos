# BuildOS fork onboarding runbook

How to provision a new customer-fork BuildOS deployment from zero. Each
customer gets their own fork; this runbook is the per-customer
checklist.

Companion docs: [ADR-002](../.agents/handoff/ADR-002-single-tenant-fork-model.md)
(why we fork-per-customer), [ARCHITECTURE.md](../.agents/handoff/ARCHITECTURE.md)
(system shape), [PRODUCTION_READINESS_PLAN.md](../.agents/handoff/PRODUCTION_READINESS_PLAN.md)
(launch criteria).

---

## What you need before you start

1. **A customer agreement.** Names, contacts, SLA tier, region preference,
   compliance requirements (SOC 2 scope, GDPR applicability), branding
   assets.
2. **Cloud / hosting access.** A Kubernetes cluster, Postgres instance,
   container registry pull credentials, secret store
   (Vault / AWS Secrets Manager / GCP Secret Manager / k8s Secrets).
3. **A Go toolchain ≥1.26** (only on the operator workstation; the
   fork's runtime image already has everything).

BuildOS is **self-contained** — there is no external identity provider,
AI gateway, or credential broker to register with. 3rd-party keys
(Anthropic for AI, Resend for password-reset email, named vendors) are
set **in-app** after onboarding via the admin integrations vault
(`PUT /api/v1/integrations/{provider}`); they are not needed to boot.
A fork with no keys configured runs fine — AI endpoints soft-fail with
`503` until a key is added.

The fork's repo doesn't need to live in `futurebuildai` — customers
can host their fork wherever (their own GitHub org, their own GitLab,
even an internal Bitbucket).

---

## Step 1 — Fork the repo

```bash
# From the canonical buildos repo:
gh repo create acme-corp/buildos-acme \
  --template futurebuildai/buildos \
  --private \
  --description "BuildOS for Acme Construction"
```

Or for an existing customer org's GitHub:

```bash
gh repo fork futurebuildai/buildos --org acme-corp \
  --fork-name buildos-acme --remote
```

Clone locally:

```bash
git clone git@github.com:acme-corp/buildos-acme.git
cd buildos-acme
```

The fork's branch should track the canonical `buildos` repo's `main`
as `upstream` so you can pull future releases:

```bash
git remote add upstream https://github.com/futurebuildai/buildos.git
```

---

## Step 2 — Generate the fork's cryptographic identity

Each fork has its own RSA-2048 keypair (BuildOS signs and verifies its
own RS256 JWTs — no external JWKS), a 32-byte AES-256 master key for the
encrypted credential vault, and a one-shot bootstrap token for the
first-owner claim.

```bash
make build-fork-init
make fork-init OUT=./fork-config/identity \
               KID=acme-2026-q2 \
               ORG_ID=11111111-1111-1111-1111-111111111111
```

Outputs five files:

| File | Status | Purpose |
|---|---|---|
| `private.pem` | **NEVER commit** | PKCS#8 RSA private key — the JWT signing key. Move into your secret store; configure as `JWT_PRIVATE_KEY_PEM`. |
| `public.pem` | safe to commit | SPKI RSA public key — the JWT verification key. Configure as `JWT_PUBLIC_KEY_PEM`. |
| `vault_master_key.txt` | **NEVER commit** | Standard-base64 32-byte AES-256 key for the encrypted credential vault. Configure as `VAULT_MASTER_KEY`. Losing or rotating it makes existing sealed credentials undecryptable. |
| `bootstrap_token.txt` | **NEVER commit** | One-shot cleartext for the onboarding wizard's first-owner claim. 32 bytes of CSPRNG, base64url-encoded. Move into your secret store as `BUILDOS_BOOTSTRAP_TOKEN`. |
| `fork.yaml` | commit | Operator-readable summary: kid, org_id, fingerprint, env-var names. |

Confirm the three secrets are in your `.gitignore`:

```
fork-config/identity/private.pem
fork-config/identity/vault_master_key.txt
fork-config/identity/bootstrap_token.txt
```

To rotate the JWT keypair only (keep onboarding state + vault): regenerate
with a new `--kid` and `--skip-bootstrap-token`, deploy with the new
`JWT_KEY_ID`. (Note: rotating the keypair invalidates outstanding access
tokens; users re-login. The vault master key is unaffected.)

To rotate everything (fresh tenant): regenerate without
`--skip-bootstrap-token`; cmd/server reseeds `setup_bootstrap_tokens`
on next boot if the org has not yet completed onboarding.

---

## Step 3 — Move the secrets into your secret store

The fork at runtime resolves secrets through the configured
`SecretSource`. The JWT private key, the vault master key, the database
URL, and (on first boot) the bootstrap token must be reachable there.

### Vault (HashiCorp)

```bash
vault kv put kv/buildos-acme/JWT_PRIVATE_KEY_PEM value=@fork-config/identity/private.pem
vault kv put kv/buildos-acme/VAULT_MASTER_KEY    value=@fork-config/identity/vault_master_key.txt
vault kv put kv/buildos-acme/DATABASE_URL        value='postgres://...'
```

Set `CONFIG_SOURCE=vault://kv/data/buildos-acme` (KV v2 spec format).

### Kubernetes Secret (file source)

```bash
kubectl create secret generic buildos-secrets \
  --from-file=JWT_PRIVATE_KEY_PEM=fork-config/identity/private.pem \
  --from-file=JWT_PUBLIC_KEY_PEM=fork-config/identity/public.pem \
  --from-file=VAULT_MASTER_KEY=fork-config/identity/vault_master_key.txt \
  --from-literal=DATABASE_URL='postgres://...' \
  --from-literal=SENTRY_DSN='https://...'
```

Mount it into the deployment at `/run/secrets/buildos/`, then set
`CONFIG_SOURCE=file:/run/secrets/buildos`.

### AWS Secrets Manager / GCP Secret Manager

Backends land in follow-up PRs; until then use the file source pattern
with a sidecar that pulls from your manager and writes to a tmpfs-
mounted dir.

---

## Step 4 — Apply migrations + deploy

```bash
# Build the production image (multi-arch in CI; single-arch locally):
make docker-build VERSION=v1.0.0 DOCKER_IMAGE=ghcr.io/acme-corp/buildos
docker push ghcr.io/acme-corp/buildos:v1.0.0

# Run migrations against the fork's database:
docker run --rm \
  -e DATABASE_URL="$DATABASE_URL" \
  -e BUILDOS_ROLE=migrate \
  ghcr.io/acme-corp/buildos:v1.0.0 up

# Deploy server + worker to your platform of choice. Both use the
# same image; BUILDOS_ROLE selects the binary.
```

Required environment variables (pulled from the SecretSource):

| Variable | Required | Notes |
|---|---|---|
| `DATABASE_URL` | yes | Postgres connection string |
| `JWT_PRIVATE_KEY_PEM` | yes | PKCS#8 RSA private key (contents of private.pem) — signs access tokens |
| `JWT_PUBLIC_KEY_PEM` | yes | SPKI RSA public key (contents of public.pem) — verifies access tokens |
| `JWT_KEY_ID` | recommended | the kid from fork.yaml (default `buildos-1`) |
| `VAULT_MASTER_KEY` | yes (for BYOK) | standard-base64 32-byte key from vault_master_key.txt; absent → integrations vault + AI/email features disabled |
| `BUILDOS_BOOTSTRAP_TOKEN` | first boot only | cleartext from bootstrap_token.txt; cmd/server seeds the hash on first boot. The first owner presents the cleartext to `POST /api/v1/auth/claim`. Unset (or rotate) after the claim. |
| `MAIL_FROM` | for password reset | sender address used with the org's Resend key |
| `APP_BASE_URL` | for password reset | base URL used to build reset links in emails |
| `SENTRY_DSN` | recommended | per-fork Sentry project |
| `CONFIG_SOURCE` | recommended | secret-source spec; defaults to env |

Optional JWT overrides: `JWT_ISSUER` (default `buildos`) and
`JWT_AUDIENCE` (default `buildos`). Leave at defaults unless you have a
reason to change the wire-protocol values.

Non-secret tunables:

| Variable | Default | Notes |
|---|---|---|
| `PORT` | 8080 | server listen port |
| `DB_POOL_MAX` | 25 | tune per database tier |
| `RATE_LIMIT_RPS` | 0 | per-IP throttle (0 → middleware default) |
| `SENTRY_ENVIRONMENT` | dev | should be `prod` in production |
| `SENTRY_TRACES_SAMPLE_RATE` | 0.0 | 0.05–0.1 in prod is typical |

---

## Step 5 — Verify

```bash
# 1. Liveness — the process is up:
curl https://buildos.acme.example/health
# → {"status":"ok","version":"v1.0.0"}

# 2. Readiness — the process can serve traffic. BuildOS is self-contained,
#    so the only hard dependency is its own Postgres:
curl https://buildos.acme.example/ready
# → {"status":"ok","components":{"database":"ok"}}

# 3. Claim the first owner with the bootstrap token (mints a real JWT):
curl -X POST https://buildos.acme.example/api/v1/auth/claim \
  -H "Content-Type: application/json" \
  -d '{"token":"<cleartext from bootstrap_token.txt>","email":"owner@acme.example","password":"<strong-password>","display_name":"Acme Owner"}'
# → 201 {"data":{"access_token":"...","refresh_token":"...","user":{...}}}

# 4. Onboarding gate is active — operational routes 403 until the
#    wizard is complete (use the access_token from the claim):
curl https://buildos.acme.example/api/v1/projects \
  -H "Authorization: Bearer $JWT"
# → 403 {"error":{"code":"SETUP_INCOMPLETE","message":"..."}}

# 5. Setup state is reachable (gate-exempt):
curl https://buildos.acme.example/api/v1/setup/state \
  -H "Authorization: Bearer $JWT"
# → 200 {"onboarding_complete":false,"completed_steps":[],"pending_steps":[...]}
```

---

## Step 6 — Run the onboarding wizard

Fresh forks land with `organizations.onboarding_complete = false`. Any
`/api/v1/*` route except the wizard, health probes (`/health`, `/ready`),
and `/metrics` returns `403 SETUP_INCOMPLETE` until the wizard completes.

The first owner is created by the native-auth claim (Step 5 above), not
by a setup route. With the resulting access token, walk the wizard:

```bash
# Wizard steps (Lit web portal drives these; raw endpoints shown):
#    Company info  → POST /api/v1/setup/company-info
#    Trades        → POST /api/v1/setup/trades
#    Cost codes    → POST /api/v1/setup/cost-codes
#    Calendar      → POST /api/v1/setup/calendars
#                    (+ POST /api/v1/setup/calendars/{calendarID}/holidays)
#    Jurisdictions → POST /api/v1/setup/jurisdictions
#    Complete      → POST /api/v1/setup/complete
# → onboarding_complete flips to true; operational routes unlock.
#   Complete requires: legal name, ≥1 trade, ≥1 cost code, a default calendar.

# After completion, unset BUILDOS_BOOTSTRAP_TOKEN in deploy secrets — it is
# now stamped used and re-presentation fails uniformly. Rotating the token
# requires re-running buildos-fork-init.

# Configure 3rd-party keys (admin-only) once onboarding is complete:
#    Anthropic (AI) → PUT /api/v1/integrations/anthropic  {"label":"prod","key":"sk-ant-..."}
#    Resend (email) → PUT /api/v1/integrations/resend      {"label":"prod","key":"re_..."}
```

---

## What ongoing maintenance looks like

- **Upstream merges.** When `futurebuildai/buildos` releases a new
  version, the fork operator pulls + merges:

  ```bash
  git fetch upstream
  git merge upstream/main
  # resolve any customizations on the fork's side
  ```

  Conflict frequency depends on how much customer-specific code the
  fork has added. Keep customizations in clearly-marked directories so
  upstream changes to core don't collide with them.

- **JWT key rotation.** Run `make fork-init OUT=... KID=acme-2027-q1`
  with a new kid to mint a fresh RS256 keypair, then deploy the new
  `JWT_PRIVATE_KEY_PEM` / `JWT_PUBLIC_KEY_PEM` / `JWT_KEY_ID`. Because
  BuildOS is its own issuer and verifier, there is no external JWKS to
  coordinate — already-issued access tokens expire within
  `AUTH_ACCESS_TTL` (15 min default) and clients silently re-auth via
  refresh token. Keep the old public key mounted until all outstanding
  access tokens have expired, then drop it.

- **Vault master-key rotation.** `VAULT_MASTER_KEY` versioning is
  tracked by `VAULT_KEY_VERSION`. Rotating the master key requires
  re-encrypting stored credentials under the new key; until that
  tooling lands, treat the master key as long-lived and protect it in
  the secret store accordingly.

- **Migrations.** Every `make migrate` run on the fork is gated by the
  same `lint-migrations` rules — destructive ops require an opt-in
  comment, every up has a paired down, indexes use CONCURRENTLY (or
  the documented opt-out for fresh tables). Customer forks inherit
  this discipline automatically; CI on the fork repo runs the same
  audit.

- **Backups.** Per-fork — each customer's database is independently
  backed up to the customer's own object store. Document RPO/RTO in
  the customer's runbook.

- **Compliance reviews.** SOC 2 / GDPR / DPA artifacts are scoped per
  fork. Each deployment is fully self-contained — no data leaves the
  fork's own database and object store, so the compliance boundary is
  the customer's own infrastructure.

---

## When something breaks

- **`/ready` reports `database="unhealthy"`** — the pgx pool can't
  reach Postgres. Check `DATABASE_URL`, network/DNS, and that the DB
  is accepting connections. BuildOS has no external runtime
  dependencies beyond its own Postgres, so `/ready` only reflects DB
  health.

- **Login returns `401 INVALID_CREDENTIALS` for a known-good user** —
  if it started after a deploy, confirm `JWT_PRIVATE_KEY_PEM` /
  `JWT_PUBLIC_KEY_PEM` weren't regenerated without intent. A new
  keypair invalidates outstanding tokens (clients re-auth) but does
  not affect stored password hashes; a credential failure points at
  the password itself or an argon2id parameter change.

- **AI endpoints return `503 SERVICE_UNAVAILABLE`** — no valid
  Anthropic key is configured (or the stored key was rejected
  upstream). Set it via `PUT /api/v1/integrations/anthropic`. This is
  a soft-fail by design: the server boots and serves all non-AI
  routes without any key.

- **Password-reset emails not arriving** — no valid Resend key is
  configured, so `internal/mailer` soft-fails. Set it via
  `PUT /api/v1/integrations/resend` and confirm `MAIL_FROM` /
  `APP_BASE_URL` are set. The reset endpoint still returns `202` even
  when mail can't be sent (to avoid leaking account existence), so
  check the structured logs for the mailer warning.

- **DLQ filling up** — `field_notification_dlq` growing means
  downstream notification delivery is degraded. Inspect rows for the
  `last_error` column; common patterns: Twilio rate-limit (raise the
  per-tenant cap), 4xx with malformed envelope (code bug — file an
  upstream issue).

- **Audit log growing without bound** — partitioning + archive policy
  lands in Phase G. For now, manually archive rows older than your
  retention policy via `pg_dump` + `DELETE`.
