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
| AI agent endpoints (`/agents/*`) | pro plan-tier + role gate per route | | | |

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

### POST /api/v1/projects/{projectID}/schedule/recalculate
- **Auth:** JWT (owner, admin, superintendent)
- **Body:** `{}` (empty — recalculates from current task/dependency state)
- **Response:** `200 { data: { cpm_result: CPMResult, recalculation_ms: int } }`
- **NFR:** <800ms end-to-end, <200ms physics computation (80-task graph)

### GET /api/v1/projects/{projectID}/schedule/gantt
- **Auth:** JWT (org-scoped)
- **Response:** `200 { data: { tasks: []TaskSchedule, critical_path: []uuid, project_end: timestamp } }`

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

### GET /api/v1/org/{orgID}/employees/{employeeID}/certifications
- **Auth:** JWT (owner, admin)
- **Response:** `200 { data: { certifications: []Certification } }`

---

## 12. AI Agent Endpoints

Native-AI-backed endpoints. BuildOS calls the Anthropic Messages API directly using the org's own key stored in the encrypted vault (BYOK). Gated by `plan_tier=pro` and up. When no key is configured, or the key is rejected by Anthropic, these endpoints **soft-fail** with `503 SERVICE_UNAVAILABLE` (`AI_UNCONFIGURED`-class) rather than erroring at boot — the server runs without keys.

### POST /api/v1/agents/daily-briefing
- **Auth:** JWT (pro plan-tier)
- **Purpose:** Generate a morning briefing for the authenticated caller. Synchronous; the mobile app calls this on launch.
- **Response:** `200 { data: { briefing: { ... } } }`
- **Errors:** `503 SERVICE_UNAVAILABLE` (no AI key configured), `429 RATE_LIMITED` (provider throttled), `502 UPSTREAM_ERROR` (provider transient / 5xx).

### POST /api/v1/projects/{projectID}/schedule/recommend-adjustments
- **Auth:** JWT (superintendent+, pro plan-tier)
- **Purpose:** Ask the AI client to suggest task duration nudges, apply them, and re-run CPM physics so floats/critical-path stay coherent.
- **Response:** `200 { adjustments, applied_deltas, skipped_rationale_only }`
  - If deltas applied but the CPM re-run was deferred, still returns `200` with the applied deltas (the next `/schedule/recalculate` catches up).
- **Errors:** `400 VALIDATION_ERROR` (project has no tasks), `404 NOT_FOUND`, `503 SERVICE_UNAVAILABLE` (no AI key / flow not available on worker binary), `429 RATE_LIMITED`, `502 UPSTREAM_ERROR`.

**AI error mapping (both endpoints):**

| Condition | HTTP | Code |
|-----------|------|------|
| No Anthropic key set for the org | 503 | `SERVICE_UNAVAILABLE` |
| Anthropic rejected the stored key (401 upstream) | 503 | `SERVICE_UNAVAILABLE` |
| Provider rate limited | 429 | `RATE_LIMITED` |
| Provider transient / circuit-open / 5xx | 502 | `UPSTREAM_ERROR` |

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
| 422 | `CROSS_CURRENCY_ERROR` | Attempted cross-currency arithmetic |
| 429 | `RATE_LIMITED` | Too many requests (or AI provider throttled) |
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
