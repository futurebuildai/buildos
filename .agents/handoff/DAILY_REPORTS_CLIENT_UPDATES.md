# Daily Reports → Client Updates — File-Level Implementation Spec

**Domain:** Close the field → office → homeowner loop.
**Pipeline stage:** Owner-chosen first operational workflow (post agentic-UX batch).
**Status:** SPEC — to be built on a fresh branch **after** the agentic-UX batch merges.
**North star:** [VISION.md](../../VISION.md) — BuildOS as an agentic OS whose harness does the GC's coordination busywork. This domain is the first concrete "harness composes a stakeholder communication" surface.
**Author note:** This spec references *behavior and contracts*, not current line numbers (they shift on the post-merge branch). File paths and symbol names are stable; verify exact signatures against the branch before coding.

---

## 0. The loop, in one breath

The field app **already captures** daily logs (`daily_logs`), crew check-ins (`crew_checkins`), and per-task progress (`task_progress`). Today that data is **write-only** — there is no operator read path. This domain adds:

1. **Operator read** of field captures, aggregated per `(project, date)` (the field→office half).
2. **AI composition** of (a) an internal **office digest** and (b) a redacted, client-safe **homeowner progress update** from the schedule snapshot + recent daily logs.
3. **A human-in-the-loop composer** — AI drafts → operator edits → operator previews → operator **sends**. Never auto-send to a client.
4. **Send** via the existing Resend mailer to the homeowner's email (v1 = **email-only**; shareable public link deferred to v2).

Three independent grounding passes converged on the same gap map: the **field→office half is data-present / read-absent**, and the **office→homeowner half is complete greenfield** (no client contact on projects, no client-update entity, no client-update AI task, no send tracking, no photo storage).

---

## 1. DOMAIN MODEL

### 1.1 Source tables (REUSE — already exist, do not modify the write path)

| Table | Migration | Day key | Notes |
|---|---|---|---|
| `daily_logs` | `006_field_sync_tables` | `log_date DATE` | Primary daily report. `work_summary TEXT NOT NULL`; `weather_conditions`, `safety_incidents` free text; `photo_asset_ids UUID[]`. Has `org_id`. Index `idx_daily_logs_project_date(project_id, log_date DESC)` already supports the operator query. |
| `crew_checkins` | `006_field_sync_tables` | **none** — bucket on `reported_at::date` | `crew_members JSONB` is **opaque** (`json.RawMessage`); v1 reads it only for a *count*, tolerating arbitrary shape. Has `org_id`. |
| `task_progress` | `001_initial_schema` | **none** — bucket on `reported_at::date` | `percent_complete`, `notes`, single `photo_asset_id`. **No `org_id`** — org isolation flows through `project_tasks → projects`. |

Aggregation correlates by `(project_id, calendar_date)`: `daily_logs.log_date == crew_checkins.reported_at::date == task_progress.reported_at::date`. `task_progress` joins to a project via `project_tasks.project_id`.

> **v1 source scope:** `daily_logs` is the spine of a daily report. `crew_checkins` contributes a **crew count** only (opaque JSONB, no enforced shape). `task_progress` contributes **per-task %-complete deltas** for the day, joined through `project_tasks`. See OQ-8.

### 1.2 New: client/homeowner contact on `projects` (load-bearing gap)

The active `projects` table has **no client contact**. Contact lives only on `pre_construction_prospects` (`client_name`/`client_email`/`client_phone`) and is **dropped on the floor** when `PipelineService.transitionToPermitIssued` calls `CreateProjectFromProspect` (which passes only OrgID/Name/Address/GSF/PermitIssuedDate). Many forks onboard projects directly, never through the CRM, so the contact may simply not exist.

**Decision (v1): add columns to `projects`** rather than a `project_contacts` table. The owner intent is a single homeowner per project; columns are the lowest-friction reuse and avoid a join on every send. A `project_contacts` table is the v2 path if multi-stakeholder (architect, lender) recipients are needed — flagged in OQ-2.

```sql
-- migration NNN_project_client_contact.up.sql
ALTER TABLE projects ADD COLUMN client_name  TEXT;
ALTER TABLE projects ADD COLUMN client_email TEXT;
ALTER TABLE projects ADD COLUMN client_phone TEXT;
-- Backfill from the originating prospect where the link exists.
UPDATE projects p
   SET client_name  = pcp.client_name,
       client_email = pcp.client_email,
       client_phone = pcp.client_phone
  FROM pre_construction_prospects pcp
 WHERE pcp.project_id = p.id
   AND p.client_email IS NULL;
```

- All three columns **nullable** (direct-onboarded projects have no prospect). No `_cents` columns → composite-currency linter rule N/A. No forbidden numeric types.
- `.down.sql` drops the three columns — **requires** the `-- buildos:destructive: revert client-contact columns on projects` header (lint rule 4).
- **PII:** `client_email`, `client_name`, `client_phone` are **Restricted**. `email`/`phone`/`display_name`/`name` already map Restricted in `pii.FieldClass`; add explicit `client_email`/`client_name`/`client_phone` entries so JSONB audit/log scrubbing catches them by exact key. Never log `client_email`; log `org_id`/`project_id` only.
- Also update `CreateProjectFromProspect` (`internal/store` + `internal/service/pipeline.go`) so future conversions carry the three fields forward — close the leak at the source, not just by backfill.
- `models.Project` gains `ClientName *string`, `ClientEmail *string`, `ClientPhone *string` (all `json:"...,omitempty"`). **`client_email` must NOT be serialized to field_worker-facing responses** — gate at the handler/role layer (it is owner/admin/superintendent operational data).

### 1.3 New: `client_updates` (the sent-update record + draft lifecycle)

A first-class entity records each homeowner-facing update: the AI draft, the operator-edited body, status, recipient, and send time. This is the audit trail of *what we told the client and when* — the canonical one-tx-per-mutation + audit pattern.

```sql
-- migration NNN_client_updates.up.sql
CREATE TABLE client_updates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    -- Reporting window the update covers (inclusive).
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    -- Draft lifecycle: 'draft' (AI-drafted / operator-editable) -> 'sent'.
    -- 'failed' records a send that errored after the operator pressed Send.
    status          TEXT NOT NULL DEFAULT 'draft'
                       CHECK (status IN ('draft','sent','failed')),
    -- AI-drafted client-safe body (markdown). Preserved verbatim for provenance.
    ai_draft        TEXT,
    -- Operator-edited body actually sent (markdown). Falls back to ai_draft if unedited.
    edited_body     TEXT NOT NULL DEFAULT '',
    subject         TEXT NOT NULL DEFAULT '',
    -- Recipient captured at send time (snapshot of projects.client_email).
    -- PII-Restricted; never logged.
    recipient_email TEXT,
    -- Who drafted / who sent.
    created_by      UUID NOT NULL REFERENCES users(id),
    sent_by         UUID REFERENCES users(id),
    sent_at         TIMESTAMPTZ,
    -- Mailer outcome detail on failure (no PII; provider error class only).
    send_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX CONCURRENTLY idx_client_updates_project
    ON client_updates(project_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_client_updates_org_status
    ON client_updates(org_id, status, created_at DESC);
```

- No `_cents` columns; no money in a homeowner update by design. Composite-currency rule N/A.
- Both indexes `CONCURRENTLY` (lint rule 5) — the table is not freshly created in a way that makes the indexes trivially small over time, so do **not** opt out.
- `.down.sql`: `DROP TABLE client_updates;` with `-- buildos:destructive: drop client_updates (v1 client-update domain)`.
- PII: `recipient_email` Restricted; `ai_draft`/`edited_body`/`subject` are operator-reviewed prose — treat as **Confidential** (may name the project / homeowner). Never include `recipient_email` in audit `metadata` JSONB; reference `client_update.id` + `project_id` only.

### 1.4 Daily-report aggregation: a **derived read model, not a table**

A "daily report" for `(project, date)` is **computed on read** from the three source tables — there is no `daily_reports` table in v1. The aggregation is a Go struct assembled by the store/service. This avoids a denormalized write path and keeps the field write contract untouched.

```go
// internal/models/dailyreport.go  (NEW)
type DailyReport struct {
    ProjectID         uuid.UUID   `json:"project_id"`
    ProjectName       string      `json:"project_name"`
    LogDate           time.Time   `json:"log_date"`          // the calendar date
    WeatherConditions string      `json:"weather_conditions,omitempty"`
    WorkSummary       string      `json:"work_summary"`
    SafetyIncidents   string      `json:"safety_incidents,omitempty"` // INTERNAL — never to client
    PhotoAssetIDs     []uuid.UUID `json:"photo_asset_ids,omitempty"`  // dangling today (see §1.5)
    PhotoCount        int         `json:"photo_count"`
    ReportedBy        Attribution `json:"reported_by"`        // display_name (Restricted) + id
    CrewCount         int         `json:"crew_count"`         // from crew_checkins JSONB length
    TaskProgress      []TaskProgressLine `json:"task_progress,omitempty"` // %-complete deltas for the day
    ReportedAt        time.Time   `json:"reported_at"`
}
```

### 1.5 Photo storage — OUT OF SCOPE for v1 (escalated)

There is **no assets/photos table and no object store / upload / presign code anywhere** (all 21+ migrations grepped). `daily_logs.photo_asset_ids` and `task_progress.photo_asset_id` are **dangling UUIDs pointing at nothing**. `bodysize.go` *reserves* `FileUploadMaxBodyBytes = 25 MiB` for a future upload but it is unbuilt.

**v1 ships text-only.** Daily Reports and client updates render **photo count** (`len(photo_asset_ids)`), not images. The AI image input (`internal/ai/image.go`) is **not** wired here. Building photo storage is a separate domain (object-store-vs-Postgres decision is a fork-deployment question per ADR-002 — see ESCALATION). **Do not block this domain on photos.**

---

## 2. OPERATOR DAILY REPORTS SURFACE (field → office half)

### 2.1 Stores (NEW read methods on `FieldStore`)

`internal/store/field.go` today has only INSERT paths. Add **read-only, org-scoped** methods (mirror existing `VerifyProjectInOrg` / org-filter discipline):

```go
// ListDailyLogsByProject returns daily logs for one project within an
// optional inclusive date range, newest first. org-scoped: caller passes
// orgID and the query filters daily_logs.org_id = $orgID AND project_id = $projectID.
// Uses idx_daily_logs_project_date.
func (s *FieldStore) ListDailyLogsByProject(ctx context.Context, orgID, projectID uuid.UUID, since, until *time.Time) ([]models.DailyLog, error)

// ListDailyLogsByOrgDate returns all daily logs across the org for one
// calendar date (the office-digest fan-out). org_id filtered directly.
func (s *FieldStore) ListDailyLogsByOrgDate(ctx context.Context, orgID uuid.UUID, day time.Time) ([]models.DailyLog, error)

// CrewCountByProjectDate buckets crew_checkins on reported_at::date and
// returns the distinct crew_members count (best-effort over opaque JSONB).
func (s *FieldStore) CrewCountByProjectDate(ctx context.Context, orgID, projectID uuid.UUID, day time.Time) (int, error)

// TaskProgressByProjectDate returns the day's %-complete lines, joined
// project_tasks -> projects to enforce org (task_progress has no org_id).
func (s *FieldStore) TaskProgressByProjectDate(ctx context.Context, orgID, projectID uuid.UUID, day time.Time) ([]models.TaskProgressLine, error)
```

Every query filters by `org_id` (per-query tenant isolation). `task_progress` queries **must** join through `project_tasks → projects` and assert `projects.org_id = $orgID` — the table has no `org_id` of its own.

### 2.2 Service (NEW `ReportsService`, read-only)

`internal/service/reports.go` (NEW). Read-only — **no audit rows** for reads (audit is for mutations; reads of operational data are not audited per house style). Methods:

- `ListProjectReports(ctx, orgID, projectID, since, until) ([]models.DailyReport, error)` — loads logs, then for each `(project, log_date)` folds in crew count + task-progress lines into `DailyReport`.
- `GetProjectReport(ctx, orgID, projectID, date) (models.DailyReport, error)` — single day.
- `enrichAttribution(...)` — resolves `reported_by → users(display_name)` for the byline.

Verify the project is in the org (`VerifyProjectInOrg`) before any read.

### 2.3 Endpoints (read-only list/detail, org+project scoped)

Mount under the existing `/api/v1/projects/{projectID}` subtree (project-scoped, where the data lives), alongside budget/schedule reads. **RBAC: `RequireMinRole(superintendent)`** — supers run the jobsite; `field_worker` is excluded (it only writes via `/field/*`). This mirrors the Briefing/Procurement gating.

| Method | Path | RBAC | Response |
|---|---|---|---|
| `GET` | `/api/v1/projects/{projectID}/daily-reports?since=&until=` | `minRole superintendent` | `200 { data: DailyReport[] }` newest-first |
| `GET` | `/api/v1/projects/{projectID}/daily-reports/{date}` | `minRole superintendent` | `200 { data: DailyReport }` (`date` = `YYYY-MM-DD`) |

- `since`/`until` optional ISO dates; default = last 14 days.
- 404 `PROJECT_NOT_FOUND` if project not in caller's org (uniform — do not leak existence).
- Add to `.agents/handoff/API_CONTRACT.md` (net-new domain section). Response envelope `{ data: ... }` matches house convention.
- **`safety_incidents` IS returned here** (internal operator surface) — it is **only** stripped on the client-facing path (§3).

### 2.4 Web surface

Per the IA grounding, **Daily Reports → Command Center group** (operational/today-focused, next to Daily Briefing/Schedule/Procurement):

- Route `/command/reports` (list) + `/command/reports/:id` (detail), gate `{ minRole: 'superintendent' }`. Register in **both** `web/src/router.ts` `RouteGate` table **and** `fb-nav-rail.ts` `NAV_MODEL` (they must stay in lockstep, and with `DESIGN_SYSTEM_COMPONENTS.md §1.3`).
- **Per-project entry point:** add a **"Daily Reports" tab** to `fb-project-detail-page.ts`'s `TABS` array (lazy-loaded via `onTab` like budget/schedule).
- Components (REUSE): `fb-data-table` (list: date, project, reporter, weather, work-summary excerpt, safety flag, photo count), `fb-state` (loading/empty/error), `fb-markdown` (render `work_summary`), `fb-breadcrumb` + `fb-tab-bar` (detail shell), `fb-chip` (project/date filter). Model the page on `fb-briefing-page.ts`.
- New web API module `web/src/api/endpoints/reports.ts` (clone `briefing.ts`/`feed.ts`).

---

## 3. AI COMPOSITION (office digest + client-safe homeowner update)

### 3.1 WHERE it lives — RECOMMENDATION: **two typed `*ai.Client` tasks**, wrapped by a service-layer orchestration (NOT the agentic harness, NOT the assistant loop)

Three shapes exist:

1. **Typed single-shot AI task** (`internal/ai/tasks.go`, the `DailyBriefing` precedent) — discriminated method on `*ai.Client`, marshals a domain request, `c.callText` + `FastModel` for prose, per-org key from `ai.ContextWithOrgID`, soft-fails `ErrUnconfigured`.
2. **Agentic orchestrator** (`internal/agentic`, the `delay_cascade` pattern) — leaf package, declares ports, service adapters implement them, load→reason→apply-in-one-tx→audit→feed cards. Used for **cross-module judgment** run from a River worker.
3. **Assistant tool loop** (`internal/service/assistant.go`) — interactive multi-tool chat. **Wrong fit** (this is a directed compose, not a conversation).

**RECOMMENDATION:** Shape 1 + a plain `service.ClientUpdateService`. Rationale:

- This is a **directed compose-and-send**, not cross-module judgment that mutates the schedule/budget. It does not need the harness's cross-module apply-in-one-tx machinery.
- Two **distinct task kinds** keep system prompts, audiences, and AI metric labels (`task_kind` on the Prometheus AI metric) separate — exactly mirroring why `daily_briefing` is its own kind.
- Add to `internal/ai/tasks.go`:
  - `DailyReportDigest(ctx, DailyReportDigestRequest) (*DailyReportDigestResponse, error)` — **office/internal** digest. Terse; INCLUDES safety incidents, crew, schedule deltas. `FastModel`, `c.callText`.
  - `ClientProgressUpdate(ctx, ClientProgressUpdateRequest) (*ClientProgressUpdateResponse, error)` — **client-safe** homeowner draft. Warm tone; EXCLUDES internal financials, safety-liability language, crew identities, GPS. `FastModel`, `c.callText`.

> **Agentic-isolation note (recorded for completeness):** IF a later phase routes this through `internal/agentic` (e.g. a per-org-configurable "client comms" capability on a daily tick), the harness MUST stay leaf — it **cannot import `internal/mailer`**. The `mailer.Send` call would then live in the service-layer Workspace adapter (the `ApplyX` port impl), exactly as `CascadeWorkspace.ApplyCascade` owns the tx + effects. For v1 this is moot: no harness involvement.

### 3.2 Prompt inputs

Both tasks take a **service-assembled structured request** built from:

- **Schedule snapshot:** project name, status, current critical-path summary / % complete (from `ScheduleStore` — same data the briefing uses). Gives the AI "where the project stands."
- **Recent daily logs:** the `[]DailyReport` over the window (weather, work_summary, task-progress deltas). For the **client** task, the service feeds a **deterministically pre-redacted** view (see §3.3) — `safety_incidents`, crew identities, GPS, and any `*_cents` are **never put in the prompt**.
- **Photo refs:** v1 passes **photo count** only (no image input until storage exists).

Mirror `DailyBriefing` exactly: marshal a domain struct to JSON, `c.callText(ctx, "<task_kind>", c.fastModel, <systemPrompt>, [textBlock(...)])`, return prose.

### 3.3 Client-safe redaction is **deterministic at the boundary, not trusted to the model** (security-critical)

The AI is **not** the redaction gate. The service builds the client-task request from an **allowlist of fields** and never includes Restricted/internal data:

- **Excluded from the client prompt entirely:** `safety_incidents`, `crew_members`/crew identities, GPS coords, `reported_by` identity, any `*_cents`/budget, internal notes.
- **Included:** project name, date range, weather, a sanitized work-summary, high-level % complete, photo count.
- The `ClientProgressUpdate` system prompt **additionally** instructs the model to write for a homeowner and omit liability/financial/internal language — belt-and-suspenders, but the allowlist is the real guarantee.
- `pii.FieldClass` classifies the excluded fields (emails/GPS/names = Restricted) — the request builder can assert no Restricted-classed field leaks into the client payload.

### 3.4 Human-in-the-loop — NEVER auto-send

The flow is strictly: **AI drafts → operator edits → operator previews → operator presses Send.** There is **no auto-send to a homeowner**, no scheduled tick that emails a client. The draft is persisted (`client_updates.status='draft'`, `ai_draft` populated) and only transitions to `sent` on an explicit operator action against an authenticated endpoint. (Contrast the office digest, which MAY surface passively as a feed card — that is internal.)

---

## 4. CLIENT UPDATE COMPOSER + SEND

### 4.1 Service (`internal/service/client_update.go`, NEW)

Modeled on `AgentsService.GenerateDailyBriefing` (load-context → AI draft) + the field service one-tx-per-mutation + audit pattern. Constructor takes `pool`, `ReportsService` (or the field/schedule stores), the two AI task interfaces (consumer-side, like `DailyBriefer`), a `clientUpdateStore`, the `mailer.Mailer`, and `AuditRecorder`. Nil mailer → `mailer.NewNoopMailer` (soft-fail posture). Nil AI → return a 503-mapped sentinel (mirror `ErrAgentsAIUnavailable`).

Methods:

| Method | Tx + Audit |
|---|---|
| `DraftClientUpdate(ctx, orgID, projectID, period) (models.ClientUpdate, error)` | Load schedule snapshot + `[]DailyReport` (redacted view), call `ClientProgressUpdate`, INSERT `client_updates{status:'draft', ai_draft, edited_body=ai_draft}` in one tx, audit `client_update.drafted`. AI soft-fail (no key) → return 503 sentinel; do NOT persist an empty draft. |
| `UpdateDraft(ctx, orgID, id, edited_body, subject) (models.ClientUpdate, error)` | Operator edit. One-tx UPDATE (status must be `draft`), audit `client_update.edited`. |
| `SendClientUpdate(ctx, orgID, id) (models.ClientUpdate, error)` | Load draft + project `client_email`. **Reject** if `client_email` empty (`ErrNoClientContact` → 422). One tx: snapshot `recipient_email`, set `sent_by`/`sent_at`/`status='sent'`, audit `client_update.sent`. **Then** `mailer.Send(ctx, orgID.String(), msg)` — see ordering note below. |
| `GenerateOfficeDigest(ctx, orgID, projectID, period) (models.FeedCard, error)` | Call `DailyReportDigest`, CreateFeedCard (`card_type` e.g. `daily_digest`) targeted to operators, audit `report.digest.generated`. Office digest = in-app feed card (lower-friction reuse than office email; office-email delivery is OQ-4). |

**Send ordering (important):** persist `status='sent'` + audit **inside the tx**, then call `mailer.Send` **after commit**. If send returns `ErrMailerUnconfigured`, that's a soft "no Resend key" — surface a clear 422/409 to the operator (`MAILER_UNCONFIGURED`) and roll the status back to `draft` (or write `status='failed'` + `send_error`), so the operator can fix the key and retry. Do **not** swallow it silently — unlike the password-reset best-effort posture, the operator *expects* this email to go out. (This is the one place the client-update flow diverges from the auth-reset soft-fail.) Never log `recipient_email`; log `org_id` + `client_update.id`.

Email composition copies the `auth.go sendResetEmail` pattern: build `mailer.Message{To: client_email, Subject, HTMLBody, TextBody}` via `fmt.Sprintf` from the edited body (render markdown → HTML). The `From` identity is whatever `cmd/server` already passes to `NewResendMailer` (reused verbatim, per-org Resend key from the vault).

### 4.2 Endpoints

**RBAC: `RequireRole(owner, admin)`** — sending an external homeowner-facing communication is an owner/admin trust action (matches HR/Org gating). A weaker `minRole superintendent` case exists if supers are expected to send; **flagged OQ-1**.

| Method | Path | RBAC | Body / Response |
|---|---|---|---|
| `POST` | `/api/v1/projects/{projectID}/client-updates` | owner,admin | `{ period_start, period_end }` → `201 { data: ClientUpdate }` (AI draft). `503 AI_UNAVAILABLE` if no key. |
| `GET` | `/api/v1/projects/{projectID}/client-updates` | owner,admin | `200 { data: ClientUpdate[] }` history, newest-first. |
| `GET` | `/api/v1/client-updates/{id}` | owner,admin | `200 { data: ClientUpdate }`. |
| `PATCH` | `/api/v1/client-updates/{id}` | owner,admin | `{ edited_body, subject }` → `200 { data: ClientUpdate }` (draft only; `409 ALREADY_SENT` otherwise). |
| `POST` | `/api/v1/client-updates/{id}/send` | owner,admin | → `200 { data: ClientUpdate }` (status `sent`). `422 NO_CLIENT_CONTACT`, `422 MAILER_UNCONFIGURED`, `409 ALREADY_SENT`. |

`ClientUpdate` JSON **omits `recipient_email`** from list responses by default (Restricted); include it only on the single-detail GET for owner/admin, or mask it. Org-scope every query; 404 uniform on cross-org `id`.

### 4.3 v1 SHARING — RECOMMENDATION: **EMAIL-ONLY (Resend)**

The SPA is **entirely behind auth**: every operational route is in the `authMiddleware` + `SetupGate` group; the only unauth routes are `MountAuthRoutes` and `spa.go` (which hard-404s any `/api/*` miss). There is **no public/tokenized read route pattern** and no "client" RBAC role (roles are owner>admin>superintendent>field_worker).

**Email-only is the safe v1:** operator composes/edits/sends; BuildOS renders HTML and sends via Resend to the homeowner. No new public route, no token lifecycle, no unauth attack surface, no public asset-serving. The only new PII is the homeowner email (already Restricted) and the content is operator-reviewed.

**Shareable public link (`/share/:token`) is v2** — it breaks the everything-behind-auth invariant, needs a CSPRNG hashed-token table (model on `setup_bootstrap_tokens`: sha256-hashed, uniform-error redemption, TTL), enumeration defense, a separate minimal render path, and reintroduces public photo serving. **Defer.** (OQ-3.)

### 4.4 Web composer (`fb-client-update-page`)

Per IA grounding, **Client Updates → Portfolio group** (per-project client-relationship artifact). Route `/portfolio/client-updates` (list/history) + composer (deep-linked from a project via the `?project=<id>` convention). Gate `{ roles: ['owner','admin'] }`.

Flow (REUSE existing components):
1. **Select project + date range** → POST draft.
2. **AI-draft step** — model on `fb-briefing-page.ts`'s AI-hero (gated/transient/ok states, `aiConfigured`/`markAiUnconfigured` key-gating, `fb-state mode="gated"` with owner Integrations deep-link on 503).
3. **Edit step** — editable `fb-markdown`/textarea, the composer affordance from `fb-assistant-page.ts` (textarea + `fb-button`). PATCH on save.
4. **Preview** — rendered `fb-markdown`.
5. **Send** — `fb-confirm`/`fb-modal` send-confirmation dialog (external email — confirm before send) → POST `/send`.

New web API module `web/src/api/endpoints/client-updates.ts`. Add a **"Send update"** action on the project detail page (and Daily Report detail "Draft client update" button) deep-linking to the composer with project preselected.

---

## 5. WIRING SUMMARY

### Migrations
- `NNN_project_client_contact.{up,down}.sql` — 3 nullable `projects` columns + prospect backfill. Destructive down header.
- `NNN_client_updates.{up,down}.sql` — `client_updates` table + 2 CONCURRENTLY indexes. Destructive down header.

### Models (`internal/models`)
- `project.go`: add `ClientName/ClientEmail/ClientPhone *string`.
- `dailyreport.go` (NEW): `DailyReport`, `TaskProgressLine`, `Attribution`.
- `client_update.go` (NEW): `ClientUpdate` struct mirroring the table.

### Stores (`internal/store`)
- `field.go`: `ListDailyLogsByProject`, `ListDailyLogsByOrgDate`, `CrewCountByProjectDate`, `TaskProgressByProjectDate` (all org-scoped reads).
- `projects.go`: extend project read/insert to carry client-contact columns; fix `CreateProjectFromProspect` to pass the three fields.
- `client_update.go` (NEW): `ClientUpdateStore` — `Create`, `Get`, `ListByProject`, `Update`, `MarkSent`/`MarkFailed` (all org-scoped; mutations take a `pgx.Tx`).

### Services (`internal/service`)
- `reports.go` (NEW): `ReportsService` (read-only; no audit).
- `client_update.go` (NEW): `ClientUpdateService` (one-tx + audit per mutation; mailer reuse).
- `pipeline.go`: backfill client-contact on `CreateProjectFromProspect`.
- `audit.go`: add `AuditResourceClientUpdate = "client_update"` and `AuditResourceDailyReport = "daily_report"` resource consts.

### AI (`internal/ai/tasks.go`)
- `DailyReportDigest` (kind `daily_report_digest`, FastModel) + `ClientProgressUpdate` (kind `client_progress_update`, FastModel). Request structs assembled by the service; client task fed the **redacted allowlist** view only.

### Audit action consts (new `report.*` / `client_update.*` actions)
`report.digest.generated`, `client_update.drafted`, `client_update.edited`, `client_update.sent`, `client_update.send_failed`. (Reads are not audited.)

### Router (`internal/api/router.go`) + handlers
- Project subtree: `GET .../daily-reports`, `GET .../daily-reports/{date}` — `RequireMinRole(superintendent)`.
- `POST/GET .../client-updates`, `GET/PATCH /client-updates/{id}`, `POST /client-updates/{id}/send` — `RequireRole(owner, admin)`.
- New handler files `internal/api/reports.go`, `internal/api/client_update.go` (thin; mirror existing handlers). Mount blocks guarded `if cfg.ReportsService != nil` / `if cfg.ClientUpdateService != nil` (matches the `agents != nil` pattern). `cmd/server` wires the services; `cmd/worker` passes nil (no client-update job in v1).

### PII (`internal/pii`)
- Add `client_email`, `client_name`, `client_phone`, `recipient_email` → Restricted in `FieldClass`. Confirm scrubbing covers nested `client_update` audit metadata.

### Nav / IA (web, lockstep)
- `router.ts` + `fb-nav-rail.ts` `NAV_MODEL` + `DESIGN_SYSTEM_COMPONENTS.md §1.3`: `/command/reports` (Command Center, `minRole superintendent`), `/portfolio/client-updates` (Portfolio, `roles owner,admin`). Project-detail `TABS`: add "Daily Reports".
- New web pages `fb-reports-page`, `fb-report-detail-page`, `fb-client-update-page`; web API modules `reports.ts`, `client-updates.ts`.

### Spec docs (dual-agent protocol)
- Author the net-new domain section in `.agents/handoff/API_CONTRACT.md` and `UX_CORE_SCREENS.md`. Per protocol, escalate the OQs below rather than improvise.

---

## 6. AGENTIC FRAMING

**How this advances VISION:** VISION casts BuildOS as an agentic OS whose harness does the GC's coordination busywork. "Daily reports → client updates" is a textbook **GC-coordination chore**: every evening a GC reads the day's field logs, writes the office an internal recap, and emails the homeowner a tidy progress note. This domain has the harness **compose both communications**, with the deterministic engine (schedule snapshot) as ground truth and the operator as the send gate. It is the first **communication/composition** surface — distinct from the existing harness role, which is cross-module *judgment that mutates the engine* (delay cascade, foresight, schedule adjust).

**RECOMMENDATION — does NOT warrant a new harness role for v1; fits the "experience/composition" lane as a plain service.** Reasons:
- v1 is a **directed compose-and-send**, not cross-module judgment that applies deltas across modules in one tx. The harness's value (load→reason→apply-in-one-tx→audit→feed cards, with per-org enable/disable + isolation gate) is overkill for a draft-edit-send.
- The **human-in-the-loop send gate** is antithetical to the harness's autonomous-tick model (`foresight_sweep`, `delay_cascade` run unattended). A client email must never auto-send.
- Keeping it a plain `service.ClientUpdateService` avoids the leaf-isolation constraint (which would force `mailer.Send` into a Workspace adapter) for no benefit yet.

**v2 harness path (record for the roadmap, do not build now):** if forks want a per-org-configurable "draft me a client update every Friday" that lands a **draft** (still never auto-sent) as a feed-card action, *that* is a real harness capability — a `client_comms` capability on a scheduled tick that produces a draft + a "review & send" feed card. At that point: declare an `agentic` port (`DraftClientUpdate`), implement the adapter in `internal/service` (it owns the tx, the `client_updates` INSERT, and — critically — any future `mailer.Send` stays in the adapter, never in `internal/agentic`), and register the capability so `make lint-isolation` stays green. The office digest is the more natural first harness candidate (it is internal, can surface passively as a feed card, and has no external-send risk). Both belong in the experience/composition lane — **no new top-level role needed**, extend the existing capability registry.

---

## 7. TEST PLAN + VERIFICATION

### Go unit tests
- `ai/tasks_test.go`: `DailyReportDigest` / `ClientProgressUpdate` marshal the right request, dispatch on the right `task_kind`, return prose; `ErrUnconfigured` soft-fails (fake AI client). **Redaction test:** assert the client-task request builder NEVER includes `safety_incidents`, crew identities, GPS, or `*_cents` (feed a `DailyReport` carrying all of them; inspect the marshaled prompt).
- `service/client_update_test.go`: draft → edit → send happy path with a fake mailer + fake AI + no-op audit; `SendClientUpdate` rejects empty `client_email` (`ErrNoClientContact`); `MAILER_UNCONFIGURED` rolls status back / records `failed`; PATCH on a `sent` row → `ErrAlreadySent`; audit actions recorded (`client_update.drafted/edited/sent`). Assert `recipient_email` never appears in any logged field (capture slog).
- `service/reports_test.go`: aggregation folds crew count + task-progress into the right `(project, date)` bucket; org-scope enforced.
- `pii_test.go`: `client_email`/`recipient_email` classify Restricted; `ScrubMap` redacts them in a nested `client_update` metadata blob.
- `lint-migrations`: the two new migrations pass all 5 rules (paired up/down, destructive headers present, CONCURRENTLY indexes, no forbidden numeric types). Add to the regression run.

### Go integration tests (`//go:build integration`, Testcontainers)
- `store/field_integration_test.go`: insert daily logs / checkins / task_progress, then `ListDailyLogsByProject` / `CrewCountByProjectDate` / `TaskProgressByProjectDate` return correct org-scoped, date-bucketed results; cross-org isolation (a second org's logs never appear); `task_progress` org-scope via `project_tasks→projects` join holds.
- `store/client_update_integration_test.go`: round-trip create/get/list/update/mark-sent; status CHECK enforced; cross-org `Get` returns not-found.
- `service` integration: full `DraftClientUpdate` → `UpdateDraft` → `SendClientUpdate` against a real DB + fake mailer, asserting `client_updates` row transitions and audit rows land in the same tx.
- `pipeline` integration: `CreateProjectFromProspect` carries `client_*` forward; the backfill `UPDATE` populates pre-existing projects.

### Web (vitest + live axe)
- `fb-reports-page` / `fb-report-detail-page`: list renders, loading/empty/error `fb-state`, project-tab lazy-load. axe clean.
- `fb-client-update-page`: AI-draft gated state on 503 (Integrations deep-link), edit textarea, preview markdown, send-confirm modal fires before POST `/send`. axe clean on every state.
- Endpoint modules: `reports.ts` / `client-updates.ts` shape tests.

### Browser verification walkthrough (proves the full loop)
With `DEV_AUTH_MODE=header`, a seeded project with a `client_email`, and a fake/sandbox Resend key:
1. **Field log lands:** POST `/api/v1/field/daily-log` (mobile shape) for the project + today.
2. **Office sees it:** as superintendent, open `/command/reports` (and the project's Daily Reports tab) → the new log appears with weather/work-summary/photo-count/crew-count; `safety_incidents` visible (internal).
3. **AI drafts:** as owner, open the composer, select the project + today, generate → a client-safe draft appears (assert `safety_incidents`/crew/GPS are NOT in it).
4. **Edit + preview:** tweak the body, preview renders markdown.
5. **Send:** press Send → confirm dialog → `client_updates` row flips to `sent`, `sent_at` set; the captured Resend request (sandbox) shows the homeowner address and the edited body; audit shows `client_update.sent`. Confirm `recipient_email` is absent from server logs.
6. **History:** the update shows in `/portfolio/client-updates` as `sent`.

`make audit` (lint-migrations + lint-migrations-test + test + test-prod + bench-physics) and `make lint-isolation` must stay green — confirm `internal/agentic` gains no new imports (it shouldn't; v1 is service-only).

---

## 8. ESCALATIONS — genuine product ambiguities (do NOT guess; resolve before building)

These belong in `.agents/handoff/ESCALATION_LOG.md`; the recommendation is the default if the owner does not object.

- **OQ-1 — Client Updates RBAC:** `{owner,admin}` (recommended — external comms = owner/admin trust) vs `{minRole superintendent}` (supers may own client comms). Daily Reports is `minRole superintendent` either way.
- **OQ-2 — Contact storage:** columns on `projects` (recommended, single homeowner) vs `project_contacts` table (multi-stakeholder: homeowner/architect/lender).
- **OQ-3 — v1 sharing:** email-only (recommended) vs public shareable link. Defer the link to v2 (breaks everything-behind-auth, reintroduces public asset serving).
- **OQ-4 — Office digest delivery:** feed card to operators (recommended, reuses `CreateFeedCard`) and/or email to office staff?
- **OQ-5 — Aggregation window:** what window does a client update cover — since last sent? a fixed date range? since last `log_date`? VISION says "aggregate," implying a period digest. Recommend operator-chosen `period_start`/`period_end` with a sensible default (since last `sent` update, else last 7 days). Dedup: no client-update dedup key in v1 (operator-gated send makes accidental double-send a UI confirm concern, not a server one).
- **OQ-6 — PHOTO STORAGE (hard blocker if photos are wanted):** there is **no assets/photos table and no object store anywhere**. ADR-002 single-tenant fork model raises: does each customer fork bring its own bucket (S3-compatible + presign) or do photos live in BuildOS Postgres (bytea/large-object)? This decision blocks any photo-inclusive report/update. **Recommendation: ship v1 text-only (photo count), escalate photo storage as its own domain.**
- **OQ-7 — Redaction trust:** confirm the deterministic allowlist (§3.3) is the redaction gate and the AI prompt is belt-and-suspenders only (recommended), vs trusting the model to redact (rejected — unsafe).
- **OQ-8 — Source breadth:** is `daily_logs.work_summary` the sole v1 source, or do `crew_checkins` (count) and `task_progress` (%-complete) fold in (recommended)? `crew_members` is opaque JSONB with no enforced shape — the daily-report view must tolerate arbitrary shapes (read count only).
