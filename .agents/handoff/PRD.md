# Product Requirements Document

**System:** BuildOS (System of Execution)
**Pipeline Stage:** 06 - Product Specification
**Date:** 2026-04-02
**Status:** COMPLETE

---

## 1. Product Overview

BuildOS is the AI-native operating system for residential general contractors managing 5-50 active projects in the $500K-$5M range (1500-6000 GSF). It covers the full project lifecycle from lead capture through construction completion — replacing the manual coordination of spreadsheets, phone calls, and disconnected tools with a probabilistic pre-construction pipeline, a deterministic physics engine, autonomous AI agents, and an offline-first field platform. The system supports multi-currency operations (USD and CAD).

### Vision Statement

Reduce administrative overhead by 60% while maintaining zero-tolerance schedule integrity through deterministic construction physics and graduated AI autonomy.

### Target Users

| Persona | Role | Primary Workspace | Platform |
|---------|------|-------------------|----------|
| Tom | Owner / GC | Portfolio Dashboard | Web Desktop |
| Sarah | Office Admin | Portfolio Dashboard + Agent Command Center | Web Desktop |
| Mike | Superintendent | Agent Command Center | Web Desktop + Tablet |
| Carlos | Field Worker | Field Portal | Flutter Mobile (offline-first) |

---

## 2. User Stories by Journey

### Journey 1: Morning Briefing Consumption (Mike)

**JTBD Reference:** J1 - "Help me know what to do today before I get to the job site"
**Scope Reference:** M4 (Agent Morning Briefing on Flutter)

#### US-1.1: Receive Push Notification for Daily Briefing

**As** Mike (Superintendent),
**I want** to receive a push notification when my daily briefing is ready,
**So that** I can review my day plan before leaving home.

**Acceptance Criteria:**

```
Given DailyFocusAgent has completed its 06:00 UTC run for Mike's assigned projects
  And Mike has the Flutter app installed with FCM push enabled
When the briefing is generated containing at least 1 feed card
Then Mike receives a push notification within 60 seconds of briefing completion
  And the notification body shows the count of projects and priority summary
  And the notification is delivered with >95% reliability via FCM
```

#### US-1.2: View Prioritized Feed Card Stack

**As** Mike,
**I want** to see my daily briefing as a prioritized stack of actionable cards,
**So that** I address critical items first.

**Acceptance Criteria:**

```
Given Mike opens the Flutter app after receiving a briefing notification
When the feed view loads
Then cards are sorted by priority: CRITICAL > URGENT > NORMAL > LOW
  And each card displays: title, project name, category icon, and timestamp
  And all text renders in the Outfit typeface for labels and JetBrains Mono for data values
  And the card stack is fully rendered within 2 seconds of app open
  And the background is Deep Space (#0A0B10) with glassmorphism card styling
```

#### US-1.3: Act on Weather Alert Card

**As** Mike,
**I want** to see weather-driven schedule impacts with actionable recommendations,
**So that** I can protect materials and adjust crew assignments.

**Acceptance Criteria:**

```
Given SWIM v2 detects >60% precipitation probability for a project site
  And the affected project has outdoor WBS tasks (7.0-9.6) scheduled within 48 hours
When the DailyFocusAgent generates the briefing
Then a WEATHER ALERT card is created with priority URGENT or CRITICAL
  And the card body includes: precipitation probability (%), temperature range, affected tasks, and recommended actions
  And the weather data is sourced from Tomorrow.io hourly forecast (not legacy static multipliers)
  And fallback to static SWIM multipliers occurs only when Tomorrow.io API is unreachable
```

#### US-1.4: Act on Procurement Alert Card

**As** Mike,
**I want** to see critical procurement deadlines in my morning briefing,
**So that** I can prevent material delays from slipping the schedule.

**Acceptance Criteria:**

```
Given ProcurementAgent has flagged an item with status CRITICAL
  And the item's MustOrderDate is within 7 days or has passed
When the briefing card is generated
Then the card displays: item name, vendor, estimated cost in USD formatted as "$X,XXX.XX" from BIGINT cents (cost_cents / 100)
  And the card includes "Order Now" and "Dismiss" action buttons
  And tapping "Order Now" triggers Tribunal ConsensusEngine evaluation
  And the cost is NEVER displayed using floating-point arithmetic — all formatting derives from int64 cents
```

#### US-1.5: Consume Briefing Offline

**As** Mike,
**I want** my last briefing to be available even without cellular connectivity,
**So that** I can review it in construction site dead zones.

**Acceptance Criteria:**

```
Given Mike has previously loaded a briefing with connectivity
When the device loses network connectivity (airplane mode or cellular dead zone)
Then the Flutter app displays the last-cached briefing from Drift local SQLite
  And all card content renders identically to the online version
  And a visible "Offline — Last synced: [timestamp]" indicator appears in Amber
  And any card actions taken offline are stored in the Drift outbox table with idempotency_key (UUID v7)
  And upon connectivity restore, workmanager drains the outbox in FIFO order
  And the server validates idempotency_key to prevent duplicate processing
```

#### US-1.6: Briefing in Spanish

**As** Carlos (Field Worker),
**I want** feed card content available in Spanish,
**So that** I can understand task assignments without language barriers.

**Acceptance Criteria:**

```
Given Carlos's Flutter app locale is set to es-US
When feed cards are displayed
Then card titles, descriptions, and action labels render in Spanish
  And data values (costs, dates, percentages) use locale-appropriate formatting
  And bilingual support covers English and Spanish only for MVP
```

---

### Journey 2: Procurement Lifecycle (Sarah + Mike + Tom)

**JTBD Reference:** J2 - "Help me prevent material delays from killing my schedule"
**Scope Reference:** M5 (Procurement Agent + Feed Cards + Tribunal Level 1)

#### US-2.1: Automatic Procurement Item Hydration

**As** the system,
**I want** to automatically populate procurement items when a project is created,
**So that** lead time monitoring begins immediately.

**Acceptance Criteria:**

```
Given a new project is created with permit_issued_date set
When TypeHydrateProject River job executes
Then ProcurementAgent.HydrateProject() creates procurement_items from the WBS template
  And each item has: name, wbs_code, estimated_cost_cents (BIGINT), lead_time_days, vendor_id
  And estimated_cost_cents is stored as int64 — no DECIMAL, FLOAT, or NUMERIC columns
  And the CPM ForwardPass calculates EarlyStart for all dependent tasks
  And MustOrderDate = EarlyStart - lead_time_days - weather_buffer_days
```

#### US-2.2: Procurement Status Transitions

**As** Sarah (Admin),
**I want** procurement items to automatically transition through WARNING and CRITICAL states,
**So that** I never miss a must-order deadline.

**Acceptance Criteria:**

```
Given ProcurementAgent daily cron runs at 05:00 UTC via River queue
When analyzeItem() evaluates each procurement_item
Then items with MustOrderDate within 14 days are set to WARNING
  And items with MustOrderDate within 3 days or passed are set to CRITICAL
  And a feed card is created for each status transition
  And notification dampening prevents duplicate cards within a 72-hour window (hasRecentCommunication check)
  And the feed card displays cost as "$X,XXX.XX" derived from BIGINT cents — never from float multiplication
```

#### US-2.3: Tribunal Level 1 Recommendation

**As** Mike,
**I want** the Tribunal to evaluate critical procurement items and recommend actions,
**So that** I can make faster decisions with AI-backed analysis.

**Acceptance Criteria:**

```
Given a procurement_item reaches CRITICAL status
When Tribunal ConsensusEngine evaluates the item
Then a recommendation card is generated with: item name, vendor suggestion, estimated_cost_cents formatted as USD, deadline, confidence score
  And the card provides "Approve" and "Dismiss" actions
  And Tribunal Level 1 is RECOMMEND ONLY — no autonomous ordering
  And the recommendation includes budget impact analysis (estimated_cost_cents vs remaining_budget_cents, both BIGINT)
  And all monetary comparisons use integer arithmetic — no floating-point variance
  And Tribunal accuracy target: >90% match with human decision over 100+ evaluations
```

#### US-2.4: Owner Approval for High-Value Orders

**As** Tom (Owner),
**I want** to approve procurement orders above my auto-approve threshold,
**So that** I maintain financial control over significant expenditures.

**Acceptance Criteria:**

```
Given a Tribunal-recommended order has estimated_cost_cents > Tom's auto_approve_threshold_cents
When the recommendation is created
Then a feed card is sent to Tom with: item name, total cost formatted from BIGINT cents, vendor, project name
  And Tom can tap "Approve" or "Reject" from both web dashboard and Flutter app
  And approval creates a PO draft with all line items in BIGINT cents
  And the PO draft is available for Sarah to finalize
  And recommendation-to-action time target: <4 hours
```

---

### Journey 3: Corporate Financial Review (Tom)

**JTBD Reference:** J3 - "Help me see if my company is making money across all projects"
**Scope Reference:** M1 (Corporate Financials Dashboard)

#### US-3.1: Portfolio Budget Summary

**As** Tom (Owner),
**I want** to see a real-time summary of estimated, committed, and actual costs across all projects,
**So that** I can assess company financial health in under 10 minutes.

**Acceptance Criteria:**

```
Given Tom logs in via The Brain JWT and lands on the Portfolio Dashboard
When fb-financials-view loads
Then fb-budget-summary displays: Total Estimated, Total Committed, Total Actual, and Variance
  And all monetary values are rendered from BIGINT cents (int64) divided by 100 for display
  And variance percentages are calculated as: ((actual_cents - estimated_cents) * 10000 / estimated_cents) / 100 — integer math, then format
  And positive variance (over budget) shows in Safety Red (#F43F5E)
  And negative variance (under budget) shows in Gable Green (#00FFA3)
  And all numerical values use JetBrains Mono typeface
  And the summary cards use glassmorphism styling (.glass-card: 60% opacity Slate Steel, 24px blur)
  And data is no older than 24 hours (sourced from TypeCorporateRollup daily cron at 04:00 UTC)
  And dashboard load time (p95) < 3 seconds for a 10-project portfolio
```

#### US-3.2: AR Aging Visualization

**As** Tom,
**I want** to see accounts receivable aging in a visual chart,
**So that** I can identify collection issues before they impact cash flow.

**Acceptance Criteria:**

```
Given CorporateFinancialsServicer.CalculateARAging() has produced an ARAgingSnapshot
When Tom views the AR Aging section
Then fb-ar-aging-chart renders a stacked horizontal bar with buckets: Current, 30-day, 60-day, 90+ day
  And each bucket amount is displayed in JetBrains Mono from BIGINT cents
  And the chart is rendered via D3.js with GableLBM color tokens
  And aging buckets match manual invoice categorization with >98% accuracy (quarterly audit)
```

#### US-3.3: Project-Level Financial Drill-Down

**As** Tom,
**I want** to drill into any project's financials by WBS phase,
**So that** I can identify which phases are over budget.

**Acceptance Criteria:**

```
Given Tom clicks a project row in fb-project-financials-table
When the project detail panel opens
Then phase-level breakdown shows: WBS code, phase name, estimated_cents, committed_cents, actual_cents, variance_cents
  And all monetary columns use JetBrains Mono with right-alignment
  And variance > 10% is highlighted in Safety Red
  And the table supports column sorting (by estimated, committed, actual, variance)
  And clicking "View Invoices" shows invoices matched to the selected WBS phase
```

#### US-3.4: AIA Draw Request Preparation

**As** Sarah (Admin),
**I want** G702/G703 forms auto-populated from CPM task completion and budget actuals,
**So that** I can prepare draw requests in under 30 minutes per project.

**Acceptance Criteria:**

```
Given Tom or Sarah clicks "Prepare Draw Request" on a project
When the AIA billing form loads
Then G702 fields are populated from: CorporateBudget totals, previous draws, current period work
  And G703 line items map to WBS phases with completion percentages from CPM task_progress records
  And all monetary values in the form are BIGINT cents formatted as USD
  And the form generates a PDF matching AIA standard format
  And preparation time target: <30 minutes per project (from current ~4 hours)
```

---

### Journey 4: Field Progress Reporting (Carlos)

**JTBD Reference:** J8 - "Help me report progress without typing"
**Scope Reference:** M4 (Flutter Field Portal)

#### US-4.1: View Today's Assigned Tasks

**As** Carlos (Field Worker),
**I want** to see my assigned tasks for today in a simple list,
**So that** I know exactly what work is expected.

**Acceptance Criteria:**

```
Given Carlos opens the Flutter app
When the task list view loads
Then tasks are filtered by: assigned_crew includes Carlos, scheduled_date = today, project = current project
  And each task shows: WBS code, task name, priority badge, completion percentage
  And the list is sorted by priority (critical first) then by WBS sequence
  And the task list is available offline from Drift local SQLite cache
  And total task list load time < 1 second
```

#### US-4.2: Report Task Progress with Photo

**As** Carlos,
**I want** to update task progress with a photo and a completion slider,
**So that** I can report in under 30 seconds without typing.

**Acceptance Criteria:**

```
Given Carlos taps a task card in the Field Portal
When he takes a photo and drags the progress slider
Then the photo is captured with GPS coordinates and timestamp metadata
  And the progress slider allows values from 0% to 100% in 10% increments
  And tapping the green checkmark creates a TaskProgress record with: percent_complete, reported_by, photo_asset_id, gps_lat, gps_lng, reported_at
  And a confirmation message shows the next dependent task from DependencyGraph
  And Mike receives a push notification: "[Carlos] completed [WBS code] [task name]"
  And total interaction time target: <45 seconds
```

#### US-4.3: Offline Progress Reporting

**As** Carlos,
**I want** to report progress even when I have no cellular signal,
**So that** site conditions don't prevent me from logging my work.

**Acceptance Criteria:**

```
Given Carlos's device has no network connectivity
When he submits a task progress update
Then the update is stored in the Drift outbox table with:
  - action_type: "task_progress"
  - payload_json: serialized TaskProgress data
  - idempotency_key: UUID v7 (generated client-side)
  - created_at: device local timestamp
  - sync_status: "pending"
And a "Pending sync" badge appears on the submitted task
  And the photo is stored in local device storage with a reference in payload_json
When network connectivity restores
Then workmanager background task detects connectivity via connectivity_plus
  And outbox entries drain in FIFO order with exponential backoff (1s, 2s, 4s, 8s, max 5min)
  And the server validates idempotency_key and rejects duplicates with HTTP 409
  And sync_status transitions: pending -> syncing -> synced (or failed after max retries)
  And the "Pending sync" badge clears upon successful sync
```

#### US-4.4: Crew Check-In

**As** Carlos,
**I want** to check in my crew at the start of each workday,
**So that** the superintendent has real-time site attendance.

**Acceptance Criteria:**

```
Given Carlos arrives at a project site
When he opens the Crew Check-in screen
Then the current project is auto-detected from GPS proximity or manual selection
  And Carlos can check in crew members from his assigned crew roster
  And each check-in records: worker_id, project_id, check_in_time, gps_lat, gps_lng
  And check-in works offline (stored in Drift outbox)
  And Mike sees crew attendance in the Agent Command Center within sync latency
```

---

### Journey 5: Sub Coordination (System + Mike)

**JTBD Reference:** J5 - "Help me coordinate subs without being a full-time phone operator"
**Scope Reference:** M5 (Procurement Agent, post-MVP Sub Liaison enhancements)

#### US-5.1: Automated Sub Confirmation Request

**As** the system,
**I want** to automatically send SMS confirmation requests to subcontractors 48 hours before their scheduled start,
**So that** Mike doesn't need to make manual phone calls.

**Acceptance Criteria:**

```
Given SubLiaisonAgent cron detects a task with EarlyStart within 48-72 hours
  And DirectoryService.GetContactForPhase returns a subcontractor contact
When the idempotency check confirms no SENT communication to this sub for this task in 24 hours
Then an SMS is sent via Twilio: "FutureBuild: Please confirm arrival for '[task name]' scheduled [date]. Reply YES to confirm."
  And a communication_log record is created with status PENDING, then updated to SENT
  And a feed card is created for Mike: "Awaiting confirmation from [sub name] for [task name]"
  And the card has priority NORMAL until 24 hours before the task, then URGENT
```

#### US-5.2: Parse Sub Response

**As** Mike,
**I want** the system to parse sub responses and flag delays automatically,
**So that** I only intervene when there's a problem.

**Acceptance Criteria:**

```
Given a subcontractor replies to the confirmation SMS
When HandleInboundMessage() processes the response
Then confirmation keywords ("yes", "confirmed", "will be there") create a LOW-priority feed card: "[Sub name] confirmed for [task name]"
  And delay keywords ("delayed", "can't make it", "postpone") create an URGENT feed card: "Delay reported by [sub name] for [task name]"
  And delay detection triggers createRiskFlag() on the affected task
  And percentage mentions (e.g., "60% done") update the task's progress record
  And unrecognized responses create a NORMAL card with the raw message for Mike's review
```

---

### Journey 6: Pre-Construction Pipeline (Tom + Sarah)

**JTBD Reference:** J9 - "Help me track every opportunity from first contact through permit issuance"
**Scope Reference:** M11 (Pre-Construction Pipeline — CRM, Estimating, Permit Tracking)
**Data Model:** Probabilistic/Kanban — transitions to deterministic CPM-res1.0 at Permit Issued milestone

> **Architectural Note:** Pre-construction phases (WBS 1.0-5.2) use a probabilistic Kanban data model with stage-based tracking and probability-weighted revenue forecasting. The deterministic CPM-res1.0 physics engine activates ONLY when the "Permit Issued" milestone is reached. This boundary is enforced at the database level: projects with `permit_issued_date IS NULL` cannot have CPM tasks created.

#### US-6.1: Lead/Prospect Entry (CRM)

**As** Sarah (Admin),
**I want** to enter a new prospect into the pre-construction pipeline,
**So that** I can track the opportunity from first contact through to permit.

**Acceptance Criteria:**

```
Given Sarah navigates to the Pre-Construction Pipeline view
When she creates a new prospect with: contact_name, contact_phone, contact_email, property_address, estimated_project_value_cents (BIGINT), currency_code (VARCHAR(3), default 'USD'), source (referral/website/drive-by), notes
Then the prospect is created with pipeline_stage = "LEAD" and probability = 10%
  And the prospect appears in the Kanban board under the LEAD column
  And estimated_project_value_cents is stored as BIGINT with accompanying currency_code
  And a created_at timestamp and assigned_to (default: Tom) are recorded
  And the Kanban board renders in Industrial Dark with Gable Green stage headers
```

#### US-6.2: Pipeline Stage Progression (Kanban)

**As** Tom (Owner),
**I want** to move prospects through pipeline stages on a Kanban board,
**So that** I can visualize my sales funnel and forecast revenue.

**Acceptance Criteria:**

```
Given Tom views the Pre-Construction Pipeline Kanban board
When he drags a prospect card between stages
Then the prospect transitions through stages in order:
  LEAD (10%) -> QUALIFIED (25%) -> ESTIMATE_SENT (50%) -> VERBAL_COMMITMENT (75%) -> PERMIT_APPLIED (85%) -> PERMIT_ISSUED (100%)
  And each stage has a default win probability used for weighted revenue forecasting
  And the pipeline summary header shows: total prospects, weighted pipeline value (sum of estimated_value_cents * probability / 100 — integer math)
  And stage transition timestamps are recorded for sales cycle analysis
  And the Kanban card shows: contact name, property address, estimated value formatted from BIGINT cents + currency_code, days in current stage
  And all monetary values display with correct currency symbol based on currency_code ('USD' -> '$', 'CAD' -> 'C$')
```

#### US-6.3: Preliminary Estimate Creation

**As** Tom (Owner),
**I want** to create a preliminary cost estimate for a prospect,
**So that** I can determine project feasibility and provide the homeowner a budget range.

**Acceptance Criteria:**

```
Given Tom opens a prospect in the pipeline
When he creates a preliminary estimate
Then the estimate form allows: line items with description, estimated_cost_cents (BIGINT), currency_code (VARCHAR(3)), category (materials/labor/equipment/permits/overhead)
  And estimate totals are calculated using integer arithmetic across all line items
  And the estimate is linked to the prospect record
  And creating an estimate auto-advances the prospect to ESTIMATE_SENT stage (if currently LEAD or QUALIFIED)
  And the estimate can be exported as a PDF with the FutureBuild letterhead
  And all currency formatting respects the prospect's currency_code
  And line item costs use the Composite Currency Pattern: amount_cents (BIGINT) + currency_code (VARCHAR(3))
```

#### US-6.4: Municipal Permit Tracking

**As** Sarah (Admin),
**I want** to track permit application status for each prospect,
**So that** I know exactly when permits are approved and construction can begin.

**Acceptance Criteria:**

```
Given a prospect has advanced to PERMIT_APPLIED stage
When Sarah records permit details
Then the permit tracker captures: permit_number, municipality, application_date, expected_decision_date, permit_type (building/electrical/plumbing/mechanical), status (applied/in_review/revision_requested/approved/denied), permit_fee_cents (BIGINT), currency_code (VARCHAR(3))
  And the system generates a daily feed card when expected_decision_date is within 7 days: "Permit decision expected for [address] — [municipality] [permit_type]"
  And revision requests create an URGENT feed card for Sarah
  And permit_fee_cents is stored with the Composite Currency Pattern (amount_cents + currency_code)
```

#### US-6.5: Permit Issued → CPM Activation (Phase Transition)

**As** the system,
**I want** to automatically transition a project from probabilistic Kanban to deterministic CPM scheduling when "Permit Issued" is confirmed,
**So that** the construction schedule is initialized with zero manual intervention.

**Acceptance Criteria:**

```
Given a prospect's permit status is updated to "approved"
When Sarah confirms permit issuance and enters permit_issued_date
Then the system performs the phase transition:
  1. Pipeline stage transitions to PERMIT_ISSUED (100% probability)
  2. A new PROJECT record is created with permit_issued_date set
  3. TypeHydrateProject River job fires to populate WBS tasks (6.0+) from template
  4. CPM ForwardPass + BackwardPass calculates the initial schedule
  5. ProcurementAgent begins lead-time monitoring from MustOrderDate calculations
  6. The project appears in the Portfolio Dashboard (M1) alongside active projects
  7. The pre-construction prospect record retains all CRM data (contact, estimates, permits) linked to the new project
And the phase transition is atomic — either all steps complete or none do (PostgreSQL transaction)
  And a feed card is generated: "🏗 [address] — Permit Issued. Construction schedule initialized. [X] tasks, critical path: [Y] days."
  And the CPM ForwardPass + BackwardPass for the new project completes in <200ms (80-task hard gate)
  And pre-construction estimates are available as reference data for budget variance analysis
```

#### US-6.6: Pipeline Revenue Forecasting

**As** Tom (Owner),
**I want** to see a weighted revenue forecast based on my pre-construction pipeline,
**So that** I can plan cash flow and resource allocation.

**Acceptance Criteria:**

```
Given Tom views the Pre-Construction Pipeline summary
When the forecast section loads
Then it displays:
  - Total Pipeline Value: sum of all prospect estimated_value_cents, formatted by currency_code
  - Weighted Pipeline Value: sum of (estimated_value_cents * probability / 100) per prospect, using integer math — separate totals per currency_code
  - Expected Permits This Month: count of prospects with expected_decision_date in current month
  - Average Sales Cycle: mean days from LEAD to PERMIT_ISSUED across historical data
And all monetary calculations use integer arithmetic with BIGINT cents
  And currency totals are grouped by currency_code (USD and CAD shown separately — NEVER summed across currencies)
  And the forecast renders in a glassmorphism summary card with JetBrains Mono for all numbers
```

---

## 3. Non-Functional Requirements

### NFR-1: Performance

| Requirement | Target | Measurement | Reference |
|-------------|--------|-------------|-----------|
| NFR-1.1: CPM ForwardPass + BackwardPass (80-task) | **<200ms** | `BenchmarkCPM80Tasks` — hard gate in `make audit` | L8 Standard |
| NFR-1.2: CPM ForwardPass + BackwardPass (200-task) | <500ms | `BenchmarkCPM200Tasks` — hard gate in `make audit` | L8 Standard |
| NFR-1.3: DHSM Duration Calculation (per task) | <1ms | `BenchmarkDHSMPerTask` | METRICS_FRAMEWORK |
| NFR-1.4: SWIM Weather Adjustment (per task) | <5ms | `BenchmarkSWIMPerTask` (includes cache lookup) | METRICS_FRAMEWORK |
| NFR-1.5: Full Schedule Recalculation (80-task, API) | <800ms | Prometheus histogram on `/api/v1/projects/{id}/schedule/recalculate` | METRICS_FRAMEWORK |
| NFR-1.6: Scoping + DependencyGraph Construction | <50ms | `BenchmarkGraphConstruction` | METRICS_FRAMEWORK |
| NFR-1.7: Dashboard Load Time (p95) | <3s | Frontend performance API on fb-financials-view | M1 Metrics |
| NFR-1.8: API Read Latency (p95) | <500ms | OpenTelemetry on Chi middleware | Technical Health |
| NFR-1.9: API Write/Compute Latency (p95) | <2s | OpenTelemetry on Chi middleware | Technical Health |
| NFR-1.10: Flutter Task List Load | <1s | App instrumentation | Field Portal |
| NFR-1.11: Largest Contentful Paint (LCP) | <2.5s | Web Vitals API | Technical Health |
| NFR-1.12: Cumulative Layout Shift (CLS) | <0.1 | Web Vitals API | Technical Health |

### NFR-2: Reliability

| Requirement | Target | Measurement |
|-------------|--------|-------------|
| NFR-2.1: API Uptime | >99.5% | 5xx rate over rolling 7 days |
| NFR-2.2: River Job Failure Rate | <1% | River job dashboard |
| NFR-2.3: River Job Queue Depth | <100 pending | Prometheus gauge, alert at >500 |
| NFR-2.4: Push Notification Delivery | >95% | FCM delivery receipts |
| NFR-2.5: Offline Sync Success Rate | >99% | sync_status transitions: pending -> synced |
| NFR-2.6: DailyFocusAgent Completion | <5 minutes for 50 projects | Agent instrumentation |
| NFR-2.7: ProcurementAgent Scan Cycle | <3 minutes | Agent instrumentation |

### NFR-3: Data Integrity

| Requirement | Specification | Enforcement |
|-------------|---------------|-------------|
| NFR-3.1: Composite Currency Pattern | All monetary values stored as `amount_cents BIGINT` + `currency_code VARCHAR(3) DEFAULT 'USD'`. No `DECIMAL`, `FLOAT`, `NUMERIC`, `REAL`, `DOUBLE PRECISION`, or `MONEY` types. Cross-currency arithmetic forbidden. | SQL Migration Linter: (a) forbidden types on monetary columns = hard CI fail; (b) `amount_cents` without `currency_code` in same table = hard CI fail |
| NFR-3.2: Go Struct Naming | All monetary fields end in `Cents` (e.g., `TotalActualCostCents`) | Go vet check / naming convention |
| NFR-3.3: TypeScript Lint | All monetary properties ending in cost/price/amount/total must use `Cents` suffix | ESLint custom rule |
| NFR-3.4: CPM Determinism | 100% bit-identical output across amd64/arm64 | `cpm_determinism_test.go` golden master — unmodifiable |
| NFR-3.5: Integer Nanosecond Math | DHSM uses int64 nanoseconds for duration calculation — no IEEE 754 drift | Architecture constraint |
| NFR-3.6: Idempotency Keys | All offline-originated operations carry UUID v7 idempotency key | Server rejects duplicates with HTTP 409 |

### NFR-4: Security

| Requirement | Specification |
|-------------|---------------|
| NFR-4.1: Authentication | JWT validation via The Brain JWKS endpoint — Clerk dependency fully removed |
| NFR-4.2: JWT Validation Latency | <10ms (JWKS cached locally with refresh on key rotation) |
| NFR-4.3: Token Claims | Access tokens carry: sub, org_id, role, plan_tier, iat, exp |
| NFR-4.4: A2A Webhook Verification | JWS signature verification on all inbound A2A webhooks from The Brain |
| NFR-4.5: SQL Injection Prevention | All queries via pgx parameterized queries — no string concatenation |
| NFR-4.6: RBAC | Role-based access: owner, admin, superintendent, field_worker |

### NFR-5: Offline & Sync

| Requirement | Specification |
|-------------|---------------|
| NFR-5.1: Drift Outbox Schema | `id`, `action_type`, `payload_json`, `idempotency_key` (UUID v7), `created_at`, `sync_status` |
| NFR-5.2: Client Retry | Exponential backoff: 1s, 2s, 4s, 8s, max 5min; FIFO ordering |
| NFR-5.3: Server Retry (Push) | River `TypeFieldNotificationRetry`: exponential backoff 30s, 1min, 2min, 5min, 15min, 1hr (6 retries) |
| NFR-5.4: Dead Letter Queue | After max retries, entry moves to `field_notification_dlq` table |
| NFR-5.5: Pull Sync Endpoint | `/api/v1/field/sync?since={timestamp}` returns all missed notifications |
| NFR-5.6: Connectivity Indicator | Mandatory Green/Amber indicator with pending queue count on all Field Portal surfaces |

### NFR-6: Accessibility & Internationalization

| Requirement | Specification |
|-------------|---------------|
| NFR-6.1: WCAG 2.1 AA | All web surfaces meet WCAG 2.1 Level AA |
| NFR-6.2: Contrast Ratios | Gable Green on Deep Space: 8.2:1 (AAA); text on Slate Steel: verified per token |
| NFR-6.3: Keyboard Navigation | All interactive elements reachable via keyboard |
| NFR-6.4: Bilingual Support | English + Spanish for Flutter Field Portal content |
| NFR-6.5: Screen Reader | All components use proper ARIA roles, labels, and live regions |

### NFR-7: Observability

| Requirement | Specification |
|-------------|---------------|
| NFR-7.1: Metrics Collection | Prometheus via OpenTelemetry on Chi middleware |
| NFR-7.2: Structured Logging | slog with JSON output; correlation IDs on all requests |
| NFR-7.3: Distributed Tracing | OpenTelemetry spans for cross-service calls |
| NFR-7.4: Dashboards | Grafana: System Health (real-time), Agent Performance (hourly), Business KPIs (daily) |
| NFR-7.5: Alerting | PagerDuty integration for: >0.5% 5xx rate, >500 River queue depth, >85% connection pool |

---

## 4. Traceability Matrix

Every P0 "Must Have" capability from SCOPE_DEFINITION.md is traced to specific user stories and NFRs.

### Walking Skeleton Traceability

| Walking Skeleton Component | User Stories | NFRs |
|---------------------------|-------------|------|
| River queue migration (daily_briefing) | US-1.1 | NFR-2.2, NFR-2.3 |
| /api/v1/org/financials endpoint | US-3.1 | NFR-1.8, NFR-3.1 |
| fb-org-shell + fb-financials-view | US-3.1, US-3.2 | NFR-1.7, NFR-1.11, NFR-1.12 |
| Flutter scaffold + Drift local DB | US-4.1, US-4.3 | NFR-5.1, NFR-1.10 |
| JWT validation from The Brain | US-3.1 (login step) | NFR-4.1, NFR-4.2, NFR-4.3 |
| CI/CD with BIGINT linter + CPM gate | — | NFR-3.1, NFR-1.1, NFR-3.4 |

### MVP Feature Traceability

| Feature | Scope Ref | User Stories | NFRs | JTBD |
|---------|-----------|-------------|------|------|
| M1: Corporate Financials Dashboard | SCOPE 4.1 | US-3.1, US-3.2, US-3.3, US-3.4 | NFR-1.7, NFR-3.1, NFR-3.2, NFR-3.3 | J3, J4 |
| M2: CPM-res1.0 Preservation + API | SCOPE 4.2 | US-2.1 (EarlyStart calc), US-4.2 (DependencyGraph) | NFR-1.1, NFR-1.2, NFR-1.3, NFR-1.5, NFR-1.6, NFR-3.4, NFR-3.5 | J1, J2 |
| M3: SWIM v2 (Tomorrow.io) | SCOPE 4.3 | US-1.3 | NFR-1.4 | J1 |
| M4: Agent Morning Briefing (Flutter) | SCOPE 4.4 | US-1.1, US-1.2, US-1.3, US-1.4, US-1.5, US-1.6, US-4.1, US-4.2, US-4.3, US-4.4 | NFR-2.4, NFR-5.1, NFR-5.2, NFR-5.3, NFR-5.4, NFR-5.5, NFR-5.6, NFR-6.4 | J1, J8 |
| M5: Procurement Agent + Tribunal L1 | SCOPE 4.5 | US-2.1, US-2.2, US-2.3, US-2.4, US-5.1, US-5.2 | NFR-2.6, NFR-2.7, NFR-3.1 | J2, J5 |
| M11: Pre-Construction Pipeline | SCOPE 4.6 | US-6.1, US-6.2, US-6.3, US-6.4, US-6.5, US-6.6 | NFR-1.1 (CPM gate on activation), NFR-3.1 (BIGINT cents + currency_code) | J9 |

### Post-MVP Feature Traceability (P1)

| Feature | Scope Ref | Target Phase | JTBD |
|---------|-----------|-------------|------|
| M6: Fleet Asset Management | SCOPE 5.1 | Phase 2 (Week 21-32) | J6 |
| M7: HR Certification Tracking | SCOPE 5.1 | Phase 2 (Week 21-32) | J7 |
| M8: AIA Billing (G702/G703) | SCOPE 5.2 | Phase 3 (Week 33-40) | J4 |
| M9: Resource Leveling | SCOPE 5.1 | Phase 2 (Week 21-32) | J1 |
| M10: A2A Agent Cards | SCOPE 5.3 | Phase 4 (Week 41-52) | J5 |

---

## 5. Data Model Constraints

### Monetary Data Rules — Composite Currency Pattern

All monetary fields in the system follow the **Composite Currency Pattern** — every financial value is stored as two columns:

1. **Storage:** PostgreSQL `BIGINT` column (`amount_cents`) paired with `VARCHAR(3)` column (`currency_code`, default `'USD'`). Example: `$1,234.56 USD` is stored as `amount_cents = 123456, currency_code = 'USD'`; `C$2,500.00 CAD` is stored as `amount_cents = 250000, currency_code = 'CAD'`
2. **Go type:** `int64` with field name suffix `Cents` + `string` with suffix `CurrencyCode`. Example: `TotalActualCostCents int64` + `TotalActualCostCurrencyCode string`
3. **TypeScript type:** `bigint` or `number` with property suffix `Cents` + `string` suffix `CurrencyCode`. Example: `totalActualCostCents: number` + `totalActualCostCurrencyCode: string`
4. **Display formatting:** Division by 100 occurs only at the presentation layer. Currency symbol is resolved from `currency_code`: `USD` -> `$`, `CAD` -> `C$`. Example: `formatCurrency(cents: number, code: string): string`
5. **Arithmetic:** All budget comparisons, variance calculations, and aggregations use integer math. **Cross-currency arithmetic is forbidden** — values with different `currency_code` MUST NOT be summed, compared, or subtracted. Aggregations must group by `currency_code`.
6. **Supported currencies:** `USD` (United States Dollar) and `CAD` (Canadian Dollar). Additional currencies require a schema migration and linter update.
7. **Enforcement:** SQL Migration Linter scans `migrations/*.sql` for: (a) forbidden types (`DECIMAL`, `FLOAT`, etc.) on monetary columns — hard CI fail; (b) any `amount_cents` column that lacks a corresponding `currency_code` column in the same table — hard CI fail. No exemptions.

### Physics Engine Constraints

1. **DependencyGraph:** Built with gonum/graph supporting 4 dependency types (FS, FF, SS, SF)
2. **Duration math:** Integer nanoseconds via DHSM — no IEEE 754 floating-point
3. **SWIM multipliers:** Applied as integer scaling factors (e.g., 115 = 1.15x)
4. **Golden master:** `cpm_determinism_test.go` is immutable — must pass unmodified from reference-vault
5. **Benchmark gates:** All physics benchmarks are integrated into `make audit` as hard CI gates

---

## 6. Dependencies and Risks

### External Dependencies

| Dependency | Required For | Risk | Mitigation |
|-----------|-------------|------|-----------|
| The Brain JWT Issuer | Walking Skeleton auth | Cross-team coordination | Define JWKS contract early; use mock JWKS for parallel development |
| Tomorrow.io API Key | M3: SWIM v2 | API availability, rate limits | Free tier (500 calls/day); fallback to legacy static multipliers |
| Firebase Project | M4: Flutter push | Push notification delivery | FCM is mature and reliable; offline outbox as backup |
| Legal Review | M5: Tribunal Level 1 | Autonomous ordering liability | Level 1 is recommend-only — no autonomous action |
| River Library | Walking Skeleton queue | Library maturity | River is PostgreSQL-native; community is active; reduces Redis dependency |

### Risk Register

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|-----------|
| CPM performance regression during River migration | Medium | High | `make audit` benchmark gates prevent merge if >200ms |
| Flutter offline sync data loss | Low | Critical | Drift outbox + idempotency keys + pull sync endpoint |
| Tomorrow.io rate limit exhaustion | Medium | Low | 6-hour cache TTL; circuit breaker at 450 calls; static fallback |
| Tribunal false positives on procurement | Medium | Medium | Level 1 is recommend-only; >90% accuracy gate before Level 2 |
| BIGINT enforcement bypass | Low | High | Multi-layer enforcement: SQL linter + Go struct + TS lint |

---

## 7. Graduation Criteria (Tribunal Autonomy)

| Transition | Required Metrics | Minimum Duration |
|-----------|-----------------|-----------------|
| Level 0 -> Level 1 (Recommend) | System deployed, Tribunal operational, >50 decisions logged | Walking skeleton complete |
| Level 1 -> Level 2 (Auto with approval) | Accuracy >90% over 100+ decisions, zero false positives on orders >$1,000, legal review complete | 30 days at Level 1 |
| Level 2 -> Level 3 (Fully autonomous) | Error rate <2% over 200+ orders, owner opt-in consent, insurance carrier approval | 90 days at Level 2 |

---

## 8. Out of Scope

| Item | Reason |
|------|--------|
| In-house payroll processing | Regulatory complexity; integrate externally |
| GPS hardware integration | Buy telematics via API |
| Commercial construction | Residential focus (1500-6000 GSF) |
| React frontend | Hard constraint: Lit 3.0 only |
| Python backend | Hard constraint: Go only |
| ORM usage | Hard constraint: raw SQL via pgx |
| Light mode UI | Dark-only: Industrial Dark is brand identity |
