# BuildOS — Prometheus rules & scraping

Operator artifacts for monitoring a BuildOS fork. Pairs with
[`docs/observability-runbook.md`](../../docs/observability-runbook.md) (what each
alert means + first response) and [`docs/deploy-runbook.md`](../../docs/deploy-runbook.md).

## Files

| File | What |
|---|---|
| [`buildos.rules.yml`](buildos.rules.yml) | Recording + alerting rules. Load via Prometheus `rule_files:`. |

## The metric surface (what BuildOS emits)

**Both** processes expose `/metrics` — the **server** (`cmd/server`: HTTP middleware +
the AI client) and, since 4b-ii, the **worker** (`cmd/worker`: a /metrics listener +
River job-outcome subscription + the AI client). So there are **two scrape targets**
(`buildos-server`, `buildos-worker`). Metrics come from a **custom registry**
(`internal/obs/metrics.go`) — there are **no** `go_*` / `process_*` metrics:

| Metric | Type | Labels | Emitted by |
|---|---|---|---|
| `buildos_http_requests_total` | counter | `route`, `method`, `status` (`2xx`/…/`5xx`) | server |
| `buildos_http_request_duration_seconds` | histogram | `route`, `method` | server |
| `buildos_http_error_responses_total` | counter | `code` (app error code), `status` | server |
| `buildos_ai_requests_total` | counter | `kind`, `model`, `outcome` (`success`/`error`) | server + worker |
| `buildos_ai_request_duration_seconds` | histogram | `kind`, `model` | server + worker |
| `buildos_river_job_runs_total` | counter | `kind`, `outcome` (`success`/`error`/`discarded`) | worker |
| `buildos_river_queue_depth` | gauge | — (available job count) | worker |
| `buildos_river_oldest_available_seconds` | gauge | — (age of oldest available job) | worker |
| `buildos_db_pool_*` | gauge | — (acquired/idle/total/max conns) | server |

`route` is the **chi route pattern** (e.g. `/api/v1/projects/{projectID}`), not the raw
URL — cardinality is bounded. `buildos_ai_*` now covers AI calls from **both** the
server (chat / daily-briefing / recommend-adjustments / invoice-ingest) and the worker
(`delay_cascade`, `foresight`). `buildos_river_job_runs_total`'s `outcome="error"` is
**attempt-level** (River retries; each failed attempt is an event; a terminal give-up
is `discarded`).

> **Instrumentation gaps (known follow-ups, each requires a Go change):**
> - **DB connection-pool exhaustion** — pgxpool's `Stat()` is not exported. Proxy:
>   `/ready` (it pings the pool) + the 5xx rate.
> - **SetupGate over-rejection** — HTTP `status` is class-only, so a 403
>   `SETUP_INCOMPLETE` storm shows only as elevated 4xx, not a dedicated signal.
>
> See [`docs/observability-runbook.md`](../../docs/observability-runbook.md) §3.

## Scrape config (example)

```yaml
# prometheus.yml
rule_files:
  - /etc/prometheus/rules/buildos.rules.yml   # mount buildos.rules.yml here

scrape_configs:
  - job_name: buildos-server                  # MUST be these names — the rules use
    metrics_path: /metrics                    # up{job="buildos-server"} / "buildos-worker"
    static_configs:
      - targets: ["buildos-server:8080"]      # or k8s SD; one target per replica
  - job_name: buildos-worker
    metrics_path: /metrics
    static_configs:
      - targets: ["buildos-worker:8080"]      # worker listens on $PORT too (separate pod)
```

The `BuildOSServerDown` / `BuildOSWorkerDown` alerts match `up{job="buildos-server"}` /
`up{job="buildos-worker"}`, so the scrape jobs MUST be named `buildos-server` and
`buildos-worker`.

`/metrics` is unauthenticated (Prometheus convention). **Restrict it at the
network layer** — a k8s NetworkPolicy / LB ACL allowing only the Prometheus
scraper — never expose it publicly.

## Validate before deploying

```bash
promtool check rules deploy/prometheus/buildos.rules.yml
# no promtool installed? run it ephemerally without adding a dependency:
go run github.com/prometheus/prometheus/cmd/promtool@latest check rules deploy/prometheus/buildos.rules.yml
```

Wire alerts to Alertmanager: route `severity: critical` to your paging channel
(PagerDuty/Opsgenie) and `severity: warning` to a non-paging channel (Slack/email
via the fork's Resend integration). Add an **`inhibit_rule`** so co-firing alerts on
the same signal don't double-notify: `BuildOSHTTP5xxRateHigh` (critical) should
inhibit `BuildOSHTTP5xxRateElevated` (warning, same 5xx signal), and
`BuildOSServerDown` should inhibit the other `buildos` alerts on the same `instance`
(a down target has no fresh data anyway).
