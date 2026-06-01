# UX Core Screens & Data Visualization Spec

**Document ID:** AG-FE-UX-CORE
**System:** BuildOS (System of Execution) — standalone / native pivot
**Created:** 2026-06-01
**Status:** DRAFT — research-grounded, pending product sign-off on open questions (§12)
**Authoring discipline:** Spec only. No application code. All data bindings cite real
endpoints in `internal/api/router.go` and fields in `internal/models/*.go` /
`internal/physics/cpm.go`. Where the backend still assumes "The Brain" but the product is
pivoting to native AI, the delta is flagged as an OPEN QUESTION rather than improvised.

---

## 0. Grounding: what the backend actually exposes today

Confirmed by reading `internal/api/router.go`, `internal/api/*.go`, `internal/models/*.go`,
`internal/physics/cpm.go`, `internal/physics/swim.go`, `internal/physics/dhsm.go`,
`internal/service/schedule.go`, `internal/service/agents.go`.

### 0.1 Route surface (authenticated unless noted)

| Domain | Method + Path | Handler | Role gate (from router.go) |
|---|---|---|---|
| Projects | `GET /api/v1/projects` | `projects.List` | all roles (read) — **STUB: `writeNotImplemented`** |
| Projects | `POST /api/v1/projects` | `projects.Create` | owner, admin — **STUB** |
| Projects | `GET /api/v1/projects/{projectID}` | `projects.Get` | all roles — **STUB** |
| Projects | `PUT /api/v1/projects/{projectID}` | `projects.Update` | owner, admin — **STUB** |
| Schedule | `POST /api/v1/projects/{projectID}/schedule/recalculate` | `schedule.Recalculate` | min superintendent |
| Schedule | `GET /api/v1/projects/{projectID}/schedule/gantt` | `schedule.Gantt` | all roles (read) |
| Schedule (AI) | `POST /api/v1/projects/{projectID}/schedule/recommend-adjustments` | `agents.RecommendScheduleAdjustments` | min superintendent + pro plan; mounts only if AgentsService wired |
| Tasks | `GET /api/v1/projects/{projectID}/tasks[?status=&is_critical=]` | `schedule.ListTasks` | all roles (read) |
| Tasks | `PUT /api/v1/projects/{projectID}/tasks/{taskID}` | `schedule.UpdateTask` | min superintendent |
| Budgets | `GET /api/v1/projects/{projectID}/budgets` | `financials.ListBudgets` | owner, admin |
| Invoices | `POST /api/v1/projects/{projectID}/invoices` | `financials.CreateInvoice` | owner, admin |
| Invoices | `PUT /api/v1/projects/{projectID}/invoices/{invoiceID}` | `financials.UpdateInvoice` | owner, admin |
| Procurement | `GET /api/v1/projects/{projectID}/procurement[?status=]` | `procurement.List` | all roles (read) |
| Procurement | `POST /api/v1/projects/{projectID}/procurement` | `procurement.Create` | owner, admin |
| Procurement | `PUT /api/v1/projects/{projectID}/procurement/{itemID}` | `procurement.Update` | owner, admin |
| Procurement | `POST /api/v1/projects/{projectID}/procurement/{itemID}/request-review` | `procurement.RequestVendorReview` | min superintendent |
| Financials | `GET /api/v1/org/{orgID}/financials/summary[?currency=]` | `financials.Summary` | min superintendent |
| Financials | `GET /api/v1/org/{orgID}/financials/ar-aging[?currency=]` | `financials.ARAging` | owner, admin |
| Financials | `GET /api/v1/org/{orgID}/financials/projects[?currency=]` | `financials.ProjectFinancials` | owner, admin |
| Pipeline | `GET /api/v1/org/{orgID}/pipeline/prospects[?stage=]` | `pipeline.ListProspects` | min superintendent (read) |
| Pipeline | `POST /api/v1/org/{orgID}/pipeline/prospects` | `pipeline.CreateProspect` | owner, admin |
| Pipeline | `GET /api/v1/org/{orgID}/pipeline/prospects/{prospectID}` | `pipeline.GetProspect` | min superintendent |
| Pipeline | `PUT .../prospects/{prospectID}` | `pipeline.UpdateProspect` | owner, admin |
| Pipeline | `POST .../prospects/{prospectID}/advance` | `pipeline.AdvanceProspect` | owner, admin |
| Pipeline | `POST .../prospects/{prospectID}/lose` | `pipeline.LoseProspect` | owner, admin |
| Pipeline | `POST .../prospects/{prospectID}/estimates` | `pipeline.CreateEstimate` | owner, admin |
| Pipeline | `POST .../prospects/{prospectID}/permits` | `pipeline.CreatePermit` | owner, admin |
| Pipeline | `PUT .../estimates/{estimateID}` / `PUT .../permits/{permitID}` | update | owner, admin |
| Pipeline | `GET /api/v1/org/{orgID}/pipeline/analytics` | `pipeline.Analytics` | owner, admin |
| Fleet | `GET /api/v1/org/{orgID}/fleet` | `fleet.ListAssets` | min superintendent |
| Fleet | `POST /api/v1/org/{orgID}/fleet` | `fleet.CreateAsset` | owner, admin |
| Fleet | `POST /api/v1/org/{orgID}/fleet/{assetID}/allocate` | `fleet.AllocateAsset` | min superintendent |
| HR | `GET /api/v1/org/{orgID}/employees` | `hr.ListEmployees` | owner, admin |
| HR | `GET /api/v1/org/{orgID}/employees/{employeeID}/certifications` | `hr.ListCertifications` | owner, admin |
| Feed | `GET /api/v1/feed[?status=&priority=&page=&per_page=]` | `feed.List` | all roles |
| Feed | `POST /api/v1/feed/{cardID}/action` | `feed.Action` | all roles |
| Feed | `POST /api/v1/feed/{cardID}/dismiss` | `feed.Dismiss` | all roles |
| AI | `POST /api/v1/agents/daily-briefing` | `agents.DailyBriefing` | pro plan; mounts only if AgentsService wired |
| Field | `GET /api/v1/field/sync[?since=]` | `field.Sync` | all roles |
| Field | `POST /api/v1/field/{progress,checkin,daily-log}` | field writes | all roles |
| Probes | `GET /health`, `/ready`, `/metrics` | — | no auth |
| Setup | `/api/v1/setup/*` | wizard | gated by auth, exempt from SetupGate |

**RBAC roles** (from CLAUDE.md / middleware): `owner` > `admin` > `superintendent` > `field_worker`.
`RequireRole(...)` = exact-membership; `RequireMinRole(x)` = x-or-higher.

### 0.2 Standalone / native pivot — what changes vs. the code as-checked-in

The product is dropping the external "The Brain" service. Native AI is gated on an in-app
Anthropic API key. Three backend surfaces are currently coded against Brain and will be
**re-pointed at native equivalents**; the frontend designs below target the *native* end
state and must degrade gracefully while the backend transition lands:

1. **AI gateway (Maestro)** → native Anthropic calls. Affects `daily-briefing` and
   `recommend-adjustments`. Plan-tier gating (`RequirePlanTier(pro)`) may be **replaced by
   a "key configured?" gate**. See OQ-1, OQ-2.
2. **A2A outbound vendor review** → today `RequestVendorReview` enqueues a JWS-signed
   webhook to Brain (returns `202 {idempotency_key}`). In the native model this becomes a
   **local feed card `vendor_review_requested`** with no external dispatch. See §5.2, OQ-5.
3. **Billing engine** → AI usage cost/markup metering moves in-app or disappears. The
   `cost_cents` / `tokens_used` blocks on AI responses may go to **raw token count only**.
   See OQ-3.

### 0.3 Cross-cutting display invariants (apply to every screen)

- **Money = integer cents + currency_code.** Every monetary field arrives as a
  `*_cents int64` paired with a `*_currency_code` / `currency_code` string. Frontend divides
  by 100 for display only; never does float math (DESIGN_SYSTEM §15). Render in
  `.data-currency` (JetBrains Mono, `tabular-nums`).
- **Never sum across currencies.** USD and CAD are the only supported codes. Aggregations
  arrive **already grouped by `currency_code`** (e.g. `CorporateBudget`, `PipelineAnalyticsRow`,
  `ProjectFinancial`, `ARAgingSnapshot` are all one-row-per-currency). The UI renders one
  visual group/column per currency and MUST NOT add a "grand total" row that combines them.
  If both USD and CAD are present, show side-by-side groups with explicit currency headers.
- **Durations / floats are integer-derived.** `total_float` is persisted as `*int` (days);
  task schedule times are RFC3339 timestamps. Render numerics in JetBrains Mono.
- **Dark-only.** No light mode, no theme toggle (DESIGN_SYSTEM §14).
- **Standard component states** (DESIGN_SYSTEM §10.1): default / hover (`.hover-lift`) /
  active / focus (2px `#00FFA3`) / disabled (40% opacity) / loading (`.skeleton` shimmer) /
  empty (centered icon + muted text) / error (3px `--fb-error` left border + text + icon) /
  offline (amber badge + dashed border on queued actions).
- **Error envelope.** API errors come back as `{code, message}` with codes
  `VALIDATION_ERROR` (400), `UNAUTHORIZED` (401), `NOT_FOUND` (404), `CONFLICT` (409),
  `SERVICE_UNAVAILABLE` (503), `UPSTREAM_ERROR` (502), `INTERNAL_ERROR` (500). Map each to a
  toast + inline treatment (§11).

---

## 1. Project Dashboard / Portfolio Rollup

**Workspace:** Portfolio. **Route:** `/portfolio` (landing) + `/portfolio/projects`.
**Primary users:** Tom (owner), Sarah (admin). Superintendents get a read-only subset.

### 1.1 Layout

Three-panel shell at ≥1440px (sidebar 280px + content flex + optional artifact 320px);
two-panel ≥1200px; single column + hamburger ≤768px (DESIGN_SYSTEM §10.3). Content is a
vertical stack:

1. **Portfolio KPI strip** — row of `<fb-stat-card>` (label Outfit, value JetBrains Mono):
   - Active projects count, Forecast project-end risk (count of projects with critical-path
     slip), Open AR total, Committed-vs-actual variance.
   - **Currency rule:** the AR / variance stat cards render **one card per currency** when
     both USD and CAD exist — no merged figure.
2. **Project grid** — responsive grid of `<fb-project-card>`, one per project.
3. **Portfolio rollup band** — embeds the Financial Summary organisms (§4) collapsed to a
   "By currency" rollup: corporate budget totals grouped by `currency_code`.

### 1.2 Components & data bindings

**`<fb-project-card>`** (one per project):
- Source: `GET /api/v1/projects` → `Project{ id, name, address, status, gsf,
  permit_issued_date, project_start_date, created_at, updated_at }` (models/project.go).
- Bindings: title = `name`; `<fb-badge>` = `status`; sub-label = `address`; GSF chip =
  `gsf` (e.g. `3,200 GSF`, JetBrains Mono); start = `project_start_date`.
- Completion bar: derived from the project's tasks (`GET .../tasks`) — average
  `percent_complete`, or `is_critical`-weighted. **OQ-4: no project-level rollup field
  exists; the card must either fan out a tasks call per project or the backend must add a
  summary endpoint.**
- Click → `/portfolio/projects/{id}` (Project Detail, tabbed: Overview / Budget / Schedule /
  Team per IA §3.1).

**Portfolio rollup band:**
- Source: `GET /api/v1/org/{orgID}/financials/summary` →
  `FinancialsSummary{ corporate_budgets[], ar_aging[] }`.
  `CorporateBudget` rows are keyed `(fiscal_year, quarter, currency_code)` with
  `total_estimated_cents / total_committed_cents / total_actual_cents / project_count`.
- Render: group by `currency_code`; within a currency, one row per quarter. Variance =
  `actual − committed` shown with `.data-positive` / `.data-negative`.

### 1.3 States

- **Loading:** KPI strip → 4 skeleton stat cards; grid → 6 skeleton project cards.
- **Empty (no projects):** centered hard-hat icon + "No active projects. Projects appear
  here once a pipeline prospect reaches PERMIT_ISSUED, or create one directly." + primary
  CTA `New Project` (owner/admin only). NOTE: `projects.List/Create` are backend stubs
  today (OQ-6) — the empty state and CTA must handle `501 NOT_IMPLEMENTED` gracefully.
- **Error:** band-level error card with retry; per-card error if a single tasks fan-out fails.
- **Role-gating:** superintendent sees project grid (read) but not the AR/variance KPI cards
  (financials are owner/admin); field_worker is routed to Field Portal, not here.

---

## 2. Schedule View — CPM Gantt

**Workspace:** Agent Command Center. **Route:** `/command/schedule` (project-scoped).
**Component:** `<fb-gantt-chart>` (DESIGN_SYSTEM §9.3) + CPM data panel + recalc controls.
**Read:** all roles. **Recalc / task edits / AI adjust:** min superintendent.

This is the product's centerpiece — the deterministic CPM physics engine made visible.

### 2.1 Data model (from cpm.go + models/project.go)

`GET /api/v1/projects/{projectID}/schedule/gantt` → `GanttView`:
```
{ "tasks": ProjectTask[], "critical_path": uuid[], "project_end": RFC3339 }
```
`ProjectTask` (persisted CPM fields):
`id, project_id, wbs_code, name, duration_days, early_start, early_finish, late_start,
late_finish, total_float (int days, nullable), is_critical (bool), status, percent_complete,
assigned_crew[]`.

`POST .../schedule/recalculate` → `{ cpm_result: CPMResult, recalculation_ms }` where
`CPMResult{ tasks: TaskSchedule[], critical_path: string[] (WBS codes), project_end,
critical_path_changed }`. NFR: physics < 200ms for 80 tasks, < 800ms end-to-end.

`GET .../tasks?is_critical=true` → filtered task list (for the critical-only toggle).

### 2.2 Gantt layout

- **Left rail (sticky, ~320px):** WBS task table. Columns (JetBrains Mono for numerics):
  `WBS Code` · `Name` (Outfit) · `Dur` (`duration_days` → e.g. `14d`) · `Float`
  (`total_float`d) · `%` (`percent_complete`). Indent by WBS depth (split `wbs_code` on `.`).
- **Right canvas:** SVG/Canvas timeline (`<fb-gantt-chart>`). X-axis = working-day calendar;
  date labels in JetBrains Mono. One horizontal bar per task spanning
  `early_start → early_finish`.
- **Today line:** vertical marker at current date.
- **Project-end marker:** vertical line at `project_end` with a flag label.

### 2.3 Data-viz specifics — critical path, float, cascade, weather/size

**(a) Critical-path highlighting.**
- A task is critical iff `is_critical == true` (backend: `|total_float| < 0.001` day,
  cpm.go `BackwardPass`). `critical_path[]` is the authoritative ordered set of critical task IDs.
- Visual: critical bars filled **Gable Green `#00FFA3`** with `--fb-glow`; non-critical bars
  filled Slate Steel `#1E2029` with a thin Blueprint Blue `#38BDF8` outline.
- Critical dependency edges drawn as connector lines also in Gable Green; non-critical edges
  in muted `--fb-border`. Edge arrowheads indicate predecessor → successor.
- Toggle chip **"Critical path only"** → calls `?is_critical=true` (server-filtered) or
  client-filters the loaded set; non-critical bars dim to 20% opacity rather than disappear
  (keeps spatial context).

**(b) Total-float display.**
- Per-bar **float tail:** a hollow/hatched extension drawn from `early_finish` to
  `late_finish` representing slack (`total_float` days). Critical tasks have zero-width tails
  (by definition `early == late`).
- Color the tail Amber `#F59E0B` when `0 < total_float ≤ warning_threshold` (near-critical),
  neutral grey when float is comfortable. **OQ-7: the near-critical threshold (e.g. ≤2 days)
  is a product decision; not in backend.**
- Left-rail `Float` column shows the integer day value; hovering a bar shows a tooltip:
  `ES <early_start> · EF <early_finish> · LS <late_start> · LF <late_finish> · Float <n>d`.

**(c) Delay-cascade visualization.**
- Recalc returns `critical_path_changed`. The backend also enqueues a `delay_cascade` River
  job when the critical path is non-empty (service/schedule.go). The frontend cannot read job
  results directly, so cascade is visualized from the **delta between the pre-recalc and
  post-recalc `GanttView`**:
  - Diff each task's `early_start` / `early_finish`. Tasks whose dates moved later are the
    cascade set.
  - Animate (Emphasized 300ms, respecting `prefers-reduced-motion`) the affected bars sliding
    to their new position; flash a Safety Red `#F43F5E` pulse on bars that slipped, and a
    Gable Green pulse on any that pulled in.
  - Show a **cascade summary banner**: "Recalc shifted N tasks; project end moved
    `<old>` → `<new>` (+Md)." Derived entirely client-side from the two payloads.
- A `delay_cascade`-originated **feed card** (`card_type` indicating cascade — see §6) is the
  asynchronous notification path; clicking it deep-links back to this Gantt with the affected
  tasks pre-highlighted.

**(d) Weather & size (GSF) adjustment indicators.**
- Physics: `swim.go` applies weather multipliers (precip >10mm ×1.15, low <0°C ×1.25, high
  >35°C ×1.10) to weather-sensitive WBS (major < 10.0, or 13.x). `dhsm.go` applies the Size
  Adjustment Factor `(GSF / standard)^exponent` to non-inspection tasks.
- These adjustments are baked into the effective `duration_days` server-side
  (`WeatherAdjustedDuration` / `CalculatedDuration` are physics-internal, JSON-excluded via
  `json:"-"`). The frontend therefore **cannot** read the multiplier directly per task today.
  **OQ-8: to show "this bar is +1.25× from frost" we need the backend to surface an
  adjustment-provenance field (e.g. `duration_basis: {base, weather_mult, saf, manual_override}`).**
- Interim design (no new field): badge tasks by *eligibility*, computed client-side:
  - **Weather-sensitive icon** (snowflake/raindrop) on bars whose `wbs_code` major is < 10.0
    or `== 13`. Tooltip: "Weather-sensitive phase — duration may include SWIM weather buffer."
  - **GSF chip** in the schedule header from `Project.gsf`: "Sized for 3,200 GSF" so the user
    knows DHSM scaling is in effect project-wide.
- When OQ-8 lands: render an inline multiplier pill on the bar (e.g. `×1.25 ❄`) and a stacked
  bar showing base duration (solid) + adjustment delta (hatched amber for weather, blueprint
  blue for size).

**(e) Recalc flow.**
1. User (≥superintendent) edits a task (`PUT .../tasks/{taskID}` — duration/status/%/crew).
   `UpdateTask` does NOT auto-recalc (service note) → schedule is now **stale**.
2. UI shows a persistent **"Schedule out of date — Recalculate"** banner (amber) once any
   edit lands, plus a "stale" badge on the Gantt header.
3. Primary CTA **Recalculate** → `POST .../schedule/recalculate`. Button enters loading
   (spinner + disabled). On success: store pre/post views for the cascade diff (c), clear
   stale banner, show `recalculation_ms` as a subtle "recomputed in 142ms" confirmation
   (reinforces the deterministic-engine value prop).
4. **Never-recalculated project:** `GanttView` returns empty `critical_path` and zero-value
   `project_end` (service note in schedule.go). Detect this → show empty state: "Schedule not
   yet computed. Run the physics engine to compute the critical path." + Recalculate CTA.

### 2.4 States

- **Loading:** left rail → skeleton rows; canvas → skeleton bars (`.skeleton-bar`).
- **Empty (no tasks):** "No tasks in this schedule yet." Recalc returns 400 if zero tasks
  (`no tasks found`), so disable the Recalculate CTA when task list is empty.
- **Error:** 404 NOT_FOUND (cross-org / missing project) → "Project not found or not in your
  org." 400 VALIDATION_ERROR on `is_critical` filter → ignore filter, log. 500 → retry toast.
- **Role-gating:** field_worker / read-only → Recalculate, task-edit, and AI-adjust controls
  hidden (not just disabled). The AI "Suggest adjustments" control hides entirely when
  `AgentsService` is not wired or the native AI key is unconfigured (§9).

---

## 3. AI Schedule Adjustments (native AI) — sub-surface of Schedule

**Endpoint:** `POST .../schedule/recommend-adjustments` →
`ScheduleAdjustmentSet{ adjustments[], applied_deltas, skipped_rationale_only, run_id,
tokens_used, cost_cents, currency_code }` (service/agents.go).
**Gate today:** min superintendent + pro plan + AgentsService wired. **Native target:** gate
on AI-key-configured (OQ-1).

### 3.1 Behavior

- Entry: "Suggest schedule adjustments" button in the Gantt toolbar (≥superintendent).
- On invoke: the backend asks the model for per-task duration nudges, **applies** the numeric
  ones via `UpdateTask`, writes an audit row, and **re-runs CPM**. The response distinguishes:
  - `applied_deltas` — durations actually changed (CPM already re-run server-side).
  - `skipped_rationale_only` — model returned a narrative but no number (no change made).
- UI renders a **review drawer** listing each `adjustment` (task WBS + name + old→new duration
  + rationale text), with applied ones marked Gable Green "applied" and rationale-only ones
  marked neutral "advisory". Because the apply already happened, this is a **post-hoc
  transparency view**, not an approve/reject gate.
- Cascade: since CPM re-ran, the Gantt should re-fetch `GanttView` and run the §2.3(c) diff
  animation against the pre-invoke snapshot.
- **"Apply succeeded; recalc deferred" case** (handler returns 200 with a non-nil result even
  on partial failure): show an amber note "Durations updated; critical path will refresh on
  next recalculation," and surface the Recalculate CTA.

### 3.2 AI unavailable / unconfigured states (native)

- **Key not configured:** the control renders **disabled with a lock glyph** + helper text
  "AI features require an Anthropic API key. Configure in Settings → AI." (owner/admin see a
  deep link; others see read-only copy). Do not 500 — the route simply won't be mounted /
  will 503; treat `503 SERVICE_UNAVAILABLE` as the canonical "AI off" signal (§9).
- **Upstream error (502 UPSTREAM_ERROR):** "AI service is temporarily unavailable. Your
  schedule is unchanged." Retry allowed.
- **Token/cost display:** show `tokens_used`; show `cost_cents`+`currency_code` only if
  billing is retained (OQ-3).

---

## 4. Financials — Budgets / Invoices / Cost (integer-cents, USD/CAD grouped)

**Workspace:** Portfolio. **Route:** `/portfolio/financials`, tabs: **Summary | AR Aging |
By Project** (IA §3.1). **Gate:** summary = min superintendent (read); ar-aging & projects =
owner/admin; invoice writes = owner/admin.

### 4.1 Summary tab

- Source: `GET /api/v1/org/{orgID}/financials/summary[?currency=]` →
  `{ corporate_budgets: CorporateBudget[], ar_aging: ARAgingSnapshot[] }`.
- **`<fb-budget-summary>`** rendered **once per `currency_code`** present in
  `corporate_budgets`. Each shows Estimated / Committed / Actual (cents → currency) with
  variance %. A currency selector chip row (All / USD / CAD) drives the `?currency=` param;
  "All" renders multiple groups, never a merged sum.
- Hard rule banner if both currencies present: a small caption "Figures shown per currency;
  USD and CAD are not combined."

### 4.2 AR Aging tab

- Source: `GET .../financials/ar-aging[?currency=]` → `ARAgingSnapshot[]` (latest per
  currency): `current_cents, days_30_cents, days_60_cents, days_90_plus_cents,
  total_receivable_cents, snapshot_date, currency_code`.
- **`<fb-ar-aging-chart>`** (D3 stacked horizontal bar): one stacked bar **per currency**,
  segments Current / 30 / 60 / 90+ colored neutral → amber → red as they age. Legend +
  per-bucket value labels in JetBrains Mono. 90+ segment uses Safety Red.
- Empty: "No receivables snapshot yet" (snapshots produced by a rollup job).

### 4.3 By Project tab

- Source: `GET .../financials/projects[?currency=]` → `ProjectFinancial[]` — one row per
  `(project, currency_code)`: `project_name, total_estimated_cents, total_committed_cents,
  total_actual_cents, phase_count`.
- **`<fb-data-table>`** sortable; numeric columns JetBrains Mono; variance coloring. Group or
  sort by `currency_code`; if a project has both USD and CAD rows they appear as **separate
  rows**, each currency labeled — the table MUST NOT collapse them.

### 4.4 Project-level budgets & invoices (within Project Detail → Budget tab)

- Budgets: `GET /api/v1/projects/{projectID}/budgets` → `ProjectBudget[]`, one per WBS phase,
  each with three currency-matched pairs (estimated/committed/actual + matching
  `*_currency_code`; backend CHECK guarantees they match within a row). Render a per-phase
  table; phase variance bars.
- Invoices: `POST .../invoices` (create) and `PUT .../invoices/{invoiceID}` (status).
  `Invoice{ vendor_name, invoice_number?, amount_cents, currency_code, wbs_code?, status,
  due_date?, paid_date? }`. Status enum: `pending | approved | rejected | paid`.
  - Invoice list (read path) — **OQ-9: there is no `GET invoices` route in router.go; only
    create/update.** The Budget tab needs a list endpoint or must derive from another source.
  - Status badges: pending = amber, approved = blueprint blue, paid = gable green,
    rejected = safety red. Due-soon (due_date within N days, unpaid) → amber "due" chip.

### 4.5 States (financials, all tabs)

- Loading: skeleton stat cards / skeleton chart / skeleton table rows.
- Empty: per-tab muted message (e.g. "No budget rows for this org/currency").
- Error: 400 if `?currency=` is an unsupported code (`validateOptionalCurrency`) → reset chip
  to All + toast "Only USD and CAD are supported." 403 (wrong role) → render a "Financials are
  restricted to owners and admins" panel instead of the data.
- Role: superintendent sees Summary only; AR Aging & By Project tabs hidden.

---

## 5. Procurement

**Workspace:** Agent Command Center. **Route:** `/command/procurement` (project-scoped, with
a portfolio roll-up list). Read: all roles; create/edit: owner/admin; request-review: ≥super.

### 5.1 PO / item surfaces

- Source: `GET /api/v1/projects/{projectID}/procurement[?status=WARNING,CRITICAL]` →
  `ProcurementItem[]`: `name, wbs_code, estimated_cost_cents (+currency), lead_time_days,
  weather_buffer_days, need_by_date?, must_order_date?, status, ordered_at?, po_number?,
  status_changed_at`.
- **Status model** (models/procurement.go): `OK → WARNING → CRITICAL → ORDERED`. The agent
  computes time-based statuses daily; humans transition to ORDERED via `PUT .../{itemID}`.
- **List as a triage board:** group/sort by `status` descending urgency. Status pills:
  CRITICAL = Safety Red + glow, WARNING = Amber, OK = neutral green, ORDERED = blueprint blue.
- Each row shows a **"must order by" countdown** from `must_order_date` (JetBrains Mono,
  e.g. `order in 3d` / `OVERDUE 2d` in red). `weather_buffer_days` shown as a small chip
  (links the weather model to procurement lead time).
- **Mark Ordered** action (owner/admin) → `PUT .../{itemID}` with `status=ORDERED`,
  `po_number`, `ordered_at`. Optimistic update; on success the pill flips to ORDERED.
- Cost: `estimated_cost_cents` + `estimated_cost_currency_code` rendered per currency; a
  per-currency subtotal at the board foot (never cross-currency).

### 5.2 Native AI vendor recommendations

- Source model: `ProcurementRecommendation` (procurement_recommendations rows):
  `vendor_name, predicted_spend_cents (+currency), confidence_pct (0..100), reasoning?,
  run_id`. Typically 3–5 rows share a `run_id`.
- **OQ-10: there is no read route for recommendations in router.go today.** Design assumes a
  future `GET .../procurement/{itemID}/recommendations` (native AI). Until then this section
  renders only when such data is available.
- **`<fb-recommendation-card>`** per vendor: vendor name, predicted spend (cents→currency),
  a **confidence meter** (0–100% bar; ≥75 green, 40–74 amber, <40 muted), and an expandable
  `reasoning` block (markdown). Cards within a `run_id` shown as a comparison set, sortable by
  predicted spend or confidence.
- **AI-off state:** when the native key is unconfigured / route 503s, replace the
  recommendation strip with the §9 "AI unavailable" inline panel.

### 5.3 Vendor review request — native `vendor_review_requested` feed card (A2A removed)

- **Backend today:** `POST .../{itemID}/request-review` enqueues a JWS A2A webhook to Brain;
  returns `202 {idempotency_key}`; body `{vendor, total_cents, currency_code, rfq_id?,
  reasoning?}`. Role ≥superintendent.
- **Native target (this spec):** the same action instead writes a **local feed card of
  `card_type: "vendor_review_requested"`** (no external dispatch). The UI contract is designed
  for that end state:
  - Trigger: from a recommendation card or item row, **"Request review"** (≥super) opens a
    small modal: vendor (prefilled from recommendation), total (cents + currency selector
    USD/CAD), optional reasoning. Validate currency ∈ {USD,CAD}, total ≥ 0, vendor non-empty
    (mirrors the emitter's wire-shape checks).
  - On submit → `202`/`201`. Show success toast "Review requested — sent to the review feed."
    The newly created `vendor_review_requested` card appears in the Feed (§6) for owners/admins
    to act on. **OQ-5: confirm the native backend writes this card synchronously and what its
    `actions[]` payload looks like (approve/reject?).**
  - Error: 503 SERVICE_UNAVAILABLE ("vendor review flow not available on this binary" — worker
    path) → "This action isn't available right now." 404 → item not found. 400 → field-level
    validation messages.

### 5.4 States (procurement)

- Loading: skeleton board columns. Empty: "No procurement items for this project." +
  Create CTA (owner/admin). Error: standard mapping. Offline (field): read-only from sync
  cache; write actions queue with amber dashed treatment.

---

## 6. Feed / Notifications (incl. `vendor_review_requested`)

**Workspace:** Agent Command Center (also surfaced in Field Portal sync). **Route:**
`/command/feed` (and the Briefing view consumes the same data). All roles.

### 6.1 Data model

- Source: `GET /api/v1/feed[?status=active&priority=&page=&per_page=]` (paginated) →
  `{ cards: FeedCard[], pagination: {page, per_page, total, total_pages} }`.
- `FeedCard{ id, project_id?, card_type, title, body, priority (critical|urgent|normal|low),
  target_user_id?|target_role?, actions (JSONB: [{label, action_type, payload}]), status
  (active|dismissed|actioned|expired), actioned_at?, expires_at?, created_at }`.
- Known `card_type`s seen in code/comments: `weather_alert`, `procurement`,
  `sub_confirmation`, `progress`, plus the **new native `vendor_review_requested`**, and
  cascade/transition cards (e.g. delay cascade, Kanban→CPM "moved into construction").

### 6.2 Layout & components

- **`<fb-feed-list>`** sorted by priority (critical > urgent > normal > low), then `created_at`
  desc. Filter chips: status (Active default) + priority. Pagination controls at foot.
- **`<fb-feed-card>`** (glass-card): priority badge (critical = Safety Red glow, urgent =
  amber, normal = blueprint, low = muted), `title` (Outfit), `body`, relative timestamp.
  - **Actions:** render one button per `actions[]` entry using `label`; click →
    `POST /api/v1/feed/{cardID}/action` `{action_type, payload}`. Dismiss (X) →
    `POST /api/v1/feed/{cardID}/dismiss`.
  - Card-type iconography + deep-links: `weather_alert` → snowflake, links to affected
    schedule; `procurement`/CRITICAL → links to §5 item; cascade → links to §2 Gantt with
    highlight; `vendor_review_requested` → opens the review detail.

### 6.3 `vendor_review_requested` card (native)

- Replaces the former A2A round-trip. Title e.g. "Vendor review requested: <vendor>"; body
  carries the quoted total (cents→currency, JetBrains Mono) + optional AI `reasoning`.
- Actions (expected): **Approve** / **Reject** → `POST .../action` with the corresponding
  `action_type`. On action, card transitions to `actioned` and drops out of the Active filter.
- **Currency:** the total in the body is a single quote in one `currency_code` — display as-is;
  no aggregation.
- **OQ-5 / OQ-11:** confirm the exact `actions[]` shape and `action_type` values the native
  backend emits for this card so buttons bind correctly.

### 6.4 States

- Loading: 4 skeleton feed cards. Empty (Active filter, 0 cards): "You're all caught up." +
  calm illustration. Error: list-level retry. Optimistic dismiss/action with rollback on 4xx.
- Real-time: poll `GET /feed` (or future SSE); new critical cards animate in with a brief glow.
- Role: cards are server-targeted by `target_user_id`/`target_role`, so each role only sees
  its own; no client-side gating needed beyond rendering.

---

## 7. Fleet

**Workspace:** Portfolio. **Route:** `/portfolio/fleet`. Read + allocate: ≥superintendent;
create: owner/admin.

- Source: `GET /api/v1/org/{orgID}/fleet[?status=]` → `FleetAsset[]`:
  `name, asset_type, serial_number?, status (available|unavailable|maintenance)`.
- **`<fb-fleet-grid>`** of asset cards; status badge (available = green, maintenance = amber,
  unavailable = muted/red). Filter chips by status. `serial_number` in JetBrains Mono.
- **Allocate** (≥super): `POST .../fleet/{assetID}/allocate` with a project + `[start_date,
  end_date)`. The backend enforces a GiST no-overlap exclusion → **409 CONFLICT**
  (`ErrAllocationConflict`). UI: on 409 show "This asset is already booked for an overlapping
  range" inline on the date field + suggest the conflicting window if returned.
- **Create asset** (owner/admin): `POST .../fleet`. Modal with name/type/serial.
- **Maintenance alerts:** assets with `status=maintenance` surface a banner; CLAUDE.md notes a
  `maintenance_reminders` job feeds the Feed (§6) — link out.
- States: loading skeleton grid; empty "No fleet assets yet" + Create CTA; allocate-conflict
  (409) inline; role-gated create/allocate controls.

---

## 8. HR & Certifications

**Workspace:** Portfolio. **Route:** `/portfolio/hr`. **Gate:** owner/admin only.

- Source: `GET /api/v1/org/{orgID}/employees` → `Employee[]`:
  `first_name, last_name, role, phone?, hire_date?, user_id?`.
- **`<fb-employee-table>`** sortable: name (Outfit) · role · hire date · phone. Click a row →
  `GET .../employees/{employeeID}/certifications` → `Certification[]`:
  `cert_type, cert_number?, issued_date?, expiry_date (NOT NULL), status
  (active|expired|revoked)`.
- **Certification expiration banner** (IA §3.1): the `certification_alerts` job flags
  near-expiry certs; the HR view shows a top banner counting certs expiring within N days,
  and per-cert chips: active = green, expiring-soon = amber (client-computed from
  `expiry_date`), expired = red, revoked = muted.
- **PII note (CLAUDE.md `internal/pii`):** `phone`, names are Restricted-class server-side.
  The frontend displays them for authorized owner/admin viewers but should avoid logging them
  and should mask in any screenshot/share affordance.
- States: loading skeleton table; empty "No employees"; error 403 → "HR is restricted to
  owners and admins."

---

## 9. AI Availability Gating (native, cross-cutting)

Because AI is now native and key-gated, every AI surface (Daily Briefing §10, Schedule
Suggest §3, Procurement recommendations §5.2) shares one availability contract:

**Signals the frontend uses to decide AI state:**
1. **Route not mounted / 404 or 503 SERVICE_UNAVAILABLE** → AI is off (no key configured, or
   binary without AgentsService). This is the canonical "unavailable" signal.
2. **502 UPSTREAM_ERROR** → transient model/provider failure; retry-able.
3. **401 UNAUTHORIZED** → key rejected; prompt owner/admin to re-check the key.

**`<fb-ai-unavailable>` inline panel** (reused everywhere): lock icon + "AI features are off.
Add an Anthropic API key in Settings → AI to enable briefings, schedule suggestions, and
vendor recommendations." Owner/admin get a deep link to Settings; others get read-only copy.
The panel replaces the AI control region in-place (no layout shift, no error toast).

**Settings → AI screen (new, owner/admin):** masked key input, "Test key" (calls a cheap
native validation endpoint — **OQ-2**), status indicator (Configured / Not configured /
Invalid), and a per-org usage readout (tokens; cost only if billing retained — OQ-3).

---

## 10. Daily Briefing (native AI)

**Workspace:** Agent Command Center. **Route:** `/command/briefing` (mobile calls on launch).
**Endpoint:** `POST /api/v1/agents/daily-briefing` → `{ briefing: DailyBriefing }` where
`DailyBriefing{ reply (string), session_id, task_count, alert_count }` (service/agents.go).
**Gate today:** pro plan + AgentsService. **Native:** AI-key-configured (§9).

### 10.1 Layout

- **Briefing card** (glass, hero): renders `reply` as formatted markdown — the model's morning
  summary assembled from the caller's open tasks + active critical/urgent feed cards.
- **Context chips:** `task_count` ("N tasks assigned") + `alert_count` ("M urgent alerts"),
  JetBrains Mono.
- **Weather card** (IA §3.2) alongside — **OQ-12: no weather read endpoint exists in
  router.go**; weather currently lives only inside the physics SWIM model. Either add a
  forecast endpoint or drop the weather card from the briefing view.
- **Priority feed list** below: the same `<fb-feed-list>` (§6) filtered to critical+urgent, so
  the briefing reads as "here's your narrative + the cards behind it."
- **`session_id`** retained client-side to enable a future follow-up-question chat (the
  endpoint is synchronous one-shot today).

### 10.2 States

- **Loading:** "Generating your briefing…" with a shimmer on the hero card (this call hits the
  model, so expect 1–3s; show progress, not a blank spinner).
- **Empty context:** if `task_count == 0 && alert_count == 0`, the model still returns a reply;
  render it but add a calm "Nothing urgent this morning" header.
- **AI unavailable (§9):** replace hero with `<fb-ai-unavailable>`; still show the priority
  feed list (feed does not require AI).
- **Error:** 502/401 mapped per §9; 503 → AI-off panel. The briefing is non-blocking — never
  hard-fail the whole Command Center if the briefing call fails.

---

## 11. Pre-Construction Pipeline (CRM Kanban → CPM)

**Workspace:** Portfolio (or its own "Sales" area). **Route:** `/portfolio/pipeline`.
**Gate:** read ≥superintendent; mutations owner/admin.

### 11.1 Kanban board

- Source: `GET /api/v1/org/{orgID}/pipeline/prospects[?stage=]` → `Prospect[]`:
  `name, client_name, client_email?, client_phone?, address?, gsf?, pipeline_stage,
  probability_pct, source?, project_id? (set on PERMIT_ISSUED), lost_reason?`.
- **Columns** = pipeline stages in order: `LEAD → QUALIFIED → ESTIMATE_SENT →
  VERBAL_COMMITMENT → PERMIT_APPLIED → PERMIT_ISSUED` (terminal), with `LOST` as a separate
  terminal lane. Stage probabilities (models/pipeline.go `Probability()`): 10/25/50/75/85/100/0.
- **`<fb-prospect-card>`:** client name (Outfit), GSF chip, latest estimate total
  (cents→currency, JetBrains Mono), `probability_pct` badge, source tag.
- **Advance / Lose:** `POST .../advance` and `POST .../lose` (owner/admin). The board enforces
  forward-only transitions client-side using `AllowedTransitions` semantics (LEAD can only go
  to QUALIFIED or LOST, etc.); illegal drags snap back with a toast.
- **PERMIT_ISSUED is special:** advancing to it atomically converts the prospect into a Project
  + CPM schedule (Kanban→CPM transition; emits a feed card). The card shows a "→ Project"
  badge and deep-links to the new project's Schedule (§2) via `project_id`.

### 11.2 Prospect detail

- Source: `GET .../prospects/{prospectID}` → `ProspectWithDetails{ prospect, estimates[],
  permits[] }`.
- **Estimates** (`PipelineEstimate`): versioned; `total_estimated_cents + currency_code`,
  `line_items[]` (each `wbs_code, description, estimated_cents, unit, quantity` — single
  currency per estimate, no cross-currency risk), `margin_pct`, `status (draft|sent|revised|
  accepted)`. Render a versioned list; line-item table with per-line and total in JetBrains
  Mono; create/update via `POST/PUT .../estimates`.
- **Permits** (`Permit`): `permit_type, jurisdiction, application_number?, submitted_date?,
  expected_issue_date?, actual_issue_date?, fee_cents (+currency), status`. Status timeline
  chip: not_submitted → submitted → under_review → revisions_requested → approved/denied.

### 11.3 Pipeline analytics

- Source: `GET .../pipeline/analytics` → `PipelineAnalyticsRow[]` — **one row per
  `currency_code`**: `total_estimated_cents` (if all close) vs `weighted_revenue_cents`
  (probability-weighted "expected"), `prospect_count`.
- **Viz:** per currency, a paired bar — Total (faint) vs Weighted (Gable Green) — and the **gap
  = pipeline risk** (the spread between booked-if-all-close and probability-adjusted). Render
  one chart group per currency; never combine USD + CAD.

### 11.4 States

- Loading: skeleton columns. Empty: "No prospects yet" + Create CTA. Error: standard.
  Role: superintendent read-only (no advance/lose/create); board drag disabled for them.

---

## 12. Open Questions for the User

| # | Question | Blocks |
|---|---|---|
| OQ-1 | In the native model, does the AI gate become "Anthropic key configured?" replacing `RequirePlanTier(pro)`? What's the exact 503-vs-404 contract when no key is set? | §3, §9, §10 |
| OQ-2 | Is there (or will there be) a native endpoint to validate/test the Anthropic key for the Settings → AI screen? | §9 Settings |
| OQ-3 | Is billing/cost metering retained in standalone? If not, drop `cost_cents`/`currency_code` from AI response displays and show raw `tokens_used` only. | §3.2, §9, §10 |
| OQ-4 | No project-level rollup field (completion %, budget health) exists. Add a summary endpoint, or have the portfolio card fan out per-project `tasks`/`budgets` calls? | §1 |
| OQ-5 | For native `vendor_review_requested`: does the backend write the feed card synchronously on request-review? What is its `actions[]` shape / `action_type` values (Approve/Reject)? Is it 201 or 202? | §5.3, §6.3 |
| OQ-6 | `projects.List/Create/Get/Update` are `writeNotImplemented` stubs. When do they land, and what's the list/detail JSON shape (esp. for portfolio cards)? | §1, §2 |
| OQ-7 | Define the "near-critical" float threshold (e.g. ≤2 days) for the amber float-tail coloring. Product decision, not in backend. | §2.3(b) |
| OQ-8 | To visualize weather/size multipliers per task, expose adjustment provenance (base duration, weather multiplier, SAF, manual override). Today these are `json:"-"`. | §2.3(d) |
| OQ-9 | No `GET invoices` list route exists (only create/update). How does the Budget tab list invoices? | §4.4 |
| OQ-10 | No read route for `ProcurementRecommendation`. Will native AI add `GET .../procurement/{itemID}/recommendations`? | §5.2 |
| OQ-11 | Canonical list of feed `card_type` values and their `actions[]` schemas (the field is opaque JSONB today). Needed to bind action buttons reliably. | §6 |
| OQ-12 | No weather/forecast read endpoint exists; weather lives only inside the SWIM physics. Add one for the Briefing weather card, or drop it? | §10.1 |
| OQ-13 | Confirm `total_float` units everywhere are integer *days* (model is `*int`; physics computes float days then rounds). The float-tail viz assumes days. | §2.3(b) |
| OQ-14 | Real-time delivery: is the feed poll-only, or will SSE/websocket land? Affects cascade-card latency and briefing freshness. | §2.3(c), §6.4 |

---

## 13. Component → Endpoint Quick Map (build reference)

| Page component | Endpoints | Key model fields |
|---|---|---|
| `<fb-project-card>` | `GET /projects` (+ per-project `tasks`) | `name,status,gsf,address` |
| `<fb-gantt-chart>` | `GET .../schedule/gantt`, `POST .../schedule/recalculate`, `GET .../tasks?is_critical=` | `early_start,early_finish,late_start,late_finish,total_float,is_critical,critical_path,project_end,critical_path_changed` |
| AI schedule drawer | `POST .../schedule/recommend-adjustments` | `adjustments,applied_deltas,skipped_rationale_only,tokens_used` |
| `<fb-budget-summary>` | `GET .../financials/summary` | `CorporateBudget.*_cents + currency_code` |
| `<fb-ar-aging-chart>` | `GET .../financials/ar-aging` | `current/30/60/90+_cents, total_receivable_cents, currency_code` |
| `<fb-project-financials-table>` | `GET .../financials/projects` | `total_estimated/committed/actual_cents, currency_code` |
| Budget tab | `GET .../budgets`, `POST/PUT .../invoices` | `ProjectBudget.*`, `Invoice.amount_cents+currency_code,status` |
| Procurement board | `GET/POST/PUT .../procurement`, `POST .../request-review` | `status,must_order_date,weather_buffer_days,estimated_cost_cents+currency` |
| `<fb-recommendation-card>` | (OQ-10) `GET .../recommendations` | `vendor_name,predicted_spend_cents+currency,confidence_pct,reasoning` |
| `<fb-feed-list>` / `<fb-feed-card>` | `GET /feed`, `POST /feed/{id}/action`, `/dismiss` | `card_type,priority,title,body,actions,status` |
| `<fb-fleet-grid>` | `GET/POST .../fleet`, `POST .../allocate` | `name,asset_type,status,serial_number` |
| `<fb-employee-table>` | `GET .../employees`, `.../certifications` | `first_name,last_name,role,expiry_date,status` |
| `<fb-briefing-view>` | `POST /agents/daily-briefing` (+ `GET /feed`) | `reply,task_count,alert_count,session_id` |
| Pipeline board / detail | `GET .../prospects`, `.../advance`, `.../lose`, `.../analytics` | `pipeline_stage,probability_pct,total_estimated_cents+currency,weighted_revenue_cents` |
```
