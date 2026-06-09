# Phase 4b-i — Prometheus alerting rules + operator/deploy runbooks

**Status:** BUILT on `feat/phase-4b-i-alerting-runbooks` (committed, not pushed/merged). Config + docs only — **zero Go changes** (`make audit` unaffected). Awaiting owner review → merge.
**Plan:** [PHASES_2-4_ULTRALOOP_PLAN.md](./PHASES_2-4_ULTRALOOP_PLAN.md) §"Phase 4" chunk 4b. Pairs with the existing [dr-runbook.md](../../docs/dr-runbook.md) + [fork-onboarding.md](../../docs/fork-onboarding.md).

## What shipped
- **`deploy/prometheus/buildos.rules.yml`** — 4 groups, 7 recording rules + **8 alerts**, every expression referencing only the metrics the code actually emits. Alerts: `BuildOSServerDown` (crit), `BuildOSHTTP5xxRateHigh` (crit) / `…Elevated` (warn), `BuildOSHTTPLatencyP95High` (warn, AI routes excluded), `BuildOSHTTP4xxRateElevated` (warn), **`BuildOSAuthRoute5xx`** (crit — login/claim outage pages independent of traffic mix), `BuildOSAIErrorRateHigh` (warn) / `BuildOSAILatencyP95High` (warn). Tuned for a **low-traffic single-tenant fork** (min-traffic floors; no "no-traffic" alert).
- **`deploy/prometheus/README.md`** — the metric surface, scrape config (`job=buildos-server`), `promtool` validation, Alertmanager routing + inhibit-rule guidance, and the instrumentation-gap notes.
- **`docs/observability-runbook.md`** — the three signals (metrics/logs/traces) + the correlation trio, per-alert response (anchors match alert names), and the gaps.
- **`docs/deploy-runbook.md`** — `BUILDOS_ROLE` dispatch, config/secret sources, `/health` (liveness, no DB) vs `/ready` (DB ping) probes, migrate-before-roll + expand/contract, and **role-split graceful-shutdown timing** (server ≥15s, worker ≥30s).

## The load-bearing grounding finding
The metrics registry (`internal/obs/metrics.go`) is **custom** — no `go_*`/`process_*`/DB-pool metrics — and **only four metrics are actually emitted + scrapeable** (`buildos_http_*`, `buildos_ai_*`, both **server-only**). The fifth, `buildos_river_job_runs_total`, is **registered but never incremented** (`ObserveJob` has no caller), and **`cmd/worker` serves no `/metrics`**. So the plan's suggested DB-pool / setup-gate / River-job alerts have **no backing metric** — shipping them would have been dead rules (false confidence). They are documented as instrumentation gaps instead.

## Surfaced follow-up (the natural next 4b sub-chunk — a Go change, out of this config-only scope)
**Worker / background-job observability:** add a `/metrics` listener to `cmd/worker` + wire `ObserveJob` via a River middleware, then re-add a `buildos-jobs` alert group (error-rate + discards). Also: export `pgxpool.Stat()` as `buildos_db_pool_*` gauges; add a per-error-code (or SetupGate-rejection) counter. Background jobs (briefings, notifications, cascades, foresight) are currently observable only via logs/Sentry/the River tables.

## Built via the ultra-loop
Grounded → authored → **caught the worker/River dead-metric problem via direct code grounding** (corrected all four artifacts) → a 4-lens adversarial-verification workflow (PromQL validity, threshold sanity, runbook-vs-code accuracy, cross-file coherence) found 8 more config/docs issues (all verified against code, all fixed): AI routes polluting the HTTP p95 (now excluded); worker shutdown mis-documented as 15s (it's 30s); the AI error metric being per-*attempt* not per-call (reworded + threshold 25%→40%); added the auth-route 5xx alert; plus count/anchor/link/inhibit polish.

## Verification
YAML parses (4 groups / 15 rules); every PromQL expr references a real emitted metric with correct labels/`histogram_quantile`/`clamp_min`; all 7 `runbook_url` anchors resolve to runbook headings; all cross-file relative links resolve. `promtool check rules` is the operator-side gate (documented; not installable here — Prometheus's module has replace directives that block `go run`). No Go changes ⇒ `make audit` provably unaffected.

## Definition of done
- [x] Spec (this file) · [x] feature branch; config+docs gates green (YAML valid, anchors/links resolve, PromQL verified; no Go ⇒ `make audit` unaffected) · [ ] `/code-review` (owner) · [x] capability demonstrable (rules load + alert on the real metric surface) · [x] HANDOFF/NEXT_STEPS updated + the worker-observability follow-up seeded.
