# BuildOS Observability Runbook

How to see what a BuildOS fork is doing, and what to do when an alert fires.
Pairs with [`deploy/prometheus/buildos.rules.yml`](../deploy/prometheus/buildos.rules.yml)
(the rules), [`deploy/prometheus/README.md`](../deploy/prometheus/README.md) (scrape
config + the metric surface), the [deploy runbook](deploy-runbook.md), and the
[DR runbook](dr-runbook.md) (backup/restore).

Audience: the operator running a single-tenant fork. There is one deployment, one
customer — "tenant isolation = deployment isolation" ([ADR-002](../.agents/handoff/ADR-002-single-tenant-fork-model.md)).

---

## 1. The three signals

All three are **turn-on-when-configured** — an empty config is a silent no-op, so a
fork can run with none of them and add them later (`internal/obs`).

| Signal | Surface | Enabled by | Notes |
|---|---|---|---|
| **Metrics** | `GET /metrics` (Prometheus) | server + worker (both expose `/metrics`) | Custom registry — nine emitted `buildos_*` metrics (see the README table). Unauth; restrict by network policy. |
| **Logs** | stdout, JSON (slog) | always | Every record carries the correlation trio (§2). Scrape into Loki/CloudWatch/etc. |
| **Traces** | OTLP → collector | `OTEL_EXPORTER_OTLP_ENDPOINT` | Every inbound request is a span (`otelhttp`); default sample rate 0.1. Empty endpoint ⇒ no-op exporter but W3C propagation still stamps `trace_id` into logs. |
| **Errors** | Sentry | `SENTRY_DSN` | Panics + tagged exceptions. PII scrubbed in `BeforeSend` via the `internal/pii` catalog (Restricted fields redacted). |

## 2. The correlation trio — how to pivot between signals

`internal/obs.CorrelatingHandler` stamps every log record with:

- `request_id` — from the chi `RequestID` middleware; also returned to clients.
- `trace_id` + `span_id` — from the active OpenTelemetry span.

So the workflow is: an alert (metrics) names a **route/kind/status** → grep logs for
that route around the alert window → take the `trace_id` → open the trace in your
collector, and/or find the matching Sentry event (tagged with the same ids). Always
log via the `*Context` slog variants so the trio is present.

## 3. The metric surface + known gaps

See [`deploy/prometheus/README.md`](../deploy/prometheus/README.md#the-metric-surface-what-buildos-emits)
for the emitted metrics and their labels. **Both the server and the worker are
scraped.** Phase 4b-iii closed the prior gaps: `buildos_db_pool_*` (pool stats),
`buildos_http_error_responses_total{code,status}` (per-error-code, incl.
`SETUP_INCOMPLETE`), and `buildos_river_queue_depth` + `buildos_river_oldest_available_seconds`
(the wedged-but-alive worker). Remaining:

- **DB pool acquire-WAIT** — the gauges expose acquired/idle/total/max conns
  (`BuildOSDBPoolNearExhaustion` alerts at >90% of max), but not the per-acquire wait
  duration. Rising HTTP p95 + the pool-near-max alert are the proxy.
- **River metrics are best-effort** — `buildos_river_job_runs_total` is fed by River's
  buffered, non-blocking subscription, so it can undercount under burst; the River job
  tables are the source of truth (see `BuildOSRiverJobsDiscarded`). The queue-depth
  gauges (4b-iii) are a direct table query and are NOT subject to that drop.

> **Resolved in 4b-iii:** `pgxpool.Stat()` → `buildos_db_pool_*`; the per-error-code
> (`SETUP_INCOMPLETE` etc.) counter; and the worker queue-depth / oldest-available gauges
> (the wedged-worker case → `BuildOSWorkerQueueBacklog`).

---

## 4. Alert response

Each alert's `runbook_url` deep-links to the matching section below. General first
moves on any alert: check the orchestrator (pod restarts / OOMKilled / recent
deploy), `/ready` (DB), Sentry, and the logs for the route/kind named in the alert.

### BuildOSServerDown
**Critical.** Prometheus can't scrape the **server** target for >2m.
1. Is the process up? (`kubectl get pods` / your orchestrator — look for crashloop,
   OOMKilled, recent rollout.)
2. Is `/health` (liveness) 200? If the process is up but `/health` is unreachable,
   suspect the network policy / service routing.
3. Is `/ready` 200? A failing `/ready` means the **DB ping failed** — the process is
   alive but can't serve; check the database (see the DR runbook) and `DATABASE_URL`.

### BuildOSWorkerDown
**Critical.** Prometheus can't scrape the **worker** target for >2m. The API still
serves, but **every background job has stopped** — daily briefings, field
notifications, delay cascades, corporate rollups, foresight sweeps — so this is a
silent functional outage, not a cosmetic one. Same triage as the server: orchestrator
(crashloop/OOMKilled/rollout), then the worker's `/health` (liveness) and `/ready` (DB
ping). Jobs that didn't run are not lost — River re-runs scheduled/periodic work once
the worker is back; check the River job tables for a backlog.

### BuildOSHTTP5xxRateHigh
**Critical.** >5% of HTTP responses are 5xx over 5m.
1. `/ready` first — a 5xx storm is most often the DB (down/saturated). 
2. Sentry for the panic/exception cluster; grab a `request_id`/`trace_id`.
3. Break down by route: `topk(10, sum by (route) (rate(buildos_http_requests_total{status="5xx"}[5m])))`
   — is it one route (a bug/dependency) or all (DB/infra)?
4. Correlate with a recent deploy — roll back if it lines up (see the deploy runbook).
`BuildOSHTTP5xxRateElevated` (warning) is the same signal at >1% / 15m — investigate
before it pages.

### BuildOSHTTPLatencyP95High
**Warning.** HTTP p95 >1s for 10m. The synchronous AI routes (daily-briefing / chat /
recommend-adjustments / invoices-ingest) are **already excluded** from this signal in
the rule (they're 5-60s by design), so a slow p95 here is real: DB contention / pool
saturation / a slow dependency. Break down by route. Check DB load and `DB_POOL_MAX`.

### BuildOSAuthRoute5xx
**Critical.** Any `/api/v1/auth/*` route (login, first-owner claim, refresh, password
reset) has returned 5xx sustained for 10m — **users may be locked out**. This fires
independent of overall traffic mix (a broken low-volume login would otherwise be
diluted below the global 5xx ratio). Check `/ready` (DB — the most common cause),
Sentry for the panic/exception, and break down by route:
`sum by (route) (rate(buildos_http_requests_total{route=~"/api/v1/auth/.*",status="5xx"}[5m]))`.

### BuildOSHTTP4xxRateElevated
**Warning.** >40% of responses are 4xx for 15m. Causes: a broken client build, an
expired-credential storm (401s), or — if the org never finished onboarding —
**SetupGate** 403ing operational routes with `SETUP_INCOMPLETE`. Confirm the cause in
logs (status is class-only here); for setup, check `GET /api/v1/setup/state` and
`/audit?action_prefix=setup.`.

### BuildOSAIErrorRateHigh
**Warning** (AI degrades gracefully — never breaks the deterministic core). >40% of
native Anthropic call **attempts** errored over 10m. The metric counts *attempts*, not
logical calls — the client retries up to 3×, so a retried-then-successful call still
emits error attempts; the 40% threshold absorbs ~1 retry per success. A sustained
breach is a real degradation:
1. Is the stored Anthropic key valid? A rejected/rotated key surfaces as 503
   `SERVICE_UNAVAILABLE` on AI endpoints — rotate it via Settings → Integrations.
2. Anthropic provider status (outage / rate-limited?).
3. Break down by kind+model: `sum by (kind,model) (rate(buildos_ai_requests_total{outcome="error"}[10m]))`.
Impact: the agentic surface (chat, daily briefing, foresight, invoice ingestion) is
degraded; the rest of the ERP is unaffected.

### BuildOSAILatencyP95High
**Warning.** Anthropic p95 >45s for 10m (the client caps near 60s). Almost always
upstream — check Anthropic status. Sustained, it risks request timeouts on the
synchronous AI endpoints.

### BuildOSRiverJobErrorRateHigh
**Warning.** >20% of River job-run **attempts** errored over 10m. The metric counts
*attempts*, not jobs — River retries, so a retried-then-successful job still emits
error attempts; a sustained ratio is systemic (DB, a downstream API, or a bug). Break
down by kind: `sum by (kind) (rate(buildos_river_job_runs_total{outcome="error"}[10m]))`,
then inspect the River job tables for the failing args + last error.

### BuildOSRiverJobsDiscarded
**Warning** (but it means a lost effect). River exhausted all retries and **gave up** —
the job's effect (a briefing, a field notification, a delay cascade, a rollup) did
**not** happen and won't retry. Find the discarded job in the River tables, fix the
root cause, and **re-enqueue** if the work still matters. A steady trickle of discards
is a bug to fix, not noise to mute.

> **Caveat — best-effort.** This alert is fed by River's buffered, non-blocking event
> subscription, so a discard event can drop under burst and the alert can miss. The
> **River job tables (`state = 'discarded'`) are the source of truth** — reconcile
> against them periodically (and after any worker restart/burst) rather than trusting
> the alert to catch every discard.

### BuildOSWorkerQueueBacklog
**Warning** — and the one `BuildOSWorkerDown` *can't* catch. The worker is **up** but
not draining: the oldest ready-to-run job has been `available` for >5m. Unlike the
job-outcome metrics (which go stale when nothing runs), this is a direct query on the
River tables, so it fires even when the worker is wedged. Causes: a jammed job pool,
advisory-lock contention, a stuck DB, or genuine overload. Check the worker logs, the
`buildos_river_queue_depth` gauge (how big), and the River `river_job` table
(`state='available'`, oldest `scheduled_at`). Restart the worker if wedged; scale or
tune `MaxWorkers` if it's real overload.

### BuildOSDBPoolNearExhaustion
**Warning.** >90% of the pgx pool's max connections (`buildos_db_pool_max_conns`) have
been in use for >5m. New requests queue for a connection — HTTP p95 climbs, then 5xx.
Check `buildos_db_pool_acquired_conns` vs `max`, hunt a slow query or a leaked
connection (a tx never committed/rolled back), and consider raising `DB_POOL_MAX` if the
load is legitimate. `/ready` (a pool `Ping`) is the binary backstop.

---

## 5. Tuning

Every threshold in `buildos.rules.yml` is a conservative default for a **low-traffic
single-tenant fork**: ratio alerts carry a minimum-traffic floor (so a couple of
errors during an idle window can't page), and there is deliberately **no "no
traffic" alert** (a small builder's fork is legitimately idle overnight). Raise/lower
the floors and ratios to match your fork's traffic, and route `critical` → paging,
`warning` → a non-paging channel.
