# BuildOS on Railway — operator runbook

How **this** fork (`buildos-fork0`, the futurebuild.ai reference deployment) runs on
Railway behind Cloudflare. This document is the Railway-specific layer ONLY — the
platform-agnostic operating manual (image roles, full env-var reference, health-probe
semantics, expand/contract migration policy, graceful-shutdown budgets) lives in
[`docs/deploy-runbook.md`](../../docs/deploy-runbook.md) and is **not** duplicated
here. Companions: [`docs/fork-onboarding.md`](../../docs/fork-onboarding.md)
(fork-init secrets + owner claim + wizard), [`docs/security-posture.md`](../../docs/security-posture.md)
("Required deployment hardening"), [`docs/dr-runbook.md`](../../docs/dr-runbook.md)
(backup/restore policy + drill).

---

## 1. Topology + contract

```
                      Cloudflare  (DNS · TLS termination · HSTS · WAF)
        staging.futurebuild.ai                      app.futurebuild.ai
              │ proxied CNAME                             │ proxied CNAME
              ▼                                           ▼
┌──────────────────────── Railway project: buildos-fork0 ────────────────────────┐
│                                                                                │
│  ┌─ environment: staging ──────────────┐  ┌─ environment: production ───────┐  │
│  │                                     │  │                                 │  │
│  │  server   BUILDOS_ROLE=server       │  │  server   BUILDOS_ROLE=server   │  │
│  │           healthcheck GET /ready    │  │           healthcheck GET /ready│  │
│  │  worker   BUILDOS_ROLE=worker       │  │  worker   BUILDOS_ROLE=worker   │  │
│  │           (/ready + /metrics too)   │  │           (/ready + /metrics)   │  │
│  │  Postgres (Railway managed)         │  │  Postgres (Railway managed)     │  │
│  └─────────────────────────────────────┘  └─────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────────────┘
              ▲ image pinned by DIGEST                    ▲ same digest, promoted
              │                                           │
   GitHub Actions:                                        │
     "CI" green on main                                   │
       └─► deploy-staging.yml ────────────────────────────┘
             build + push ghcr.io/futurebuildai/buildos:staging-<short-sha>
             one-shot migrate (docker run, from the runner) → pin digest → smoke
           promote-production.yml (manual, takes the staging-verified digest;
             NEVER builds) → one-shot migrate → pin same digest → smoke
```

The contract, exactly:

- **One Railway project** `buildos-fork0`; **two environments** `staging` and
  `production`; **in each environment two services**, `server` and `worker`, plus a
  Railway **managed Postgres**. Server and worker run the **same image**;
  `BUILDOS_ROLE` selects the binary (see deploy-runbook §1).
- **Migrations are NOT a Railway service.** Each deploy workflow runs the migrate
  role as a one-shot `docker run -e BUILDOS_ROLE=migrate …` **from the GitHub
  runner** against that environment's `DATABASE_URL` — migrate-before-roll, per
  deploy-runbook §4.
- **Image:** `ghcr.io/futurebuildai/buildos` (built by this repo's CI/release
  pipeline; the Dockerfile already contains the web console and the migrations
  directory). Staging deploys build + push tag `staging-<short-sha>` and pin the
  Railway services **by the pushed digest**. **Production never builds** —
  `promote-production` takes a digest (the one staging verified) as input.
- **Server:** Railway injects `PORT`; the app reads `$PORT` (default 8080).
  Railway healthcheck path is `GET /ready` (DB-backed readiness — deploy-runbook §3).
- **Worker:** same image with `BUILDOS_ROLE=worker`; it serves `/ready` and
  `/metrics` on its own `$PORT` too, so give it the same healthcheck.

### Per-service environment variables

Set on **both** `server` and `worker`, in **each** environment (one exception:
`BUILDOS_BOOTSTRAP_TOKEN` goes on the **server only** — see its row). Full
reference + secret-source semantics: deploy-runbook §2.

| Variable | Value | Notes |
|---|---|---|
| `DATABASE_URL` | `${{Postgres.DATABASE_URL}}` | Use the **Railway reference** to that environment's Postgres — private-network DSN, rotates with the DB, never copy-pasted. |
| `JWT_PRIVATE_KEY_PEM` | from fork-init `private.pem` | Never commit; paste via the Railway dashboard (see "never echo secrets" below). |
| `JWT_PUBLIC_KEY_PEM` | from fork-init `public.pem` | |
| `VAULT_MASTER_KEY` | from fork-init `vault_master_key.txt` | Losing it makes sealed credentials undecryptable. |
| `BUILDOS_BOOTSTRAP_TOKEN` | from fork-init `bootstrap_token.txt` | **Server service ONLY** — only `cmd/server` seeds it; the worker never reads it. **First boot only:** remove (or rotate) it after the owner claim succeeds. |
| `SENTRY_ENVIRONMENT` | `staging` \| `production` | Per environment. |
| `SENTRY_DSN` | per-fork Sentry project | Optional; empty = no-op. |
| `TRUSTED_PROXY_CIDRS` | `100.64.0.0/10,10.0.0.0/8` *(placeholder)* | **REQUIRED** — Railway's edge proxies every request, so an empty value collapses all clients into one rate-limit bucket (security-posture item 2). The placeholder MUST be verified and tightened at first deploy — see §3 below. |

**Never echo secret values.** Don't pass secrets as command argv (`railway
variables --set KEY=value` lands in shell history and `ps`); paste them in the
Railway dashboard, or feed scripts via files/stdin. The deploy workflows follow the
same rule — secrets travel as env/file inputs, never interpolated into logged
commands (GitHub masks registered secrets, but masking is a backstop, not a design).

---

## 2. One-time provisioning

### Prerequisites

1. **Railway CLI** (`npm i -g @railway/cli` or the install script), logged in —
   needed for the manual fallback steps below even though `provision.sh` itself
   talks to the GraphQL API.
2. **`RAILWAY_API_TOKEN`** — create a token in the Railway dashboard (Account →
   Tokens). Export it in your shell for the provisioning run; later it also
   becomes the GitHub Actions secret of the same name. **Token type matters:**
   a **workspace/team token** cannot run the `me { projects }` lookup that
   `provision.sh` uses for by-name idempotency (Railway answers `Not
   Authorized`). With a team token — the recommended kind here — create the
   empty project once in the dashboard and pass **`--project-id <id>`** so the
   script skips lookup/create entirely. Only a **personal** token can use the
   by-name lookup/create path with no extra flag. If the lookup hits `Not
   Authorized`, `provision.sh` prints exactly this guidance and exits 2.
3. **One fork-init output directory PER environment** — `make fork-init
   OUT=./forks/fork0-staging/secrets KID=fork0-staging-2026-q2 ORG_ID=<uuid>` and
   again with a fresh `OUT`/`KID` for production, per
   [`docs/fork-onboarding.md`](../../docs/fork-onboarding.md) Step 2 (never commit
   the private files). This is **mandatory, not advisory**: `provision.sh` refuses
   to reuse one directory for two environments and refuses world-readable
   directories — production runs on a fresh keypair, so a staging credential leak
   can't mint production tokens.

### GHCR pull access

`ghcr.io/futurebuildai/buildos` is **private** (GHCR images are private by
default, and this repo is proprietary). Railway cannot pull a private image
without registry credentials — without them **every roll 401s at image pull and
`/ready` never goes 200**, including the very first deploy.

1. Create a fine-scoped GitHub **PAT (classic)** with **`read:packages` only** —
   no `repo` scope, nothing else. A **machine account** with read access to the
   package is recommended over a human account (survives offboarding, smallest
   blast radius).
2. Put the token in a file (`chmod 600`; the script refuses world-readable token
   files, same as the secrets dirs) and pass both flags to `provision.sh`:
   `--ghcr-pull-username <machine-user> --ghcr-pull-token-file <path>`. The
   script injects `registryCredentials {username, password}` into the service
   source config (`ServiceCreateInput` / `ServiceInstanceUpdateInput` — same
   validate-on-first-run caveat as every other mutation); the token travels by
   file, never argv, and is redacted in all output including `--dry-run`.
3. **Dashboard fallback** if the API path errors on the first run: Railway
   service → **Settings → Source → registry credentials** — set the username +
   PAT on all four service instances (server/worker × staging/production).

### `./provision.sh`

```bash
cd deploy/railway
export RAILWAY_API_TOKEN=...

# ALWAYS dry-run first: prints every GraphQL request body (secrets redacted)
# without sending.
# Team token? Add --project-id <id> (prerequisite 2) — the by-name lookup is
# personal-token only.
./provision.sh --project-name buildos-fork0 \
  --secrets-dir staging=../../forks/fork0-staging/secrets \
  --secrets-dir production=../../forks/fork0-production/secrets \
  --staging-domain staging.futurebuild.ai \
  --production-domain app.futurebuild.ai \
  --ghcr-pull-username <machine-user> \
  --ghcr-pull-token-file <path-to-read-packages-pat> \
  --dry-run

# Real run: same command without --dry-run.
```

More flags for non-default setups — `--project-id <id>` (skip project
lookup/create; required with team tokens, prerequisite 2),
`--environments <a,b,...>` (default `staging,production`) and `--image <ref>`
(the bootstrap image, default `ghcr.io/futurebuildai/buildos:latest`; deploys
re-pin by digest anyway). `./provision.sh --help` prints the authoritative
flag reference.

It creates the project (or reuses `--project-id`), the `staging`/`production`
environments, and the `server`/`worker` services (project-scoped, one instance
per environment; image source `ghcr.io/futurebuildai/buildos`, with
`registryCredentials` when the `--ghcr-pull-*` flags are given). Service
creation carries the **full variable map in `ServiceCreateInput.variables`** so
the first deployment boots configured instead of crash-looping with no env, and
every run (re)applies each service's variables with a **single
`variableCollectionUpsert` per service per environment** (per-variable upserts
can each trigger their own redeploy). It points healthchecks at `GET /ready`
and attaches the custom domains to the **server** service, printing each
domain's **DNS (CNAME) target** for the §3 Cloudflare step — all via GraphQL
`POST https://backboard.railway.com/graphql/v2`
(`Authorization: Bearer $RAILWAY_API_TOKEN`).

> **GraphQL schema caveat (applies to every script in this directory):** Railway's
> public API docs are thin. The exact mutation/field names (`serviceCreate`,
> `serviceInstanceUpdate`, `serviceInstanceDeployV2`, `variableCollectionUpsert`,
> `registryCredentials`, …) must be **validated against
> the live introspected schema on the first credentialed run** — that's why every
> script ships `--dry-run` that prints the GraphQL bodies without sending. If a
> mutation 400s on a field name, introspect and fix the script, don't hand-create
> drift in the dashboard.

On success it writes **`provision-output.env`** — the generated project,
environment, and service IDs in `KEY=VALUE` form, keyed exactly like the GitHub
secrets in §4. The file contains IDs only (no secrets) but treat it as
operator-private anyway; don't commit it.

### What provision.sh cannot do (manual steps)

1. **Railway Postgres.** If the API path for database provisioning fails (the
   plugin/database mutations are the least stable part of the schema), create the
   managed Postgres **per environment** with the CLI:

   ```bash
   railway link            # select buildos-fork0
   railway environment staging
   railway add --database postgres
   railway environment production
   railway add --database postgres
   ```

2. **Capture the IDs into GitHub secrets.** Copy each value from
   `provision-output.env` into the repo's Actions secrets (names in §4). If the
   Postgres was created manually, also capture each environment's **public**
   connection string into `DATABASE_URL_STAGING` / `DATABASE_URL_PRODUCTION`
   (Postgres service → Connect → public/TCP-proxy URL — the GitHub runner is
   outside Railway's private network, so the migrate one-shot and the nightly
   backup need the public DSN; the *services* keep using the private reference
   variable).

3. **Verify per-service variables.** `provision.sh` applies them via the API
   (BUILDOS_ROLE, DATABASE_URL reference, JWT PEMs, vault key, bootstrap token —
   **server service only**, SENTRY_ENVIRONMENT, TRUSTED_PROXY_CIDRS placeholder)
   — spot-check them in each service's Variables tab, and remember the two
   follow-ups: tighten `TRUSTED_PROXY_CIDRS` at first deploy, delete (or rotate)
   `BUILDOS_BOOTSTRAP_TOKEN` on the server service after the owner claim.

4. **Verify healthchecks.** `provision.sh` sets healthcheck path `/ready` via
   `serviceInstanceUpdate`; given the schema caveat above, confirm it stuck on all
   four service instances (service → Settings → Deploy → Healthcheck Path).

---

## 3. Cloudflare: DNS, TLS, HSTS, proxy-chain verification, /metrics

### DNS + TLS

1. In Railway, attach the custom domains: staging `server` service →
   `staging.futurebuild.ai`; production `server` service → `app.futurebuild.ai`.
   Railway shows a CNAME target per domain (`provision.sh` already attaches
   these via `customDomainCreate` and prints each domain's DNS target — use
   that, or read it from the dashboard).
2. In Cloudflare DNS, create **proxied** (orange-cloud) CNAMEs:
   `staging.futurebuild.ai` and `app.futurebuild.ai` → the Railway service domains.
   (If Railway's certificate issuance stalls behind the proxy, gray-cloud the
   record until the cert issues, then re-enable the proxy.)
3. Cloudflare SSL/TLS mode: **Full (strict)** — the Cloudflare→Railway leg is TLS
   with a validated certificate. Anything weaker ("Flexible") would send the
   origin leg in cleartext over the public internet.
4. **Enable HSTS at Cloudflare** (SSL/TLS → Edge Certificates → HSTS). This is
   **required deployment hardening items 1–2** in
   [`docs/security-posture.md`](../../docs/security-posture.md): the Go server
   speaks **plain HTTP behind TLS termination** and deliberately does not emit
   `Strict-Transport-Security` itself — the terminator must. Start without
   `includeSubDomains`/preload until you're sure every `futurebuild.ai` subdomain
   is HTTPS-only.

### `TRUSTED_PROXY_CIDRS` — VERIFY AT FIRST DEPLOY

The request path is **Cloudflare → Railway edge → app**, i.e. an **X-Forwarded-For
CHAIN**: Cloudflare sets `X-Forwarded-For: <client>`, Railway's edge appends its
view, and the app's TCP peer is a Railway-internal proxy address. `mw.RealIP`
([`internal/api/middleware/realip.go`](../../internal/api/middleware/realip.go))
honors forwarding headers **only** when the immediate TCP peer is inside
`TRUSTED_PROXY_CIDRS`, then walks the XFF chain **right-to-left** and keys the rate
limiter on the **first entry that is not itself a trusted proxy**. Consequences:

- If the Railway peer subnet is **not** trusted → headers ignored, every client
  collapses into one bucket keyed on the proxy IP, the whole org trips 429s
  together (the server logs a boot WARN only when the value is *empty*, not when
  it's *wrong* — so verify, don't assume).
- If the Railway peer **is** trusted but Cloudflare's egress ranges are **not** →
  the right-to-left walk stops at Cloudflare's egress IP, so all clients behind a
  Cloudflare colo share a handful of buckets. To recover the real client IP, the
  **Cloudflare published ranges (<https://www.cloudflare.com/ips/>) must also be in
  `TRUSTED_PROXY_CIDRS`** so the walk skips the Cloudflare hop and lands on the
  client.

Procedure, on the first staging deploy:

1. Deploy with the placeholder `100.64.0.0/10,10.0.0.0/8`.
2. Read the server boot log: find the line
   `trusted proxy CIDRs configured: X-Forwarded-For honored from these peers`
   and check its `cidrs` field — it lists the **parsed** entries, and
   `parseCIDRs` **silently drops malformed entries**, which is exactly what
   this step catches: the list must match what you set, entry for entry
   (`["100.64.0.0/10","10.0.0.0/8"]` for the placeholder); anything missing is
   a typo'd CIDR that is silently not trusted. If instead the
   "TRUSTED_PROXY_CIDRS is empty" WARN appears, the variable never reached the
   service at all.
3. `curl https://staging.futurebuild.ai/api/v1/nope` from a workstation whose
   public IP you know; find the request in the structured logs and check the
   recorded client IP. Your workstation IP → chain fully trusted. A Cloudflare IP
   → add Cloudflare's ranges. A `100.64.0.0/10`-ish IP → the Railway peer subnet
   isn't covered; read the observed peer from the logs and add its subnet.
4. **Tighten:** replace the broad `10.0.0.0/8` placeholder with the actual
   observed Railway peer subnet + the Cloudflare ranges, redeploy, re-verify.
5. Optional but recommended (security-posture item 5): run the k6
   `auth_login.js` flood (`scripts/k6/`) against staging — expect 429s keyed
   per-client and zero 5xx.

### Restrict `/metrics`

Prometheus scrapes are not internet traffic (security-posture item 4,
deploy-runbook §6). Two layers:

1. **Cloudflare WAF custom rule** on both hostnames:
   `http.request.uri.path eq "/metrics"` → Block. This covers the proxied domains.
2. **The WAF does not cover Railway's generated `*.up.railway.app` domains, which
   bypass Cloudflare entirely.** Remove the generated public domain from the
   `server` services once the custom domains work, and give the `worker` services
   **no public domain at all** — scrape both over **Railway private networking**
   (a Prometheus service inside the project, or a tunnel), per
   [`deploy/prometheus/README.md`](../prometheus/README.md).

---

## 4. CI/CD flow

```
push to main ─► "CI" (the existing workflow, must be green)
   └─► deploy-staging.yml   (auto, workflow_run on CI success)
         build + push  ghcr.io/futurebuildai/buildos:staging-<short-sha>
         capture the pushed DIGEST
         one-shot migrate:  docker run --rm -e BUILDOS_ROLE=migrate
                              -e DATABASE_URL  <image>@<digest>
                            (DATABASE_URL inherits DATABASE_URL_STAGING from the
                            step env — value-less -e keeps the DSN off argv;
                            cmd/migrate defaults to the "up" direction)
         GraphQL: serviceInstanceUpdate(source.image = <image>@<digest>)
                  + serviceInstanceDeployV2   — server then worker, staging IDs
         smoke against vars.STAGING_BASE_URL
   ── human verifies on https://staging.futurebuild.ai ──
   └─► promote-production.yml   (manual workflow_dispatch; INPUT = the digest
         staging verified; NEVER builds)
         one-shot migrate against DATABASE_URL_PRODUCTION
         pin server + worker (production IDs) to the SAME digest and roll
         (serviceInstanceUpdate + serviceInstanceDeployV2)
         smoke against vars.PROD_BASE_URL  → https://app.futurebuild.ai
```

Digest pinning is the point: what production runs is byte-identical to what staging
verified — a tag (`staging-abc1234`, `latest`) can be re-pushed; a digest cannot.

**Smoke spec** (both deploy workflows fail the job otherwise):

1. Poll `<BASE_URL>/ready` until HTTP 200 — timeout ~5 min, 10 s interval.
2. `GET /health` == 200.
3. `GET /` returns HTML containing `<title>BuildOS` (the same-origin web console —
   see [`web/README.md`](../../web/README.md) "Production serving").
4. `GET /api/v1/nope` returns **404** with `Content-Type: application/json` (the
   SPA fallback must not swallow API misses).

### GitHub Actions secrets — exactly these names, and where each comes from

| Secret | Source |
|---|---|
| `RAILWAY_API_TOKEN` | Railway dashboard → Account → Tokens (workspace/team token). |
| `RAILWAY_PROJECT_ID` | `provision-output.env` (from `./provision.sh`). |
| `RAILWAY_ENV_ID_STAGING` / `RAILWAY_ENV_ID_PRODUCTION` | `provision-output.env`. |
| `RAILWAY_SVC_ID_SERVER_STAGING` / `RAILWAY_SVC_ID_WORKER_STAGING` | `provision-output.env`. |
| `RAILWAY_SVC_ID_SERVER_PRODUCTION` / `RAILWAY_SVC_ID_WORKER_PRODUCTION` | `provision-output.env`. |
| `DATABASE_URL_STAGING` / `DATABASE_URL_PRODUCTION` | Each environment's Railway Postgres **public** (TCP-proxy) DSN — used only by the runner-side migrate one-shots; the production one also feeds the nightly + pre-promote backups. |
| `R2_ENDPOINT` / `R2_ACCESS_KEY_ID` / `R2_SECRET_ACCESS_KEY` / `R2_BUCKET` | Cloudflare dashboard → R2 → Manage API Tokens (a token scoped to the backup bucket only). |

Repo **variables** (`vars.*`, not secrets): `STAGING_BASE_URL=https://staging.futurebuild.ai`,
`PROD_BASE_URL=https://app.futurebuild.ai`.

### First-deploy sequence (end to end)

1. **Fork-init secrets** — generate one output dir **per environment** per
   [`docs/fork-onboarding.md`](../../docs/fork-onboarding.md) Step 2 (§2
   prerequisite 3); `provision.sh` consumes them next.
2. **Provision** — §2 (`provision.sh` creates the estate — with GHCR pull
   credentials — and applies the per-service variables: `JWT_PRIVATE_KEY_PEM`,
   `JWT_PUBLIC_KEY_PEM`, `VAULT_MASTER_KEY`, `BUILDOS_BOOTSTRAP_TOKEN` (server
   only), the rest of the §1 table — via the API; then the manual Postgres
   fallback, GitHub secrets, and the variable/healthcheck verification).
3. **Cloudflare DNS + TLS — BEFORE the first deploy** — the §3 DNS + TLS work
   for **both** hostnames (proxied CNAMEs to the Railway DNS targets
   `provision.sh` printed, Full (strict), HSTS), **then wait until the proxied
   CNAME actually resolves** (`dig +short staging.futurebuild.ai` returns
   Cloudflare edge IPs). DNS must precede the deploy because the deploy
   workflow polls `<vars.STAGING_BASE_URL>/ready` and smokes
   `https://staging.futurebuild.ai` — a hostname that doesn't resolve yet makes
   the first deploy guaranteed red.
4. **First staging deploy** — merge to `main` with green CI (or
   `workflow_dispatch` `deploy-staging.yml`). The workflow migrates, pins,
   rolls (`serviceInstanceUpdate` + `serviceInstanceDeployV2`), and smokes.
5. **Owner claim** — `POST https://staging.futurebuild.ai/api/v1/auth/claim` with
   the bootstrap-token cleartext (fork-onboarding Step 5). Then **remove (or
   rotate) `BUILDOS_BOOTSTRAP_TOKEN` on the `server` service** (the only place
   it is set) — it is single-use, and a stale secret in the dashboard is pure
   liability.
6. **Onboarding wizard** — fork-onboarding Step 6 (company info → trades → cost
   codes → calendar → jurisdictions → complete); operational routes 403
   `SETUP_INCOMPLETE` until done.
7. **Smoke + harden** — the §3 `TRUSTED_PROXY_CIDRS` verification, the `/metrics`
   restriction, the k6 flood. Then repeat 4–6 for production (its variables were
   already provisioned in step 2 and its DNS in step 3) via
   `promote-production.yml` with the staging-verified digest.

---

## 5. Backups

`.github/workflows/backup-nightly.yml` runs
[`scripts/backup-db.sh`](../../scripts/backup-db.sh) on a nightly schedule against
`DATABASE_URL_PRODUCTION` (**production only** — staging is rebuilt from
migrations + seeds and is not backed up), uploading the dump + its `.sha256`
sidecar to the R2 bucket's `nightly/` prefix (`R2_*` secrets) via the script's
storage-agnostic `BACKUP_UPLOAD_CMD` hook, then verifying both objects actually
landed. `promote-production.yml` additionally takes a pre-promote snapshot to
the `pre-promote/` prefix before every promote (no backup, no promote).

**Retention lives in R2 lifecycle rules, not on the runner.** The script's local
prune (`BACKUP_RETENTION_DAYS` / `BACKUP_RETAIN_MIN`) is moot here — the GitHub
runner is ephemeral, so nothing local survives the job anyway. Configure the
bucket's lifecycle rules for the GFS tiering you want (e.g. transition to
infrequent access after 30 d, expire after 365 d) — per the dr-runbook, lifecycle
rules are the idiomatic, auditable place for retention, and reimplementing GFS in
bash is deliberately avoided.

Alert on the workflow's failure (a silent backup failure is the most common DR
trap). Restore policy, the quarterly **restore drill**, and the failure playbook:
[`docs/dr-runbook.md`](../../docs/dr-runbook.md) — a backup you have never
restored is a hope, not a backup.

---

## 6. Tearing down the legacy Kelbrook attempts

Earlier manual deployment attempts ("Kelbrook") left orphaned Railway projects and
stale Cloudflare DNS records. [`./teardown-kelbrook-legacy.sh`](teardown-kelbrook-legacy.sh)
deletes them. Collect the ids first — the click-paths are in the
[Teardown checklist (§8)](#8-teardown-checklist-legacy-kelbrook-decommission):

```bash
export RAILWAY_API_TOKEN=...      # needed for --railway-project-id targets
export CLOUDFLARE_API_TOKEN=...   # needed for --cf-record-id targets
./teardown-kelbrook-legacy.sh \
  --cf-zone-id <zone-id> --cf-record-id <record-id> \
  --railway-project-id <project-id> \
  --dry-run                       # ALWAYS first; then re-run without --dry-run
```

Safety model — deletion is the one operation you can't roll back, so the script is
deliberately rigid:

- **Explicit ids only.** Every target is passed by hand: `--railway-project-id`
  (repeatable) and `--cf-zone-id` + `--cf-record-id` (repeatable) for stale DNS.
  It deletes only those exact ids — **never** anything pattern-matched by name,
  never anything found by list-and-filter (a name-glob teardown one typo away from
  deleting `buildos-fork0` is not a tool, it's a trap).
- **Per-item confirmation.** Each deletion fetches and prints what the id currently
  points at (project name + environments + services + domains; DNS record
  type/name/content), then requires the resource's **exact name typed at
  `/dev/tty`** — it cannot be piped in, and there is no `--yes-to-all`. A mismatch
  skips that one target and moves on.
- **DNS first, then projects.** Records stop resolving before the services they
  point at die (enforced by the script's phase order).
- `--dry-run` performs the read-only fetches, prints what each id is, and shows the
  exact mutation/DELETE bodies it *would* send — no prompts, no deletions (same
  schema caveat as §2: validate field names on the first credentialed run).

---

## 7. Provisioning builder #2 (later)

The same script, a different project name (the per-environment `--secrets-dir`
flags are **required** — the script exits before doing anything, even in
`--dry-run`, without one fork-init dir per environment):

```bash
./provision.sh --project-name buildos-<builder> \
  --secrets-dir staging=<builder-staging-fork-init-dir> \
  --secrets-dir production=<builder-production-fork-init-dir> \
  --dry-run
# then the same command without --dry-run
```

Per [ADR-002](../../.agents/handoff/ADR-002-single-tenant-fork-model.md) each
builder is a **fork** — its own repo, its own GHCR image, its own fork-init
identity (keypair, vault key, bootstrap token), its own GitHub secrets
(`provision-output.env` from its own provisioning run), its own Cloudflare
hostnames, and its own R2 backup bucket. Nothing in this directory is shared
state between builders; `provision.sh` is just the reusable bootstrap. Walk the
same §2 → §4 first-deploy sequence on the new fork.

---

## 8. Teardown checklist (legacy Kelbrook decommission)

Companion to §6. The script deletes **only** the explicit ids you hand it — no name
matching, no list-and-filter — so collecting the *right* ids is on you. Work
through this list in order.

### 8.1 Collect the legacy Railway project IDs

- Railway dashboard → your workspace → open the old Kelbrook project.
- The project ID is in the URL: `https://railway.com/project/<PROJECT_ID>`
  (also under **Project → Settings → General**).
- Before noting an ID, eyeball the project's services and domains in the dashboard
  and confirm it is a FAILED legacy attempt — not the live `buildos-fork0`. The
  script re-fetches and prints name/environments/services/domains before deleting,
  but the dashboard check is your first tripwire, not your last.
- Repeat for every legacy attempt; the flag is repeatable
  (`--railway-project-id <id> --railway-project-id <id> ...`).

### 8.2 Collect the Cloudflare zone + record IDs

- **Zone ID:** Cloudflare dashboard → select the `futurebuild.ai` zone →
  **Overview** → right-hand column → *Zone ID*.
- **Record IDs:** list them read-only via the API and copy the `id` of each STALE
  record (one whose content points at a legacy Railway deployment) — the teardown
  script itself never lists, by design:

  ```bash
  curl -sS -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" \
    "https://api.cloudflare.com/client/v4/zones/<ZONE_ID>/dns_records?name=<fqdn>" \
    | jq '.result[] | {id, type, name, content}'
  ```

- Double-check the `content` of every candidate: the records serving the CURRENT
  fork (`staging.futurebuild.ai`, `app.futurebuild.ai` → the new `buildos-fork0`
  services) must **not** go on the list.

### 8.3 Run the teardown — DNS first, then projects

- Order matters: DNS records first, so nothing resolves to a service that is being
  destroyed; Railway projects second. A single invocation enforces this internally
  (phase 1 = Cloudflare, phase 2 = Railway); if you split it across runs, keep the
  same order yourself.
- Always start with `--dry-run` and read every line it prints — it fetches each
  target and shows exactly what would be deleted and the bodies it would send.
- Real run: each deletion demands the resource's exact name typed at the terminal.
  Type only the names you just verified; anything else skips that target.

### 8.4 Verify afterwards

- `dig +short staging.futurebuild.ai` and `dig +short app.futurebuild.ai` resolve
  to the current fork's Cloudflare-proxied records (no stale answers; allow for
  DNS TTL before judging).
- `curl -fsS https://staging.futurebuild.ai/ready` and
  `curl -fsS https://app.futurebuild.ai/ready` both return 200.
- `curl -fsS https://staging.futurebuild.ai/` returns the operator console HTML
  (`<title>BuildOS`).
- Railway dashboard shows the legacy projects gone and `buildos-fork0` untouched.
