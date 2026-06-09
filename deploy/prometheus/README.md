# BuildOS — Prometheus rules & scraping

Operator artifacts for monitoring a BuildOS fork. Pairs with
[`docs/observability-runbook.md`](../../docs/observability-runbook.md) (what each
alert means + first response) and [`docs/deploy-runbook.md`](../../docs/deploy-runbook.md).

## Files

| File | What |
|---|---|
| [`buildos.rules.yml`](buildos.rules.yml) | Recording + alerting rules. Load via Prometheus `rule_files:`. |

## The metric surface (what BuildOS emits)

Only the **server** (`cmd/server`) exposes `/metrics` — it is the single process that
wires `obs.NewMetrics()`. The **worker** (`cmd/worker`) serves no HTTP and emits no
metrics today. So there is **one scrape target** (`buildos-server`). Metrics come
from a **custom registry** (`internal/obs/metrics.go`) — there are **no** `go_*` /
`process_*` / DB-pool metrics. The actually-emitted set is four:

| Metric | Type | Labels |
|---|---|---|
| `buildos_http_requests_total` | counter | `route`, `method`, `status` (`2xx`/`3xx`/`4xx`/`5xx`/`1xx`/`0xx`) |
| `buildos_http_request_duration_seconds` | histogram | `route`, `method` |
| `buildos_ai_requests_total` | counter | `kind`, `model`, `outcome` (`success`/`error`) — server-side AI only |
| `buildos_ai_request_duration_seconds` | histogram | `kind`, `model` |

`route` is the **chi route pattern** (e.g. `/api/v1/projects/{projectID}`), not the
raw URL — cardinality is bounded. `buildos_ai_*` covers only **server-side** AI calls
(chat / daily-briefing / recommend-adjustments / invoice-ingest); AI work done in the
worker (`delay_cascade`, `foresight`) is **not** counted (the worker has no metrics).

> **Instrumentation gaps (known follow-ups, each requires a Go change — out of scope
> for this config-only chunk):**
> - **Worker / background-job observability.** `cmd/worker` serves no `/metrics`, and
>   `buildos_river_job_runs_total` is registered but **never incremented**
>   (`ObserveJob` has no caller) — so job failures/discards and worker-side AI are
>   invisible to Prometheus. Needs a worker `/metrics` listener + `ObserveJob` wired
>   via a River middleware. **This is the highest-value next 4b sub-chunk.**
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
  - job_name: buildos-server                  # MUST be this name — the rules use
    metrics_path: /metrics                    # up{job="buildos-server"}
    static_configs:
      - targets: ["buildos-server:8080"]      # or k8s SD; one target per replica
  # No buildos-worker job — the worker exposes no /metrics (see the gaps note above).
```

The `BuildOSServerDown` alert matches `up{job="buildos-server"}`, so the server scrape
job MUST be named `buildos-server`.

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
