# API Contract Specification

**System:** BuildOS (System of Execution)
**Pipeline Stage:** 07 - Architecture Spec
**Date:** 2026-04-02 (rev. 2026-06-01 — standalone pivot: native auth, BYOK vault, native AI; Brain/A2A/billing removed)
**Status:** COMPLETE
**Base URL:** `/api/v1`
**Auth:** Native email/password. BuildOS owns identity — it mints and validates its own RS256 JWTs (no external OIDC provider). All endpoints require a Bearer access token EXCEPT the public probes (`/health`, `/ready`, `/metrics`) and the unauthenticated `/api/v1/auth/*` surface that issues the tokens.

---

## 1. Authentication & Authorization

BuildOS is a self-contained standalone deployment. It issues native access tokens (RS256, signed with the fork's `JWT_PRIVATE_KEY_PEM`) and server-revocable opaque refresh tokens. There is no external identity provider.

### 1.1 Native Auth Endpoints (unauthenticated)

These mint the credentials the rest of the API requires, so they mount OUTSIDE the auth middleware and are exempt from the SetupGate.

#### POST /api/v1/auth/claim
- **Purpose:** Redeem a one-time bootstrap token to create the fork's first owner with a native credential.
- **Body:** `{ token, email, password, display_name }`
- **Response:** `201 { data: { access_token, token_type, expires_in, refresh_token, user } }`
- **Errors:** `401 INVALID_BOOTSTRAP_TOKEN` (uniform on any bootstrap-token failure), `409 FIRST_OWNER_EXISTS`, `400 VALIDATION_ERROR`

#### POST /api/v1/auth/login
- **Body:** `{ email, password }`
- **Response:** `200 { data: { access_token, token_type, expires_in, refresh_token, user } }`
- **Errors:** `401 INVALID_CREDENTIALS`

#### POST /api/v1/auth/refresh
- **Body:** `{ refresh_token }` (rotated — the presented token is consumed and a new one returned)
- **Response:** `200 { data: { access_token, token_type, expires_in, refresh_token, user } }`
- **Errors:** `401 INVALID_REFRESH_TOKEN`

#### POST /api/v1/auth/logout
- **Body:** `{ refresh_token }`
- **Response:** `204 No Content` (idempotent — 204 even if the token was already revoked)

#### POST /api/v1/auth/password-reset/request
- **Body:** `{ email }`
- **Response:** `202 Accepted` — always 202, never reveals whether the email matched a user (no enumeration). Delivery is via the org's configured Resend key.

#### POST /api/v1/auth/password-reset/confirm
- **Body:** `{ token, password }`
- **Response:** `204 No Content` — consumes the reset token and revokes all of the user's active sessions.
- **Errors:** `400 INVALID_RESET_TOKEN`

**Token shape:** `token_type` is `"Bearer"`; `expires_in` is the access-token lifetime in seconds (default 900 = 15 min). Refresh tokens default to 30 days; password-reset tokens to 1 hour.

### 1.2 JWT Claims (BuildOS-issued)

| Claim | Type | Description |
|-------|------|-------------|
| `sub` | string | User ID (subject) |
| `org_id` | string | Organization ID |
| `role` | string | `owner`, `admin`, `superintendent`, `field_worker` |
| `plan_tier` | string | `free`, `pro`, `enterprise` (gates AI endpoints) |
| `iss` | string | `buildos` |
| `aud` | string | `buildos` |
| `exp` | int | Expiry timestamp |
| `iat` | int | Issued-at timestamp |

### 1.3 Role-Based Access Control

| Endpoint Group | owner | admin | superintendent | field_worker |
|---------------|-------|-------|----------------|-------------|
| Auth endpoints (`/auth/*`) | Public (unauthenticated) | | | |
| Setup wizard (`/setup/*`) | ✓ | ✓ | ✗ | ✗ |
| Integrations vault (`/integrations/*`) | ✓ | ✓ | ✗ | ✗ |
| Agent config registry (`/admin/agents/*`) | ✓ | ✓ | ✗ | ✗ |
| Connector registry (`/admin/connectors/*`) | ✓ | ✓ | ✗ | ✗ |
| Financial endpoints | ✓ | ✓ | Read-only | ✗ |
| Schedule endpoints | ✓ | ✓ | ✓ | Read-only |
| Pipeline endpoints | ✓ | ✓ | Read-only | ✗ |
| Fleet endpoints | ✓ | ✓ | ✓ | Read-only |
| HR endpoints | ✓ | ✓ | Read-only | ✗ |
| Feed endpoints | ✓ | ✓ | ✓ | ✓ |
| Field endpoints | ✓ | ✓ | ✓ | ✓ |
| AI agent endpoints (`/agents/*`) | role gate per route (no plan-tier gate — ESC-002) | | | |

**SetupGate:** every authenticated request to an org with `onboarding_complete=false` gets `403 SETUP_INCOMPLETE`, except the exempt prefixes `/api/v1/setup`, `/health`, `/ready`, `/metrics`.

---

## 2. Common Response Patterns

### 2.1 Success Response
```json
{
  "data": { ... },
  "meta": {
    "request_id": "uuid",
    "timestamp": "2026-04-02T12:00:00Z"
  }
}
```

### 2.2 Error Response
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable message",
    "details": [ { "field": "name", "reason": "required" } ]
  },
  "meta": {
    "request_id": "uuid",
    "timestamp": "2026-04-02T12:00:00Z"
  }
}
```

### 2.3 Pagination
```
GET /api/v1/resource?page=1&per_page=50
Response includes:
  "meta": { "page": 1, "per_page": 50, "total": 142, "total_pages": 3 }
```

### 2.4 Monetary Values (Composite Currency Pattern)

ALL monetary fields follow the Composite Currency Pattern:
- `*_cents` (BIGINT) paired with `*_currency_code` or `currency_code` (string, "USD" or "CAD")
- Cross-currency arithmetic is FORBIDDEN — clients must never sum values with different currency_codes
- Aggregation endpoints group by currency_code automatically
- Display formatting (e.g. "$1,234.56") is a frontend concern only

---

## 3. Health Check

### GET /health
- **Auth:** None
- **Response:** `200 { "status": "ok", "version": "1.0.0" }`

---

## 4. Project Endpoints

### GET /api/v1/projects
- **Auth:** JWT (org-scoped)
- **Query:** `?status=active&page=1&per_page=50`
- **Response:** `200 { data: { projects: []Project }, meta }`

### POST /api/v1/projects
- **Auth:** JWT (owner, admin)
- **Body:** `{ name, address?, permit_issued_date?, gsf?, project_start_date? }`
- **Response:** `201 { data: { project: Project } }`

### GET /api/v1/projects/{projectID}
- **Auth:** JWT (org-scoped)
- **Response:** `200 { data: { project: Project } }`

### PUT /api/v1/projects/{projectID}
- **Auth:** JWT (owner, admin)
- **Body:** `{ name?, address?, status?, gsf? }`
- **Response:** `200 { data: { project: Project } }`

### Project Object
```json
{
  "id": "uuid",
  "org_id": "uuid",
  "name": "string",
  "address": "string",
  "permit_issued_date": "date",
  "project_start_date": "date",
  "status": "active|completed|archived",
  "gsf": 3200,
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

---

## 5. Schedule Endpoints

### POST /api/v1/projects/{projectID}/schedule/import
- **Auth:** JWT (owner, admin, superintendent — `RequireMinRole(superintendent)`; CPM-affecting structural data, same gate as `/recalculate`)
- **Purpose:** Author a whole task graph (tasks + dependencies) atomically, then run CPM so the Gantt is populated in the same request. The operator ingress path for a fresh project.
- **Body:**
  ```json
  {
    "tasks": [
      { "wbs_code": "01-00", "name": "Site Prep", "duration_days": 3,
        "status": "pending", "percent_complete": 0, "assigned_crew": [] }
    ],
    "dependencies": [
      { "predecessor_code": "01-00", "successor_code": "03-30", "dependency_type": "FS", "lag_days": 0 }
    ],
    "recalculate": true
  }
  ```
  - `tasks` required, non-empty. Per task: `wbs_code`, `name`, `duration_days` mandatory. `status` defaults `"pending"`, `percent_complete` defaults `0`. CPM-output columns are ignored if present.
  - `dependencies` optional, **wbs_code-keyed** (the client doesn't know server task UUIDs; codes are resolved server-side). `dependency_type` defaults `"FS"`; `lag_days` defaults `0`.
  - `recalculate` optional, **defaults `true`** (the keystone goal is a populated Gantt).
- **Response:** `201 { data: { tasks: []ProjectTask, dependency_count: int, cpm_result: CPMResult|null, recalculation_ms: int } }`
- **Validation (all pre-write; `400 VALIDATION_ERROR`):** non-empty tasks; `duration_days` ∈ **[1, 36500]** (lower bound 1 — `physics.getTaskDuration` rejects 0); `status` ∈ {pending,in_progress,completed}; `percent_complete` ∈ [0,100]; `wbs_code` unique in batch (and vs existing rows → 23505→400); `dependency_type` ∈ {FS,SS,FF,SF}; dep endpoints must exist in the batch; **self-loops rejected** (`predecessor_code == successor_code`); no duplicate dep pair; `lag_days` ∈ [-3650, 3650]; **dependency cycles rejected** (named WBS codes). A cross-tenant project → `404 NOT_FOUND`.

### POST /api/v1/projects/{projectID}/tasks
- **Auth:** JWT (owner, admin, superintendent — `RequireMinRole(superintendent)`)
- **Purpose:** Create a single task. Reuses the import store insert; **does NOT auto-recalc** (operator POSTs `/schedule/recalculate` afterward, mirroring `PUT /tasks/{taskID}`).
- **Body:** `{ wbs_code, name, duration_days, status?, percent_complete?, assigned_crew? }`
- **Response:** `201 { data: { task: ProjectTask } }`
- **Validation:** same task rules as import (`400 VALIDATION_ERROR`); cross-tenant project → `404`.

### POST /api/v1/projects/{projectID}/schedule/recalculate
- **Auth:** JWT (owner, admin, superintendent)
- **Body:** `{}` (empty — recalculates from current task/dependency state)
- **Response:** `200 { data: { cpm_result: CPMResult, recalculation_ms: int } }`
- **NFR:** <800ms end-to-end, <200ms physics computation (80-task graph)

### GET /api/v1/projects/{projectID}/schedule/gantt
- **Auth:** JWT (org-scoped)
- **Response:** `200 { data: { tasks: []TaskSchedule, critical_path: []uuid, project_end: timestamp, dependencies: []TaskDependency } }`
- **`dependencies`** is the task-dependency edge set (`{ id, project_id, predecessor_id, successor_id, dependency_type: 'FS'|'SS'|'FF'|'SF', lag_days }`), loaded from `task_dependencies` in the same read-only tx; stable `[]` (never `null`) when the project has no edges. The frontend draws dependency arrows from these (FS chains in v1).

### GET /api/v1/projects/{projectID}/tasks
- **Auth:** JWT (org-scoped)
- **Query:** `?status=pending&is_critical=true`
- **Response:** `200 { data: { tasks: []ProjectTask } }`

### PUT /api/v1/projects/{projectID}/tasks/{taskID}
- **Auth:** JWT (owner, admin, superintendent)
- **Body:** `{ percent_complete?, assigned_crew?, status? }`
- **Response:** `200 { data: { task: ProjectTask } }`

### TaskSchedule Object
```json
{
  "id": "uuid",
  "wbs_code": "9.2",
  "name": "Roof Framing",
  "duration_days": 5,
  "early_start": "timestamp",
  "early_finish": "timestamp",
  "late_start": "timestamp",
  "late_finish": "timestamp",
  "total_float": 2,
  "is_critical": true,
  "status": "pending|in_progress|completed",
  "percent_complete": 0,
  "assigned_crew": ["uuid"]
}
```

---

## 6. Financial Endpoints (Composite Currency Pattern)

### GET /api/v1/org/{orgID}/financials/summary
- **Auth:** JWT (owner, admin; superintendent read-only)
- **Query:** `?currency=USD` (optional — omit for all currencies)
- **Response:** `200 { data: { corporate_budgets: []CorporateBudget, ar_aging: []ARAgingSnapshot } }`
- **Note:** Results grouped by currency_code. No cross-currency aggregation.

### GET /api/v1/org/{orgID}/financials/ar-aging
- **Auth:** JWT (owner, admin)
- **Query:** `?currency=USD`
- **Response:** `200 { data: { snapshots: []ARAgingSnapshot } }`

### GET /api/v1/org/{orgID}/financials/projects
- **Auth:** JWT (owner, admin)
- **Query:** `?currency=USD`
- **Response:** `200 { data: { projects: []ProjectFinancial } }`

### GET /api/v1/projects/{projectID}/budgets
- **Auth:** JWT (owner, admin)
- **Response:** `200 { data: { budgets: []ProjectBudget } }`

### POST /api/v1/projects/{projectID}/budgets
- **Auth:** JWT (owner, admin — `RequireRole(owner, admin)`; financial data, same gate as `GET /budgets`)
- **Purpose:** Write a batch budget baseline (one line per WBS phase). Composite Currency Pattern.
- **Body:**
  ```json
  { "budgets": [
      { "wbs_code": "01-00", "phase_name": "Site Prep", "estimated_cost_cents": 4500000, "currency_code": "USD" },
      { "wbs_code": "03-30", "phase_name": "Foundation", "estimated_cost_cents": 12000000, "currency_code": "USD" }
  ] }
  ```
  - Per line required: `wbs_code`, `phase_name`, `estimated_cost_cents` (`>= 0`), `currency_code` (USD|CAD; **empty rejected**). `committed_cost_cents` / `actual_cost_cents` optional, **default 0**.
  - **One `currency_code` per line, fanned by the server into all three columns** (estimated/committed/actual) to satisfy `chk_budget_currency_match`. Mixed currencies *across* lines are allowed (rollup groups by `currency_code`).
  - `wbs_code` is free TEXT — **no FK to `cost_codes`** (see escalation note below); aligns with `project_tasks.wbs_code` by convention.
- **Response:** `201 { data: { budgets: []ProjectBudget } }`
- **Validation:** all lines validated before any insert (a bad line rejects the whole batch — no partial state). `400 VALIDATION_ERROR` (incl. unsupported/empty currency, negative cents, duplicate wbs in batch, `UNIQUE(project_id, wbs_code)` 23505→400 reject-on-conflict). `422 CROSS_CURRENCY_ERROR` on forbidden cross-currency math. Cross-tenant project → `404`.

### POST /api/v1/projects/{projectID}/invoices
- **Auth:** JWT (owner, admin)
- **Body:** `{ vendor_name, amount_cents, currency_code, wbs_code?, invoice_number?, due_date? }`
- **Response:** `201 { data: { invoice: Invoice } }`

### PUT /api/v1/projects/{projectID}/invoices/{invoiceID}
- **Auth:** JWT (owner, admin)
- **Body:** `{ status?, paid_date? }`
- **Response:** `200 { data: { invoice: Invoice } }`

### ProjectBudget Object
```json
{
  "id": "uuid",
  "project_id": "uuid",
  "wbs_code": "9.0",
  "phase_name": "Roofing",
  "estimated_cost_cents": 450000,
  "estimated_cost_currency_code": "USD",
  "committed_cost_cents": 380000,
  "committed_cost_currency_code": "USD",
  "actual_cost_cents": 320000,
  "actual_cost_currency_code": "USD"
}
```

### CorporateBudget Object
```json
{
  "id": "uuid",
  "org_id": "uuid",
  "fiscal_year": 2026,
  "quarter": 2,
  "currency_code": "USD",
  "total_estimated_cents": 15000000,
  "total_committed_cents": 12000000,
  "total_actual_cents": 9500000,
  "project_count": 8
}
```

---

## 7. Pre-Construction Pipeline Endpoints

### 7.1 Prospects (CRM)

### GET /api/v1/org/{orgID}/pipeline/prospects
- **Auth:** JWT (owner, admin)
- **Query:** `?stage=LEAD&page=1&per_page=50`
- **Response:** `200 { data: { prospects: []Prospect }, meta }`

### POST /api/v1/org/{orgID}/pipeline/prospects
- **Auth:** JWT (owner, admin)
- **Body:** `{ name, client_name, client_email?, client_phone?, address?, gsf?, source? }`
- **Response:** `201 { data: { prospect: Prospect } }`

### GET /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
- **Auth:** JWT (owner, admin)
- **Response:** `200 { data: { prospect: Prospect, estimates: []Estimate, permits: []Permit } }`

### PUT /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
- **Auth:** JWT (owner, admin)
- **Body:** `{ name?, client_name?, client_email?, client_phone?, address?, gsf?, notes? }`
- **Response:** `200 { data: { prospect: Prospect } }`

### 7.2 Stage Transitions

### POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/advance
- **Auth:** JWT (owner, admin)
- **Body:** `{ target_stage: "QUALIFIED"|"ESTIMATE_SENT"|"VERBAL_COMMITMENT"|"PERMIT_APPLIED"|"PERMIT_ISSUED", permit_issued_date?: "date" }`
- **Response:** `200 { data: { prospect: Prospect, project_id?: "uuid" } }`
- **Note:** `PERMIT_ISSUED` triggers atomic Kanban→CPM transition. Returns the newly created `project_id`. Requires `permit_issued_date` in body. The transition:
  1. Creates a new Project from prospect data
  2. Sets prospect.project_id to the new project
  3. Hydrates WBS template via physics scoping engine
  4. Enqueues initial CPM calculation via River job
- **Validation:** Source stage must be the immediately preceding stage (LEAD→QUALIFIED→...→PERMIT_ISSUED). Cannot skip stages.

### POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/lose
- **Auth:** JWT (owner, admin)
- **Body:** `{ reason: "string" }`
- **Response:** `200 { data: { prospect: Prospect } }`
- **Note:** Moves prospect to LOST stage. Irreversible.

### Pipeline Stages
| Stage | Probability | Allowed Transitions |
|-------|------------|-------------------|
| LEAD | 10% | → QUALIFIED, → LOST |
| QUALIFIED | 25% | → ESTIMATE_SENT, → LOST |
| ESTIMATE_SENT | 50% | → VERBAL_COMMITMENT, → LOST |
| VERBAL_COMMITMENT | 75% | → PERMIT_APPLIED, → LOST |
| PERMIT_APPLIED | 85% | → PERMIT_ISSUED, → LOST |
| PERMIT_ISSUED | 100% | Terminal (triggers CPM) |
| LOST | — | Terminal |

### 7.3 Estimates

### POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/estimates
- **Auth:** JWT (owner, admin)
- **Body:**
```json
{
  "line_items": [
    { "wbs_code": "6.0", "description": "Foundation", "estimated_cents": 2500000, "unit": "sqft", "quantity": 1800 }
  ],
  "margin_pct": 15,
  "currency_code": "USD"
}
```
- **Response:** `201 { data: { estimate: Estimate } }`

### PUT /api/v1/org/{orgID}/pipeline/estimates/{estimateID}
- **Auth:** JWT (owner, admin)
- **Body:** `{ line_items?, margin_pct?, status? }`
- **Response:** `200 { data: { estimate: Estimate } }`

### Estimate Object
```json
{
  "id": "uuid",
  "prospect_id": "uuid",
  "version": 1,
  "total_estimated_cents": 45000000,
  "currency_code": "USD",
  "line_items": [ ... ],
  "margin_pct": 15,
  "status": "draft|sent|revised|accepted",
  "sent_at": "timestamp"
}
```

### 7.4 Permits

### POST /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/permits
- **Auth:** JWT (owner, admin)
- **Body:** `{ permit_type, jurisdiction, application_number?, submitted_date?, fee_cents?, fee_currency_code? }`
- **Response:** `201 { data: { permit: Permit } }`

### PUT /api/v1/org/{orgID}/pipeline/permits/{permitID}
- **Auth:** JWT (owner, admin)
- **Body:** `{ status?, actual_issue_date?, expected_issue_date?, notes? }`
- **Response:** `200 { data: { permit: Permit } }`

### Permit Object
```json
{
  "id": "uuid",
  "prospect_id": "uuid",
  "permit_type": "building|electrical|plumbing|mechanical",
  "jurisdiction": "City of Austin",
  "application_number": "BP-2026-1234",
  "submitted_date": "date",
  "expected_issue_date": "date",
  "actual_issue_date": "date",
  "fee_cents": 250000,
  "fee_currency_code": "USD",
  "status": "not_submitted|submitted|under_review|revisions_requested|approved|denied"
}
```

### 7.5 Pipeline Analytics

### GET /api/v1/org/{orgID}/pipeline/analytics
- **Auth:** JWT (owner, admin)
- **Response:**
```json
{
  "data": {
    "by_currency": [
      {
        "currency_code": "USD",
        "total_weighted_revenue_cents": 125000000,
        "stages": [
          { "stage": "LEAD", "count": 5, "weighted_revenue_cents": 15000000 },
          { "stage": "QUALIFIED", "count": 3, "weighted_revenue_cents": 22500000 }
        ]
      },
      {
        "currency_code": "CAD",
        "total_weighted_revenue_cents": 8500000,
        "stages": [ ... ]
      }
    ]
  }
}
```
- **Note:** Revenue forecasting grouped by currency_code. No cross-currency sums.

---

## 8. Procurement Endpoints

### GET /api/v1/projects/{projectID}/procurement
- **Auth:** JWT (owner, admin, superintendent)
- **Query:** `?status=WARNING,CRITICAL`
- **Response:** `200 { data: { items: []ProcurementItem } }`

### POST /api/v1/projects/{projectID}/procurement
- **Auth:** JWT (owner, admin)
- **Body:** `{ name, wbs_code, estimated_cost_cents, estimated_cost_currency_code, lead_time_days, need_by_date }`
- **Response:** `201 { data: { item: ProcurementItem } }`

### PUT /api/v1/projects/{projectID}/procurement/{itemID}
- **Auth:** JWT (owner, admin)
- **Body:** `{ status?, po_number?, ordered_at? }`
- **Response:** `200 { data: { item: ProcurementItem } }`

### POST /api/v1/projects/{projectID}/procurement/{itemID}/request-review
- **Auth:** JWT (superintendent+)
- **Purpose:** Surface a vendor's material quote for human review by creating a local `vendor_review_requested` feed card. Fully self-contained — no outbound webhook.
- **Body:** `{ vendor, total_cents, currency_code, rfq_id?, reasoning? }`
  - `vendor`: required, non-empty.
  - `total_cents`: required, non-negative integer (Composite Currency Pattern).
  - `currency_code`: required, `USD` or `CAD`.
  - `rfq_id`: optional UUID; omit for AI-driven flows with no formal RFQ.
  - `reasoning`: optional AI narrative.
- **Response:** `201 { data: { feed_card_id: "uuid" } }` — the id of the feed card the operator will action.
- **Errors:** `400 VALIDATION_ERROR`, `404 NOT_FOUND` (item missing or in another org), `503 SERVICE_UNAVAILABLE` (flow not available on the worker binary).

### ProcurementItem Object
```json
{
  "id": "uuid",
  "project_id": "uuid",
  "name": "Roof Trusses",
  "wbs_code": "9.1",
  "estimated_cost_cents": 850000,
  "estimated_cost_currency_code": "USD",
  "lead_time_days": 14,
  "weather_buffer_days": 3,
  "need_by_date": "date",
  "must_order_date": "date",
  "status": "OK|WARNING|CRITICAL|ORDERED",
  "po_number": "PO-2026-0042"
}
```

---

## 9. Feed Endpoints

### GET /api/v1/feed
- **Auth:** JWT (org-scoped)
- **Query:** `?status=active&priority=critical,urgent&page=1&per_page=50`
- **Response:** `200 { data: { cards: []FeedCard }, meta }`

### POST /api/v1/feed/{cardID}/action
- **Auth:** JWT (org-scoped)
- **Body:** `{ action_type: "string", payload: {} }`
- **Response:** `200 { data: { card: FeedCard, result: ActionResult } }`

### POST /api/v1/feed/{cardID}/dismiss
- **Auth:** JWT (org-scoped)
- **Response:** `200 { data: { card: FeedCard } }`

### FeedCard Object
```json
{
  "id": "uuid",
  "org_id": "uuid",
  "project_id": "uuid",
  "card_type": "weather_alert|procurement|vendor_review_requested|sub_confirmation|progress|delay|permit_update",
  "title": "string",
  "body": "string",
  "priority": "critical|urgent|normal|low",
  "actions": [{ "label": "Approve", "action_type": "approve_quote", "payload": {} }],
  "status": "active|dismissed|actioned|expired",
  "created_at": "timestamp"
}
```

---

## 10. Field Sync Endpoints (Flutter Mobile)

### GET /api/v1/field/sync
- **Auth:** JWT (field_worker+)
- **Query:** `?since={ISO8601_timestamp}`
- **Response:**
```json
{
  "data": {
    "notifications": [{ "type": "task_assigned", "payload": {} }],
    "tasks": [{ "id": "uuid", "wbs_code": "9.2", "percent_complete": 60 }],
    "server_time": "2026-04-02T14:30:00Z"
  }
}
```
- **Note:** Pull-based sync. Client stores `server_time` and passes as `since` on next sync.

### POST /api/v1/field/progress
- **Auth:** JWT (field_worker+)
- **Body:** `{ task_id, percent_complete, photo_asset_id?, gps_lat?, gps_lng?, idempotency_key }`
- **Response:** `201 Created` | `409 Conflict` (duplicate idempotency_key)
- **Note:** Idempotency key (UUID v7) prevents duplicate processing from offline outbox drain.

### POST /api/v1/field/checkin
- **Auth:** JWT (field_worker+)
- **Body:** `{ project_id, crew_members: [{ worker_id, gps_lat, gps_lng }], idempotency_key }`
- **Response:** `201 Created` | `409 Conflict`

### POST /api/v1/field/daily-log
- **Auth:** JWT (field_worker+)
- **Body:** `{ project_id, weather_conditions, work_summary, safety_incidents?, photos?: [], idempotency_key }`
- **Response:** `201 Created` | `409 Conflict`

---

## 11. Fleet & HR Endpoints

### GET /api/v1/org/{orgID}/fleet
- **Auth:** JWT (owner, admin, superintendent)
- **Response:** `200 { data: { assets: []FleetAsset } }`

### POST /api/v1/org/{orgID}/fleet
- **Auth:** JWT (owner, admin)
- **Body:** `{ name, asset_type, serial_number? }`
- **Response:** `201 { data: { asset: FleetAsset } }`

### POST /api/v1/org/{orgID}/fleet/{assetID}/allocate
- **Auth:** JWT (owner, admin, superintendent)
- **Body:** `{ project_id, start_date, end_date }`
- **Response:** `201 { data: { allocation: EquipmentAllocation } }`
- **Error:** `409` if allocation conflicts with existing booking (GiST exclusion constraint)

### GET /api/v1/org/{orgID}/employees
- **Auth:** JWT (owner, admin)
- **Response:** `200 { data: { employees: []Employee } }`

### POST /api/v1/org/{orgID}/employees
- **Auth:** JWT (owner, admin — group `RequireRole(owner, admin)`)
- **Purpose:** Create an employee record (the trade/worker directory; distinct from a login `users` row).
- **Body:** `{ first_name, last_name, role, phone?, hire_date?, user_id? }`
  - Required: `first_name`, `last_name`, `role` (free-text trade role — NOT the RBAC enum).
  - `hire_date` accepts `YYYY-MM-DD` or RFC3339. `user_id` optional; when supplied it is **org-verified** before insert (a cross-org `user_id` → `400`). `org_id` is taken from the caller's claim, never the body.
- **Response:** `201 { data: { employee: Employee } }`
- **Errors:** `400 VALIDATION_ERROR`, `403 FORBIDDEN` (URL `{orgID}` vs claim mismatch).

### GET /api/v1/org/{orgID}/employees/{employeeID}/certifications
- **Auth:** JWT (owner, admin)
- **Response:** `200 { data: { certifications: []Certification } }`

### POST /api/v1/org/{orgID}/employees/{employeeID}/certifications
- **Auth:** JWT (owner, admin — group gate)
- **Purpose:** Add a certification for an employee. Tenant isolation is **indirect** (certifications has no `org_id`): the employee is org-verified before insert; a cross-org `{employeeID}` → `404 NOT_FOUND` (existence never leaked).
- **Body:** `{ cert_type, expiry_date, cert_number?, issued_date?, status? }`
  - Required: `cert_type`, `expiry_date` (schema NOT NULL; `YYYY-MM-DD` or RFC3339).
  - `status` ∈ {active, expired, revoked}, defaults `"active"`.
- **Response:** `201 { data: { certification: Certification } }`
- **Errors:** `400 VALIDATION_ERROR`, `404 NOT_FOUND` (cross-org / unknown employee).

> **Escalation resolution (OQ-7, cost_code ↔ budget linkage):** `project_budgets.wbs_code` and the org `cost_codes` catalog are kept **decoupled** in Phase 0C. Budgets key on the per-project WBS namespace (aligned with `project_tasks.wbs_code` by convention), with **no FK or validation against `cost_codes`**. Linking them is a new schema relationship deferred to a later phase pending owner confirmation.

---

## 12. AI Agent Endpoints

Native-AI-backed endpoints. BuildOS calls the Anthropic Messages API directly using the org's own key stored in the encrypted vault (BYOK). Gated by **role only** per route (the former `plan_tier=pro` gate was removed — ESC-002 — post-pivot billing is gone). When no key is configured, or the key is rejected by Anthropic, these endpoints **soft-fail** with `503 SERVICE_UNAVAILABLE` (`AI_UNCONFIGURED`-class) rather than erroring at boot — the server runs without keys.

### POST /api/v1/agents/daily-briefing
- **Auth:** JWT (any authenticated role)
- **Purpose:** Generate a morning briefing for the authenticated caller. Synchronous; the mobile app calls this on launch.
- **Response:** `200 { data: { briefing: { ... } } }`
- **Errors:** `503 SERVICE_UNAVAILABLE` (no AI key configured), `429 RATE_LIMITED` (provider throttled), `502 UPSTREAM_ERROR` (provider transient / 5xx).

### POST /api/v1/projects/{projectID}/schedule/recommend-adjustments
- **Auth:** JWT (superintendent+)
- **Query:** `?dry_run=true` — **PREVIEW-FIRST (ESC-AUX-01: AI proposes, human commits).** Returns enriched per-row proposals and **mutates nothing** (no duration write, no recalc, no audit). The "Suggest adjustments" UI uses this, then commits selected rows via the sibling apply endpoint. Omitting the flag (or `dry_run=false`) keeps the legacy one-shot auto-apply path.
- **Purpose:** Ask the AI client to **propose** task duration nudges, joined against the loaded task graph so each row carries its identity + old/new duration.
- **Response:** `200 ScheduleAdjustmentSet`:
  ```json
  {
    "adjustments": [
      { "task_id": "uuid", "wbs_code": "1.0", "name": "Foundation",
        "old_duration_days": 5, "new_duration_days": 9, "rationale": "…",
        "is_critical": true, "proposed_change": true, "applied": false }
    ],
    "dry_run": true,
    "proposed_changes": 1,     // rows with a real duration change to apply
    "advisory_count": 1,       // monitor-only rows (no proposed change)
    "applied_deltas": 0,       // rows written (always 0 on dry-run)
    "critical_recomputed": false,
    "skipped_rationale_only": 1 // == advisory_count (wire-compat alias)
  }
  ```
  - `new_duration_days` is omitted on advisory rows. `applied` is `true` only on the legacy auto-apply path (never on dry-run).
  - On the legacy path, if deltas applied but the CPM re-run was deferred, still returns `200` (the next `/schedule/recalculate` catches up).
- **Errors:** `400 VALIDATION_ERROR` (project has no tasks), `404 NOT_FOUND`, `503 SERVICE_UNAVAILABLE` (no AI key / flow not available on worker binary), `429 RATE_LIMITED`, `502 UPSTREAM_ERROR`.

### POST /api/v1/projects/{projectID}/schedule/adjustments/apply
- **Auth:** JWT (superintendent+ — same CPM-affecting gate as `/recommend-adjustments` and `/recalculate`)
- **Purpose:** **PREVIEW-FIRST commit (ESC-AUX-01).** Apply the user-selected duration proposals from a dry-run preview in one tx, then re-run CPM so floats/critical-path recompute.
- **Body:** `{ adjustments: [{ wbs_code: string, new_duration_days: int }] }` (≥1 row)
- **Validation:** each row's `wbs_code` must exist in the project and `new_duration_days` must be in `[1, 36500]`; duplicate/unknown wbs or out-of-range duration → `400 VALIDATION_ERROR` (all-or-nothing — a bad row fails the whole batch).
- **Response:** `200 { applied_deltas: int, critical_recomputed: bool }`
  - Audits one `schedule.adjustments.applied` row with per-task deltas `{ wbs, old, new }` (no free-text rationale in audit metadata).
  - Recalc-deferred case still returns `200` with the applied deltas.
- **Errors:** `400 VALIDATION_ERROR`, `404 NOT_FOUND` (cross-org), `503 SERVICE_UNAVAILABLE` (schedule trio not wired on the worker binary).

**AI error mapping (both endpoints):**

| Condition | HTTP | Code |
|-----------|------|------|
| No Anthropic key set for the org | 503 | `SERVICE_UNAVAILABLE` |
| Anthropic rejected the stored key (401 upstream) | 503 | `SERVICE_UNAVAILABLE` |
| Provider rate limited | 429 | `RATE_LIMITED` |
| Provider transient / 5xx (non-circuit) | 502 | `UPSTREAM_ERROR` |
| Provider circuit-open | 503 | `AI_CIRCUIT_OPEN` (Retry-After set) |

---

## 13. Integrations Vault (BYOK)

Admin-gated encrypted credential store. Per-org 3rd-party API keys (Anthropic, Resend, named vendors such as Gable/LocalBlue) are AES-256-GCM encrypted at rest with the fork's `VAULT_MASTER_KEY`. The API only ever exposes **metadata** — secret bytes are never returned.

### GET /api/v1/integrations
- **Auth:** JWT (admin+)
- **Response:** `200 { data: { integrations: []IntegrationCredential } }`

### PUT /api/v1/integrations/{provider}
- **Auth:** JWT (admin+)
- **Purpose:** Store or rotate the active credential for a provider (e.g. `anthropic`, `resend`). `{provider}` is lowercased server-side.
- **Body:** `{ label, key }` (`key` required, non-empty — the raw secret; encrypted before storage)
- **Response:** `200 { data: { integration: IntegrationCredential } }`

### DELETE /api/v1/integrations/{provider}
- **Auth:** JWT (admin+)
- **Purpose:** Deactivate the active credential for a provider.
- **Response:** `204 No Content`
- **Errors:** `404 NOT_FOUND`

### IntegrationCredential Object
```json
{
  "id": "uuid",
  "provider": "anthropic",
  "label": "Production key",
  "last4": "x9f2",
  "is_active": true,
  "created_by": "user-sub",
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

### GET /api/v1/capabilities
- **Auth:** JWT (any authenticated role — NOT admin-gated, unlike the vault surface above). Every role's UI gates its AI/email affordances on these flags, so all roles must be able to read it.
- **Purpose:** Report which vault-backed features are live for the caller's org, derived from active-credential **presence** (no upstream validation): `ai_configured` = an active `anthropic` credential exists; `email_configured` = an active `resend` credential exists. Never decrypts — `fingerprint` is the plaintext `last4` metadata.
- **Response:** `200 { data: Capabilities }`
- **Mounting:** Only mounted when the vault is wired (`VAULT_MASTER_KEY` configured), alongside the integrations routes. When unmounted, clients fall back to assume-on (capabilities outage must never brick the UI).

#### Capabilities Object
```json
{
  "ai_configured": true,
  "email_configured": false,
  "providers": [
    {
      "provider": "anthropic",
      "configured": true,
      "fingerprint": "x9f2",
      "created_at": "timestamp",
      "created_by": "user-sub"
    },
    { "provider": "resend", "configured": false }
  ]
}
```
`fingerprint`, `created_at`, and `created_by` are omitted for unconfigured providers.

---

## 13b. Agent Config Registry (Phase 3a)

Admin-gated, per-org enable/tune of the agentic harness capabilities
(`delay_cascade`, `foresight`, `experience`) — post-deploy, no redeploy. Mounted
under `/api/v1/admin/agents` (a new `/api/v1/admin/*` operator namespace),
deliberately **off** the pro-tier `/api/v1/agents` tree so the kill-switch is
reachable regardless of plan tier. Behind auth + the **SetupGate** (config is
operational, not bootstrap → 403 `SETUP_INCOMPLETE` before onboarding completes)
and **`admin+`** RBAC. Config values are **tuning only — never secrets** (those
live in the vault, `/integrations/*`).

The in-code catalog is the existence authority: a capability not built into the
binary returns `404`. Absence of an override row means "enabled with the catalog
default", so a row only ever encodes an override; `DELETE` resets to default.

### GET /api/v1/admin/agents
- **Auth:** JWT (admin+)
- **Purpose:** List every catalog capability with its effective config for the caller's org (override row, else catalog default).
- **Response:** `200 { data: { agents: []EffectiveAgentConfig } }`

### PUT /api/v1/admin/agents/{capability}
- **Auth:** JWT (admin+)
- **Purpose:** Upsert the override for a capability. Full-document semantics: `enabled` is authoritative; an omitted/null `config` resets the capability's tuning to the catalog default. (Not PATCH — no partial merge.)
- **Body:** `{ enabled: bool, config?: object }` (`config` is a JSON object; for `foresight`, `schedule_float_days` / `budget_burn_percent` must be non-negative integers)
- **Response:** `200 { data: { agent: AgentConfig } }`
- **Errors:** `404 NOT_FOUND` (capability unknown to the catalog), `400 VALIDATION_ERROR` (config not an object / invalid foresight ints)

### DELETE /api/v1/admin/agents/{capability}
- **Auth:** JWT (admin+)
- **Purpose:** Remove the override row (reset the capability to the catalog default).
- **Response:** `204 No Content` — **idempotent** whether or not an override existed.
- **Errors:** `404 NOT_FOUND` (capability unknown to the catalog)

### EffectiveAgentConfig Object
```json
{
  "capability": "foresight",
  "description": "Periodically surface material standing cross-module risks…",
  "enabled": true,
  "config": { "schedule_float_days": 2, "budget_burn_percent": 80 },
  "source": "default",
  "updated_by": "",
  "updated_at": null
}
```
`source` is `"default"` (no override row) or `"override"` (an explicit per-org row); `updated_by` / `updated_at` are populated only for overrides.

### AgentConfig Object (PUT response)
```json
{
  "id": "uuid",
  "org_id": "uuid",
  "capability": "foresight",
  "enabled": false,
  "config": { "budget_burn_percent": 50 },
  "updated_by": "user-sub",
  "created_at": "timestamp",
  "updated_at": "timestamp"
}
```

**Capability gate behavior:** when a capability is disabled, `delay_cascade` and `foresight` (River worker flows) become clean no-ops; the synchronous `experience` endpoint (`POST /api/v1/agents/chat`) returns `403 CAPABILITY_DISABLED` (distinct from the `503` a missing AI key produces and from RBAC `403 FORBIDDEN`).

---

## 13c. Connector Registry (Phase 3b)

Admin-gated, per-org enable/config of the integration **connectors** — named
providers of agentic tools the assistant can call. 3b-i ships an in-process,
read-only built-in (`reference`); 3b-ii adds vault-backed MCP connectors. Mounted
under `/api/v1/admin/connectors` (the `/api/v1/admin/*` operator namespace), off
the pro-tier `/api/v1/agents` tree, behind auth + the **SetupGate** + **`admin+`**
RBAC. Connectors are **DEFAULT-OFF** per org (explicit admin opt-in); their tools
mount only in the chat assistant (`POST /api/v1/agents/chat`), are **namespaced**
(`conn__<connector>__<tool>`) and **MinRole-floored at admin**, and **fail closed**
(any error mounts zero connector tools, never breaking chat). The built-in catalog
is the existence authority (unknown connector ⇒ 404). Config is non-secret —
**credentials go in the vault** (`/integrations/*`), never here.

### GET /api/v1/admin/connectors
- **Auth:** JWT (admin+)
- **Response:** `200 { data: { connectors: []EffectiveConnector } }` (catalog merged with per-org config; `enabled` defaults `false`).

### PUT /api/v1/admin/connectors/{connector}
- **Auth:** JWT (admin+)
- **Body:** `{ enabled: bool, kind?: "mcp", config?: object }`.
  - A **built-in** name (e.g. `reference`) toggles enable/config; `config` must be a JSON object.
  - **Any other name is an MCP server INSTANCE** (Phase 3b-ii): `kind` must be `mcp` (or omitted), the name must match `^[a-z0-9][a-z0-9_-]{1,40}$`, and `config` must carry `{"endpoint":"https://…"}` (https only; a host that is a literal private/metadata IP is rejected — the dial-time SSRF guard is authoritative). The instance is created or updated. Set its credential separately via `PUT /api/v1/integrations/connector:<name>`.
- **Response:** `200 { data: { connector: ConnectorConfig } }`
- **Errors:** `400 VALIDATION_ERROR` (bad config / endpoint / instance name, or `kind=mcp` on a built-in).

### POST /api/v1/admin/connectors/{connector}/refresh
- **Auth:** JWT (admin+) · **MCP instances only.**
- **Purpose:** Connect to the MCP server (`initialize` + `tools/list`), bound the (untrusted) tool metadata, and replace the cached tool set the assistant mounts.
- **Response:** `200 { data: { connector, tools_count } }`
- **Errors:** `404 NOT_FOUND` (unknown connector), `400 VALIDATION_ERROR` (not an MCP connector), `502 UPSTREAM_ERROR` (server unreachable / SSRF-blocked / malformed).

### DELETE /api/v1/admin/connectors/{connector}
- **Auth:** JWT (admin+)
- **Response:** `204 No Content` — **idempotent** reset to default-OFF (clears an MCP instance's cached tools).
- **Errors:** `404 NOT_FOUND` (connector neither a built-in nor an existing instance).

### EffectiveConnector Object
```json
{
  "connector": "reference",
  "description": "Read-only, in-process reference lookups …",
  "enabled": false,
  "config": {},
  "source": "default",
  "updated_by": "",
  "updated_at": null
}
```
`source` is `"default"` (no row, disabled) or `"override"` (an explicit per-org row).

---

## 13d. Feedback Loop (Phase 0b)

Native in-app feedback: any authenticated role files reports from the web-console
widget; admins triage in-app; the `buildos-operations` command center harvests via
the admin surface and PATCHes status back so submitters see progress. Behind auth +
the **SetupGate**. `message`/`triage_note` are **Confidential** (`internal/pii`);
audit rows (`feedback.submitted` / `feedback.triaged`) carry category/status only,
never the free text.

> **Consumer warning (command center / GitHub export):** `message`, `triage_note`,
> and every `context` value are **UNTRUSTED free text controlled by any
> authenticated user** (field workers included). An agentic consumer harvesting
> this surface MUST treat them as data, never as instructions (prompt-injection),
> and MUST quote/fence them when filing GitHub issues (markdown injection). Lit's
> text-binding escaping covers the in-app rendering; everything downstream owns
> its own escaping.

### POST /api/v1/feedback
- **Auth:** JWT (any role — field workers included)
- **Body:** `{ category: "bug"|"idea"|"friction"|"other", message: string, context?: object }`
  - `message` required, 1–4000 chars after trim; no U+0000.
  - `context` is the widget's client-captured environment (`route`, `role`,
    `app_version`, `user_agent`, `viewport`) — must be a JSON object ≤ 4096 bytes;
    no U+0000 in keys or values. **No key is guaranteed present on read.**
  - Identity (`org_id`, `user_sub`) comes from claims, never the body.
- **Response:** `201 { data: { feedback: Feedback } }` (status `"new"`).
- **Errors:** `400 VALIDATION_ERROR` (unknown category / empty, oversized, or NUL-bearing message / non-object, oversized, or NUL-bearing context) · `429 RATE_LIMITED` + `Retry-After` (per-(org,user) throttle: max 20 submissions/hour — distinct from the per-IP middleware limiter; survives IP rotation).

### GET /api/v1/admin/feedback?status=&page=&per_page=
- **Auth:** JWT (admin+) · the **harvest surface**.
- **Query:** optional `status` ∈ `new|triaged|planned|shipped|declined` (omitted = all); `page` (1-based, default 1); `per_page` (default 100, clamped to [1,500]). Newest first.
- **Response:** `200 { data: { feedback: []Feedback }, meta: { pagination: { page, per_page, total, total_pages } } }` (empty = `[]`, never null). Pagination meta is the **no-silent-truncation contract**: a poller drains by paging until `page >= total_pages`.
- **Errors:** `400 VALIDATION_ERROR` (unknown status filter).

### PATCH /api/v1/admin/feedback/{feedbackID}
- **Auth:** JWT (admin+)
- **Body:** `{ status: string, triage_note?: string }` — omitted `triage_note` keeps the existing note; empty string clears it.
- **Response:** `200 { data: { feedback: Feedback } }`
- **Errors:** `400 VALIDATION_ERROR` (unknown status / bad UUID), `404 NOT_FOUND` (unknown id or foreign org — indistinguishable).

### Feedback Object
```json
{
  "id": "uuid",
  "org_id": "uuid",
  "user_sub": "jwt-subject",
  "category": "bug",
  "message": "Gantt bars misalign on resize",
  "context": { "route": "/projects/x/schedule", "role": "superintendent", "app_version": "0.1.0" },
  "status": "new",
  "triage_note": "",
  "created_at": "2026-06-10T00:00:00Z",
  "updated_at": "2026-06-10T00:00:00Z"
}
```

---

## 13e. Assets (Object-Storage Substrate — Chunk A)

The per-fork S3-compatible (Cloudflare R2) blob store for jobsite photos. Bytes
go **direct to/from R2 via presigned URLs** — they never transit the Go server on
the happy path. Every path is **org+project-scoped**; a cross-org id is a uniform
`404 NOT_FOUND`. The raw object key is **never** serialized to clients (they get
short-lived signed URLs). Each mutation writes an `asset.*` audit row.

Storage is **opt-in per fork**: endpoint + bucket via env
(`OBJECT_STORE_ENDPOINT` / `OBJECT_STORE_BUCKET`, falling back to `R2_ENDPOINT` /
`R2_BUCKET`); access key + secret sealed in the vault under provider
`object_store`. **Unconfigured ⇒ every upload/serve path soft-fails `503
STORAGE_UNAVAILABLE`** (server still boots), the same posture as AI/email. Limits
(spec §9): content-type allowlist `image/jpeg|png|webp|heic`, size ≤ 15 MiB,
≤ 20 photos/daily-log (enforced at daily-log persist in Chunk B).

### POST /api/v1/projects/{projectID}/assets/presign-put
- **Auth:** JWT, **minRole superintendent** (the field_worker-facing variant lands in Chunk B).
- **Body:** `{ content_type: string, byte_size: number, filename?: string }`
  - `content_type` ∈ `image/jpeg|image/png|image/webp|image/heic`; `byte_size` in `(0, 15 MiB]`.
  - The content-type + content-length are **signed into the presigned PUT**, so R2 rejects a mismatched body.
- **Response:** `201 { data: { asset_id, upload_url, signed_headers, expires_at } }`. Creates a `pending` asset row; the client PUTs bytes to `upload_url` echoing `signed_headers`, then calls confirm.
- **Errors:** `400 VALIDATION_ERROR` (bad type/size), `404 NOT_FOUND` (cross-org/missing project), `503 STORAGE_UNAVAILABLE`.

### POST /api/v1/assets/{id}/confirm
- **Auth:** JWT, **minRole superintendent**.
- **Body:** `{ checksum_sha256?: string }` (optional).
- **Response:** `200 { data: Asset }` (`status: "ready"`). Only a `pending` row transitions (replay-safe). Daily-log linking (Chunk B) requires `ready`.
- **Errors:** `404 NOT_FOUND` (cross-org/missing/already-confirmed).

### GET /api/v1/assets/{id}
- **Auth:** JWT, **minRole superintendent**.
- **Response:** `302` redirect to a short-lived (15 min) presigned GET URL for a `ready`, org-owned asset. (The same-origin EXIF-stripping proxy is the path the public page uses in Chunk E.)
- **Errors:** `404 NOT_FOUND` (cross-org/missing/not-ready), `503 STORAGE_UNAVAILABLE`.

### GET /api/v1/projects/{projectID}/assets
- **Auth:** JWT, **minRole superintendent**.
- **Response:** `200 { data: []Asset }` (project gallery; `ready` only; newest-first; empty = `[]`).
- **Errors:** `404 NOT_FOUND` (cross-org/missing project).

### Asset Object
```json
{
  "id": "uuid",
  "org_id": "uuid",
  "project_id": "uuid|null",
  "content_type": "image/jpeg",
  "size_bytes": 204800,
  "status": "pending|ready|failed",
  "uploaded_by": "jwt-subject",
  "checksum_sha256": "hex|null",
  "created_at": "2026-06-11T00:00:00Z",
  "confirmed_at": "2026-06-11T00:01:00Z|null"
}
```
> `storage_key` (the opaque bucket object key) is **never** present in the wire shape (`json:"-"`).

---

## 13f. Photo Upload + Daily-Log Linking (Chunk B)

Chunk B opens photo upload to the **field worker** and links confirmed photos to a
specific **daily log** for a `(project, date)`. The link model **reuses the
existing `daily_logs.photo_asset_ids UUID[]`** column (no new table). A photo may
be linked only if it is a **confirmed (`ready`), org-owned** asset whose project
(if pinned) matches — enforced by a deterministic validation gate that closes the
dangling-id gap. `storage_configured` is added to `GET /api/v1/capabilities` so
the console can gate the upload affordance.

### POST /api/v1/field/assets/presign
- **Auth:** JWT, **any authenticated role (incl. `field_worker`)** — the one asset path open to field workers (the operator presign is superintendent+). Caller-scoped: project comes in the **body**, verified in the caller's org.
- **Body:** `{ project_id: uuid, content_type: string, byte_size: number }` (same content-type/size limits as the operator presign).
- **Response:** `201 { data: { asset_id, upload_url, signed_headers, expires_at } }`.
- **Errors:** `400 VALIDATION_ERROR`, `404 NOT_FOUND` (project not in caller's org — uniform), `503 STORAGE_UNAVAILABLE`.

### POST /api/v1/field/assets/{id}/confirm
- **Auth:** JWT, **any authenticated role**.
- **Body:** `{ checksum_sha256?: string }`.
- **Response:** `200 { data: Asset }` (`status: "ready"`).
- **Errors:** `404 NOT_FOUND` (cross-org/missing/already-confirmed).

### POST /api/v1/projects/{projectID}/daily-reports/{date}/photos
- **Auth:** JWT, **minRole superintendent** (the operator "Add photos" affordance on the daily-report view). `date` = `YYYY-MM-DD`.
- **Body:** `{ asset_ids: uuid[] }` — already-confirmed assets to associate with that day's daily log. De-duped, capped at 20/log; idempotent (re-linking is a no-op union).
- **Response:** `200 { data: DailyLog }` (the updated row, `photo_asset_ids` now including the linked ids).
- **Errors:** `400 INVALID_PHOTO_ASSET` (an id is not a confirmed, org-owned photo for this project), `400 VALIDATION_ERROR` (empty/oversize set), `404 NOT_FOUND` (cross-org project **or** no daily log exists for that day — record a daily log first).

### POST /api/v1/field/daily-log (Chunk B addition)
- The existing field daily-log write now **validates `photo_asset_ids`**: every id must be a confirmed, org-owned asset for the project, else `400 INVALID_PHOTO_ASSET`. ≤ 20 photos/log. When object storage is unconfigured the validation is skipped (text-only logs still work).

### Flutter field-app flow (documented fast-follow — backend ready, not yet implemented)
The backend presign+confirm+link endpoints are **field-worker usable today** so the
Flutter app can adopt them later. Planned mobile flow (`photos_screen.dart` /
`sync_service.dart` / `outbox_action.dart`): on capture, enqueue an outbox chain
`POST /field/assets/presign` → `PUT` bytes direct to R2 with `signed_headers` →
`POST /field/assets/{id}/confirm`; once confirmed, attach the real `asset_id`s to
the daily-log outbox action. Offline-first: the chain drains on reconnect; the
daily log references confirmed ids only.

---

## 13g. Daily Reports (Chunk C)

A **derived** read model (no `daily_reports` table) aggregating `daily_logs` +
`crew_checkins` + `task_progress` per `(project, date)`, plus two native-AI
compositions. Reads are **minRole superintendent**; the client-update DRAFT is
**owner/admin** (external-comms trust). The two AI methods soft-fail to `503
SERVICE_UNAVAILABLE` when no Anthropic key is configured; the text reads always
work. `safety_incidents` IS returned here (internal operator surface) and is
stripped on the client path.

### GET /api/v1/projects/{projectID}/daily-reports?since=&until=
- **Auth:** JWT, minRole superintendent. Bounds are inclusive `YYYY-MM-DD`; omit both for the last-14-days default.
- **Response:** `200 { data: DailyReportSummary[] }` newest-first.

### GET /api/v1/projects/{projectID}/daily-reports/{date}
- **Auth:** JWT, minRole superintendent. `date` = `YYYY-MM-DD`.
- **Response:** `200 { data: DailyReport }` (incl. signed photo thumbnails when storage is on; `404 PROJECT_NOT_FOUND` uniform on cross-org).

### POST /api/v1/projects/{projectID}/daily-reports/{date}/digest
- **Auth:** JWT, minRole superintendent. Generates the INTERNAL office digest.
- **Response:** `200 { data: { digest: string } }`. `503 SERVICE_UNAVAILABLE` when AI is off.

### POST /api/v1/projects/{projectID}/daily-reports/{date}/client-update-draft
- **Auth:** JWT, **owner/admin**. Generates the client-SAFE homeowner draft behind a deterministic redaction allowlist (the service is the gate, not the model).
- **Response:** `200 { data: ClientUpdateDraft }` (`{ subject, body, period_start, period_end, photo_count }`). `503 SERVICE_UNAVAILABLE` when AI is off.

---

## 13h. Client Updates (Chunk D)

The **human-in-the-loop composer**: an AI draft (13g) is persisted as a `draft`,
the operator edits the client-safe subject/body and curates which of the
period's confirmed photos the homeowner sees, then explicitly **sends** via the
existing Resend mailer (post-commit). **Never auto-sent.** ALL routes are
**owner/admin** (external-comms trust — §9-1). A failed send (`NO_CLIENT_CONTACT`
/ `MAILER_UNCONFIGURED`) is surfaced — the operator MUST know it did not go out
(this diverges from the auth-reset best-effort posture). `recipient_email` is a
send-time snapshot and is **never serialized** in any response.

### POST /api/v1/projects/{projectID}/client-updates
- **Auth:** JWT, owner/admin. Creates a draft from a date's redacted AI draft.
- **Body:** `{ report_date: "YYYY-MM-DD" }` (`period_start` accepted as an alias).
- **Response:** `201 { data: ClientUpdate }` (`status: "draft"`). `503 SERVICE_UNAVAILABLE` (no AI key), `404 NOT_FOUND` (cross-org project), `400 VALIDATION_ERROR` (bad date).

### GET /api/v1/projects/{projectID}/client-updates
- **Auth:** JWT, owner/admin.
- **Response:** `200 { data: ClientUpdate[] }` newest-first (history). `recipient_email` omitted.

### GET /api/v1/client-updates/{id}
- **Auth:** JWT, owner/admin.
- **Response:** `200 { data: ClientUpdate }`. `404 NOT_FOUND` uniform on cross-org.

### PATCH /api/v1/client-updates/{id}
- **Auth:** JWT, owner/admin. Operator edit; draft only.
- **Body:** `{ subject: string, edited_body: string, photo_asset_ids?: uuid[] }` (curated photos validated `ready`+org+project-matched).
- **Response:** `200 { data: ClientUpdate }`. `409 ALREADY_SENT` (a sent update is immutable), `400 INVALID_PHOTO_ASSET`, `404 NOT_FOUND`.

### POST /api/v1/client-updates/{id}/send
- **Auth:** JWT, owner/admin. The human-pressed send.
- **Response:** `200 { data: ClientUpdate }` (`status: "sent"`). On failure the row is marked `failed` (with `send_error`) and the error is surfaced: `422 NO_CLIENT_CONTACT` (project has no client email), `422 MAILER_UNCONFIGURED` (no Resend key — NOT sent), `502 SEND_FAILED` (provider rejected — NOT sent), `409 ALREADY_SENT`.

### ClientUpdate Object
```json
{
  "id": "uuid", "org_id": "uuid", "project_id": "uuid",
  "period_start": "2026-06-09", "period_end": "2026-06-09",
  "status": "draft|sent|failed",
  "ai_draft": "the original AI draft (preserved)",
  "edited_body": "the operator-edited, client-safe body that is sent",
  "subject": "the email subject",
  "photo_asset_ids": ["uuid"],
  "created_by": "uuid", "sent_by": "uuid|null",
  "sent_at": "RFC3339|null", "send_error": "string|null",
  "created_at": "RFC3339", "updated_at": "RFC3339"
}
```
`recipient_email` is held server-side (snapshot at send) and is intentionally
**absent** from this shape.

---

## 13i. Share Links + Public Progress Page (Chunk E)

The **first surface outside the everything-behind-auth invariant**: a homeowner
can be given an unauthenticated, **token-gated, read-only** progress page for a
**sent** client update. The operator surface (mint / list / revoke) is
owner/admin and lives inside the auth group; the public surface (`/p/*`) is
unauthenticated and mounted as a **sibling of the auth routes** so it inherits
the global stack (RealIP, rate limiter, security headers) but **bypasses Auth +
SetupGate** without weakening either.

**Token model (mirrors the bootstrap token):** 32-byte CSPRNG cleartext,
base64url (43 chars), shown **once** at create (it becomes the `/p/<token>` URL).
Only the sha256 hash is stored. Resolution filters `expires_at > now() AND
revoked_at IS NULL` and returns a **uniform 404** on any failure (missing /
expired / revoked / malformed / mismatch) — enumeration defense. Default TTL **30
days** (operator-overridable via `ttl_days`, capped at 365), revocable any time.

### Operator surface (owner/admin, authenticated)

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/client-updates/{id}/share-links` | Mint a public link for a **sent** update. Body `{ "ttl_days"?: number }`. `201 { url, link }` — `url` is the one-time `/p/<token>` URL. `422 UPDATE_NOT_SENT` if the update is not sent. |
| GET | `/api/v1/client-updates/{id}/share-links` | List the update's links (active/expired/revoked). `200 ShareLink[]`. No cleartext, no hash. |
| DELETE | `/api/v1/share-links/{linkID}` | Revoke a link. `204`. After revoke, `/p/{token}` 404s. |

Audit: `client_update.share_link.created` / `client_update.share_link.revoked`
(reference the link id + update id only — never the token/hash).

#### ShareLink Object (operator view)
```json
{
  "id": "uuid", "client_update_id": "uuid",
  "status": "active|revoked|expired",
  "expires_at": "RFC3339", "revoked_at": "RFC3339|null",
  "last_viewed_at": "RFC3339|null", "view_count": 0,
  "created_at": "RFC3339"
}
```
The cleartext token + its hash are NEVER in any response; the cleartext appears
exactly once in the create response's `url`.

### Public surface (UNAUTHENTICATED, token-gated)

| Method | Path | Auth | Response |
|--------|------|------|----------|
| GET | `/p/{token}` | **none** | `200 text/html` — a minimal, self-contained, server-rendered page (NOT the SPA) of a redaction-safe projection: project display name, period date, the operator-edited client-safe body, and the curated photos. `404` (uniform) on any invalid/expired/revoked token. |
| GET | `/p/{token}/photos/{assetID}` | **none** | `200 image/*` — same-origin proxy of an **EXIF-stripped** photo, **only if** `assetID` is in this update's operator-curated set (else `404`). The R2 host/key never appears in client HTML. |

The page carries **only** the allowlisted fields — never safety incidents, crew
identities, GPS/EXIF, `*_cents`/budget, `recipient_email`, internal notes,
schedule internals, or sibling-project data (built from a dedicated
`PublicUpdate` projection that physically cannot carry ERP). Page security
headers (per-response): `Content-Security-Policy: default-src 'none'; img-src
'self'; style-src 'self' 'unsafe-inline'; base-uri 'self'; form-action 'none';
frame-ancestors 'none'; object-src 'none'; connect-src 'none'` (no script-src →
no JS, no `/api/*` calls), `Cache-Control: private, no-store`, no cookies. The
`/p/*` group is rate-limited by a **dedicated stricter per-IP limiter** (10 rps /
20 burst) on top of the inherited global limiter.

---

## 14. Setup Wizard

Embedded onboarding. Every fork must complete the wizard before the SetupGate opens operational traffic. All steps require admin minimum. The first-owner claim happens earlier at `POST /api/v1/auth/claim`.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/setup/state` | Current wizard progress |
| POST | `/api/v1/setup/company-info` | Legal name / company info |
| POST | `/api/v1/setup/trades` | Add a trade |
| POST | `/api/v1/setup/cost-codes` | Add a cost code |
| POST | `/api/v1/setup/calendars` | Create a working calendar |
| POST | `/api/v1/setup/calendars/{calendarID}/holidays` | Add a holiday |
| POST | `/api/v1/setup/jurisdictions` | Add a permit jurisdiction |
| POST | `/api/v1/setup/complete` | Finalize (requires legal name, ≥1 trade, ≥1 cost code, a default calendar; idempotent) |

Every step writes a `setup.*` audit action.

---

## 15. Error Codes

| HTTP Status | Code | Description |
|-------------|------|-------------|
| 400 | `VALIDATION_ERROR` | Request body validation failed |
| 400 | `INVALID_PHOTO_ASSET` | A `photo_asset_id` is not a confirmed, org-owned photo for the project (uniform — no enumeration signal) |
| 400 | `INVALID_RESET_TOKEN` | Password-reset token invalid or expired |
| 401 | `UNAUTHORIZED` | Missing or invalid JWT |
| 401 | `INVALID_CREDENTIALS` | Email/password login failed |
| 401 | `INVALID_REFRESH_TOKEN` | Refresh token invalid or expired |
| 401 | `INVALID_BOOTSTRAP_TOKEN` | Bootstrap token invalid/expired (uniform — no probe leak) |
| 403 | `FORBIDDEN` | Insufficient role for this operation |
| 403 | `SETUP_INCOMPLETE` | Onboarding wizard not yet complete for this org |
| 404 | `NOT_FOUND` | Resource does not exist or not in user's org |
| 409 | `CONFLICT` | Resource conflict |
| 409 | `FIRST_OWNER_EXISTS` | An owner already exists for this deployment |
| 409 | `INVALID_TRANSITION` | Invalid pipeline stage transition |
| 409 | `ALREADY_SENT` | Edit/re-send of an already-sent client update |
| 422 | `CROSS_CURRENCY_ERROR` | Attempted cross-currency arithmetic |
| 422 | `NO_CLIENT_CONTACT` | Project has no client email; cannot send a client update |
| 422 | `MAILER_UNCONFIGURED` | No Resend API key configured — the client update was NOT sent (operator must know) |
| 429 | `RATE_LIMITED` | Too many requests (or AI provider throttled) |
| 502 | `SEND_FAILED` | Email provider rejected the client-update send — NOT sent |
| 502 | `UPSTREAM_ERROR` | AI provider transient / 5xx |
| 503 | `SERVICE_UNAVAILABLE` | AI not configured/unreachable, or flow unavailable on this binary |
| 500 | `INTERNAL_ERROR` | Server error |

---

## 16. Rate Limits

| Tier | Requests/min | Burst |
|------|-------------|-------|
| free | 60 | 10 |
| pro | 300 | 50 |
| enterprise | 1000 | 200 |

Rate limit headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`

---

## 17. Versioning

- API version is embedded in the URL path (`/api/v1/...`)
- Breaking changes will increment the version (`/api/v2/...`)
- Deprecation notices via `Sunset` header (RFC 8594) with 6-month lead time
