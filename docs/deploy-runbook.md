# BuildOS Deploy Runbook

Operating a BuildOS fork in production: image roles, configuration, health probes,
rollouts, and graceful shutdown. Pairs with
[`docs/fork-onboarding.md`](fork-onboarding.md) (first-time provisioning),
[`docs/observability-runbook.md`](observability-runbook.md) (monitoring + alerts),
[`docs/dr-runbook.md`](dr-runbook.md) (backup/restore), and
[`deploy/prometheus/`](../deploy/prometheus/) (scrape + rules).

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
+ env. Run **migrate** as a one-shot Job/init step (§4). Only the **server** exposes
`/metrics` (and `/health`, `/ready`); the worker serves no HTTP today (its
observability is a known gap — see the observability runbook §3).

The image is built `-tags=prod` (D8 hardening): the dev `X-Dev-Auth` bypass is a
no-op, and **the server refuses to start if `DEV_AUTH_MODE` is set** — fail fast
beats serving uniform 401s. Never set `DEV_AUTH_MODE` in production.

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
DB connect timeout, `OTEL_*` sample/insecure flags, `SENTRY_ENVIRONMENT`/`SENTRY_RELEASE`.

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

(The worker serves no HTTP at all — no `/health`/`/ready`/`/metrics`; gate it on
process liveness only. A worker `/metrics` + probes is a tracked follow-up.)

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

`/metrics` is unauthenticated (Prometheus convention) and mounts when the metrics
registry is wired. **Restrict it at the network layer** — a NetworkPolicy / LB ACL
allowing only the Prometheus scraper. Scrape config + the alert rules:
[`deploy/prometheus/README.md`](../deploy/prometheus/README.md).
