# BuildOS Deploy Runbook

Operating a BuildOS fork in production: image roles, configuration, health probes,
rollouts, and graceful shutdown. Pairs with
[`docs/fork-onboarding.md`](fork-onboarding.md) (first-time provisioning),
[`docs/observability-runbook.md`](observability-runbook.md) (monitoring + alerts),
[`docs/dr-runbook.md`](dr-runbook.md) (backup/restore), and
[`deploy/prometheus/`](../deploy/prometheus/) (scrape + rules).

> **Railway (the canonical hosting platform):** this file stays platform-agnostic;
> the concrete Railway runbook — provisioning, Cloudflare DNS/TLS, the staging →
> production promotion pipeline, nightly backups, legacy teardown — lives in
> [`deploy/railway/README.md`](../deploy/railway/README.md) (Phase 1).

Deployment model: **single-tenant per customer fork** ([ADR-002](../.agents/handoff/ADR-002-single-tenant-fork-model.md)).
One image, one database, one customer. No RLS, no per-tenant routing.

---

## 1. One image, three roles

`make docker-build` produces a single distroless image. The entrypoint selects the
binary by **`BUILDOS_ROLE`** (default `server`):

| `BUILDOS_ROLE` | Runs | Purpose |
|---|---|---|
| `server` (default) | `bin/server` | Chi HTTP API on `$PORT` (default 8080). |
| `worker` | `bin/worker` | River job daemon (briefings, notifications, cascades, rollups…). |
| `migrate` | `bin/migrate` | Applies River internal migrations then `migrations/NNN_*.up.sql`, then exits. |

Run **server** and **worker** as separate long-lived deployments off the same image
+ env. Run **migrate** as a one-shot Job/init step (§4). **Both** the server and the
worker expose `/metrics`, `/health`, and `/ready` on `$PORT` (the worker's HTTP +
job-outcome metrics were wired in 4b-ii) — scrape and probe both.

The image is built `-tags=prod` (D8 hardening): the dev `X-Dev-Auth` bypass is a
no-op, and **the server refuses to start if `DEV_AUTH_MODE` is set** — fail fast
beats serving uniform 401s. Never set `DEV_AUTH_MODE` in production.

**The image also contains the web console** (Phase 0a): a `node:20-alpine` build
stage compiles `web/` and the server role serves the bundle same-origin from
`WEB_DIST_DIR` (baked in at `/var/lib/buildos/web`). `GET /` is the operator
console; `/api/*`, `/health`, `/ready`, `/metrics` are untouched. SPA fallback,
asset caching, and the console's `Content-Security-Policy` are owned by
`internal/api/spa.go` — no separate static host or proxy-level header config is
needed. The server **fails at boot** if `WEB_DIST_DIR` is set but unreadable;
unset it to run API-only.

## 2. Configuration

Secrets are resolved through `CONFIG_SOURCE` (`internal/config`): empty/`env` reads
env vars; `file:/path` reads `<path>/<KEY>` (k8s secret-mount convention);
`chain:a,b,…` is priority-ordered fallback; `vault://…` for HashiCorp Vault KV v2.
Transport errors short-circuit (a Vault outage is **not** a silent downgrade to env).

**Required (secret-bearing):**

| Key | What | Notes |
|---|---|---|
| `DATABASE_URL` | Postgres DSN | Required; boot fails without it. |
| `JWT_PRIVATE_KEY_PEM` / `JWT_PUBLIC_KEY_PEM` | RS256 signing/verification keypair | Required (server) unless `DEV_AUTH_MODE=header`. Per-fork; from `make fork-init`. |
| `VAULT_MASTER_KEY` | AES-256 key for the encrypted credential vault | Enables `/integrations` + `/capabilities`. From `make fork-init`. |
| `BUILDOS_BOOTSTRAP_TOKEN` | one-shot first-owner claim token | Optional; seeded at boot (idempotent) for `POST /api/v1/auth/claim`. From `make fork-init`. |
| `SENTRY_DSN` | error reporting | Optional; empty = no-op. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | trace collector | Optional; empty = no-op exporter (W3C propagation still works). |

**Non-secret scalars (direct env):** `PORT` (8080), `DB_POOL_MAX` (25), `DB_POOL_MIN`,
DB connect timeout, `OTEL_*` sample/insecure flags, `SENTRY_ENVIRONMENT`/`SENTRY_RELEASE`,
`WEB_DIST_DIR` (same-origin web console dir; baked into the image as
`/var/lib/buildos/web` — override only to relocate, unset to run API-only).

First-time keypair/vault-key/bootstrap-token generation is one command —
`make fork-init OUT=… KID=… ORG_ID=…` — see [fork-onboarding.md](fork-onboarding.md).
`private.pem`, `vault_master_key.txt`, `bootstrap_token.txt` are **never committed**;
`public.pem` is committable.

## 3. Health probes

| Probe | Endpoint | Checks | Use for |
|---|---|---|---|
| **Liveness** | `GET /health` | process is up (NO DB check) | k8s `livenessProbe` — restart on hang/crash. |
| **Readiness** | `GET /ready` | `pool.Ping` (DB reachable) | k8s `readinessProbe` / LB gate — pull from rotation when the DB is unreachable. |

Liveness deliberately does **not** touch the DB: a transient DB blip should pull the
pod from rotation (readiness), not kill+restart it (liveness). Example:

```yaml
livenessProbe:
  httpGet: { path: /health, port: 8080 }
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet: { path: /ready, port: 8080 }
  initialDelaySeconds: 5
  periodSeconds: 10
  failureThreshold: 3
```

(The worker exposes the same `/health`, `/ready`, and `/metrics` on `$PORT`, so probe
+ scrape it the same way as the server.)

## 4. Rollouts & migrations

**Migrate before you roll.** Run the one-shot `migrate` role to completion **before**
rolling the server/worker to the new image:

```
BUILDOS_ROLE=migrate <image>   # k8s: a Job or an initContainer that must succeed first
```

Migrations are written **expand/contract** so a migration is backward-compatible with
the *previously running* code — that's what makes a zero-downtime rolling update safe
(old pods keep serving against the migrated schema while new pods roll in). The
migration linter (`make lint-migrations`, a hard CI gate) enforces the guardrails:
paired `up`/`down`, `CREATE INDEX CONCURRENTLY` (or an explicit `lock-ok` opt-out),
and a `-- buildos:destructive:` header before any `DROP`/`TRUNCATE`. Destructive
(contract) steps ship in a *later* release than the code that stopped using the
column — never in the same rollout as the expand.

Preflight checklist for a release:
1. CI green (`make audit` — which already includes `lint-isolation` — plus
   `govulncheck` + `make test-integration`).
2. **Back up the database** (`scripts/backup-db.sh`; see [dr-runbook.md](dr-runbook.md)).
3. Run `BUILDOS_ROLE=migrate` to completion; confirm exit 0.
4. Roll `server`, then `worker` (rolling update; readiness gates traffic).
5. Watch `BuildOSHTTP5xxRateHigh` + Sentry for ~10m (see the observability runbook).

**Rollback:** roll the image back. If a release included a *contract* migration, the
old image may not run against the new schema — prefer rolling forward with a fix, or
restore from the pre-deploy backup ([dr-runbook.md](dr-runbook.md)).

## 5. Graceful shutdown

On `SIGINT`/`SIGTERM` both roles drain gracefully, but their budgets differ — set
`terminationGracePeriodSeconds` per role so the orchestrator doesn't `SIGKILL`
mid-drain:

- **Server:** stops accepting new connections and drains in-flight requests with a
  **10s** timeout (`srv.Shutdown`), then flushes OpenTelemetry (5s) and Sentry → set
  `terminationGracePeriodSeconds` ≥ **15s**. Readiness starts failing on shutdown, so
  the LB pulls the pod first.
- **Worker:** drains in-flight River jobs with a **30s** `Client.Stop` budget
  (`cmd/worker/main.go`) → set `terminationGracePeriodSeconds` ≥ **30s** (or ≥ your
  tuned drain budget) so long-running jobs finish instead of being killed and retried.

## 6. `/metrics` exposure

Both the server and the worker serve `/metrics` (unauthenticated, Prometheus
convention) on `$PORT`. They are separate pods in production, each with its own `PORT`,
so there's no conflict — but if you **co-locate them on one host** (docker-compose,
local dev), give them **distinct `PORT` values**, or the worker's HTTP listener fails
to bind (it fails fast + loud, rather than running blind). **Restrict `/metrics` at the
network layer** — a NetworkPolicy / LB ACL allowing only the Prometheus scraper. Scrape
config (two jobs: `buildos-server`, `buildos-worker`) + the alert rules:
[`deploy/prometheus/README.md`](../deploy/prometheus/README.md).
