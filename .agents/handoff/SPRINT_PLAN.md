# Sprint Plan

**System:** BuildOS (System of Execution)
**Pipeline Stage:** 08 - Implementation Plan
**Date:** 2026-04-02 (revised 2026-04-29 per [ADR-001-vision-alignment.md](./ADR-001-vision-alignment.md) D16)
**Status:** REVISED — see "Revision history" below
**Sprint Duration:** 2 weeks (Sprint 1.5 spike: 3 days)
**Total Sprints:** 8 + 1 spike + 1 frontend (S0, S1, S1.5, S2–S7, S8)
**Estimated Timeline:** ~18.5 weeks

### Revision history

| Date | Change | Rationale |
|---|---|---|
| 2026-04-29 | Inserted **Sprint 1.5 (Brain Client Foundation)**, 3-day spike, after Sprint 1 | ADR-001 D5/D6: every Brain call (Maestro, Hub, 3p clients) needs a typed retrying transport. Building this as scaffolding before Sprint 2 prevents N stub-and-rewrite cycles in later sprints. |
| 2026-04-29 | Swapped contents of **Sprint 4 ↔ Sprint 5**: A2A receiver moves from S5 to S4; AI Agents + Feed move from S4 to S5 | ADR-001 D14: LocalBlue → BuildOS lead auto-flow lands as an A2A webhook into the pre-construction pipeline (built in S3). Receiving infrastructure must follow the pipeline immediately, not two weeks later. Agents (DailyFocus etc.) need both Brain Maestro (S1.5) and feed cards from A2A (new S4) in place, so they fit naturally as S5. |

---

## Cross-System Dependency Map

```
The Brain Sprint 0 ──► BuildOS Sprint 0
  (OIDC issuer)        (JWT middleware — BLOCKED until Brain JWKS is live)

The Brain Sprint 3 ──► BuildOS Sprint 4
  (A2A Client)          (A2A webhook receiver — BLOCKED until Brain emits signed webhooks)

BuildOS Sprint 0 ──► BuildOS Sprint 1
  (Core schema + River)  (Physics engine needs project_tasks table)

BuildOS Sprint 1 ──► BuildOS Sprint 2
  (Schedule engine)      (Financial module references schedule data)

BuildOS Sprint 2 ──► BuildOS Sprint 3
  (Financial tables)     (Pre-construction estimates use Composite Currency Pattern)

BuildOS Sprints 0–6 ──► BuildOS Sprint 7
  (All backend)          (Lit Web Frontend — BLOCKED until all API endpoints exist)

BuildOS Sprints 0–5 ──► BuildOS Sprint 6
  (Backend + A2A)        (Flutter Field Portal needs sync + field endpoints)
```

---

## Sprint 0: Walking Skeleton (Weeks 1–2)

**Goal:** Authenticated API request to create a project succeeds. River queue processes jobs. JWT validated against The Brain.

### Dependencies

- **BLOCKED BY The Brain Sprint 0:** JWT middleware requires Brain's `/jwks` endpoint to be live
- **Mitigation:** Use static JWKS fixture for Week 1; switch to live Brain JWKS in Week 2

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | PostgreSQL schema: `organizations`, `users` (with `oidc_subject`) | `migrations/001_initial_schema.sql` | P0 |
| 2 | PostgreSQL schema: `projects`, `project_tasks`, `task_dependencies` | `migrations/001_initial_schema.sql` | P0 |
| 3 | River queue setup (migration, river_job tables) | `migrations/002_river_setup.sql` | P0 |
| 4 | River periodic jobs: initial 6 cron entries | `migrations/002_river_setup.sql` | P0 |
| 5 | Chi router with middleware stack | `internal/api/router.go` | P0 |
| 6 | JWT validation middleware (JWKS from The Brain, 1hr cache) | `internal/api/middleware/auth.go` | P0 |
| 7 | RBAC middleware (owner, admin, superintendent, field_worker) | `internal/api/middleware/rbac.go` | P0 |
| 8 | OpenTelemetry + Prometheus middleware | `internal/api/middleware/telemetry.go` | P1 |
| 9 | Project CRUD: `GET/POST/PUT /api/v1/projects` | `internal/api/projects.go` | P0 |
| 10 | pgxpool initialization + config | `internal/store/pool.go`, `internal/config/config.go` | P0 |
| 11 | River client initialization + worker registry | `cmd/worker/main.go`, `internal/worker/registry.go` | P0 |
| 12 | `cmd/server/main.go` — wires router, services, middleware | `cmd/server/main.go` | P0 |
| 13 | Docker Compose: PostgreSQL 16 + pgvector | `docker-compose.yml` | P0 |
| 14 | Dockerfile: multi-stage Go build | `Dockerfile` | P1 |
| 15 | CI pipeline: SQL migration linter (Composite Currency Pattern enforcement), go test, golangci-lint | `.github/workflows/ci.yml`, `scripts/lint-migrations.sh` | P0 |
| 16 | Makefile: build, test, lint, audit targets | `Makefile` | P0 |

### Exit Criteria

- `POST /api/v1/projects` with valid The Brain JWT returns 201
- `POST /api/v1/projects` with invalid/expired JWT returns 401
- RBAC: `field_worker` role cannot create projects (403)
- River worker starts and processes a no-op test job
- SQL migration linter passes (no forbidden types, no orphan `amount_cents`)
- CI pipeline passes: lint + test + migration check

---

## Sprint 1: CPM Physics Engine + Schedule (Weeks 3–4)

**Goal:** Deterministic schedule computation works end-to-end. Physics benchmarks pass CI.

### Dependencies

- **Depends on Sprint 0:** `project_tasks`, `task_dependencies` tables must exist

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | Physics: `cpm.go` — BuildDependencyGraph, ForwardPass, BackwardPass, TopologicalSort, DetectCycle | `internal/physics/cpm.go` | P0 |
| 2 | Physics: `dhsm.go` — Duration/Hours State Machine (integer nanoseconds) | `internal/physics/dhsm.go` | P0 |
| 3 | Physics: `swim.go` — Surface Weather Impact Model (multipliers) | `internal/physics/swim.go` | P0 |
| 4 | Physics: `scoping.go` — WBS template hydration | `internal/physics/scoping.go` | P0 |
| 5 | Physics: `equipment_validator.go` — Earthwork equipment constraints | `internal/physics/equipment_validator.go` | P0 |
| 6 | Determinism test: golden master (IMMUTABLE) | `internal/physics/cpm_determinism_test.go` | P0 |
| 7 | Benchmark tests: BenchmarkCPM80Tasks (<200ms), BenchmarkCPM200Tasks (<500ms) | `internal/physics/cpm_test.go` | P0 |
| 8 | Benchmark tests: BenchmarkDHSMPerTask (<1ms), BenchmarkSWIMPerTask (<5ms) | `internal/physics/dhsm_test.go`, `internal/physics/swim_test.go` | P0 |
| 9 | `make audit` with bench-gate tool | `Makefile`, `tools/bench-gate/main.go` | P0 |
| 10 | PostgreSQL schema: `task_progress` (with `idempotency_key`) | `migrations/003_schedule.sql` | P0 |
| 11 | Schedule endpoints: `POST /api/v1/projects/{id}/schedule/recalculate` | `internal/api/schedule.go` | P0 |
| 12 | Schedule endpoints: `GET /api/v1/projects/{id}/schedule/gantt` | `internal/api/schedule.go` | P0 |
| 13 | Schedule endpoints: `GET/PUT /api/v1/projects/{id}/tasks` | `internal/api/schedule.go` | P0 |
| 14 | ScheduleServicer: calls physics engine, persists results | `internal/service/schedule.go` | P0 |
| 15 | River jobs: `hydrate_project`, `delay_cascade` (transactional insert) | `internal/worker/hydrate_project.go`, `internal/worker/delay_cascade.go` | P0 |
| 16 | Weather cache table | `migrations/003_schedule.sql` | P1 |

### Exit Criteria

- `POST /api/v1/projects/{id}/schedule/recalculate` returns CPM result in <800ms (80-task graph)
- Physics engine benchmarks pass: CPM80 <200ms, CPM200 <500ms, DHSM <1ms, SWIM <5ms
- Golden master determinism test passes (identical output for same input)
- `make audit` passes in CI
- `delay_cascade` job triggers CPM recalculation when task progress changes critical path

---

## Sprint 1.5: Brain Client Foundation (3-day spike, end of Week 4)

**Goal:** A typed, retrying HTTP client for The Brain that all subsequent sprints reuse. Transport only — no business logic. Inserted per [ADR-001 D5/D6](./ADR-001-vision-alignment.md).

### Dependencies

- **Depends on Sprint 0:** Config wiring (`BRAIN_*` env vars), Auth middleware
- **No cross-system blocker:** built against `cmd/dev-idp` mock issuer or unit-tested with mock RoundTripper

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | `BrainClient` struct: timeouts, retries (3 attempts, exponential backoff), context propagation, structured slog logging | `internal/brain/client.go` | P0 |
| 2 | Maestro task envelope types: one Go type per `task_type` (`DailyBriefing`, `IntentClassify`, `InvoiceExtract`, `ProcurementRecommend`, `TribunalReview`) | `internal/brain/maestro.go` | P0 |
| 3 | `BrainClient.Maestro.Run(ctx, task)` typed entry point with task-type dispatch | `internal/brain/maestro.go` | P0 |
| 4 | Hub client method stubs per upstream (QuickBooks, Gable, LocalBlue, XUI, 1Build, Gmail, Outlook) — interface only, returns `ErrNotImplemented` | `internal/brain/clients.go` | P0 |
| 5 | Token-source helper for service-to-service auth Brain expects from BuildOS (HMAC-signed bearer or OIDC client_credentials, per Brain spec) | `internal/brain/auth.go` | P0 |
| 6 | Test scaffolding: `brain.NewMockClient` for downstream test injection; `httptest`-based fixtures | `internal/brain/mock.go` | P0 |
| 7 | `internal/brain/README.md` linking to ADR-001 D5/D6 + Brain-side `BRAIN_CLIENT_CONTRACT.md` | `internal/brain/README.md` | P1 |

### Exit Criteria

- `BrainClient` retries on 5xx and respects `context.Canceled`; unit test covers a mock RoundTripper that 503s twice then 200s
- Maestro `daily_briefing` task type compiles end-to-end: request marshal → POST → response unmarshal
- All Sprint 4–6 Brain calls land through `internal/brain/`, not via ad-hoc `http.Client`
- Mock client lets downstream sprints write tests without standing up `cmd/dev-idp`
- Doc rendered and linked from CLAUDE.md

---

## Sprint 2: Financial Module — Composite Currency Pattern (Weeks 5–6)

**Goal:** Full financial stack with multi-currency support. No floating-point anywhere.

### Dependencies

- **Depends on Sprint 1:** Financial aggregations reference project/schedule data

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | PostgreSQL schema: `project_budgets` (3 paired cents+currency_code columns, CHECK constraint) | `migrations/004_financials.sql` | P0 |
| 2 | PostgreSQL schema: `corporate_budgets` (with `currency_code`, UNIQUE per org+year+quarter+currency) | `migrations/004_financials.sql` | P0 |
| 3 | PostgreSQL schema: `ar_aging_snapshots` (with `currency_code`, one row per currency per date) | `migrations/004_financials.sql` | P0 |
| 4 | PostgreSQL schema: `invoices` (with `currency_code`) | `migrations/004_financials.sql` | P0 |
| 5 | Financial endpoints: `GET /api/v1/org/{orgID}/financials/summary?currency=USD` | `internal/api/financials.go` | P0 |
| 6 | Financial endpoints: `GET /api/v1/org/{orgID}/financials/ar-aging?currency=USD` | `internal/api/financials.go` | P0 |
| 7 | Financial endpoints: `GET /api/v1/org/{orgID}/financials/projects?currency=USD` | `internal/api/financials.go` | P0 |
| 8 | Financial endpoints: `GET /api/v1/projects/{id}/budgets` | `internal/api/financials.go` | P0 |
| 9 | Financial endpoints: `POST/PUT /api/v1/projects/{id}/invoices` | `internal/api/financials.go` | P0 |
| 10 | BudgetServicer: BIGINT arithmetic, cross-currency validation | `internal/service/budget.go` | P0 |
| 11 | CorporateFinancialsServicer: aggregation grouped by `currency_code` | `internal/service/corporate_financials.go` | P0 |
| 12 | River job: `corporate_rollup` (daily aggregation, separate rows per currency) | `internal/worker/corporate_rollup.go` | P0 |
| 13 | Go models with Composite Currency Pattern naming: `EstimatedCostCents` + `EstimatedCostCurrencyCode` | `internal/models/` | P0 |
| 14 | Cross-currency arithmetic guard (returns `CROSS_CURRENCY_ERROR` 422) | `internal/service/budget.go` | P0 |

### Exit Criteria

- `GET /financials/summary?currency=USD` returns USD-only aggregations
- `GET /financials/summary` (no filter) returns separate rows per currency
- `corporate_rollup` produces separate `corporate_budgets` rows for USD and CAD
- Attempting to sum USD + CAD invoices returns 422 `CROSS_CURRENCY_ERROR`
- All Go monetary fields follow `FieldCents` + `FieldCurrencyCode` naming
- SQL migration linter confirms all monetary columns have paired `currency_code`

---

## Sprint 3: Pre-Construction Pipeline (Weeks 7–8)

**Goal:** CRM pipeline works from lead entry through permit issuance. Kanban→CPM transition is atomic.

### Dependencies

- **Depends on Sprint 2:** Estimates use Composite Currency Pattern
- **Depends on Sprint 1:** Kanban→CPM transition creates a Project and runs physics hydration

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | PostgreSQL schema: `pre_construction_prospects` | `migrations/005_pipeline.sql` | P0 |
| 2 | PostgreSQL schema: `pre_construction_estimates` (with `currency_code`) | `migrations/005_pipeline.sql` | P0 |
| 3 | PostgreSQL schema: `pre_construction_permits` (with `fee_currency_code`) | `migrations/005_pipeline.sql` | P0 |
| 4 | PipelineServicer: prospect CRUD, stage validation | `internal/service/pipeline.go` | P0 |
| 5 | Pipeline endpoints: `GET/POST/PUT /api/v1/org/{orgID}/pipeline/prospects` | `internal/api/pipeline.go` | P0 |
| 6 | Pipeline endpoints: `POST .../prospects/{id}/advance` (stage transition) | `internal/api/pipeline.go` | P0 |
| 7 | Pipeline endpoints: `POST .../prospects/{id}/lose` | `internal/api/pipeline.go` | P0 |
| 8 | Kanban→CPM atomic transition: `TransitionToConstruction()` in single PostgreSQL transaction | `internal/service/pipeline.go` | P0 |
| 9 | Stage transition validation: LEAD→QUALIFIED→...→PERMIT_ISSUED (no skipping) | `internal/service/pipeline.go` | P0 |
| 10 | Pipeline endpoints: `POST/PUT .../estimates` | `internal/api/pipeline.go` | P0 |
| 11 | Pipeline endpoints: `POST/PUT .../permits` | `internal/api/pipeline.go` | P0 |
| 12 | Pipeline analytics: `GET /api/v1/org/{orgID}/pipeline/analytics` (grouped by currency) | `internal/api/pipeline.go` | P0 |
| 13 | River job: `pipeline_analytics` (daily weighted revenue recalculation) | `internal/worker/pipeline_analytics.go` | P1 |
| 14 | River job: `permit_issued_transition` (event-driven) | `internal/worker/permit_issued_transition.go` | P0 |
| 15 | Go models: Prospect, PipelineEstimate, Permit, PipelineStage enum | `internal/models/pipeline.go` | P0 |
| 16 | Store: prospect, estimate, permit queries + transition query | `internal/store/pipeline.go` | P0 |

### Exit Criteria

- Full pipeline flow: LEAD → QUALIFIED → ESTIMATE_SENT → VERBAL_COMMITMENT → PERMIT_APPLIED → PERMIT_ISSUED
- `PERMIT_ISSUED` advance creates Project, hydrates WBS, enqueues CPM — all in single transaction
- Attempting to skip stages (e.g., LEAD→PERMIT_ISSUED) returns 409 `INVALID_TRANSITION`
- Pipeline analytics returns separate revenue forecasts per currency_code
- `project_id` is NULL on prospect until PERMIT_ISSUED
- Lost prospects cannot be advanced

---

## Sprint 4: A2A Webhook Receiver + LocalBlue Lead Flow + Notifications (Weeks 9–10)

**Goal:** BuildOS receives and processes JWS-signed webhooks from The Brain. LocalBlue leads auto-flow into the pre-construction pipeline. (Pulled from former S5 per [ADR-001 D14](./ADR-001-vision-alignment.md).)

### Dependencies

- **BLOCKED BY The Brain Sprint 3:** Brain must be emitting valid JWS-signed webhooks
- **Mitigation:** Test with locally signed mock webhooks until Brain Sprint 3 deploys
- **Depends on Sprint 3:** `pre_construction_prospects` table must exist for the LocalBlue lead handler to land rows

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | A2A webhook receiver: `POST /api/v1/a2a/webhook` | `internal/api/a2a.go` | P0 |
| 2 | JWS verification using Brain's public key (from `/jwks`) | `internal/api/a2a.go` | P0 |
| 3 | Idempotency key deduplication (409 on duplicate) | `internal/api/a2a.go` | P0 |
| 4 | Event handler: `review_material_quote` → create feed card with quote data | `internal/api/a2a.go` | P0 |
| 5 | Event handler: `review_labor_bid` → create feed card with bid data | `internal/api/a2a.go` | P0 |
| 6 | Event handler: `update_schedule` → trigger CPM recalculation | `internal/api/a2a.go` | P0 |
| 7 | Event handler: `delivery_confirmation` → update procurement status | `internal/api/a2a.go` | P0 |
| 8 | Event handler: `create_feed_card` → insert feed card | `internal/api/a2a.go` | P0 |
| 9 | Event handler: `localblue.lead_captured` → insert into `pre_construction_prospects` (stage='lead', source='localblue') and emit a feed card | `internal/api/a2a.go` | P0 |
| 10 | River job: `a2a_webhook_dispatch` (outbound webhooks from OS to Brain) | `internal/worker/a2a_webhook_dispatch.go` | P0 |
| 11 | PostgreSQL schema: `field_notification_dlq` | `migrations/006_notifications.sql` | P1 |
| 12 | River job: `field_notification_retry` (backoff: 30s→1hr, 6 retries) | `internal/worker/field_notification_retry.go` | P1 |
| 13 | Verify: `currency_code` from A2A payloads propagated to feed cards and procurement items | Integration test | P0 |

### Exit Criteria

- `POST /api/v1/a2a/webhook` with valid JWS signature returns 200
- Invalid JWS signature returns 401
- Duplicate idempotency key returns 409
- `review_material_quote` creates feed card with correct `currency_code`
- `update_schedule` triggers CPM recalculation via `delay_cascade` job
- `localblue.lead_captured` lands a prospect row visible in the contractor's pipeline view, with a feed card emitted to the appropriate role
- End-to-end: Brain MaterialsFlow → A2A webhook → OS feed card visible

---

## Sprint 5: AI Agents + Feed + Procurement (Weeks 11–12)

**Goal:** Autonomous agents generate feed cards. Procurement monitoring is active. All AI calls route through `internal/brain/` (Sprint 1.5) to The Brain's Maestro gateway. (Moved from former S4 per [ADR-001 D14/D16](./ADR-001-vision-alignment.md).)

### Dependencies

- **Depends on Sprint 1:** Agents reference project tasks and schedule data
- **Depends on Sprint 1.5:** All Maestro calls go through `internal/brain/maestro.go`
- **Depends on Sprint 2:** Procurement items use Composite Currency Pattern
- **Depends on Sprint 4:** A2A receiver provides upstream events that some agent flows react to (quote approval, etc.)

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | PostgreSQL schema: `procurement_items` (with `estimated_cost_currency_code`) | `migrations/007_agents.sql` | P0 |
| 2 | PostgreSQL schema: `feed_cards`, `communication_logs` | `migrations/007_agents.sql` | P0 |
| 3 | DailyFocusAgent: morning briefing generation via Maestro `daily_briefing` task type | `internal/agents/daily_focus.go` | P0 |
| 4 | ProcurementAgent: lead time scanning, WARNING/CRITICAL transitions | `internal/agents/procurement.go` | P0 |
| 5 | SubLiaisonAgent: SMS confirmation via Twilio | `internal/agents/sub_liaison.go` | P1 |
| 6 | FutureShade service + Tribunal consensus engine | `internal/futureshade/service.go`, `internal/futureshade/tribunal/` | P1 |
| 7 | Feed endpoints: `GET /api/v1/feed`, `POST /feed/{id}/action`, `POST /feed/{id}/dismiss` | `internal/api/feed.go` | P0 |
| 8 | Procurement endpoints: `GET/POST/PUT /api/v1/projects/{id}/procurement` | `internal/api/` | P0 |
| 9 | FeedServicer: card creation, targeting (user + role-based) | `internal/service/feed.go` | P0 |
| 10 | River jobs: `daily_briefing`, `procurement_check`, `sub_liaison_scan` | `internal/worker/` | P0 |
| 11 | River jobs: `certification_alerts`, `maintenance_reminders` | `internal/worker/` | P1 |
| 12 | WeatherServicer: Tomorrow.io API client (proxied through `internal/brain/clients.go` per ADR-001 D6) | `internal/service/weather.go` | P1 |
| 13 | InvoiceServicer: extraction via Maestro `invoice_extract` task type | `internal/service/invoice.go` | P2 |
| 14 | NotificationServicer: Twilio SMS, FCM push | `internal/service/notification.go` | P1 |

### Exit Criteria

- `daily_briefing` River job generates feed cards per active project at 06:00 UTC
- `procurement_check` transitions items to WARNING (7 days) / CRITICAL (3 days) based on `must_order_date`
- Feed cards are targeted by user ID or role; filterable by priority
- Feed action dispatches to appropriate service (e.g., approve quote → update procurement status)
- Procurement items display `estimated_cost_cents` + `estimated_cost_currency_code`
- All AI calls in agent code route through `internal/brain/maestro.go` — no direct Anthropic SDK usage

---

## Sprint 6: Fleet, HR + Field Sync Backend (Weeks 13–14)

**Goal:** Fleet management, employee tracking, and field sync API ready for Flutter.

### Dependencies

- **Depends on Sprint 0:** Core tables (organizations, users)
- **No cross-system dependencies**

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | PostgreSQL schema: `fleet_assets`, `equipment_allocations` (GiST exclusion) | `migrations/008_fleet_hr.sql` | P0 |
| 2 | PostgreSQL schema: `employees`, `certifications` | `migrations/008_fleet_hr.sql` | P0 |
| 3 | Fleet endpoints: `GET/POST /api/v1/org/{orgID}/fleet`, `POST .../fleet/{id}/allocate` | `internal/api/fleet.go` | P0 |
| 4 | HR endpoints: `GET /api/v1/org/{orgID}/employees`, `GET .../employees/{id}/certifications` | `internal/api/hr.go` | P0 |
| 5 | FleetServicer: allocation conflict detection (GiST exclusion constraint) | `internal/service/fleet.go` | P0 |
| 6 | EmployeeServicer: certification tracking | `internal/service/employee.go` | P0 |
| 7 | Field sync: `GET /api/v1/field/sync?since={timestamp}` | `internal/api/field.go` | P0 |
| 8 | Field sync: `POST /api/v1/field/progress` (with idempotency_key) | `internal/api/field.go` | P0 |
| 9 | Field sync: `POST /api/v1/field/checkin` (with idempotency_key) | `internal/api/field.go` | P0 |
| 10 | Field sync: `POST /api/v1/field/daily-log` (with idempotency_key) | `internal/api/field.go` | P0 |
| 11 | Field sync store: idempotency key validation, 409 on duplicate | `internal/store/field_sync.go` | P0 |
| 12 | River job: `resource_conflict_scan` (daily fleet conflict detection) | `internal/worker/` | P2 |

### Exit Criteria

- Equipment allocation returns 409 on overlapping date ranges (GiST exclusion)
- Certification expiry query returns certs expiring within 30 days
- `POST /api/v1/field/progress` returns 201 first time, 409 on duplicate idempotency_key
- `GET /api/v1/field/sync?since=...` returns notifications and task updates after timestamp
- All field endpoints accept `field_worker` role

---

## Sprint 7: Flutter Field Portal (Weeks 15–16)

**Goal:** Offline-first mobile app for field workers. Full sync lifecycle works.

### Dependencies

- **Depends on Sprint 6:** Field sync endpoints must be live
- **Depends on Sprint 0:** JWT auth for mobile login

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | Flutter project setup: pubspec.yaml, dependencies | `mobile/pubspec.yaml` | P0 |
| 2 | Drift database: Tasks table | `mobile/lib/database/tables.dart` | P0 |
| 3 | Drift database: Outbox table (actionType, payloadJson, idempotencyKey, syncStatus) | `mobile/lib/database/tables.dart` | P0 |
| 4 | Drift database: CachedBriefings table | `mobile/lib/database/tables.dart` | P1 |
| 5 | SyncService: pull sync (GET /field/sync?since=) | `mobile/lib/services/sync_service.dart` | P0 |
| 6 | SyncService: push outbox drain (FIFO, idempotency keys) | `mobile/lib/services/sync_service.dart` | P0 |
| 7 | SyncService: retry with backoff (1s, 2s, 4s, 8s, max 5min) | `mobile/lib/services/sync_service.dart` | P0 |
| 8 | AuthService: JWT token management (The Brain OIDC) | `mobile/lib/services/auth_service.dart` | P0 |
| 9 | PushService: FCM integration | `mobile/lib/services/push_service.dart` | P1 |
| 10 | TaskListScreen: assigned tasks with completion percentage | `mobile/lib/screens/task_list_screen.dart` | P0 |
| 11 | DailyLogScreen: weather, summary, safety, photos | `mobile/lib/screens/daily_log_screen.dart` | P0 |
| 12 | CrewCheckinScreen: GPS + worker selection | `mobile/lib/screens/crew_checkin_screen.dart` | P0 |
| 13 | SyncStatusScreen: pending count, last sync time, connectivity indicator | `mobile/lib/screens/sync_status_screen.dart` | P0 |
| 14 | connectivity_plus: network state detection | `mobile/lib/services/` | P0 |
| 15 | workmanager: background sync on connectivity restore | `mobile/lib/services/` | P0 |
| 16 | Offline mode: amber indicator, outbox count badge | UI | P0 |
| 17 | CI: flutter analyze + flutter test | `.github/workflows/ci.yml` | P0 |

### Exit Criteria

- Offline: task progress report saved to Outbox
- Online restore: workmanager drains Outbox in FIFO order with idempotency keys
- Duplicate outbox entry returns 409 from server, marked as synced
- Pull sync updates local Drift tables with server data
- Connectivity indicator: green (online), amber (offline + N pending)
- Flutter analyze: zero warnings

---

## Sprint 8: Lit Web Frontend (Weeks 17–18)

**Goal:** Full web application with all pages functional. Live data from all API endpoints.

### Dependencies

- **BLOCKED BY Sprints 0–6:** All backend API endpoints must be live
- **No cross-system dependency** (UI talks only to OS backend)

### Deliverables

| # | Task | Package | Priority |
|---|------|---------|----------|
| 1 | FBBaseElement (glassmorphism, glow, hover-lift, skeleton utilities) | `frontend/src/components/base/fb-element.ts` | P0 |
| 2 | GableLBM design tokens (CSS custom properties: Gable Green, Deep Space, Slate Steel) | `frontend/src/styles/variables.css` | P0 |
| 3 | Atom components: fb-button, fb-icon, fb-badge, fb-text, fb-input, fb-select, fb-chip, fb-spinner, fb-avatar | `frontend/src/components/atoms/` | P0 |
| 4 | Molecule components: fb-feed-card, fb-stat-card, fb-nav-item, fb-data-cell, fb-search-bar, fb-toast, fb-tab-bar, fb-breadcrumb | `frontend/src/components/molecules/` | P0 |
| 5 | Organism: fb-org-shell (app shell + sidebar navigation) | `frontend/src/components/organisms/fb-org-shell.ts` | P0 |
| 6 | Organism: fb-nav-sidebar | `frontend/src/components/organisms/fb-nav-sidebar.ts` | P0 |
| 7 | Organism: fb-data-table (sortable, paginated, JetBrains Mono for numbers) | `frontend/src/components/organisms/fb-data-table.ts` | P0 |
| 8 | Organism: fb-gantt-chart (CPM visualization with critical path highlight) | `frontend/src/components/organisms/fb-gantt-chart.ts` | P0 |
| 9 | Organism: fb-budget-summary (3 stat cards) | `frontend/src/components/organisms/fb-budget-summary.ts` | P0 |
| 10 | Organism: fb-ar-aging-chart (D3 stacked bar) | `frontend/src/components/organisms/fb-ar-aging-chart.ts` | P0 |
| 11 | Organism: fb-feed-list | `frontend/src/components/organisms/fb-feed-list.ts` | P0 |
| 12 | Pipeline: fb-pipeline-kanban (6-stage drag board) | `frontend/src/components/organisms/fb-pipeline-kanban.ts` | P0 |
| 13 | Pipeline: fb-pipeline-summary (weighted revenue by currency) | `frontend/src/components/organisms/fb-pipeline-summary.ts` | P0 |
| 14 | Pipeline: fb-prospect-detail, fb-estimate-form, fb-permit-tracker | `frontend/src/components/organisms/` | P0 |
| 15 | Page: fb-financials-view (budget summary + AR aging + project table) | `frontend/src/components/pages/` | P0 |
| 16 | Page: fb-schedule-view (Gantt chart + task list) | `frontend/src/components/pages/` | P0 |
| 17 | Page: fb-briefing-view (feed list) | `frontend/src/components/pages/` | P0 |
| 18 | Page: fb-procurement-view | `frontend/src/components/pages/` | P0 |
| 19 | Page: fb-pipeline-view (Kanban board + summary + analytics) | `frontend/src/components/pages/` | P0 |
| 20 | Page: fb-fleet-view, fb-hr-view, fb-settings-view | `frontend/src/components/pages/` | P1 |
| 21 | Client-side router | `frontend/src/router.ts` | P0 |
| 22 | Signals-based state management | `frontend/src/state/` | P0 |
| 23 | Currency display formatting (frontend-only, uses `currency_code` for symbol) | `frontend/src/utils/currency.ts` | P0 |
| 24 | Vite build + Lighthouse CI | `frontend/vite.config.ts` | P0 |

### Exit Criteria

- All 8 pages render with live data from API endpoints
- Gantt chart highlights critical path in Gable Green
- Financial views filter by currency (USD/CAD tabs or dropdown)
- Pipeline Kanban board supports drag-and-drop stage advancement
- Pipeline summary shows separate revenue totals per currency
- All numerical data fields render in JetBrains Mono
- Glassmorphism applied to all cards and panels
- Lighthouse: Performance >90, Accessibility >95
- Zero TypeScript strict-mode errors

---

## Sprint Summary

| Sprint | Weeks | Focus | Key Deliverables | Blocks / Blocked By |
|--------|-------|-------|-----------------|---------------------|
| S0 | 1–2 | Walking Skeleton | Core schema, JWT middleware, River setup, Project CRUD, CI | **Blocked by** Brain S0 (JWKS) |
| S1 | 3–4 | Physics Engine | CPM/DHSM/SWIM from vault, benchmarks, schedule API, `make audit` | Blocks S2 |
| **S1.5** | **end of 4** | **Brain Client Foundation (3-day spike)** | **Typed retrying `internal/brain/` package: BrainClient, Maestro envelopes, Hub stubs, mock client** | **Blocks S4, S5** |
| S2 | 5–6 | Financials | Composite Currency Pattern tables, endpoints, corporate rollup | Blocks S3 |
| S3 | 7–8 | Pre-Construction | Pipeline CRM, estimates, permits, Kanban→CPM transition | Blocks S4 (LocalBlue lead handler needs prospects table) |
| S4 | 9–10 | A2A + LocalBlue + Notifications | Webhook receiver, JWS verify, 6 event handlers (incl. `localblue.lead_captured`), notifications | **Blocked by** Brain S3 (A2A Client) |
| S5 | 11–12 | AI Agents + Feed | DailyFocus, Procurement, SubLiaison agents (via Maestro), feed cards, procurement | Depends on S1.5, S4 |
| S6 | 13–14 | Fleet/HR + Field Sync | Fleet, employees, certifications, field sync endpoints | — |
| S7 | 15–16 | Flutter Mobile | Drift, outbox, sync service, 4 screens, offline mode | Depends on S6 |
| S8 | 17–18 | Lit Web Frontend | All pages, pipeline Kanban, Gantt, design system, Lighthouse | Depends on S0–S6 |

### Cross-System Sprint Alignment

```
Week:    1    2    3    4    5    6    7    8    9   10   11   12   13   14   15   16   17   18
Brain:  [  S0  ][  S1  ][  S2  ][  S3  ][  S4  ][  S5  ]
OS:     [  S0  ][  S1  ]½[ S2  ][  S3  ][  S4  ][  S5  ][  S6  ][  S7  ][  S8  ]
         ▲              ▲                ▲
         │              │                │
    Brain OIDC     Brain client     Brain A2A Client
    unblocks       transport in     unblocks OS S4
    OS S0          place for S2+    (now A2A receiver)

       ½ = Sprint 1.5 (Brain Client Foundation, 3-day spike)
```

### Velocity Assumptions

- 2 backend engineers + 1 frontend engineer + 1 mobile engineer
- Sprint 0–6: backend-heavy (2 BE full-time, mobile engineer starts Sprint 7)
- Sprint 7: mobile-only (1 engineer, BE engineers on bug fixes and polish)
- Sprint 8: frontend-heavy (1 FE + 1 BE for API adjustments)
- Each sprint includes: code, unit tests, integration tests (Testcontainers), documentation

### Risk Register

| Risk | Sprint | Mitigation |
|------|--------|-----------|
| Brain OIDC delays block OS S0 | S0 | Static JWKS fixture for Week 1; live switch in Week 2 |
| Physics engine vault code doesn't compile | S1 | Allocate buffer for API modernization (gonum v0.15) |
| CPM benchmarks fail on CI hardware | S1 | Benchmark on consistent hardware; use relative gates |
| Brain A2A delays block OS S4 | S4 | Mock JWS signer for integration tests |
| Flutter offline sync edge cases | S7 | Use Testcontainers for simulated connectivity drops |
| Lit component count exceeds sprint capacity | S8 | Prioritize P0 pages; defer P1/P2 to post-MVP |
