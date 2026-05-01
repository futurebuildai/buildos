# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**BuildOS** — a Go backend (the "system of execution") for residential construction project management. Multi-tenant, REST API + River job queue, with auth/AI/3rd-party APIs/billing all delegated to a proprietary central service called **The Brain** (see "The Brain dependency" below). The core domain object is a project schedule computed by a deterministic **Critical Path Method (CPM) physics engine**.

Companion frontends (Lit web, Flutter mobile for field surfaces) live in other repos. This repo is backend-only.

**Deployment model:** every customer gets their own forked BuildOS repo and instance — they own the core code and data. A possible future co-op variant runs multi-tenant within one BuildOS deployment for member organizations.

Status: Sprint 1. CPM engine, schedule service, auth middleware, worker registry, and migrations are real. Most other domain handlers (`internal/api/financials.go`, `pipeline.go`, `fleet.go`, `procurement.go`, `feed.go`, `field.go`) are intentional stubs from the Sprint 0 walking skeleton — don't mistake them for bugs.

## Common commands

```bash
make build              # builds bin/server and bin/worker
make build-migrate      # builds bin/migrate (separate target)
make test               # unit tests only (no Docker)
make test-integration   # integration tests via Testcontainers (Docker required)
make lint               # golangci-lint run ./...
make lint-migrations    # bash scripts/lint-migrations.sh — see "Composite Currency" below
make db-up              # docker compose up -d db (Postgres 16, port 5433)
make db-down
make migrate            # runs migrations up against $DATABASE_URL
make migrate-down       # rolls back
make bench-physics      # CPM benchmarks through tools/bench-gate (CI hard gate)
make audit              # lint-migrations + test + bench-physics — full pre-merge gate
```

Run a single Go test: `go test ./internal/physics/... -run TestCPMDeterminism -count=1`
Run a single benchmark: `go test -bench=BenchmarkCPM80Tasks -benchtime=10x ./internal/physics/...`
Run a single integration test: `go test -tags=integration -count=1 -run TestFinancialsStore_CreateInvoice_RoundTrip ./internal/store/...`

Local Postgres listens on **port 5433** (not 5432) to avoid conflicts. Default DSN is hardcoded into the `Makefile`.

Integration tests live behind the `//go:build integration` build tag. They spawn ephemeral Postgres 16 containers via Testcontainers, apply all `migrations/*.up.sql`, and tear down at test exit. The shared fixtures live in [internal/testdb](internal/testdb/) — call `testdb.NewPool(t)` from any test file with the integration tag and you get a freshly migrated pool with auto-cleanup.

## Architecture

Three binaries under `cmd/`:
- `cmd/server` — Chi HTTP API on `$PORT` (default 8080). Loads config, opens pgxpool, builds JWKS provider for JWT validation, mounts routes, graceful shutdown.
- `cmd/worker` — River job daemon. Same DB pool. Registers job kinds (DailyBriefing, ProcurementCheck, HydrateProject, DelayCascade, A2AWebhookDispatch, etc.) defined in [internal/worker/jobs.go](internal/worker/jobs.go).
- `cmd/migrate` — runs River's internal migrations first, then `migrations/NNN_*.up.sql` / `.down.sql` against `schema_migrations`.

Internal package layout:
- [internal/api](internal/api/) — Chi handlers, thin. [router.go](internal/api/router.go) is the single source of truth for routes and RBAC. Auth and RBAC middleware live in [internal/api/middleware](internal/api/middleware/).
- [internal/physics](internal/physics/) — the CPM engine. **Determinism is non-negotiable**: durations and times are integer nanoseconds, never floats. [cpm.go](internal/physics/cpm.go) does the forward/backward pass over a gonum DAG; [dhsm.go](internal/physics/dhsm.go) scales durations by GSF (Gross Square Footage); [swim.go](internal/physics/swim.go) applies weather adjustments. The golden-master test in `cpm_determinism_test.go` will catch any drift.
- [internal/service](internal/service/) — business logic. [schedule.go](internal/service/schedule.go) is the canonical pattern: open a pgx tx, load via store, run physics, persist via store, enqueue follow-up River jobs, commit. Don't enqueue outside the tx (no phantom jobs).
- [internal/store](internal/store/) — raw `pgx/v5` SQL. **No ORM**. Tenant isolation is per-query; every query filters by `org_id`.
- [internal/worker](internal/worker/) — River client setup and job arg types.
- [internal/models](internal/models/) — domain structs.
- [internal/config](internal/config/) — env loading.

Request flow for a schedule recalc: HTTP → JWT middleware → RBAC middleware → handler → `ScheduleService.RecalculateSchedule` (begins tx) → `ScheduleStore` loads tasks/deps → `physics.ForwardPass` + `BackwardPass` → `ScheduleStore.UpdateSchedule` writes `early_start`, `late_finish`, `total_float`, `is_critical` → if critical path changed, enqueue `DelayCascadeArgs` River job → commit.

## Composite Currency Pattern (hard CI gate)

**All monetary values are stored as `amount_cents BIGINT` paired with `currency_code VARCHAR(3) DEFAULT 'USD'`. No floats, ever.** Supported currencies: USD, CAD. Cross-currency arithmetic is forbidden — aggregations must group by `currency_code`.

[scripts/lint-migrations.sh](scripts/lint-migrations.sh) is a hard CI fail with no exemptions. It scans `migrations/*.up.sql` for two violations:
1. Forbidden types (`DECIMAL`, `NUMERIC`, `REAL`, `DOUBLE PRECISION`, `MONEY`, `FLOAT`) on columns matching `cost|price|amount|total|budget|cents|fee|payment|invoice|balance|revenue|expense`. The only allowed `DOUBLE PRECISION` columns are GPS coords (`gps_lat`, `gps_lng`, `lat`, `lng`).
2. Any `_cents` column without a `currency_code` column in the same `CREATE TABLE`.

Go convention: monetary fields end in `Cents` (e.g. `TotalActualCostCents`) with a sibling `CurrencyCode` field. Don't introduce `float64` for money.

## The Brain dependency

Every BuildOS deployment is a relying party of **The Brain**, a separate proprietary service operated by FutureBuild AI. Customers run their own forked BuildOS repo, but five surfaces stay owned by The Brain:

1. **OIDC identity provider** — login, MFA, password reset, JWKS. BuildOS only validates the resulting JWTs (RS256) against The Brain's `BRAIN_JWKS_URL` with a 5-minute key cache. Validation lives in [internal/api/middleware/auth.go](internal/api/middleware/auth.go); RBAC roles enforced locally are `owner` > `admin` > `superintendent` > `field_worker`.
2. **AI gateway (Maestro)** — BuildOS does not call Anthropic directly. AI features route through The Brain, which holds the API key, meters tokens, and applies markup.
3. **Hub credential vault** — per-tenant 3rd-party API keys (Gable, LocalBlue, …) are encrypted (AES-256-GCM) and stored in The Brain. BuildOS asks The Brain to make upstream calls; raw credentials never enter the fork.
4. **3rd-party API proxy** — Brain owns the upstream client integrations and resolves credentials per request.
5. **Billing engine** — AI markup, ecosystem transaction fees, and PO-routing brokerage fees accrue in The Brain.

A standalone BuildOS deployment with no Brain connection has no auth, no AI features, no 3rd-party integrations, and no cross-product workflows. The Brain is load-bearing, not optional.

The A2A webhook receiver uses JWS signature verification (not JWT) — different code path, different key. See [internal/api/a2a.go](internal/api/a2a.go).

### Alternative auth for non-production

Two mechanisms cover dev, CI, staging, and sales demos:

**`DEV_AUTH_MODE=header`** (dev / CI) — middleware reads claims directly from an `X-Dev-Auth: <sub>,<org_id>,<role>[,<plan_tier>]` request header instead of validating a JWT. Any role/org/persona can be exercised per request without infra. Implementation: [auth.go](internal/api/middleware/auth.go) `claimsFromDevHeader`. Leave the env unset (or `""`) in production.

**`cmd/dev-idp`** (staging / sales demos) — a mock OIDC issuer that mints real RS256 JWTs against an in-process JWKS. BuildOS treats it as a stand-in for The Brain; no middleware change. Endpoints: `GET /jwks`, `POST /token`, `POST /demo/login` (pre-seeded personas: alice/owner, bob/admin, carol/superintendent, dave/field_worker), `GET /personas`. Run with `make dev-idp` (binds `:8083`); point BuildOS at it via `BRAIN_JWKS_URL=http://localhost:8083/jwks` and `BRAIN_ISSUER_URL=http://localhost:8083`. Keypair regenerates on every restart, so JWKS cache (5 min) will lag briefly after a dev-idp restart.

Production-hardening TODO: build-tag gate the header path so `DEV_AUTH_MODE=header` cannot be reactivated by env flip on a prod binary. Until then, [cmd/server/main.go](cmd/server/main.go) logs a loud warning at startup if the env is set.

### Wire-protocol values still on legacy names

JWT `iss` is `"fb-brain"` and `aud` is `"fb-os"` — see [auth.go:184](internal/api/middleware/auth.go:184). These are wire-protocol contracts with The Brain and cannot be renamed unilaterally; coordinate with the Brain team before changing.

## Performance gates

`make bench-physics` enforces:
- `BenchmarkCPM80Tasks` ≤ **200ms**
- `BenchmarkCPM200Tasks` ≤ **500ms**

Failures block CI via [tools/bench-gate](tools/bench-gate/). When changing anything in `internal/physics/`, run `make bench-physics` locally before pushing.

## Dual-agent workflow

This repo is co-developed by two agents under `.agents/`:
- **Antigravity** designs specs (PRD, ARCHITECTURE, API_CONTRACT, DESIGN_SYSTEM, EXECUTION_PLAN) into `.agents/handoff/`.
- **Claude Code** (you) executes them as a zero-trust auditor. Full protocol in [.agents/claude_instructions.md](.agents/claude_instructions.md).

Practical implications when working here:
- The single source of truth for tool/library choices is [.agents/TECH_STACK.md](.agents/TECH_STACK.md). Do not introduce dependencies not listed there without flagging.
- API routes, schemas, and status codes must match `.agents/handoff/API_CONTRACT.md` exactly.
- If a spec is ambiguous, contradictory, missing, or impractical, **do not improvise**: write to `.agents/handoff/ESCALATION_LOG.md` and pause for the user.
- Anthropic Claude is the default and only AI provider. Open-source models are permitted only for on-device Flutter inference or domain-specific embeddings.

## Local dependency: futurebuild-brain

[go.mod](go.mod) ends with `replace github.com/futurebuild/futurebuild-brain => ../futurebuild-brain`. The sibling repo must be checked out at `../futurebuild-brain` for builds to resolve. If you see a missing-module error, that's the cause.
