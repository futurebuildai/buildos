# Phase 4b-ii — worker observability (worker `/metrics` + River job-outcome metrics)

**Status:** DONE — merged (fast-forward) + PUSHED to `origin/main` (HEAD `19fc7d3`). Go + the re-added River rules/docs. Built + verified via the ultra-loop (3-lens adversarial verification, 2 findings fixed).
**Completes the gap 4b-i documented.** Pairs with [PHASE_4B_I_ALERTING.md](./PHASE_4B_I_ALERTING.md). Plan: [PHASES_2-4_ULTRALOOP_PLAN.md](./PHASES_2-4_ULTRALOOP_PLAN.md) §"Phase 4".

## Why
4b-i found the worker emitted **no** metrics: `buildos_river_job_runs_total` was registered but `ObserveJob` had no caller, and `cmd/worker` served no `/metrics` — so every background job (briefings, notifications, delay cascades, rollups, foresight) was invisible to Prometheus. 4b-i shipped the River/worker alerts as *documented gaps*. 4b-ii closes them.

## What changed (Go)
- **`internal/worker/metrics.go` (new):** `jobOutcome(*river.Event)` maps a River **terminal** job state → `success` / `error` / `discarded` (`completed`→success; `retryable`→error, i.e. a failed *attempt* River will retry; `discarded`/`cancelled`→discarded; else skip). `(*Registry).ObserveJobMetrics(metrics, logger)` **subscribes** to `JobCompleted`/`JobFailed`/`JobCancelled` and runs `drainJobEvents` (extracted for testability) on a goroutine that records each via `metrics.ObserveJob` until the channel closes; returns the subscription cancel as `stop`. No change to the worker's public surface (uses the exposed `Registry.Client`).
- **`cmd/worker/main.go`:** builds `obs.NewMetrics()` once; passes `Metrics` into the worker's `ai.NewClient` (so worker-side `delay_cascade`/`foresight` AI calls now feed `buildos_ai_*`); calls `ObserveJobMetrics` (deferred stop) before `Start`; serves a small HTTP server on `$PORT` — `/metrics`, `/health` (liveness, no DB), `/ready` (`pool.Ping`, 2s) — mirroring the server's probes; graceful shutdown order is `Client.Stop(30s)` → `httpSrv.Shutdown(5s)`. A bind failure (e.g. `PORT` collision when co-located with the server) **fails fast + loud** via the run-loop select, never runs blind.
- **Tests:** `metrics_test.go` — `TestJobOutcome` (every state) + `TestDrainJobEvents` (synthetic channel + thread-safe observer; proves forward/skip/close-exit). The Go design (goroutine lifecycle, double-cancel safety, shutdown ordering, mapping completeness, thread-safety, single `NewMetrics`) was independently verified against River's source.

## Config/docs (re-added what 4b-i deferred)
- **`deploy/prometheus/buildos.rules.yml`:** re-added the river recording rules + the `buildos-jobs` group (`BuildOSRiverJobErrorRateHigh` — error *attempt* ratio >20%/10m + floor; `BuildOSRiverJobsDiscarded` — `increase(...discarded[15m])>0`, with a **best-effort caveat**) + `BuildOSWorkerDown` (`up{job="buildos-worker"}==0`, 2m, critical). 11 alerts, 9 records.
- **README + observability + deploy runbooks:** both processes scraped (jobs `buildos-server` + `buildos-worker`); five emitted metrics; `buildos_ai_*` now server+worker; worker probes; the dropped-event caveat + the "worker alive but not draining jobs" known gap.

## Verification (3-lens adversarial pass)
Go correctness: **sound** (verified vs River source — no leak, double-cancel safe, correct shutdown ordering, complete mapping, thread-safe). Docs accuracy: **clean** (every PromQL/anchor/link/claim verified). Two medium findings, both fixed: (1) the worker HTTP bind error was swallowed in the goroutine → now fails fast + a co-located-`PORT` note in the deploy runbook; (2) the dropped-event risk (River's non-blocking channel can drop a discard event → missed page) was code-only → now an operator-facing caveat in the rules file + runbook pointing at the River job tables as source of truth.

## Surfaced follow-ups (Go, separate chunks)
- `pgxpool.Stat()` → `buildos_db_pool_*` gauges; a per-error-code / SetupGate-rejection counter; a queue-depth / oldest-available-job gauge for the "worker alive but stuck" case.
- **4b-iii · error-path UX** (Retry-After on 5xx/429, AI circuit-breaker surfacing, `cmd/migrate --dry-run`).

## Gates
`make audit` ALL PASSED (incl. the new worker unit tests + test-prod + bench) · `make lint-isolation` PASSED · build/vet clean (default/prod) · `make test-integration` exit 0 · YAML valid (11 alerts / 9 records) · all runbook anchors + cross-file links resolve · `go test -race` clean (per the review).

## Definition of done
- [x] Spec (this file) · [x] feature branch; Go gates green (`make audit` + isolation + integration) + config/docs validated · [x] adversarial verification triaged + fixed · [ ] `/code-review` (owner) · [x] capability demonstrable (the worker now emits `buildos_river_job_runs_total` + serves `/metrics`; the River alerts have backing data) · [x] HANDOFF/NEXT_STEPS updated.
