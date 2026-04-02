# Scope Definition

**Date:** 2026-04-02
**Pipeline Stage:** 04 - Scope & Prioritization
**Status:** PAUSED AT APPROVAL GATE 1

---

## Walking Skeleton (Week 1-4)

The walking skeleton proves the architecture end-to-end with minimal functionality. It must demonstrate: Go backend with River queue -> Lit dashboard with GableLBM tokens -> Flutter app with offline storage.

| Component | Walking Skeleton Scope |
|-----------|----------------------|
| Backend | River queue migration (1 task type: daily_briefing). New /api/v1/org/financials endpoint returning mock CorporateBudget data. |
| Frontend | fb-org-shell with fb-org-nav. One view: fb-financials-view with fb-budget-summary card showing hardcoded data. GableLBM tokens applied. |
| Mobile | Flutter scaffold with Drift local database. One screen: task list with offline read. |
| Auth | JWT validation from FB-Brain (remove Clerk dependency). |
| CI/CD | GitHub Actions pipeline building all three targets. Includes BIGINT cents SQL migration linter (hard fail), CPM benchmark gate (`make audit`), and contract tests. |

**Exit Criteria:** User can log in via FB-Brain JWT, see a financial summary card in the Industrial Dark dashboard, and open the Flutter app to see a task list.

---

## MVP Definition (Week 5-20)

### MVP Feature Set

| # | Feature | Concepts | Priority | Weeks |
|---|---------|----------|----------|-------|
| M1 | Corporate Financials Dashboard | C3, C5A | P0 | 6 |
| M2 | CPM-res1.0 Preservation + API | C1 (base) | P0 | 3 |
| M3 | SWIM v2 (Tomorrow.io) | C2 | P0 | 4 |
| M4 | Agent Morning Briefing (Flutter) | C4 | P0 | 3 |
| M5 | Procurement Agent + Feed Cards | C6 (Level 1) | P0 | 4 |
| M6 | Fleet Asset Management | C3, C5C | P1 | 4 |
| M7 | HR Certification Tracking | C3, C5B | P1 | 3 |
| M8 | AIA Billing (G702/G703) | C5A | P1 | 4 |
| M9 | Resource Leveling (single project) | C1 | P1 | 4 |
| M10 | A2A Agent Cards | C4 | P2 | 3 |
| M11 | Pre-Construction Pipeline (CRM + Estimating + Permits) | New | P0 | 4 |

### M1: Corporate Financials Dashboard (P0, 6 weeks)

**Backend:**
- Migrate TypeCorporateRollup from Asynq to River
- Enhance RollupCorporateBudget to compute per-project variance
- Add /api/v1/org/{orgID}/financials/summary endpoint
- Add /api/v1/org/{orgID}/financials/ar-aging endpoint
- Add /api/v1/org/{orgID}/financials/projects endpoint (project-level breakdown)

**Frontend:**
- fb-org-shell with responsive layout (collapse nav on mobile)
- fb-financials-view with three sections:
  1. fb-budget-summary: Total Estimated / Committed / Actual with variance percentages
  2. fb-ar-aging-chart: D3.js stacked horizontal bar (Current / 30 / 60 / 90+)
  3. fb-project-financials-table: Sortable table with project name, estimated, committed, actual, variance, status. JetBrains Mono for numbers. Green/red variance coloring.
- Signals-based state management for real-time updates
- GableLBM tokens: Deep Space background, Gable Green accent, Glassmorphism cards

**Data Flow:**
```
TypeCorporateRollup (River, daily 04:00 UTC)
  -> CorporateFinancialsServicer.RollupCorporateBudget()
    -> Aggregates PROJECT_BUDGETS across all projects
    -> Writes CorporateBudget record
  -> CalculateARAging()
    -> Scans INVOICES for payment status
    -> Writes ARAgingSnapshot

/api/v1/org/{orgID}/financials/summary
  -> Returns latest CorporateBudget + ARAgingSnapshot
  -> Frontend renders fb-budget-summary + fb-ar-aging-chart
```

### M2: CPM-res1.0 Preservation + API (P0, 3 weeks)

**Backend:**
- Extract physics package from reference-vault into new codebase
- Preserve all existing code: cpm.go, dhsm.go, swim.go, scoping.go, equipment_validator.go
- Preserve cpm_determinism_test.go as golden master
- Add /api/v1/projects/{id}/schedule/recalculate endpoint
- Add /api/v1/projects/{id}/schedule/gantt endpoint (existing GetGanttData)
- Ensure ForwardPass material constraints work with ProcurementAgent

**Tests:**
- All existing golden master tests must pass without modification
- Add integration test: create project -> hydrate tasks -> run CPM -> verify critical path

### M3: SWIM v2 (Tomorrow.io) (P0, 4 weeks)

**Backend:**
- Create internal/weather/tomorrow_client.go implementing WeatherServicer
- Add hourly forecast caching (Redis or PostgreSQL, 6-hour TTL)
- Enhance ApplyWeatherAdjustment to accept hourly forecast array
- Calculate task-level weather risk score based on planned date range overlap with bad weather windows
- Maintain legacy static multipliers as fallback when API unavailable
- Add forecast data to DailyFocusAgent briefing prompt

**Config:**
```go
type TomorrowIOConfig struct {
    APIKey              string
    BaseURL             string // default: https://api.tomorrow.io/v4
    ForecastHorizonDays int    // default: 14
    CacheTTL            time.Duration // default: 6 hours
    RateLimitPerDay     int    // default: 500 (free tier)
}
```

### M4: Agent Morning Briefing on Flutter (P0, 3 weeks)

**Mobile:**
- Flutter app scaffold with Drift local database
- Push notification integration (Firebase Cloud Messaging)
- Feed card list view with priority sorting (critical > urgent > normal > low)
- Card actions: "View Details", "Order Now", "Call Sub", "Dismiss"
- Offline-capable: cache last briefing locally, display even without connectivity
- Bilingual support: English + Spanish for card content

**Offline Sync & A2A Retry Architecture (L8 Requirement):**

Field devices on construction sites frequently lose connectivity. Deterministic triggers (e.g., "Inspection Task Ready", "Material Delivery Confirmed") must NEVER be lost.

*Flutter Client-Side (Drift Outbox):*
- `outbox` table in Drift local SQLite stores all actions taken while offline (crew check-in, daily log, photo upload, card actions)
- Each outbox entry has: `id`, `action_type`, `payload_json`, `idempotency_key` (UUID v7), `created_at`, `sync_status` (pending/synced/failed)
- `workmanager` background task monitors `connectivity_plus` network state; drains outbox on connectivity restore
- FIFO ordering preserved; failed entries retry with exponential backoff (1s, 2s, 4s, 8s, max 5min)
- Server validates `idempotency_key` to prevent duplicate processing

*Server-Side (River Webhook Retry Queue):*
- Brain→OS A2A webhooks are server-to-server (always online) — no gap there
- OS→Flutter push notifications that fail delivery enqueue a River retry job: `TypeFieldNotificationRetry`
- River job uses exponential backoff: 30s, 1min, 2min, 5min, 15min, 1hr (max 6 retries over ~1.5 hours)
- After max retries, entry moves to `field_notification_dlq` table for manual review
- When field device reconnects and syncs, it pulls any missed notifications from a `/api/v1/field/sync` endpoint that returns all notifications since the device's last sync timestamp

### M5: Procurement Agent + Feed Cards + Tribunal Level 1 (P0, 4 weeks)

**Backend:**
- Migrate ProcurementAgent from Asynq to River
- Add Tribunal Level 1: when CRITICAL status detected, ConsensusEngine evaluates and generates recommendation card
- Feed card includes: item name, vendor suggestion, estimated cost, deadline, "Approve" / "Dismiss" actions
- "Approve" action triggers PO draft creation (manual execution by Sarah)

**Frontend:**
- fb-procurement-feed component in dashboard showing active procurement alerts
- Status badges: OK (green), WARNING (amber), CRITICAL (red), CONFIG_ERROR (gray)

### M11: Pre-Construction Pipeline — CRM, Estimating, Permit Tracking (P0, 4 weeks)

**Data Model:** Probabilistic/Kanban (NOT CPM). The pre-construction pipeline uses a stage-based Kanban data model with probability-weighted revenue forecasting. The deterministic CPM-res1.0 engine activates ONLY when "Permit Issued" milestone is reached.

**Backend:**
- `pre_construction_prospects` table: id, contact_name, contact_phone, contact_email, property_address, estimated_value_cents (BIGINT), currency_code (VARCHAR(3) DEFAULT 'USD'), source (referral/website/drive-by), pipeline_stage (LEAD/QUALIFIED/ESTIMATE_SENT/VERBAL_COMMITMENT/PERMIT_APPLIED/PERMIT_ISSUED), probability (INTEGER, 10-100), assigned_to, created_at, updated_at
- `pre_construction_estimates` table: id, prospect_id, line items with description, estimated_cost_cents (BIGINT), currency_code (VARCHAR(3)), category
- `pre_construction_permits` table: id, prospect_id, permit_number, municipality, application_date, expected_decision_date, permit_type, status, permit_fee_cents (BIGINT), currency_code (VARCHAR(3))
- All monetary columns follow the Composite Currency Pattern: `amount_cents BIGINT` + `currency_code VARCHAR(3)`
- `/api/v1/pre-construction/prospects` CRUD endpoint
- `/api/v1/pre-construction/estimates` CRUD endpoint
- `/api/v1/pre-construction/permits` CRUD endpoint
- `/api/v1/pre-construction/forecast` endpoint (weighted pipeline value by currency)

**Phase Transition Logic:**
```
When permit status = "approved" AND permit_issued_date is set:
  BEGIN TRANSACTION
    1. Set pipeline_stage = PERMIT_ISSUED, probability = 100
    2. INSERT INTO projects (permit_issued_date, ...)
    3. Enqueue TypeHydrateProject River job
    4. Link prospect_id to new project_id
  COMMIT
  -> River job fires CPM ForwardPass + BackwardPass
  -> ProcurementAgent begins lead-time monitoring
```

**Frontend:**
- fb-pipeline-kanban: Drag-and-drop Kanban board with 6 stage columns
- fb-pipeline-summary: Weighted pipeline value, prospect count, expected permits this month
- fb-prospect-detail: Prospect card with estimate, permit, and contact info
- fb-estimate-form: Line-item estimate builder with PDF export
- All components use GableLBM Industrial Dark tokens
- Currency display respects currency_code: USD -> "$", CAD -> "C$"

**Constraints:**
- Projects with `permit_issued_date IS NULL` cannot have CPM tasks — enforced by database constraint
- Pipeline forecasting groups monetary totals by currency_code — cross-currency aggregation is forbidden
- Kanban stage transitions are audited with timestamps for sales cycle analysis

---

## Post-MVP Phases

### Phase 2: Operations (Week 21-32)
- M6: Fleet Asset Management (dashboard + allocation calendar)
- M7: HR Certification Tracking (dashboard + alerts)
- M9: Resource Leveling (single project, equipment type)
- Sub Liaison Agent enhancement (auto-escalation after 24h no-response)

### Phase 3: Billing & Compliance (Week 33-40)
- M8: AIA Billing (G702/G703 PDF generation from budget data)
- QuickBooks GL Sync integration
- Certified payroll export (WH-347 form template)
- Invoice extraction accuracy improvements

### Phase 4: Advanced AI (Week 41-52)
- M10: A2A Agent Cards (FB-Brain integration)
- Tribunal Level 2: auto-order with 24h owner approval window
- Multi-project resource leveling
- SWIM v2 historical calibration
- Monte Carlo schedule risk analysis

### Phase 5: Scale & Polish (Week 53+)
- Tribunal Level 3: fully autonomous ordering within guardrails
- Samsara telematics integration
- Advanced reporting (custom dashboards, scheduled PDF reports)
- Payroll integration (Hammr API)
- Third-party agent marketplace (A2A protocol)

---

## Out of Scope (Explicitly Excluded)

| Item | Reason |
|------|--------|
| In-house payroll processing | Regulatory complexity; integrate with external provider |
| GPS hardware integration | Buy telematics data via API, not build hardware |
| Commercial construction | Focus on residential (1500-6000 GSF) |
| React frontend | Hard constraint: Lit only |
| Python backend logic | Hard constraint: Go only |
| ORM usage | Hard constraint: raw SQL via pgx |

---

## Dependencies

| Dependency | Blocker For | Owner | Status |
|-----------|-------------|-------|--------|
| FB-Brain JWT issuer endpoint | Walking skeleton auth | FB-Brain team | NEEDED |
| Tomorrow.io API key | M3: SWIM v2 | DevOps | NEEDED |
| River library evaluation | Walking skeleton queue | Backend team | NEEDED |
| Firebase project setup | M4: Flutter push | DevOps | NEEDED |
| GableLBM Figma designs | M1: Dashboard | Design team | NEEDED |
| Legal review: autonomous ordering | M5: Tribunal Level 1 | Legal | NEEDED |
