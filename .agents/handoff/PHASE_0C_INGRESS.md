# PHASE 0C — Operational-Data Ingress Layer (Implementation Spec)

**Branch:** `feature/phase-0c-ingress`
**Status:** SPEC — ready to implement.
**Author:** Claude Code (zero-trust auditor), grounded in actual code @ this branch.

## 0. Problem & goal

Today the deterministic CPM engine has no operator-authored input path. Projects are createable
(`POST /api/v1/projects` → `ProjectStore.Create`, `store/projects.go:166`) but **tasks, dependencies,
employees, certifications, and budget baselines are not** — all five stores are read-only/update-only:

- `ScheduleStore` (`store/schedule.go`): `GetProjectTasks`/`GetProjectDependencies`/`UpdateSchedule` (UPDATE)/`UpdateTask` (UPDATE). **No InsertTask, no InsertDependency.**
- `HRStore` (`store/hr.go`): `VerifyEmployeeInOrg`/`ListEmployees`/`ListCertifications`. **No inserts.**
- `FinancialsStore` (`store/financials.go`): `ListProjectBudgets` + `CreateInvoice` (invoices only). **No project_budgets insert.**
- `hydrate_project` worker is a logging stub (`worker/jobs.go:163`).

A freshly created project has zero tasks, so `RecalculateSchedule` returns `no tasks found for project`
(`service/schedule.go:69-71`) and the Gantt/Financials/HR screens all render empty states.

**Goal:** add the minimal, pattern-faithful write surface so operators (and a seed script) can author the
operational data, and so created tasks+deps flow through the EXISTING CPM engine to produce a real Gantt.

**Non-goals:** web authoring UI (deferred — see §6), CSV/file upload, the `hydrate_project` WBS-template
generator (deferred — see §3.4), cost_code↔budget linkage (escalated — see §8 OQ-3).

---

## 1. SCOPE — endpoints in priority order

| # | Endpoint | Keystone? | RBAC | Notes |
|---|----------|-----------|------|-------|
| 1 | `POST /api/v1/projects/{projectID}/schedule/import` | **YES** | min `superintendent` | Batch tasks+deps in ONE tx, cycle-rejected pre-persist, then auto-recalc. The Gantt lighter. |
| 2 | `POST /api/v1/projects/{projectID}/tasks` | no | min `superintendent` | Single-task create (cheap; reuses the batch store insert). NO auto-recalc. |
| 3 | `POST /api/v1/org/{orgID}/employees` | no | `owner`/`admin` | Single employee create. |
| 4 | `POST /api/v1/org/{orgID}/employees/{employeeID}/certifications` | no | `owner`/`admin` | Single cert create. |
| 5 | `POST /api/v1/projects/{projectID}/budgets` | no | `owner`/`admin` | Batch budget baseline (composite currency). |

**Project bulk-import (project + tasks + deps + budgets in one call): NOT a new endpoint.**
**Recommendation: compose the granular endpoints from the seed script (§7).** Reasoning:

- A single mega-endpoint would have to re-implement project creation (`ProjectService.CreateProject`
  already exists and is owner/admin-gated) plus tasks+deps+budgets, mixing two RBAC tiers
  (`owner/admin` for project/budget, `superintendent` for schedule) in one handler — a smell.
- The granular endpoints are independently testable and each follows one canonical service pattern.
- The seed script already has the `psql_scalar` RETURNING-id chaining idiom (`scripts/e2e-backend.sh`);
  driving it over HTTP exercises validation end-to-end (the point of a seed-through-the-API approach).
- If a true atomic project-import is later required, add it as a thin `POST /api/v1/projects/import`
  (owner/admin) that internally calls `CreateProject` then the schedule-import + budget-import tx flows.
  Flag the deferral in NEXT_STEPS rather than build speculative surface now.

---

## 2. Per-endpoint contracts

### 2.1 KEYSTONE — `POST /api/v1/projects/{projectID}/schedule/import`

Authors a whole task graph (tasks + dependencies) atomically, validates acyclicity, persists, then runs
CPM so the Gantt is populated in the same request. Dependencies are **wbs_code-keyed** (the import client
does not know server-assigned task UUIDs).

**RBAC:** `mw.RequireMinRole(mw.RoleSuperintendent)` — same gate as `/schedule/recalculate`
(`router.go:336`). CPM-affecting structural data.

**Request JSON:**
```json
{
  "tasks": [
    { "wbs_code": "01-00", "name": "Site Prep",  "duration_days": 3,
      "status": "pending", "percent_complete": 0, "assigned_crew": [] },
    { "wbs_code": "03-30", "name": "Foundation", "duration_days": 5 },
    { "wbs_code": "06-10", "name": "Framing",    "duration_days": 8 }
  ],
  "dependencies": [
    { "predecessor_code": "01-00", "successor_code": "03-30", "dependency_type": "FS", "lag_days": 0 },
    { "predecessor_code": "03-30", "successor_code": "06-10", "dependency_type": "FS" }
  ],
  "recalculate": true
}
```
- `tasks` required, non-empty. `dependencies` optional (a single-task or all-roots graph is legal).
- Per task: `wbs_code` (non-empty), `name` (non-empty), `duration_days` (required) are mandatory.
  `status` defaults `"pending"`, `percent_complete` defaults `0`, `assigned_crew` optional.
  CPM-output columns are NOT accepted in the body (ignored if present).
- Per dependency: `predecessor_code`, `successor_code` reference `tasks[].wbs_code` in **this batch**
  (a future variant may resolve against already-persisted tasks; v1 requires both endpoints in-batch).
  `dependency_type` defaults `"FS"`; `lag_days` defaults `0`.
- `recalculate` optional, **defaults `true`** (the whole point is a populated Gantt; see §8 OQ-1).

**Response 201:**
```json
{ "data": {
    "tasks": [ /* full ProjectTask rows incl. server IDs; CPM cols null until recalc */ ],
    "dependency_count": 2,
    "cpm_result": { /* physics.CPMResult, or null when recalculate=false */ },
    "recalculation_ms": 4
} }
```

**Validation rules (all BEFORE any write; return `service.ErrInvalidInput`):**
1. `len(tasks) >= 1` (else `ErrInvalidInput: tasks is required`). Mirrors the recalc "no tasks" guard.
2. Each `duration_days` in **`[1, 36500]`**. Lower bound is **1, not 0**: `migration 019` CHECK allows
   `0..36500`, but `physics.getTaskDuration` (`cpm.go:339-343`) rejects `DurationDays==0` with
   `ErrInvalidTaskDuration` (the float override fields are `json:"-"`, never loaded, so `DurationDays` is
   the *only* effective duration on ingress). Accepting 0 would persist a task that breaks the next recalc.
3. `status` ∈ {`pending`,`in_progress`,`completed`} via `isValidTaskStatus` (`service/schedule.go:354`).
   No DB CHECK exists; enforce service-side. Default `pending`.
4. `percent_complete` ∈ `[0,100]` (mirror `UpdateTask` guard `schedule.go:317`). Default 0.
5. `wbs_code` unique **within the batch** (in-memory set check → `ErrInvalidInput: duplicate wbs_code`).
   Also `UNIQUE(project_id, wbs_code)` against existing rows → a 23505 must map to `ErrInvalidInput`
   (409/400, see §2.6), not bubble as 500.
6. `dependency_type` ∈ {`FS`,`SS`,`FF`,`SF`} (`models/types/dependency.go:7-12`). **No `.Valid()` method
   exists** — validate against the four consts directly. An invalid value silently falls through to FS in
   `calculateConstraintDate` default branch (`cpm.go`), so reject at ingress.
7. `predecessor_code` and `successor_code` MUST both exist in the batch's task set
   (`ErrInvalidInput: dependency references unknown wbs_code`). `BuildDependencyGraph` silently SKIPS deps
   whose endpoints aren't nodes (`cpm.go:217-220`); the engine cannot be relied on to validate refs.
8. **Self-loop rejection:** `predecessor_code == successor_code` → `ErrInvalidInput`. A self-edge passes
   the schema (UNIQUE allows it, FKs satisfied) and then **PANICS** gonum `SetEdge`
   ("simple: adding self edge", `directed.go:200`). Hard create-time reject.
9. **No duplicate dependency pair** within batch (`UNIQUE(predecessor_id, successor_id)`); inverse pair
   (A→B + B→A) is NOT blocked by the UNIQUE and is a cycle → caught by rule 10.
10. **Cycle rejection:** build the in-memory graph from the about-to-insert tasks+deps and run
    `physics.DetectCycle` (see §3.2). On cycle → `ErrInvalidInput: dependency cycle: <wbs codes>`.
11. **`lag_days` bound:** enforce `[-3650, 3650]` (±10y). The column is CHECK-less and a huge magnitude
    has the same CPU-loop DoS shape that motivated `migration 019`'s duration cap. Negative lead is allowed
    (CPM math accepts it); bound the magnitude. (Owner may tune — see §8 OQ-5.)

**Service tx flow** (`ScheduleService.ImportSchedule`, new method, mirrors `RecalculateSchedule` shape):
```
validate ALL input (rules 1–11)         // before tx; rejected import leaves zero state
pgx.BeginTxFunc(ctx, pool, {}, func(tx):
  store.VerifyProjectInOrg(tx, projectID, callerOrgID)        // cross-tenant guard (store/projects.go:24)
  inserted := scheduleStore.InsertTasks(tx, projectID, taskParams)   // RETURNING ids; build wbs_code→UUID map
  resolve dependencies: predecessor_code/successor_code → predecessor_id/successor_id via the map
  scheduleStore.InsertDependencies(tx, projectID, depParams)
  audit.Record(tx, {Action:"schedule.imported", ResourceType:AuditResourceSchedule, ResourceID:projectID,
                    Metadata:{task_count, dependency_count}})
  if recalculate:
     // reuse the EXACT load→physics→persist→enqueue body of RecalculateSchedule on the SAME tx
     runRecalcOnTx(tx, projectID, callerOrgID, callerUserSub)   // see §3.1
)
map store errors (23505 → ErrInvalidInput; ErrNotFound → ErrNotFound)
```

> **Refactor note (do this first):** extract the existing tx body of `RecalculateSchedule`
> (`schedule.go:60-196`, everything inside `BeginTxFunc`) into an unexported
> `func (s *ScheduleService) recalcOnTx(ctx, tx, projectID, callerOrgID, callerUserSub) (*physics.CPMResult, time.Duration, error)`.
> `RecalculateSchedule` then becomes `BeginTxFunc → VerifyProjectInOrg → recalcOnTx`. `ImportSchedule`
> calls the same `recalcOnTx` inside its own tx after inserting. This guarantees imported data flows
> through the identical engine path with no duplicated CPM logic. The DelayCascade enqueue stays inside
> the tx (no phantom jobs). On a fresh project the prior critical set is empty, so the first import
> computes a clean baseline and enqueues a cascade (critical set changed from ∅).

### 2.2 `POST /api/v1/projects/{projectID}/tasks` (single-task)

Cheap convenience built on the same store insert. **RBAC:** min `superintendent`.

**Request:** one task object (same fields as a `tasks[]` element). **Response 201:** `{ "data": { "task": {…} } }`.
**Validation:** rules 2–5 above. **NO auto-recalc** (matches `UpdateTask` which explicitly does not recalc,
`schedule.go:314`); operator POSTs `/schedule/recalculate` after. Service method `CreateTask`; reuses
`InsertTasks` with a 1-element slice.

### 2.3 `POST /api/v1/org/{orgID}/employees`

**RBAC:** inherits the existing `/employees` group gate `r.Use(mw.RequireRole(RoleOwner, RoleAdmin))`
(`router.go:440`). **`requireOrgIDFromURL`** (`pipeline.go:532`) 403s on URL-vs-claim org mismatch.

**Request:**
```json
{ "first_name": "Dana", "last_name": "Cole", "role": "Foreman",
  "phone": "+1-555-0100", "hire_date": "2024-03-01", "user_id": null }
```
- Required (NOT NULL, no default): `first_name`, `last_name`, `role` (all TrimSpace, non-empty).
  `role` is the **trade/worker role** (free TEXT, no DB CHECK, distinct from the RBAC `users.role`) — do
  NOT validate against the RBAC enum.
- Optional: `phone` (`*string`, empty→nil), `hire_date` (`*time.Time`, RFC3339 or `YYYY-MM-DD` via
  `parseOptionalDate`), `user_id` (`*uuid.UUID`).
- `user_id` policy (see §8 OQ-2): if supplied, **verify it belongs to caller's org** before insert (new
  `HRStore.VerifyUserInOrg`); else reject `ErrInvalidInput` (prevents cross-org user_id leak). v1 default
  recommendation: **accept only `null`** and document `user_id` linking as a follow-on — the seed needs
  unlinked records (cert chips only need employee+cert rows, Domain 5 OQ-6). Implement the verify path but
  the seed sends null.
- Server-set: `org_id` from `callerOrgID` (NEVER body), `id`/`created_at` DB-defaulted.

**Response 201:** `{ "data": { "employee": {…models.Employee} } }`.

### 2.4 `POST /api/v1/org/{orgID}/employees/{employeeID}/certifications`

**RBAC:** owner/admin (inherits group gate). Path-scoped to `{employeeID}`.

**Request:**
```json
{ "cert_type": "osha_10", "cert_number": "OSHA-12345",
  "issued_date": "2024-01-15", "expiry_date": "2027-01-15", "status": "active" }
```
- Required: `cert_type` (non-empty), `expiry_date` (schema NOT NULL — required; parse `YYYY-MM-DD`/RFC3339).
- Optional: `cert_number` (`*string`), `issued_date` (`*time.Time`).
- `status` validated via `models.IsValidCertificationStatus` (`models/hr.go:33`) → only
  {`active`,`expired`,`revoked`}; default `"active"` when omitted (matches schema DEFAULT).
- `certifications` has **no org_id column** — tenant isolation is INDIRECT. Service MUST call
  `HRStore.VerifyEmployeeInOrg(tx, employeeID, callerOrgID)` (`store/hr.go:27`) **inside the tx BEFORE
  insert**; a cross-org `employeeID` returns `ErrEmployeeNotFound` → 404 (never leak existence, never 403).

**Response 201:** `{ "data": { "certification": {…models.Certification} } }`.

### 2.5 `POST /api/v1/projects/{projectID}/budgets` (batch baseline)

**RBAC:** `mw.RequireRole(RoleOwner, RoleAdmin)` — same as the existing `GET /budgets` (`router.go:359`)
and the `/invoices` group (`router.go:364`). Financial data, exact-role gated.

**Request (batch — a baseline is naturally a set of WBS-phase lines):**
```json
{ "budgets": [
    { "wbs_code": "01-00", "phase_name": "Site Prep",  "estimated_cost_cents": 4500000, "currency_code": "USD" },
    { "wbs_code": "03-30", "phase_name": "Foundation", "estimated_cost_cents": 12000000, "currency_code": "USD" }
] }
```
- Per line required: `wbs_code` (non-empty TEXT), `phase_name` (non-empty), `estimated_cost_cents`
  (`int64`, `>= 0` — column DEFAULT 0; no negative baselines), `currency_code` (validated via
  `currency.Validate` → USD|CAD only; empty string REJECTED, `currency/currency.go:33`).
- **ONE `currency_code` per line, fanned out to all three columns** (estimated/committed/actual) by the
  service to satisfy `chk_budget_currency_match` (`migration 002:22`). Do NOT expose three currency inputs.
- `committed_cost_cents`/`actual_cost_cents` optional, **default 0** (true baseline = estimate only;
  committed/actual accrue later via invoices/procurement — see §8 OQ-4 recommendation: accept but default 0).
- Mixed currencies across lines in one project: **allowed** by schema + rollup (GROUP BY currency_code).
  Validate each line independently; no project-wide single-currency enforcement (OQ-6: defer).
- `wbs_code` is free TEXT with NO FK to `cost_codes`. Budgets share the per-project WBS namespace with
  `project_tasks` by convention only; do NOT introduce a hidden join (OQ-3: cost_code linkage escalated).

**Validation:** all lines validated BEFORE any insert (a bad line rejects the whole batch, no partial
state — mirrors `createInvoiceTx`'s validate-before-write). `UNIQUE(project_id, wbs_code)` collision →
map 23505 to `ErrInvalidInput` (reject, not silent UPSERT — see §8 OQ-7 recommendation).

**Response 201:** `{ "data": { "budgets": [ {…full ProjectBudget rows} ] } }`.

### 2.6 Error mapping (all handlers)

Reuse the existing per-handler `writeServiceError` mappers verbatim:
`ErrNotFound→404 NOT_FOUND`, `ErrInvalidInput→400 VALIDATION_ERROR`,
`ErrCrossCurrency→422 CROSS_CURRENCY_ERROR` (budget only), default→`500 INTERNAL_ERROR` (opaque).
- Schedule handler: `writeScheduleError` (`schedule.go`) — extend if it lacks `ErrInvalidInput` mapping.
- HR handler: `HRHandler.writeServiceError` (`fleet.go:219`) — already maps the needed sentinels.
- Budget handler: `FinancialsHandler.writeServiceError` (`financials.go:218`) — already complete.

**23505 (unique violation) mapping:** add to `mapStoreError` (or a sibling) a `pgconn.PgError` check:
```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) && pgErr.Code == "23505" { return ErrInvalidInput }
```
Pattern precedent: `auth.go:629-637` already does a 23505 check. Put a shared helper in `service` (e.g.
`isUniqueViolation(err) bool`) so schedule + budget services reuse it.

---

## 3. CPM integration (keystone detail)

### 3.1 How imported data reaches physics

The import tx inserts tasks (RETURNING ids), resolves wbs_code→UUID for deps, inserts deps, then — when
`recalculate=true` — calls the extracted `recalcOnTx` (§2.1 refactor note) **on the same tx**. `recalcOnTx`
runs the unchanged pipeline: `GetProjectTasks` + `GetProjectDependencies` (now returning the just-inserted
rows, visible within the tx) → `physics.BuildDependencyGraph(tasks, deps)` (`cpm.go:196`) →
`ForwardPass`/`BackwardPass` → `UpdateSchedule` writes `early_*`,`late_*`,`total_float`,`is_critical`.
Determinism preserved: `duration_days int` flows into `DurationDays` (integer-ns engine); no floats added.

**project_start_date anchor:** CPM roots at `COALESCE(project_start_date, permit_issued_date, created_at)`
(`GetProjectStartDate`, `store/schedule.go:84`), falling back to `now()` if all null. The seed sets
`project_start_date` at project create so the Gantt has a deterministic, demo-stable root (§7).

### 3.2 Cycle rejection BEFORE any write

In `ImportSchedule`, **before** `BeginTxFunc**, build the in-memory graph from the proposed rows:
```go
// synthesize ProjectTask/TaskDependency with deterministic placeholder IDs keyed by wbs_code
graph := physics.BuildDependencyGraph(proposedTasks, proposedDeps)
if err := physics.DetectCycle(graph); err != nil {           // cpm.go:251 — topo.Sort + Unorderable
    return nil, 0, fmt.Errorf("%w: %v", ErrInvalidInput, err) // names cyclic WBS codes
}
```
- Self-loops are rejected by rule 8 **before** this call (gonum `SetEdge` panics on self-edge, so it must
  never reach `BuildDependencyGraph`). Guard self-loops in the validation pass.
- `DetectCycle` uses `topo.Sort`; on a true cycle it returns `topo.Unorderable` and reports the WBS codes
  (`cpm.go:262-272`) for a useful 400 message. The current `RecalculateSchedule` never calls `DetectCycle`
  (a cycle surfaces only as a recalc-time "topological sort failed"); the import path calls it explicitly
  up front so the engine never sees an unschedulable graph and no partial rows are written.
- Use stable synthetic UUIDs (e.g. `uuid.NewSHA1(namespace, []byte(wbs_code))`) for the pre-persist graph
  so node identity is consistent; the real DB UUIDs are assigned at insert.

### 3.3 What triggers CPM after import

`recalculate=true` (default) → in-tx recalc → DelayCascade enqueued iff critical set changed (always true
on first compute: ∅ → non-empty). `recalculate=false` → tasks land with NULL CPM cols; operator hits
`/schedule/recalculate` later (the e2e `--seed-schedule` behavior). Both supported; default is recalc.

### 3.4 `hydrate_project` stub — leave as-is

Do NOT repurpose `hydrate_project` (`worker/jobs.go:163`) as the import worker in this phase. The
synchronous `/schedule/import` endpoint is the deliverable; an async WBS-template generator that fills
`hydrate_project` is a separate, larger piece (needs a template catalog + DHSM seeding — no template data
or loader exists today; `models.WBSTask`/`WBSTemplateDep` are unused shells). Note this in NEXT_STEPS.

---

## 4. New code inventory

### 4.1 Store methods (`internal/store/schedule.go`)
```go
type InsertTaskParams struct {
    ProjectID uuid.UUID; WBSCode, Name, Status string
    DurationDays, PercentComplete int; AssignedCrew []uuid.UUID
}
// InsertTasks bulk-inserts tasks, RETURNING full rows (id + DB defaults). CPM cols left default/NULL.
func (s *ScheduleStore) InsertTasks(ctx, tx, params []InsertTaskParams) ([]models.ProjectTask, error)

type InsertDependencyParams struct {
    ProjectID, PredecessorID, SuccessorID uuid.UUID
    DependencyType string; LagDays int
}
func (s *ScheduleStore) InsertDependencies(ctx, tx, params []InsertDependencyParams) error
```
INSERT shape mirrors `CreateInvoice` (`financials.go:306`): explicit column list, `RETURNING` the read
columns, never set CPM-output columns. Use one multi-row INSERT or a loop; a loop is fine (batches are small).

### 4.2 Store methods (`internal/store/hr.go`)
```go
type CreateEmployeeParams struct {
    OrgID uuid.UUID; FirstName, LastName, Role string
    Phone *string; HireDate *time.Time; UserID *uuid.UUID
}
func (s *HRStore) CreateEmployee(ctx, tx, p CreateEmployeeParams) (models.Employee, error)

type CreateCertificationParams struct {
    EmployeeID uuid.UUID; CertType string; CertNumber *string
    IssuedDate *time.Time; ExpiryDate time.Time; Status string
}
func (s *HRStore) CreateCertification(ctx, tx, p CreateCertificationParams) (models.Certification, error)

// Optional (user_id linking path):
func (s *HRStore) VerifyUserInOrg(ctx, tx, userID, orgID uuid.UUID) error
```

### 4.3 Store methods (`internal/store/financials.go`)
```go
type CreateProjectBudgetParams struct {
    ProjectID uuid.UUID; WBSCode, PhaseName, CurrencyCode string
    EstimatedCostCents, CommittedCostCents, ActualCostCents int64
}
// CreateProjectBudget inserts one row, fanning CurrencyCode into all three *_currency_code columns.
func (s *FinancialsStore) CreateProjectBudget(ctx, tx, p CreateProjectBudgetParams) (models.ProjectBudget, error)
```

### 4.4 Service methods
- `ScheduleService.ImportSchedule(ctx, projectID, callerOrgID, callerUserSub string, in ImportScheduleInput) (*ImportScheduleResult, error)` and `CreateTask(...)` (`service/schedule.go`). Extract `recalcOnTx` (§2.1).
- `HRService` — **widen `NewHRService` to take an `AuditRecorder`** (currently `NewHRService(pool, hr)`,
  `hr.go:33`, no audit). Add `CreateEmployee` + `CreateCertification` mirroring `FleetService.CreateAsset`
  (`fleet.go:94-146`): one `BeginTxFunc`, verify, insert, audit on same tx.
- `BudgetService.CreateProjectBudgets(ctx, callerOrgID, callerUserSub, projectID, lines []CreateProjectBudgetLine) ([]models.ProjectBudget, error)` — mirror `createInvoiceTx` validate-before-write; `VerifyProjectInOrg` once per batch; one audit row per line (mirrors invoice-per-row, OQ-8 recommendation).

### 4.5 New `AuditResource*` constants (`internal/service/audit.go:17-38`)
`AuditResourceProjectTask` (="project_task") and `AuditResourceSchedule` (="schedule") **already exist**.
Add:
```go
AuditResourceTaskDependency = "task_dependency"
AuditResourceEmployee       = "employee"
AuditResourceCertification  = "certification"
AuditResourceProjectBudget  = "project_budget"
```
Action strings (lower-snake `<resource>.<verb>`): `schedule.imported`, `task.created`,
`hr.employee.created`, `hr.certification.created`, `budget.created`. (Resource types are free-form TEXT in
`audit_log.resource_type` — **no migration needed.**)

### 4.6 Models — no new structs required
`models.ProjectTask`, `TaskDependency`, `Employee`, `Certification`, `ProjectBudget` already exist with
correct json tags. Request/response DTOs are handler-local request structs (like `createInvoiceRequest`,
`createAssetRequest`) + the standard `{data,…}` envelope. The wbs_code-keyed dep input is a handler struct
with `predecessor_code`/`successor_code` (mirrors `models.WBSTemplateDep`, `project.go:81-86`).

### 4.7 Handlers (`internal/api/`)
- `ScheduleHandler.Import` + `ScheduleHandler.CreateTask` (`schedule.go`); extend `ScheduleServicer`
  interface (`schedule.go:21`) with `ImportSchedule` + `CreateTask`.
- `HRHandler.CreateEmployee` + `HRHandler.CreateCertification` (`fleet.go`); extend `HRServicer`
  (`fleet.go:166`).
- `FinancialsHandler.CreateBudgets` (`financials.go`); extend `BudgetServicer` (`financials.go:21`).
All handlers: `parseUUIDFromURL`/`requireOrgIDFromURL` + `callerOrgIDFromClaims` +
`mw.MustClaimsFromContext(...).Sub`; decode body → 400 on bad JSON; `writeJSON(..., StatusCreated, …)`.

---

## 5. WIRING CHECKLIST

`HRService`, `ScheduleService`, `BudgetService` are **already non-optional `RouterConfig` fields**
(`router.go:46-50`) and their handlers are already instantiated in `NewRouter` (`router.go:180-188`).
So no new config fields — just extend the existing `Servicer` interfaces and mount new routes.

1. **Interfaces (`internal/api`):** add `ImportSchedule`+`CreateTask` to `ScheduleServicer`,
   `CreateEmployee`+`CreateCertification` to `HRServicer`, `CreateProjectBudgets` to `BudgetServicer`.
   (No new RouterConfig fields.)
2. **Routes (`router.go`)** — inline in existing trees:
   - Under `/api/v1/projects/{projectID}/schedule` (router.go:335):
     `r.With(mw.RequireMinRole(mw.RoleSuperintendent)).Post("/import", schedule.Import)`
   - Under `/api/v1/projects/{projectID}` next to `/tasks` (router.go:354):
     `r.With(mw.RequireMinRole(mw.RoleSuperintendent)).Post("/tasks", schedule.CreateTask)`
   - Under `/api/v1/projects/{projectID}` next to `GET /budgets` (router.go:359):
     `r.With(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin)).Post("/budgets", financials.CreateBudgets)`
   - Inside the `/api/v1/org/{orgID}/employees` group (router.go:439, owner/admin already on group):
     `r.Post("/", hr.CreateEmployee)` and
     `r.Post("/{employeeID}/certifications", hr.CreateCertification)`
3. **`cmd/server/main.go`:** change line 204 to
   `hrService := service.NewHRService(pool, hrStore, auditService)` (thread the existing `auditService`).
   `scheduleService` (192) and `budgetService` (188) already receive `auditService`; `scheduleService`
   already holds `riverClient` (for the DelayCascade enqueue on import-recalc). No RouterConfig literal
   change (fields already wired at main.go:376/378/382).
4. **Tests:** handlers depend on the narrow `*Servicer` interfaces → fake them (pattern:
   `mockScheduleService`, `schedule_test.go:18`). Add `ImportSchedule`/`CreateTask`/etc. to the mocks.

---

## 6. WEB SCOPE — API-only (recommend; defer web authoring)

**Recommendation: ship API + seed only; no web client methods this phase.** All target screens already
have working READ clients (`listProjectTasks`, `getGantt`, `listProjectBudgets`, `listEmployees`,
`listCertifications`, `getFinancialsSummary`); once ingress persists data they render with **zero frontend
change**. `task_dependencies` has no web read-client and is never rendered (deps are pure CPM input), so the
dependency-import API needs no matching web method.

If/when in-console authoring becomes a confirmed requirement (follow-on phase), add thin wrappers following
the existing convention (`api.post<T>(url, body).then(r => r.x)`, `normalizeCents` on `*_cents` responses,
`encodeURIComponent` path params): `importSchedule`/`createTask` → `api/endpoints/schedule.ts`,
`createEmployee`/`createCertification` → `hr.ts`, `createBudgets` → a budgets client. Flagged in NEXT_STEPS.

---

## 7. SEED DESIGN — `scripts/seed-fork-demo.sh`

New script, **API-driven, owner-authenticated, parameterized, idempotent-by-reset**. Drives the NEW
endpoints over HTTP (exercises validation end-to-end) rather than raw SQL. Models the
`psql_scalar`/RETURNING-id chaining of `scripts/e2e-backend.sh` but with `curl` + `jq` capturing JSON ids.

**Parameters (env, with defaults):** `BASE_URL` (default `http://localhost:8080`), `ORG_SLUG`/`ORG_ID`,
owner credentials (`SEED_OWNER_EMAIL`/`SEED_OWNER_PASSWORD`) OR a `BUILDOS_BOOTSTRAP_TOKEN` to claim,
`PROJECT_NAME` (default "Kelbrook Residence"), `GSF` (default 3200, in the documented 1500–6000 envelope),
`CURRENCY` (default USD).

**Flow:**
1. **Auth:** `POST /api/v1/auth/claim` (with bootstrap token, first run) or `POST /api/v1/auth/login`;
   capture the access token for `Authorization: Bearer`.
2. **Onboarding:** the org must be `onboarding_complete=true` or every ingress route 403s `SETUP_INCOMPLETE`
   (SetupGate, router 238-496). The seed runs the `/api/v1/setup/*` wizard steps (company → ≥1 trade →
   ≥1 cost_code → default calendar → complete) OR — for a fast staging reset — flips
   `onboarding_complete` via SQL (documented escape hatch, matching e2e `--seed-field`). Recommend driving
   the wizard so the path is exercised.
3. **Project:** `POST /api/v1/projects` (owner/admin) with `name`, `gsf`, and an explicit
   `project_start_date` (deterministic Gantt root). Capture `project_id`.
4. **Schedule:** `POST /api/v1/projects/{id}/schedule/import` with the Kelbrook task graph (a realistic
   ~10–15 WBS-phase residential build: sitework → foundation → framing → MEP rough → drywall → finishes,
   linear+branch FS deps) and `recalculate:true` → Gantt populated + critical path + cascade feed card.
5. **Budgets:** `POST /api/v1/projects/{id}/budgets` with one baseline line per task WBS phase (so
   `project_budgets.wbs_code` aligns with `project_tasks.wbs_code`). Lights up Project-detail Budget tab.
6. **HR:** `POST /api/v1/org/{orgID}/employees` for ~4 employees, each
   `POST .../{employeeID}/certifications` with a mix of expiry dates (one near-expiry, one expired, one
   far-future) so the HR screen's cert-alert status chips render all states.
7. **Financials rollup:** the Financials Summary/By-Project tabs INNER JOIN `project_budgets` and read
   `corporate_budgets` — so after seeding budgets the seed must **trigger the `corporate_rollup` River
   job** (enqueue via an admin path or run `bin/worker` once / direct `RunCorporateRollup`) or the Summary
   tab stays empty. Document this step explicitly.

**Repeatability:** guard with a `--reset` flag that deletes the demo project + its tasks/deps/budgets and
the demo employees first (the UNIQUE(project_id, wbs_code) constraints make a re-run otherwise 400 on
duplicate wbs_code — by design, since v1 budget/task import is reject-on-conflict, not upsert).

---

## 8. OPEN QUESTIONS — resolved or escalated

- **OQ-1 (auto-recalc?)** RESOLVED: `/schedule/import` **auto-recalcs by default** (`recalculate:true`),
  overridable to `false`. The keystone goal is a populated Gantt; fresh-project recalc computes a clean
  baseline (prior critical set ∅).
- **OQ-2 (batch vs per-row tasks/deps)** RESOLVED: **single atomic batch** `/schedule/import`
  (tasks+deps in one tx). Per-row deps cannot guarantee acyclicity without re-validating the whole project
  graph each call. A cheap single-task `POST /tasks` is added for convenience but does NOT carry deps.
- **OQ-3 (deps keyed by wbs_code vs UUID)** RESOLVED: **wbs_code-keyed** (`predecessor_code`/
  `successor_code`), resolved to UUIDs server-side post-insert. A fresh import has no task UUIDs client-side.
- **OQ-4 (re-import / upsert vs reject)** RESOLVED for v1: **reject** on `UNIQUE(project_id, wbs_code)`
  conflict (23505 → 400). No destructive DELETE/UPSERT migration in scope. Seed re-runs use `--reset`.
  (If product later wants idempotent re-import, add an explicit `replace=true` flag + a
  `-- buildos:destructive:` annotated migration — flagged in NEXT_STEPS.)
- **OQ-5 (lag_days bound)** RESOLVED: enforce `[-3650, 3650]` (±10y) service-side (CHECK-less column,
  DoS-shape). Owner may tune the constant.
- **OQ-6 (budget single currency per project?)** RESOLVED: **not enforced** — schema + rollup permit mixed
  currency across phases; validate each line independently.
- **OQ-7 (cost_code ↔ budget linkage)** **ESCALATE to owner.** Today there is ZERO relationship between
  `project_budgets.wbs_code` (per-project WBS, aligns with tasks) and the org `cost_codes` catalog (CSI
  MasterFormat, `UNIQUE(org_id,code)`, FK to organizations only). Linking them is a NEW schema relationship
  (FK or validation) beyond this phase. **Recommend: keep them decoupled; budgets key on project WBS.**
  Owner to confirm before any cost-code-keyed estimate work. (Default to decoupled if no objection.)
- **OQ-8 (employee user_id on create)** RESOLVED for v1: implement the org-verified path but the seed sends
  `null` (unlinked records light up HR fine). Document that operator-supplied `user_id` is org-verified.
- **OQ-9 (audit granularity for batches)** RESOLVED: schedule import → one `schedule.imported` audit row
  (project-level, with counts); budgets → one `budget.created` row per line (mirrors invoice-per-row).
- **OQ-10 (date-range sanity, e.g. issued ≤ expiry)** Deferred (no such check exists today); not blocking.

**API_CONTRACT.md:** add the five new endpoints (method, path, request/response, status codes, RBAC) — the
contract currently documents only `GET /budgets` and the List HR endpoints. Per the dual-agent protocol,
routes/schemas/status codes must match the contract exactly; update it as part of implementation.
