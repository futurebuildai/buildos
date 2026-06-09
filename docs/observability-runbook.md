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
| **Metrics** | `GET /metrics` (Prometheus) | server only (the worker emits none) | Custom registry — four emitted `buildos_*` metrics (see the README table). Unauth; restrict by network policy. |
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
for the four emitted metrics and their labels. **Only the server is scraped** — these
things you **cannot** alert on today (each needs a Go change — tracked as follow-ups,
out of scope for the config-only 4b-i chunk):

- **Worker / background jobs (the biggest gap).** `cmd/worker` runs every River job —
  daily briefings, field notifications, delay cascades, corporate rollups, foresight
  sweeps — but serves **no `/metrics`**, and `buildos_river_job_runs_total` is
  registered yet **never incremented** (`ObserveJob` has no caller). So job
  failures/discards and worker-side AI calls are **invisible to Prometheus**. Until
  it's wired (a worker `/metrics` listener + `ObserveJob` via a River middleware),
  watch background jobs through **logs**, **Sentry**, and the **River job tables**
  directly. *This is the highest-value next 4b sub-chunk.*
- **DB connection-pool exhaustion** — pgxpool's `Stat()` (acquired/idle/total conns,
  acquire wait) is not exported as a metric. Proxy signals: `/ready` failing (it
  `Ping`s the pool), rising HTTP p95, and `BuildOSHTTP5xxRateHigh`.
- **SetupGate over-rejection** — the gate 403s operational traffic with
  `SETUP_INCOMPLETE` until onboarding completes, but HTTP `status` is class-only
  (`4xx`), so this shows only as elevated 4xx (`BuildOSHTTP4xxRateElevated`), not a
  dedicated signal. Confirm via logs (`action`/error code) or `/audit?action_prefix=setup.`.

> **Follow-up to close these (one small Go chunk each):** a worker `/metrics` server
> + `ObserveJob` wiring (then re-add a `buildos-jobs` alert group); export
> `pgxpool.Stat()` as `buildos_db_pool_*` gauges; add a per-error-code (or
> SetupGate-rejection) counter. Filed against Phase 4b.

---

## 4. Alert response

Each alert's `runbook_url` deep-links to the matching section below. General first
moves on any alert: check the orchestrator (pod restarts / OOMKilled / recent
deploy), `/ready` (DB), Sentry, and the logs for the route/kind named in the alert.

### BuildOSServerDown
**Critical.** Prometheus can't scrape the target for >2m.
1. Is the process up? (`kubectl get pods` / your orchestrator — look for crashloop,
   OOMKilled, recent rollout.)
2. Is `/health` (liveness) 200? If the process is up but `/health` is unreachable,
   suspect the network policy / service routing.
3. Is `/ready` 200? A failing `/ready` means the **DB ping failed** — the process is
   alive but can't serve; check the database (see the DR runbook) and `DATABASE_URL`.
4. If only the **worker** target is down, the API still serves but background jobs
   (briefings, notifications, cascades) stop — escalate but it's not a full outage.

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

> **No River/worker alerts yet** — background-job failures are not alertable today
> (§3). Watch them via logs, Sentry, and the River job tables until worker
> observability is wired.

---

## 5. Tuning

Every threshold in `buildos.rules.yml` is a conservative default for a **low-traffic
single-tenant fork**: ratio alerts carry a minimum-traffic floor (so a couple of
errors during an idle window can't page), and there is deliberately **no "no
traffic" alert** (a small builder's fork is legitimately idle overnight). Raise/lower
the floors and ratios to match your fork's traffic, and route `critical` → paging,
`warning` → a non-paging channel.
