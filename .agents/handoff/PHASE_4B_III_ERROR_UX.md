# Phase 4b-iii — error-path UX + metric follow-ups

**Status:** BUILT on `feat/phase-4b-iii-error-ux` (committed, not pushed/merged). All gates green. Awaiting owner review → merge. **This is the last Phase-4 remainder** (4a/4b-i/4b-ii/4c already shipped).

## What this is
The grab-bag of operator/error-path hardening 4b-i/4b-ii deferred. Seven sub-features, grounded across the actual code via a parallel workflow, built coherently, then adversarially verified.

### Error-path UX
1. **Retry-After helpers** — `writeErrorResponseRetry` + `retryAfterSeconds` (ceil-seconds, min 1, RFC 7231) in `api/response.go`, refactored to share `writeErr` with `writeErrorResponse`.
2. **AI circuit-breaker → 503 + Retry-After** (the keystone). `circuitBreaker.allow()` now returns the remaining open window; the client returns a typed `*ai.CircuitOpenError{RetryAfter}` (`Is(ErrCircuitOpen)` true, so existing `errors.Is` checks keep working). `writeAIServiceError` (agents.go) + `writeIngestError` (invoice_ingest.go) **split** `ErrCircuitOpen` out of the `ErrTransient`→502 leg into a dedicated **503 `AI_CIRCUIT_OPEN` + Retry-After** (the remaining window, or `DefaultOpenDuration`=30s fallback) — honoring the long-documented "surface as 503" contract that was previously mis-mapped to 502.
3. **429 envelope alignment** — the rate limiter dropped its hand-rolled JSON literal for the shared `writeError` (consistent envelope + the metrics observer); kept `Retry-After: 1`; removed the dead Brain/Maestro/per-tenant-credit comments.

### Metric follow-ups (close the 4b-i/4b-ii gaps)
4. **`buildos_db_pool_*`** — `RegisterPoolGauges(closure)` → 4 `GaugeFunc`s from `pgxpool.Stat()` (acquired/idle/total/max). Closure form keeps `internal/obs` free of a pgxpool import. Wired in `cmd/server`.
5. **`buildos_http_error_responses_total{code,status}`** — a per-error-code counter (`SETUP_INCOMPLETE`, `AI_CIRCUIT_OPEN`, `RATE_LIMITED`, …) finer than the class-only HTTP status. Threaded into BOTH error writers (api + middleware) via package-level observers the router wires from `MetricsRecorder.ObserveErrorResponse`; reset to nil when metrics are off.
6. **`buildos_river_queue_depth` + `_oldest_available_seconds`** — the wedged-but-alive worker (up==1 but not draining), which `BuildOSWorkerDown` can't catch. `Registry.ObserveQueueDepth` samples `river_job` (`state='available'`) every 15s; wired in `cmd/worker` alongside `ObserveJobMetrics`.

### Tooling
7. **`cmd/migrate --dry-run`** — lists pending migrations without applying. Reworked the positional arg parse into `parseArgs` (the footgun: `migrate --dry-run` used to set `direction="--dry-run"` → the DOWN/rollback branch); gates BOTH river stages + the app-migration apply; `make migrate-dry-run` target.

### Alerts (the metric payoff)
- **`BuildOSWorkerQueueBacklog`** (oldest-available > 5m) — the wedged-worker alert 4b-ii said it couldn't write.
- **`BuildOSDBPoolNearExhaustion`** (acquired/max > 90%).
Runbook sections + the README metric table + the §3 "known gaps" updated (3 gaps now resolved).

## Review (`/code-review`-style, 2 adversarial verifiers)
Core logic verified **correct + test-backed**: no double-counting in the error-observer seam (each response → exactly one writer), no data race (observer set once at boot), the breaker split complete (`errors.Is`/`As` work through the service's `%w` wrap — pinned by a test), the queue-depth SQL valid against the River v0.32 schema (proven by a live-container test), dry-run truly non-mutating. **Fixed from the review:** a stale default-case comment, the test now wraps the error to exercise the through-`%w` path, and a stray `migrate` ELF binary was removed + `/migrate` added to `.gitignore`.

## Gates
`make audit` ALL PASSED · `make lint-isolation` (leaf intact) · build/vet (default + prod) · `make test-integration` (migrate dry-run + worker queue-depth + the app-migration runner) · new unit tests (Retry-After mapping, breaker duration, error/pool/queue metrics, `parseArgs` footgun) · rules YAML valid (15 alerts/6 groups) + anchors/links resolve.

## Definition of done
- [x] All 7 sub-features + the 2 alerts · [x] adversarial verification + fixes · [x] gates green · [x] docs (runbook/README/rules) · [ ] `/code-review` (owner) · [x] spec + HANDOFF/NEXT_STEPS. **Closes the Phase-4 backlog.**
