# Production Readiness Plan — BuildOS + The Brain

**Status:** DRAFT — owner sign-off pending
**Date:** 2026-04-30
**Author:** Claude (executing the L8 review the owner requested)
**Scope:** path from current branch state to a launchable single-customer fork of BuildOS connected to a production-deployed Brain
**Companion docs:** [ADR-001](./ADR-001-vision-alignment.md) (alignment decisions), [VISION_ALIGNMENT.md](./VISION_ALIGNMENT.md) (product framing), [SPRINT_PLAN.md](./SPRINT_PLAN.md) (sprint sequence)

This document is **primarily a BuildOS plan** with the Brain-side coordination called out where required. Brain is treated as an upstream dependency: items that affect the integration boundary are listed; items internal to Brain (e.g., OIDC consent UI styling) are owner-discretion.

---

## 1. Definition of "production-ready"

The product is production-ready when **all** of the following are true. Each criterion is verifiable; this list is what owner sign-off is checked against at launch.

### 1.1 Functional bar

- Every user story in [PRD.md](./PRD.md) §6 — all 28 — passes its Given/When/Then acceptance criteria against staging.
- The 6 user journeys (Morning Briefing, Procurement Lifecycle, Financial Review, Field Progress, Sub Coordination, Pre-Construction Pipeline) execute end-to-end against staging without manual intervention.
- The atomic Kanban → CPM transition produces a valid CPM-recalculated project on `permit_issued_date` advance for one prospect carried across the full pipeline (LEAD → QUALIFIED → ESTIMATE_SENT → VERBAL_COMMITMENT → PERMIT_APPLIED → PERMIT_ISSUED), with at least one estimate version and one permit row attached.
- LocalBlue → BuildOS lead auto-flow: a synthetic LocalBlue lead arriving via `/api/v1/a2a/webhook` lands as a `pre_construction_prospects` row at `stage='LEAD'` and produces an owner-targeted feed card.
- Multi-currency: every monetary endpoint accepts `?currency=USD` and `?currency=CAD`; cross-currency arithmetic returns `422 CROSS_CURRENCY_ERROR`.
- Field portal sync (Flutter) survives a 1-hour offline window and replays every queued action via `/api/v1/field/*` on reconnect with no duplicates.

### 1.2 Non-functional bar

| Dimension | Bar | Verifiable by |
|---|---|---|
| **Performance** | CPM ForwardPass+BackwardPass <200ms (80-task graph); API p95 <500ms; full schedule recalc <800ms; dashboard load p95 <3s | `make bench-physics`, k6 load test, RUM in staging |
| **Availability** | 99.9% uptime SLO; graceful degradation when Brain is unreachable | Health endpoint differentiates database-up vs Brain-up; status page; 30-day rolling SLO dashboard |
| **Reliability** | At-least-once A2A processing with idempotency dedup; outbound notifications retry up to 6 times (30s → 1hr backoff); failed deliveries go to DLQ | Integration tests against real DB (already green for inbound dedup); DLQ endpoint surfaces failures |
| **Security** | All inputs validated; no JWT bypass paths in `prod` build; SQL linter blocks composite-currency violations; Brain JWKS rotation tested; secrets never logged | Build-tag `prod` excludes header-mode bypass; CI runs `make audit`; static analysis (gosec); pen test report |
| **Observability** | Structured slog JSON; OpenTelemetry traces spanning OS → Brain hops; Prometheus metrics exported; Sentry error tracking; per-request request IDs correlate across both services | Staging dashboard rendering p95s; trace search by request_id; sample error in Sentry |
| **Data integrity** | Composite Currency Pattern enforced at DB (CHECK), migration linter (CI), Go convention (linter), TS (when frontend lands); RLS policies on `org_id` for co-op variants | `make audit` green; query for cross-tenant data returns zero rows |
| **Operations** | Runbooks for the top 5 incident types; on-call rotation set up; deploy and rollback procedures rehearsed; canary deploy for migrations | Runbooks in `.agents/handoff/runbooks/`; rehearsal logged |
| **Compliance** | AI usage metering accurate within ±1%; PII handling documented; audit-log retention defined; GDPR/CCPA basics covered | Metering reconciliation report; PRIVACY.md; audit-log schema |

### 1.3 Brain-side bar (required for BuildOS production)

- OIDC issuer reachable, JWKS rotation tested, MFA available.
- Wire-protocol values stable: either kept as legacy (`iss="fb-brain"`, `aud="fb-os"`) for life of the contract, OR migrated via dual-aud cutover per ADR-001 D4.
- A2A emitter populates `WebhookEvent.OrgID` for every event (current gap — see §3.B1).
- LocalBlue webhook routing live: chatbot lead → Brain → `localblue.lead_captured` A2A out (current gap — see §3.B5).
- Per-org isolation in Hub credential vault enforced.
- Stripe billing integration live (current gap — see §3.G).

---

## 2. Current state — branch `claude/optimistic-thompson-a2fdf9`

### 2.1 BuildOS — done

| Sprint | Status | Highlights |
|---|---|---|
| S0 walking skeleton | ✅ | Schema, JWT middleware, River setup, project CRUD scaffold |
| S1 CPM physics + schedule | ✅ | CPM engine, Gantt, ListTasks, UpdateTask wired (Sprint 1 was completed in PR `4da8d27`) |
| S1.5 Brain client foundation | ✅ | `internal/brain/` typed client with retry/backoff/auth; Maestro + Billing sub-clients; 13 tests |
| S2 Financials | ✅ | `internal/currency` package, BudgetService, all 6 financial endpoints, corporate_rollup River job |
| S3 Pre-Construction Pipeline | ✅ | Prospect/Estimate/Permit CRUD, atomic Kanban → CPM transition with `river.InsertTx`, analytics endpoint |
| S4 PR 1 A2A receiver | ✅ | JWS verify, idempotency dedup, 5 event handlers (review_material_quote, review_labor_bid, update_schedule, delivery_confirmation, create_feed_card) |
| S4 PR 2 LocalBlue lead | ✅ | `localblue.lead_captured` handler + per-event orgID extractor + prospect insert + feed card |
| Testcontainers infra | ✅ | `internal/testdb/`, 17+ integration tests across financials/pipeline/schedule/a2a stores |
| Rebrand | ✅ | FutureBuild OS → BuildOS, FB-Brain → The Brain across all docs + comments |
| ADR-001 vision alignment | ✅ | 16 strategic decisions documented |
| Module path rename | ✅ | `github.com/futurebuildai/buildos` |
| `cmd/dev-idp` | ✅ | Mock OIDC issuer for staging + sales demos with 4 personas |
| Alt-auth modes | ✅ | `DEV_AUTH_MODE=header` for dev/CI; mock IdP for staging |

### 2.2 BuildOS — gaps to production

| Gap | Sprint slot | Effort |
|---|---|---|
| **Outbound notifications retry queue + DLQ** | S4 PR 3 | ~1 PR, ~400 LOC |
| **AI agents** (DailyFocus, Procurement, SubLiaison) | S5 (3 PRs) | ~3 PRs, ~1500 LOC |
| **Feed handler + endpoints** | S5 PR 1 | ~1 PR, ~400 LOC |
| **Procurement endpoints + service** | S5 PR 2 | ~1 PR, ~600 LOC |
| **Fleet/HR + Field Sync** | S6 (3 PRs) | ~3 PRs, ~1200 LOC |
| **Lit web frontend** | S8 (separate repo) | weeks of work, see §4.C |
| **Flutter field portal** | S7 (separate repo) | weeks of work, see §4.C |

### 2.3 Brain — done

12 internal packages: `a2a`, `api/middleware`, `billing`, `clients`, `config`, `financial`, `hub`, `maestro`, `mcp`, `models`, `oidc`, `store`. Full OIDC IdP (zitadel/oidc), 7 MCP servers registered, A2A emit + receive, Maestro 3-tier classifier, billing metering with markup, financial engine, Hub credential vault. All tests pass on `main`. Plus the new `feat/localblue-lead-captured-event` branch (just pushed) adds the type + payload for the LocalBlue event.

### 2.4 Brain — gaps for BuildOS production

| Gap | Section ref | Effort |
|---|---|---|
| **`WebhookEvent.OrgID` envelope field + emitters populating it** | §3.B1 | ~1 PR Brain-side, small |
| **Wire-protocol cutover** (`iss`/`aud` → URL form per ADR D4) | §3.B2 | coordinated, medium |
| **LocalBlue webhook ingestion** (chatbot lead → Brain → A2A out) | §3.B5 | ~1 PR Brain-side, medium |
| **Per-org Hub credential isolation** | §3.E | ~1 PR Brain-side, medium |
| **Maestro task-type catalog stable contract** | §3.B4 | spec lock + 1 PR each side |
| **Stripe billing integration** | §3.G | larger; commercial |
| **Customer-fork OIDC client provisioning workflow** | §3.F | medium |
| **JWKS rotation playbook + automated rotation** | §3.F | small + ops doc |

---

## 3. Phase plan

Phases run partially in parallel — Phase A (server-side feature) and Phase D (observability/ops) can run together; Phase C (frontends) needs Phase A + B to settle. Critical path is **A → B → F → H**.

Effort estimates assume one engineer + Claude pair-programming at the cadence of the past two days (multiple PRs per day). Calendar time scales with availability.

### Phase A — Backend feature completion

| ID | What | Repo | Cross-repo? | Notes |
|---|---|---|---|---|
| **A1** | Sprint 4 PR 3 — `field_notification_dlq` table + retry River job (30s → 1hr backoff, 6 attempts) | BuildOS | no | Closes S4 exit criteria. |
| **A2** | Feed endpoints (`GET /api/v1/feed`, action, dismiss) | BuildOS | no | S5 PR 1 part 1. |
| **A3** | Procurement endpoints + ProcurementAgent (lead-time scan → WARNING/CRITICAL transitions) | BuildOS | uses Brain client | S5 PR 1 part 2. |
| **A4** | DailyFocusAgent (Maestro `daily_briefing` task → feed card per active project at 06:00 UTC) | BuildOS | uses Brain client | S5 PR 2. First real consumer of `internal/brain/`. |
| **A5** | SubLiaisonAgent (Twilio SMS confirmation flow) | BuildOS | no | S5 PR 3. |
| **A6** | Fleet endpoints + FleetService (allocation conflict detection via GiST exclusion constraint) | BuildOS | no | S6 PR 1. |
| **A7** | HR endpoints (employees + certifications) | BuildOS | no | S6 PR 2. |
| **A8** | Field sync endpoints (sync, progress, checkin, daily-log) with idempotency keys | BuildOS | no | S6 PR 3. |

**Phase A definition of done:** every API endpoint in [API_CONTRACT.md](./API_CONTRACT.md) returns real data from a real service (not `writeNotImplemented`); CI green for unit + integration tests; `make audit` green.

### Phase B — Brain integration deepening

| ID | What | Repo | Cross-repo? | Notes |
|---|---|---|---|---|
| **B1** | Brain `WebhookEvent` envelope: add `OrgID` field; update every emitter (MaterialsFlow, LaborFlow, post-approval) to populate it | Brain | yes | Removes BuildOS's `DefaultOrgID` fallback need for non-LocalBlue events; multi-tenant correctness. |
| **B2** | Wire-protocol cutover: dual-`aud` window per ADR D4. Brain emits `["fb-os", "buildos.api"]`; OS validates either; later drop legacy. | Both | yes | Two PRs each side, two deploys per side. |
| **B3** | BuildOS `internal/brain/Client.Hub` sub-client: `tools/call` against Brain's `/mcp` endpoint per ADR D6 | BuildOS | reads Brain MCP | Unblocks 3p integrations called from OS code. |
| **B4** | Maestro task-type catalog: lock the set of `task_type` strings (daily_briefing, intent_classify, invoice_extract, procurement_recommend, tribunal_review). Add typed Go wrappers per task. | Both | spec lock + parallel PRs | Brain treats unknown task_type as 400; OS client never sends unknown. |
| **B5** | Brain LocalBlue webhook ingestion: receive lead from chatbot → emit `localblue.lead_captured` A2A | Brain | yes | Activates the moment-of-truth flow that's currently only testable via synthetic A2A. |
| **B6** | A2A outbound from BuildOS to Brain (approval callbacks): when a feed card is actioned (e.g., quote approved), BuildOS POSTs `/api/v1/a2a/webhook` on Brain | Both | yes | Closes the loop in MaterialsFlow / LaborFlow orchestrators. |

**Phase B definition of done:** end-to-end LocalBlue → Brain → BuildOS lead flow runs in staging with real LocalBlue payloads; quote/bid approval flows complete the round-trip to Brain.

### Phase C — Frontends

Two separate repos. Each is large; treat as independent workstreams.

| ID | What | Repo | Notes |
|---|---|---|---|
| **C1** | Lit web frontend — Sprint 8 deliverables: design tokens (GableLBM), atom/molecule/organism/page components, signals state, router, all 8 pages with live data, Lighthouse >90 perf / >95 a11y | new repo (`buildos-web` or in `BuildOS-main` `frontend/`) | Per existing SPRINT_PLAN S8 |
| **C2** | Flutter field portal — Sprint 7 deliverables: Drift schema, Outbox + SyncService, FCM, 4 screens (TaskList, DailyLog, CrewCheckin, SyncStatus), offline indicator, workmanager background sync | new repo (`buildos-mobile`) | Per existing SPRINT_PLAN S7 |

**Phase C definition of done:** web frontend deployable to Vercel/CloudFlare and renders all 8 pages against staging API; Flutter app testable on a real iOS + Android device, completes the field-progress journey offline + online.

### Phase D — Production hardening (parallel with A)

| ID | What | Repo | Effort |
|---|---|---|---|
| **D1** | OpenTelemetry instrumentation: span every HTTP request, every River job, every cross-service Brain call. OTLP exporter to a vendor (Honeycomb, Datadog, or self-hosted Jaeger). | both | 1 PR each side |
| **D2** | Prometheus metrics export at `/metrics` (request counts, latencies, river queue depth, CPM duration, feed card creation rate). | both | 1 PR each side |
| **D3** | Sentry SDK wired to capture panics + tagged errors; integration into existing slog. | both | small |
| **D4** | Health vs readiness split: `/health` is liveness (process up); `/ready` checks DB + Brain reachability + JWKS cache freshness. | BuildOS | small |
| **D5** | Per-handler rate limits (existing global limit too coarse for production). Auth-sensitive endpoints get stricter; A2A inbound stricter. | BuildOS | small |
| **D6** | Body size limits per-endpoint (currently global 1MB on A2A only). | BuildOS | small |
| **D7** | Audit log surface: an `audit_log` table that records every write op (org_id, user_sub, action, resource_id, before/after JSON). Service-layer middleware appends. | BuildOS | medium |
| **D8** | Build-tag harden: `//go:build !prod` on `DEV_AUTH_MODE=header` path so `go build -tags=prod` excludes the bypass entirely. Dockerfile uses `-tags=prod`. | BuildOS | small but important |
| **D9** | Migration linter coverage: extend `scripts/lint-migrations.sh` to additionally check (a) every new migration has a matching `*.down.sql`, (b) destructive ops (DROP TABLE, ALTER COLUMN drop) require an explicit comment opt-in. | BuildOS | small |

**Phase D definition of done:** dashboards visible for both services rendering p95 latency + error rate; trace shows OS → Brain hop with end-to-end request_id; `go build -tags=prod` produces a binary where `DEV_AUTH_MODE=header` has no effect.

### Phase E — Multi-tenant hardening — **DEFERRED** (per ADR-002)

**Status:** Removed from BuildOS critical path 2026-05-01 per
[ADR-002](./ADR-002-single-tenant-fork-model.md). BuildOS is single-tenant
per customer fork; tenant isolation = deployment isolation. The Brain remains
multi-tenant and retains its E2/E3 obligations.

| ID | What | Repo | Notes |
|---|---|---|---|
| ~~E1~~ | ~~Postgres RLS on BuildOS tables~~ | ~~BuildOS~~ | **Dropped.** Single-tenant fork → no cross-tenant attack surface. RLS would add ~3-10% query overhead for zero security benefit. If the future co-op variant ever ships, RLS lands on that variant specifically. |
| **E2** | Hub credential per-org isolation: Brain `hub_store` ensures all OAuth credentials and tokens are encrypted with a per-org-derived key, not a global key. Audit cross-org access rejection. | Brain | medium |
| **E3** | AI usage metering per-org rate limits: prevent runaway spend by capping daily/monthly token spend per org; soft warning at 80%, hard cutoff at 100% with override. | Brain | medium; ties to D15 from ADR-001 |

**Phase E definition of done (Brain-side only):** Hub credential vault never returns another tenant's credentials; AI metering caps work in staging.

### Phase F — Deployment + ops

| ID | What | Repo | Notes |
|---|---|---|---|
| **F1** | GitHub Actions CI: build, vet, lint, lint-migrations, test, test-integration, bench-physics. Status checks block PR merge. | BuildOS | medium |
| **F2** | GitHub Actions CD: on `main` push → build container → push to registry → deploy to staging Railway. Manual gate for prod. | BuildOS | medium |
| **F3** | Railway/DigitalOcean staging environment for both BuildOS and Brain (probably one per environment per service). | both | ops work |
| **F4** | Production environment: separate cluster, separate database, secrets via Railway vars or Doppler. | both | ops work |
| **F5** | Customer-fork template repo per ADR D8: `BuildOS-main` becomes a GitHub template; `gh repo create --template` flow documented. | BuildOS | medium |
| **F6** | `make brand` tooling per ADR D11: reads `branding/customer.yaml`, generates `branding/applied.go` build-info, swaps logo/favicon, rewrites display strings. | BuildOS | medium |
| **F7** | Migration runbook: how to apply, how to roll back, what to do if a migration fails mid-way (locked tables). | BuildOS | doc only, ~1 day |
| **F8** | Rollback playbook: containers tagged by SHA, prior-version one click roll-back, monitoring during the roll-back. | BuildOS | doc only |
| **F9** | Per-environment configs: `.env.example` already exists; add `.env.staging.example` and `.env.production.example` with the canonical variable set. | BuildOS | small |
| **F10** | OIDC client provisioning workflow: when a customer fork goes live, Brain provisions an OIDC client (client_id, client_secret) for the fork's BuildOS instance. CLI tool or admin endpoint. | Brain | medium |
| **F11** | JWKS key rotation: scheduled rotation every 90 days; old key kept for 24h grace; downstream services (BuildOS) re-fetch JWKS on rotation event. | Brain | medium |

**Phase F definition of done:** a fresh customer fork can be created from the template, branded, deployed to staging, registered with Brain, and serving real requests within an afternoon.

### Phase G — Compliance + commercial

| ID | What | Repo | Notes |
|---|---|---|---|
| **G1** | AI pricing tier definition (per ADR D15) — owner decides the bundled token allowance, overage rate, unlimited tier. | n/a | owner decision |
| **G2** | Stripe integration in Brain: per-org subscription, metered billing for AI overage, monthly invoice generation, PaymentMethod management. | Brain | larger; depends on G1 |
| **G3** | Privacy policy + data retention: PII fields cataloged (contact_name, contact_email, contact_phone, address); retention windows documented; deletion request workflow. | both | compliance work |
| **G4** | GDPR/CCPA basics: data-export endpoint per user, deletion endpoint per user, cookie consent on web frontend. | both | compliance work |
| **G5** | Audit log retention policy: 7-year retention for financial events, 1-year for non-financial. Cold storage strategy. | BuildOS | doc + ops |
| **G6** | SOC 2 readiness assessment (advisory) — what would it take? Gap report. | both | scoping |

**Phase G definition of done:** Stripe production billing live; privacy policy published; audit log retention enforced.

### Phase H — Pre-launch

| ID | What | Repo | Notes |
|---|---|---|---|
| **H1** | Load testing: k6 script simulating 100 concurrent users + 10 concurrent CPM recalcs + 50 concurrent feed reads. Target: p95 <500ms, zero 5xx. | both | medium |
| **H2** | Security audit (third party): OWASP top 10, JWT validation, JWS verification, RLS effectiveness. | both | external; scoping |
| **H3** | Penetration test (third party): authenticated + unauthenticated paths, A2A endpoint with malformed JWS, idempotency replay. | both | external |
| **H4** | Customer onboarding playbook: from initial contract to first user logging in. | n/a | doc + workflow |
| **H5** | First customer fork bootstrap: clone template, brand, register with Brain, seed demo data, verify smoke tests, hand off. | BuildOS | per-customer |
| **H6** | Production deploy dry run: full release pipeline from PR → staging → prod, with rollback rehearsed. | both | one-day exercise |
| **H7** | Status page for both services (statuspage.io or similar). | both | ops |
| **H8** | Runbooks for top 5 incidents: Brain down, database down, JWKS rotation, A2A backlog, AI overage spike. | both | doc |

**Phase H definition of done:** owner signs off; first customer is live in production.

---

## 4. Cross-repo coordination matrix

Items that touch both repos and need lockstep PRs:

| Item | BuildOS PR | Brain PR | Order |
|---|---|---|---|
| Wire-protocol dual-aud cutover (B2) | accept both in `AnyAudience` | emit both in token aud | OS first, then Brain, then drop legacy |
| `WebhookEvent.OrgID` (B1) | drop `DefaultOrgID` fallback (or keep optional) | add field + populate emitters | Brain first, OS adopts, then OS hardens |
| Maestro task-type catalog (B4) | typed wrappers per task in `internal/brain/maestro.go` | accept exactly the locked set; reject unknown | Brain first (validation), OS publishes wrappers |
| LocalBlue lead end-to-end (B5) | already done (PR `a566ad8`) | webhook ingestion + emitter wiring | Brain side now; OS already ready |
| A2A approval callbacks (B6) | feed-card action triggers OS-side A2A POST | receiver in Brain accepts callback events | Brain receiver first, OS emitter follows |
| Stripe billing (G2) | usage display from Brain's `/api/billing/usage` (already wired) | Stripe subscription + invoicing | Brain only; OS already a consumer |
| OIDC client provisioning (F10) | doc the flow; brand applies the issued client_id | admin endpoint or CLI | Brain implements, OS docs the flow |

---

## 5. Sequencing recommendation

The fastest path to a launchable single-customer fork:

```
weeks 1-3   Phase A (backend feature completion)   [unblocks customer-visible features]
            └─ A1 → A2 → A3 → A4 → A5 → A6/A7/A8 (parallelizable)

weeks 1-3   Phase D (production hardening) — runs parallel with A

weeks 3-5   Phase B (Brain integration deepening)
            └─ B1 must be done before B6 (org_id needed for callbacks)
            └─ B5 requires LocalBlue ops work outside this repo

weeks 4-8   Phase C (frontends) — can start once API stabilizes (~end of A)
            ├─ C1 web (3-4 weeks)
            └─ C2 mobile (4-6 weeks)

weeks 5-7   Phase F (deployment + ops) — needs working backend first

weeks 6-8   Phase E (RLS) — defense in depth; can also be later

weeks 7-9   Phase G (compliance + Stripe)

weeks 9-10  Phase H (pre-launch) — load test, audit, dry run, bootstrap
```

**Aggressive total: ~10 weeks to launch** with one engineer + Claude pair-programming.
**Realistic total: ~14 weeks** accounting for owner-side decisions (G1 pricing, F10 customer agreements), third-party scheduling (H2 security audit, H3 pentest), and frontend work that's bigger than backend per-feature.

---

## 6. Decision points (owner-only)

These can't move without owner input:

| # | Decision | Blocks |
|---|---|---|
| **1** | AI pricing tier (per-seat allowance, overage rate, unlimited tier) — ADR D15 | G1 → G2 → H |
| **2** | First customer's identity and timeline — affects whether we hard-code "single customer" vs "co-op" assumptions in the first deploy | E (RLS urgency), H4, H5 |
| **3** | OAuth client model: each customer fork = new OIDC client with separate client_id, OR shared client with org-routing? | F10 |
| **4** | Frontend hosting: Vercel vs CloudFlare Pages vs Railway-served from BuildOS Go server? | C1, F1-F4 |
| **5** | Self-hosted vs FutureBuild-managed for first customer — affects who runs Phase F | F2-F4, H5 |
| **6** | Wire-protocol cutover (B2) priority: ship soon vs leave forever? | B2 timing |
| **7** | Whether the Brain repo should be renamed to align with public branding (currently `futurebuild-brain`) — ADR-001 D3 says no | nothing immediate; cosmetic |

---

## 7. Risks

| Risk | Mitigation |
|---|---|
| **Anthropic API outage** affects Maestro path → daily briefings stop, intent classification falls back | Brain's 3-tier classifier already does regex fallback; staging tests run with API key absent to verify graceful behavior |
| **Brain JWKS rotation breaks BuildOS** if rotation isn't coordinated | F11 includes 24h grace window; OS auto-refreshes on `kid` mismatch |
| **LocalBlue contractor binding** requires LocalBlue-side config (`site_id` → `contractor_org_id`) outside our repo | Document the binding step in customer onboarding (H4) |
| **Customer fork drift** over time as customers diverge from upstream | Per ADR D9: minimize drift via `branding/` isolation; periodic upstream-merge prompts via `make upgrade-check` |
| **Database migration failures mid-run** (lock contention) | F7 runbook; pre-flight `EXPLAIN` on long-running migrations; pg_stat_activity check |
| **PII leakage via logs** | D8 build-tag harden ensures DEV_AUTH paths can't be reactivated in prod; structured logging avoids interpolated user data |
| **AI token-cost runaway** | E3 caps and hard cutoffs; alerting at 50%/80%/100% spend |
| **Cross-currency arithmetic bugs slipping past linter** | D9 extends linter; runtime `currency.SumByCurrency` aggregator returns map keyed by currency, never single int64 |
| **Single-tenant fork assumptions baked into the data layer** make co-op variant hard later | E1 RLS up-front, even if not strictly needed for first single-tenant launch — pays itself back in correctness |

---

## 8. Out of scope (for first production launch)

These are valuable but explicitly post-launch:

- GableLBM ↔ BuildOS direct procurement integration (currently MCP-proxied via Brain — works for v1)
- Mobile push notifications beyond FCM basics (no rich content, no actions)
- Maestro tool-use beyond the 5 locked task types
- Self-service customer onboarding (hands-on for first 5+ customers)
- Tribunal autonomous-decision engine (SPRINT_PLAN P1; defer to v2)
- pgvector AI features (semantic search, doc similarity) — listed in TECH_STACK but not used by any v1 feature
- LocalBlue chat conversational UI in BuildOS web frontend (currently leads land via webhook only)

---

## 9. What "ready to ship" means for the next commit

If the owner says "go" with this plan accepted, the immediate next slice is **A1 — Sprint 4 PR 3** (`field_notification_dlq` + retry queue). Tight scope, no cross-repo coordination, finishes Sprint 4. Then A2 (feed endpoints) which the agents in A3-A5 depend on.

The default cadence per the [ship-at-L8-quality memory rule](../../../../.claude/projects/-Users-colton-Desktop-futurebuild-os-v2/memory/feedback_ship_at_L8_quality.md) holds: ship without permission-asking when each PR is L8; pause and ask only on direction-changing decisions (e.g., "should we start Phase C now or finish Phase A first?").
