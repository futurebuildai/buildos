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
3. **A Brain admin account.** You'll register the fork's public key
   and OIDC client there.
4. **A Go toolchain ≥1.26** (only on the operator workstation; the
   fork's runtime image already has everything).

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

Each fork has its own RSA-2048 keypair for signing outbound A2A
webhooks. The Brain verifies signatures against the public key
published in the fork's JWKS.

```bash
make build-fork-init
make fork-init OUT=./fork-config/identity \
               KID=acme-2026-q2 \
               ORG_ID=11111111-1111-1111-1111-111111111111
```

Outputs four files:

| File | Status | Purpose |
|---|---|---|
| `private.pem` | **NEVER commit** | RSA private key. Move into your secret store. |
| `public.pem` | safe to commit | RSA public key in PEM. For docs / non-JWKS verifiers. |
| `jwks.json` | paste into Brain | The form Brain expects when registering the fork. |
| `fork.yaml` | commit | Operator-readable summary: kid, org_id, fingerprint, env-var names. |

Confirm `private.pem` is in your `.gitignore`:

```
fork-config/identity/private.pem
```

To rotate later: regenerate with a new `--kid`, register the new public
key with Brain alongside the old one, deploy with the new
`A2A_KEY_ID`. Old key remains valid until removed from Brain's JWKS.

---

## Step 3 — Move the private key into your secret store

The fork at runtime expects the private key to be readable through the
configured `SecretSource`. Options:

### Vault (HashiCorp)

```bash
vault kv put secret/buildos-acme/A2A_SIGNING_KEY \
  value=@fork-config/identity/private.pem
```

Set `CONFIG_SOURCE=vault://secret/buildos-acme` (Vault backend lands
in a follow-up; until then use the file source below).

### Kubernetes Secret (file source)

```bash
kubectl create secret generic buildos-secrets \
  --from-file=A2A_SIGNING_KEY=fork-config/identity/private.pem \
  --from-literal=DATABASE_URL='postgres://...' \
  --from-literal=BRAIN_JWKS_URL='https://brain.example/jwks' \
  --from-literal=BRAIN_ISSUER_URL='https://brain.example' \
  --from-literal=BRAIN_OUTBOUND_URL='https://brain.example/api/v1/a2a/webhook' \
  --from-literal=SENTRY_DSN='https://...'
```

Mount it into the deployment at `/run/secrets/buildos/`, then set
`CONFIG_SOURCE=file:/run/secrets/buildos`.

### AWS Secrets Manager / GCP Secret Manager

Backends land in follow-up PRs; until then use the file source pattern
with a sidecar that pulls from your manager and writes to a tmpfs-
mounted dir.

---

## Step 4 — Register the fork with The Brain

Open the Brain admin UI (or use its API) and create a new fork
registration with:

- **Fork name:** `buildos-acme`
- **Org ID:** the same UUID you passed to `--org-id` above
- **JWKS:** paste the contents of `jwks.json` (or upload the file)
- **A2A webhook URL:** `https://buildos.acme.example/api/v1/a2a/webhook`
- **Region:** customer's preferred deployment region
- **Plan tier:** as per the customer agreement (free / starter / pro / enterprise)

The Brain returns:
- An OIDC client_id + client_secret (used for the fork's BRAIN_ISSUER_URL flow)
- The Brain's JWKS endpoint URL (BRAIN_JWKS_URL)
- The Brain's webhook target URL (BRAIN_OUTBOUND_URL)

Move the OIDC client_secret into your secret store too.

---

## Step 5 — Apply migrations + deploy

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
| `BRAIN_JWKS_URL` | yes | from Brain registration |
| `BRAIN_ISSUER_URL` | yes | from Brain registration |
| `BRAIN_OUTBOUND_URL` | yes | from Brain registration |
| `A2A_SIGNING_KEY_PATH` | yes | path to private.pem inside the secret-mount |
| `A2A_KEY_ID` | yes | the kid from fork.yaml |
| `DEFAULT_ORG_ID` | yes | the org_id from fork.yaml |
| `SENTRY_DSN` | recommended | per-fork Sentry project |
| `CONFIG_SOURCE` | recommended | secret-source spec; defaults to env |

Non-secret tunables:

| Variable | Default | Notes |
|---|---|---|
| `PORT` | 8080 | server listen port |
| `DB_POOL_MAX` | 25 | tune per database tier |
| `RATE_LIMIT_RPS` | 50 | per-IP DDoS throttle |
| `SENTRY_ENVIRONMENT` | dev | should be `prod` in production |
| `SENTRY_TRACES_SAMPLE_RATE` | 0.0 | 0.05–0.1 in prod is typical |

---

## Step 6 — Verify

```bash
# 1. Liveness — the process is up:
curl https://buildos.acme.example/health
# → {"status":"ok","version":"v1.0.0"}

# 2. Readiness — the process can serve traffic (DB + Brain reachable):
curl https://buildos.acme.example/ready
# → {"status":"ok","components":{"database":"ok","brain":"ok","jwks":"ok"}}

# 3. End-to-end A2A: ask Brain to send a test webhook to the fork.
#    The fork's audit_log table should record a row for the event.

# 4. Smoke a JWT round-trip:
#    Open the Brain login flow → BuildOS frontend → confirm the
#    user lands on a real protected endpoint with their org_id +
#    role pulled from the JWT.
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

- **Key rotation.** Run `make fork-init OUT=... KID=acme-2027-q1`
  with a new kid; register the new key with Brain alongside the old;
  deploy with the new `A2A_KEY_ID`; remove the old key from Brain's
  JWKS after a 24h grace.

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
  fork. The Brain has its own compliance posture for the metering
  data it holds.

---

## When something breaks

- **`/ready` reports `brain="unhealthy"`** — Brain's `/health` is
  unreachable. Likely network or DNS; the fork falls back to using
  the cached JWKS for token validation, but new tokens stop validating
  after the cache TTL (5 min default). Page Brain on-call.

- **A2A webhooks rejected by Brain** — usually a `kid` mismatch.
  Confirm `A2A_KEY_ID` matches what's registered in Brain's per-fork
  JWKS. If you rotated keys, double-check both are present in Brain
  during the grace window.

- **DLQ filling up** — `field_notification_dlq` or `a2a_outbound_dlq`
  growing means downstream is degraded. Inspect rows for the
  `last_error` column; common patterns: Brain returning 5xx (transient
  — drain on its own), Twilio rate-limit (raise the per-tenant cap),
  4xx with malformed envelope (code bug — file an upstream issue).

- **Audit log growing without bound** — partitioning + archive policy
  lands in Phase G. For now, manually archive rows older than your
  retention policy via `pg_dump` + `DELETE`.
