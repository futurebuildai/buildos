# Architecture Specification

**System:** BuildOS (System of Execution)
**Pipeline Stage:** 07 - Architecture Spec
**Date:** 2026-04-02
**Status:** COMPLETE

---

## 1. System Overview

BuildOS is a Go backend + Lit frontend + Flutter mobile system that serves as the execution plane for residential construction management. It covers the full project lifecycle from lead capture through construction completion — owning the pre-construction pipeline (CRM, estimating, permits), deterministic scheduling (CPM physics engine), financial records (Composite Currency Pattern: USD + CAD), autonomous AI agents, and field operations.

```
┌─────────────────────────────────────────────────────────────────┐
│                        BuildOS                           │
│                                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │
│  │ Lit Web  │  │ Flutter  │  │ REST API │  │ River Worker │   │
│  │ Frontend │  │ Mobile   │  │ (Chi)    │  │ (Background) │   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────┬───────┘   │
│       │              │             │                │           │
│       └──────────────┴─────────────┴────────────────┘           │
│                            │                                    │
│  ┌─────────────────────────┴─────────────────────────────┐     │
│  │              Service Layer (Interfaces)                │     │
│  │  ProjectServicer · ScheduleServicer · BudgetServicer  │     │
│  │  CorporateFinancialsServicer · FleetServicer          │     │
│  │  EmployeeServicer · FeedServicer · A2AServicer        │     │
│  │  PipelineServicer (Pre-Construction Kanban)            │     │
│  └─────────────────────────┬─────────────────────────────┘     │
│                            │                                    │
│  ┌────────────┐  ┌─────────┴─────────┐  ┌──────────────────┐  │
│  │  Physics   │  │    Repository     │  │    AI Agents     │  │
│  │  Engine    │  │    (pgx/v5)       │  │ DailyFocus       │  │
│  │ CPM · DHSM │  │                   │  │ Procurement      │  │
│  │ SWIM · EV  │  │                   │  │ SubLiaison       │  │
│  └────────────┘  └─────────┬─────────┘  └──────────────────┘  │
│                            │                                    │
│                   ┌────────┴────────┐                          │
│                   │  PostgreSQL 16  │                          │
│                   │  + pgvector     │                          │
│                   │  + River jobs   │                          │
│                   └─────────────────┘                          │
└─────────────────────────────────────────────────────────────────┘
          │                                    ▲
          │ JWT Validation (JWKS)              │ A2A Webhooks (JWS)
          ▼                                    │
    ┌─────────────┐                     ┌──────────────┐
    │  The Brain   │────────────────────▶│  The Brain    │
    │ OIDC Issuer │  (Identity Source)  │ A2A Emitter  │
    └─────────────┘                     └──────────────┘
```

---

## 2. Go Package Structure

```
futurebuild-os/
├── cmd/
│   ├── server/          # Main API server binary
│   │   └── main.go      # Wires router, services, middleware, starts HTTP
│   └── worker/          # River worker binary
│       └── main.go      # Wires River client, registers job workers, starts processing
├── internal/
│   ├── api/             # HTTP handlers (thin layer — delegates to services)
│   │   ├── router.go    # Chi router setup with middleware stack
│   │   ├── middleware/   # JWT validation, RBAC, request logging, CORS
│   │   │   ├── auth.go       # JWT validation via The Brain JWKS
│   │   │   ├── rbac.go       # Role-based access control
│   │   │   └── telemetry.go  # OpenTelemetry span + Prometheus middleware
│   │   ├── financials.go     # /api/v1/org/{orgID}/financials/* handlers
│   │   ├── projects.go       # /api/v1/projects/* handlers
│   │   ├── schedule.go       # /api/v1/projects/{id}/schedule/* handlers
│   │   ├── fleet.go          # /api/v1/org/{orgID}/fleet/* handlers
│   │   ├── hr.go             # /api/v1/org/{orgID}/employees/* handlers
│   │   ├── feed.go           # /api/v1/feed/* handlers
│   │   ├── field.go          # /api/v1/field/* handlers (sync, tasks)
│   │   ├── pipeline.go       # /api/v1/pipeline/* handlers (pre-construction CRM)
│   │   └── a2a.go            # /api/v1/a2a/webhook receiver (JWS verification)
│   ├── physics/         # Deterministic construction physics (PRESERVED FROM VAULT)
│   │   ├── cpm.go            # CPM forward/backward pass with gonum DAG
│   │   ├── cpm_test.go       # Benchmark tests: BenchmarkCPM80Tasks, BenchmarkCPM200Tasks
│   │   ├── cpm_determinism_test.go  # IMMUTABLE golden master test
│   │   ├── dhsm.go           # Duration/hours calculation (integer nanoseconds)
│   │   ├── dhsm_test.go      # BenchmarkDHSMPerTask
│   │   ├── swim.go           # Surface Weather Impact Model
│   │   ├── swim_test.go      # BenchmarkSWIMPerTask
│   │   ├── scoping.go        # WBS scoping and task hydration
│   │   └── equipment_validator.go  # Equipment constraint validation for WBS 7.x
│   ├── service/         # Business logic layer (interface implementations)
│   │   ├── interfaces.go     # All Servicer interfaces (23+ interfaces)
│   │   ├── project.go        # ProjectServicer implementation
│   │   ├── schedule.go       # ScheduleServicer — calls physics engine
│   │   ├── budget.go         # BudgetServicer — BIGINT cents arithmetic
│   │   ├── corporate_financials.go  # CorporateFinancialsServicer
│   │   ├── fleet.go          # FleetServicer
│   │   ├── employee.go       # EmployeeServicer
│   │   ├── feed.go           # FeedServicer (portfolio feed)
│   │   ├── weather.go        # WeatherServicer (Tomorrow.io client)
│   │   ├── invoice.go        # InvoiceServicer (extraction + matching)
│   │   ├── notification.go   # NotificationServicer (SMS via Twilio, email)
│   │   └── pipeline.go       # PipelineServicer (pre-construction Kanban, Kanban→CPM transition)
│   ├── agents/          # AI-powered autonomous agents
│   │   ├── daily_focus.go    # DailyFocusAgent — morning briefing generation
│   │   ├── procurement.go    # ProcurementAgent — lead time monitoring
│   │   └── sub_liaison.go    # SubLiaisonAgent — sub confirmation SMS
│   ├── futureshade/     # Decision engine framework
│   │   ├── service.go        # FutureShade orchestrator
│   │   ├── types.go          # Core FutureShade types
│   │   └── tribunal/         # Consensus-based decision engine
│   │       ├── engine.go     # ConsensusEngine — multi-model voting
│   │       └── types.go      # DecisionStatus, VoteType, TribunalRequest/Response
│   ├── models/          # Domain models (Go structs)
│   │   ├── project.go        # Project, ProjectTask, TaskDependency
│   │   ├── corporate_financials.go  # CorporateBudget, ARAgingSnapshot, GLSyncLog
│   │   ├── procurement.go    # ProcurementItem, ProcurementStatus
│   │   ├── feed.go           # FeedCard, FeedAction
│   │   ├── fleet.go          # FleetAsset, EquipmentAllocation, MaintenanceLog
│   │   ├── employee.go       # Employee, Certification, TimeLog
│   │   ├── pipeline.go       # Prospect, PipelineEstimate, Permit, PipelineStage
│   │   └── types/            # Shared enums and value types
│   │       └── dependency.go # DependencyType (FS, FF, SS, SF)
│   ├── store/           # Data access layer (raw SQL via pgx)
│   │   ├── pool.go           # pgxpool.Pool initialization
│   │   ├── project.go        # Project CRUD queries
│   │   ├── schedule.go       # Task/dependency queries
│   │   ├── financials.go     # Budget + corporate financials queries
│   │   ├── fleet.go          # Fleet asset queries
│   │   ├── employee.go       # Employee + certification queries
│   │   ├── feed.go           # Feed card queries
│   │   ├── field_sync.go     # Offline sync + idempotency key validation
│   │   └── pipeline.go       # Prospect, estimate, permit queries + Kanban→CPM transition
│   ├── worker/          # River job definitions
│   │   ├── registry.go       # Registers all job workers with River client
│   │   ├── daily_briefing.go     # TypeDailyBriefing worker
│   │   ├── procurement_check.go  # TypeProcurementCheck worker
│   │   ├── hydrate_project.go    # TypeHydrateProject worker
│   │   ├── corporate_rollup.go   # TypeCorporateRollup worker
│   │   ├── certification_alerts.go   # TypeCertificationAlerts worker
│   │   ├── maintenance_reminders.go  # TypeMaintenanceReminders worker
│   │   ├── field_notification_retry.go  # TypeFieldNotificationRetry worker
│   │   ├── a2a_webhook_dispatch.go     # TypeA2AWebhookDispatch worker
│   │   └── delay_cascade.go  # TypeDelayCascade worker
│   └── config/          # Configuration
│       └── config.go         # Environment variable parsing
├── migrations/          # PostgreSQL migrations (goose)
│   ├── 001_initial_schema.sql
│   ├── 002_river_setup.sql
│   └── ...
├── frontend/            # Lit Web Components
│   ├── src/
│   │   ├── components/
│   │   │   ├── base/
│   │   │   │   └── fb-element.ts        # FBBaseElement (glassmorphism, glow, skeleton)
│   │   │   ├── atoms/                   # fb-button, fb-icon, fb-badge, fb-input, etc.
│   │   │   ├── molecules/               # fb-feed-card, fb-stat-card, fb-data-cell, etc.
│   │   │   ├── organisms/               # fb-data-table, fb-gantt-chart, fb-nav-sidebar, etc.
│   │   │   └── pages/                   # fb-financials-view, fb-schedule-view, etc.
│   │   ├── styles/
│   │   │   └── variables.css            # GableLBM CSS custom properties
│   │   ├── state/                       # Signals-based state management
│   │   └── router.ts                    # Client-side routing
│   ├── tailwind.config.ts               # Tailwind CSS 4 @theme extensions
│   └── vite.config.ts
├── mobile/              # Flutter Field Portal
│   ├── lib/
│   │   ├── main.dart
│   │   ├── database/
│   │   │   ├── database.dart            # Drift database definition
│   │   │   ├── tables.dart              # Task, outbox, sync tables
│   │   │   └── daos/                    # Data access objects
│   │   ├── models/                      # Dart domain models
│   │   ├── services/
│   │   │   ├── sync_service.dart        # Outbox drain + pull sync
│   │   │   ├── auth_service.dart        # JWT token management
│   │   │   └── push_service.dart        # FCM integration
│   │   ├── screens/
│   │   │   ├── task_list_screen.dart
│   │   │   ├── daily_log_screen.dart
│   │   │   ├── crew_checkin_screen.dart
│   │   │   └── sync_status_screen.dart
│   │   └── widgets/                     # Reusable Flutter widgets
│   └── pubspec.yaml
├── Makefile             # Build, test, lint, audit commands
├── Dockerfile           # Multi-stage Go + frontend build
└── docker-compose.yml   # Local dev (PostgreSQL, Redis)
```

---

## 3. PostgreSQL Schema

### 3.1 Core Tables

```sql
-- Organizations (multi-tenant root)
CREATE TABLE organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    plan_tier   TEXT NOT NULL DEFAULT 'free',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Users (identity from The Brain OIDC)
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    oidc_subject    TEXT NOT NULL UNIQUE,  -- The Brain OIDC sub claim
    org_id          UUID NOT NULL REFERENCES organizations(id),
    email           TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'field_worker',  -- owner, admin, superintendent, field_worker
    locale          TEXT NOT NULL DEFAULT 'en-US',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_oidc_subject ON users(oidc_subject);
CREATE INDEX idx_users_org_id ON users(org_id);
```

### 3.2 Project & Schedule Tables

```sql
-- Projects
CREATE TABLE projects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    name                TEXT NOT NULL,
    address             TEXT,
    permit_issued_date  DATE,
    project_start_date  DATE,
    status              TEXT NOT NULL DEFAULT 'active',  -- active, completed, archived
    gsf                 INTEGER,  -- Gross Square Footage (1500-6000)
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_projects_org_id ON projects(org_id);

-- Project Tasks (WBS-based)
CREATE TABLE project_tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    wbs_code        TEXT NOT NULL,         -- e.g., "9.2"
    name            TEXT NOT NULL,
    duration_days   INTEGER NOT NULL,
    early_start     TIMESTAMPTZ,           -- Computed by CPM ForwardPass
    early_finish    TIMESTAMPTZ,
    late_start      TIMESTAMPTZ,           -- Computed by CPM BackwardPass
    late_finish     TIMESTAMPTZ,
    total_float     INTEGER,               -- In working days
    is_critical     BOOLEAN DEFAULT false,
    status          TEXT NOT NULL DEFAULT 'pending',
    percent_complete INTEGER NOT NULL DEFAULT 0,
    assigned_crew   UUID[],                -- Array of user IDs
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, wbs_code)
);
CREATE INDEX idx_tasks_project ON project_tasks(project_id);
CREATE INDEX idx_tasks_status ON project_tasks(status);

-- Task Dependencies (4 types: FS, FF, SS, SF)
CREATE TABLE task_dependencies (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    predecessor_id  UUID NOT NULL REFERENCES project_tasks(id),
    successor_id    UUID NOT NULL REFERENCES project_tasks(id),
    dependency_type TEXT NOT NULL DEFAULT 'FS',  -- FS, FF, SS, SF
    lag_days        INTEGER NOT NULL DEFAULT 0,
    UNIQUE(predecessor_id, successor_id)
);

-- Task Progress (field reports from Carlos)
CREATE TABLE task_progress (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         UUID NOT NULL REFERENCES project_tasks(id),
    reported_by     UUID NOT NULL REFERENCES users(id),
    percent_complete INTEGER NOT NULL,
    notes           TEXT,
    photo_asset_id  UUID,
    gps_lat         DOUBLE PRECISION,
    gps_lng         DOUBLE PRECISION,
    reported_via    TEXT NOT NULL DEFAULT 'web',  -- web, mobile
    idempotency_key UUID UNIQUE,  -- For offline dedup
    reported_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_progress_task ON task_progress(task_id);
CREATE INDEX idx_progress_idempotency ON task_progress(idempotency_key);
```

### 3.3 Financial Tables (Composite Currency Pattern)

```sql
-- Project Budgets (per WBS phase) — Composite Currency Pattern
CREATE TABLE project_budgets (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id              UUID NOT NULL REFERENCES projects(id),
    wbs_code                TEXT NOT NULL,
    phase_name              TEXT NOT NULL,
    estimated_cost_cents    BIGINT NOT NULL DEFAULT 0,
    estimated_cost_currency_code VARCHAR(3) NOT NULL DEFAULT 'USD',
    committed_cost_cents    BIGINT NOT NULL DEFAULT 0,
    committed_cost_currency_code VARCHAR(3) NOT NULL DEFAULT 'USD',
    actual_cost_cents       BIGINT NOT NULL DEFAULT 0,
    actual_cost_currency_code VARCHAR(3) NOT NULL DEFAULT 'USD',
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, wbs_code),
    CONSTRAINT chk_budget_currency_match CHECK (
        estimated_cost_currency_code = committed_cost_currency_code
        AND committed_cost_currency_code = actual_cost_currency_code
    )
);

-- Corporate Budget Rollups — grouped by currency_code (no cross-currency arithmetic)
CREATE TABLE corporate_budgets (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                  UUID NOT NULL REFERENCES organizations(id),
    fiscal_year             INTEGER NOT NULL,
    quarter                 INTEGER NOT NULL,
    currency_code           VARCHAR(3) NOT NULL DEFAULT 'USD',
    total_estimated_cents   BIGINT NOT NULL DEFAULT 0,
    total_committed_cents   BIGINT NOT NULL DEFAULT 0,
    total_actual_cents      BIGINT NOT NULL DEFAULT 0,
    project_count           INTEGER NOT NULL DEFAULT 0,
    last_rollup_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(org_id, fiscal_year, quarter, currency_code)
);
CREATE INDEX idx_corp_budget_org ON corporate_budgets(org_id);

-- AR Aging Snapshots — one row per currency per snapshot date
CREATE TABLE ar_aging_snapshots (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                  UUID NOT NULL REFERENCES organizations(id),
    snapshot_date           DATE NOT NULL,
    currency_code           VARCHAR(3) NOT NULL DEFAULT 'USD',
    current_cents           BIGINT NOT NULL DEFAULT 0,
    days_30_cents           BIGINT NOT NULL DEFAULT 0,
    days_60_cents           BIGINT NOT NULL DEFAULT 0,
    days_90_plus_cents      BIGINT NOT NULL DEFAULT 0,
    total_receivable_cents  BIGINT NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Invoices — Composite Currency Pattern
CREATE TABLE invoices (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    vendor_name         TEXT NOT NULL,
    invoice_number      TEXT,
    amount_cents        BIGINT NOT NULL,
    currency_code       VARCHAR(3) NOT NULL DEFAULT 'USD',
    wbs_code            TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',  -- pending, approved, rejected, paid
    due_date            DATE,
    paid_date           DATE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.4 Procurement & Feed Tables

```sql
-- Procurement Items
CREATE TABLE procurement_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id          UUID NOT NULL REFERENCES projects(id),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    name                TEXT NOT NULL,
    wbs_code            TEXT NOT NULL,
    estimated_cost_cents BIGINT NOT NULL DEFAULT 0,
    estimated_cost_currency_code VARCHAR(3) NOT NULL DEFAULT 'USD',
    lead_time_days      INTEGER NOT NULL DEFAULT 0,
    weather_buffer_days INTEGER NOT NULL DEFAULT 0,
    vendor_id           UUID,
    need_by_date        DATE,
    must_order_date     DATE,          -- Computed: need_by - lead_time - weather_buffer
    status              TEXT NOT NULL DEFAULT 'OK',  -- OK, WARNING, CRITICAL, ORDERED
    ordered_at          TIMESTAMPTZ,
    po_number           TEXT,
    status_changed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_procurement_project ON procurement_items(project_id);
CREATE INDEX idx_procurement_status ON procurement_items(status);

-- Feed Cards
CREATE TABLE feed_cards (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    project_id      UUID REFERENCES projects(id),
    card_type       TEXT NOT NULL,     -- weather_alert, procurement, sub_confirmation, progress, etc.
    title           TEXT NOT NULL,
    body            TEXT,
    priority        TEXT NOT NULL DEFAULT 'normal',  -- critical, urgent, normal, low
    target_user_id  UUID REFERENCES users(id),
    target_role     TEXT,              -- Alternative to target_user: role-based targeting
    actions         JSONB,             -- [{label, action_type, payload}]
    status          TEXT NOT NULL DEFAULT 'active',  -- active, dismissed, actioned, expired
    actioned_at     TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_feed_org_status ON feed_cards(org_id, status);
CREATE INDEX idx_feed_target ON feed_cards(target_user_id);

-- Communication Logs (Sub Liaison)
CREATE TABLE communication_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    task_id         UUID NOT NULL REFERENCES project_tasks(id),
    contact_name    TEXT NOT NULL,
    contact_phone   TEXT,
    message_type    TEXT NOT NULL,      -- sms_confirmation, sms_reminder
    message_body    TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'PENDING',  -- PENDING, SENT, DELIVERED, FAILED
    response_body   TEXT,
    response_parsed TEXT,               -- confirmed, delayed, unrecognized
    idempotency_key UUID UNIQUE,
    sent_at         TIMESTAMPTZ,
    response_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 3.5 Fleet & HR Tables

```sql
-- Fleet Assets
CREATE TABLE fleet_assets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    name            TEXT NOT NULL,
    asset_type      TEXT NOT NULL,      -- excavator, compactor, grader, crane
    serial_number   TEXT,
    status          TEXT NOT NULL DEFAULT 'available',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Equipment Allocations
CREATE TABLE equipment_allocations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset_id        UUID NOT NULL REFERENCES fleet_assets(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    start_date      DATE NOT NULL,
    end_date        DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    EXCLUDE USING gist (asset_id WITH =, daterange(start_date, end_date) WITH &&)
);

-- Employees
CREATE TABLE employees (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    user_id         UUID REFERENCES users(id),
    first_name      TEXT NOT NULL,
    last_name       TEXT NOT NULL,
    role            TEXT NOT NULL,
    phone           TEXT,
    hire_date       DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Certifications
CREATE TABLE certifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    employee_id     UUID NOT NULL REFERENCES employees(id),
    cert_type       TEXT NOT NULL,      -- contractor_license, insurance, osha_10, etc.
    cert_number     TEXT,
    issued_date     DATE,
    expiry_date     DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_certs_expiry ON certifications(expiry_date);
```

### 3.6 Field Sync Tables

```sql
-- Field Notification Dead Letter Queue
CREATE TABLE field_notification_dlq (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id),
    notification_type TEXT NOT NULL,
    payload         JSONB NOT NULL,
    retry_count     INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Weather Forecast Cache
CREATE TABLE weather_cache (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lat             DOUBLE PRECISION NOT NULL,
    lng             DOUBLE PRECISION NOT NULL,
    forecast_data   JSONB NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_weather_cache_location ON weather_cache(lat, lng);
CREATE INDEX idx_weather_cache_expiry ON weather_cache(expires_at);
```

### 3.7 Pre-Construction Pipeline Tables

```sql
-- Pre-Construction Prospects (CRM Kanban)
CREATE TABLE pre_construction_prospects (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID NOT NULL REFERENCES organizations(id),
    name                TEXT NOT NULL,
    client_name         TEXT NOT NULL,
    client_email        TEXT,
    client_phone        TEXT,
    address             TEXT,
    gsf                 INTEGER,  -- Gross Square Footage estimate
    pipeline_stage      TEXT NOT NULL DEFAULT 'LEAD',
    -- LEAD (10%) → QUALIFIED (25%) → ESTIMATE_SENT (50%)
    -- → VERBAL_COMMITMENT (75%) → PERMIT_APPLIED (85%) → PERMIT_ISSUED (100%)
    probability_pct     INTEGER NOT NULL DEFAULT 10,
    source              TEXT,     -- referral, website, repeat_client
    notes               TEXT,
    lost_reason         TEXT,     -- Only set if stage = LOST
    project_id          UUID REFERENCES projects(id),  -- Set on PERMIT_ISSUED transition
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_prospects_org ON pre_construction_prospects(org_id);
CREATE INDEX idx_prospects_stage ON pre_construction_prospects(pipeline_stage);

-- Pre-Construction Estimates (preliminary, pre-permit)
CREATE TABLE pre_construction_estimates (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id         UUID NOT NULL REFERENCES pre_construction_prospects(id),
    version             INTEGER NOT NULL DEFAULT 1,
    total_estimated_cents BIGINT NOT NULL DEFAULT 0,
    currency_code       VARCHAR(3) NOT NULL DEFAULT 'USD',
    line_items          JSONB NOT NULL DEFAULT '[]',
    -- [{wbs_code, description, estimated_cents, unit, quantity}]
    margin_pct          INTEGER NOT NULL DEFAULT 15,
    status              TEXT NOT NULL DEFAULT 'draft', -- draft, sent, revised, accepted
    sent_at             TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_estimates_prospect ON pre_construction_estimates(prospect_id);

-- Pre-Construction Permits (municipal tracking)
CREATE TABLE pre_construction_permits (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prospect_id         UUID NOT NULL REFERENCES pre_construction_prospects(id),
    permit_type         TEXT NOT NULL,  -- building, electrical, plumbing, mechanical
    jurisdiction        TEXT NOT NULL,
    application_number  TEXT,
    submitted_date      DATE,
    expected_issue_date DATE,
    actual_issue_date   DATE,
    fee_cents           BIGINT NOT NULL DEFAULT 0,
    fee_currency_code   VARCHAR(3) NOT NULL DEFAULT 'USD',
    status              TEXT NOT NULL DEFAULT 'not_submitted',
    -- not_submitted, submitted, under_review, revisions_requested, approved, denied
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_permits_prospect ON pre_construction_permits(prospect_id);
CREATE INDEX idx_permits_status ON pre_construction_permits(status);
```

### 3.8 River Queue Setup

```sql
-- River migration (auto-managed by River library)
-- Creates: river_job, river_leader, river_queue tables
-- See: https://riverqueue.com/docs/migrations

-- Periodic jobs (replaces Asynq scheduler cron entries)
INSERT INTO river_periodic_job (kind, period, args) VALUES
    ('daily_briefing',            '0 6 * * *',    '{}'),   -- 06:00 UTC daily
    ('procurement_check',         '0 5 * * *',    '{}'),   -- 05:00 UTC daily
    ('corporate_rollup',          '0 4 * * *',    '{}'),   -- 04:00 UTC daily
    ('certification_alerts',      '0 7 * * 1',    '{}'),   -- 07:00 UTC Monday
    ('maintenance_reminders',     '0 8 * * 1',    '{}'),   -- 08:00 UTC Monday
    ('resource_conflict_scan',    '0 3 * * *',    '{}');   -- 03:00 UTC daily
```

---

## 4. River Job Definitions

All River jobs replace legacy Asynq task types. Each job is a Go struct implementing `rivertype.JobArgs`.

| Job Kind | Schedule | Legacy Type | Description |
|----------|----------|-------------|-------------|
| `daily_briefing` | 06:00 UTC daily | TypeDailyBriefing | DailyFocusAgent generates morning briefing cards per project |
| `procurement_check` | 05:00 UTC daily | TypeProcurementCheck | ProcurementAgent scans items, transitions WARNING/CRITICAL |
| `hydrate_project` | On project create | TypeHydrateProject | Populates procurement_items + runs initial CPM |
| `corporate_rollup` | 04:00 UTC daily | TypeCorporateRollup | Aggregates PROJECT_BUDGETS -> CorporateBudget + ARAgingSnapshot |
| `certification_alerts` | 07:00 UTC Monday | TypeCertificationAlerts | Scans certifications for 30/14/7 day expiry |
| `maintenance_reminders` | 08:00 UTC Monday | TypeMaintenanceReminders | Checks fleet maintenance schedules |
| `field_notification_retry` | Event-driven | TypeFieldNotificationRetry | Retries failed push notifications (backoff: 30s→1hr, 6 retries) |
| `delay_cascade` | Event-driven | TypeDelayCascade | Recalculates CPM when task delays are reported |
| `a2a_webhook_dispatch` | Event-driven | TypeA2AWebhookDispatch | Dispatches A2A webhooks to The Brain |
| `sub_liaison_scan` | 12:00 UTC daily | (new) | SubLiaisonAgent scans tasks starting within 48-72h |
| `pipeline_analytics` | 06:00 UTC daily | (new) | Recalculates weighted pipeline revenue per currency_code |
| `permit_issued_transition` | Event-driven | (new) | Kanban→CPM atomic transition on permit issuance |

**Transactional Job Insertion Pattern:**

```go
// All job insertions happen within the same PostgreSQL transaction as the triggering operation
// This prevents "phantom jobs" that reference data not yet committed
func (s *ScheduleService) RecalculateSchedule(ctx context.Context, projectID uuid.UUID) error {
    return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
        // 1. Run CPM ForwardPass + BackwardPass
        result, err := s.physics.Calculate(ctx, tx, projectID)
        if err != nil { return err }

        // 2. Persist schedule results
        if err := s.store.UpdateSchedule(ctx, tx, result); err != nil { return err }

        // 3. Enqueue delay cascade if critical path changed (SAME TRANSACTION)
        if result.CriticalPathChanged {
            _, err := s.river.InsertTx(ctx, tx, &DelayCascadeArgs{
                ProjectID: projectID,
            }, nil)
            return err
        }
        return nil
    })
}
```

**Kanban→CPM Phase Transition (Atomic):**

```go
// Triggered when pipeline_stage transitions to PERMIT_ISSUED
// Executes in a single PostgreSQL transaction to guarantee atomicity
func (s *PipelineService) TransitionToConstruction(ctx context.Context, prospectID uuid.UUID, permitIssuedDate time.Time) error {
    return pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
        // 1. Validate prospect is in PERMIT_APPLIED stage
        prospect, err := s.store.GetProspect(ctx, tx, prospectID)
        if err != nil { return err }
        if prospect.PipelineStage != "PERMIT_APPLIED" {
            return fmt.Errorf("invalid transition: prospect stage is %s, expected PERMIT_APPLIED", prospect.PipelineStage)
        }

        // 2. Create Project from prospect data
        project := models.Project{
            OrgID:            prospect.OrgID,
            Name:             prospect.Name,
            Address:          prospect.Address,
            PermitIssuedDate: &permitIssuedDate,
            GSF:              prospect.GSF,
            Status:           "active",
        }
        projectID, err := s.store.CreateProject(ctx, tx, project)
        if err != nil { return err }

        // 3. Update prospect: set project_id, stage = PERMIT_ISSUED, probability = 100
        if err := s.store.UpdateProspectStage(ctx, tx, prospectID, "PERMIT_ISSUED", 100, &projectID); err != nil {
            return err
        }

        // 4. Hydrate WBS template via physics scoping engine
        if err := s.store.HydrateWBSTemplate(ctx, tx, projectID, prospect.GSF); err != nil {
            return err
        }

        // 5. Enqueue initial CPM calculation (SAME TRANSACTION)
        _, err = s.river.InsertTx(ctx, tx, &HydrateProjectArgs{
            ProjectID: projectID,
        }, nil)
        return err
    })
}
```

**Constraint:** `pre_construction_prospects.project_id IS NULL` until `PERMIT_ISSUED`. No CPM tasks can reference a prospect that hasn't completed the transition. The database enforces this via the foreign key on `project_id → projects(id)`.

---

## 5. API Route Contracts

### 5.1 Authentication Middleware

All routes (except `/health`) require JWT validation:

```go
// JWT claims from The Brain OIDC
type Claims struct {
    Sub      string `json:"sub"`       // OIDC subject (user ID in Brain)
    OrgID    string `json:"org_id"`    // Organization ID
    Role     string `json:"role"`      // owner, admin, superintendent, field_worker
    PlanTier string `json:"plan_tier"` // free, pro, enterprise
    jwt.RegisteredClaims
}
```

### 5.2 Financial Endpoints (Composite Currency Pattern)

```
GET  /api/v1/org/{orgID}/financials/summary?currency=USD
     Response: { corporate_budget: CorporateBudget, ar_aging: ARAgingSnapshot }
     Note: Results grouped by currency_code. Omit ?currency to get all currencies.

GET  /api/v1/org/{orgID}/financials/ar-aging?currency=USD
     Response: { snapshots: []ARAgingSnapshot }
     Note: One snapshot per currency_code per snapshot_date.

GET  /api/v1/org/{orgID}/financials/projects?currency=USD
     Response: { projects: []ProjectFinancial }
     ProjectFinancial: { project_id, name, currency_code,
                         estimated_cost_cents, committed_cost_cents,
                         actual_cost_cents, variance_cents, variance_pct }
     Note: Cross-currency aggregation is FORBIDDEN. Filter or group by currency.
```

### 5.3 Schedule Endpoints

```
POST /api/v1/projects/{id}/schedule/recalculate
     Response: { cpm_result: CPMResult, recalculation_ms: int }
     NFR: <800ms end-to-end, <200ms physics computation

GET  /api/v1/projects/{id}/schedule/gantt
     Response: { tasks: []TaskSchedule, critical_path: []uuid, project_end: timestamp }
```

### 5.4 Feed Endpoints

```
GET  /api/v1/feed?status=active&priority=critical,urgent
     Response: { cards: []FeedCard }

POST /api/v1/feed/{cardID}/action
     Body: { action_type: string, payload: object }
     Response: { card: FeedCard, result: ActionResult }
```

### 5.5 Field Sync Endpoints

```
GET  /api/v1/field/sync?since={timestamp}
     Response: { notifications: []Notification, tasks: []TaskUpdate, server_time: timestamp }

POST /api/v1/field/progress
     Body: { task_id, percent_complete, photo_asset_id, gps_lat, gps_lng, idempotency_key }
     Response: 201 Created | 409 Conflict (duplicate idempotency_key)

POST /api/v1/field/checkin
     Body: { project_id, crew_members: []{ worker_id, gps_lat, gps_lng }, idempotency_key }
     Response: 201 Created | 409 Conflict
```

### 5.6 Pre-Construction Pipeline Endpoints

```
# Prospects (CRM)
GET    /api/v1/org/{orgID}/pipeline/prospects?stage={stage}
       Response: { prospects: []Prospect }

POST   /api/v1/org/{orgID}/pipeline/prospects
       Body: { name, client_name, client_email, client_phone, address, gsf, source }
       Response: 201 { prospect: Prospect }

GET    /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
       Response: { prospect: Prospect, estimates: []Estimate, permits: []Permit }

PUT    /api/v1/org/{orgID}/pipeline/prospects/{prospectID}
       Body: { ...fields }
       Response: { prospect: Prospect }

# Stage Transitions (Kanban)
POST   /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/advance
       Body: { target_stage: "QUALIFIED"|"ESTIMATE_SENT"|..., permit_issued_date?: date }
       Response: { prospect: Prospect, project_id?: uuid }
       Note: PERMIT_ISSUED triggers atomic Kanban→CPM transition, returns new project_id

POST   /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/lose
       Body: { reason: string }
       Response: { prospect: Prospect }

# Estimates
POST   /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/estimates
       Body: { line_items: [], margin_pct, currency_code }
       Response: 201 { estimate: Estimate }

PUT    /api/v1/org/{orgID}/pipeline/estimates/{estimateID}
       Body: { line_items: [], margin_pct }
       Response: { estimate: Estimate }

# Permits
POST   /api/v1/org/{orgID}/pipeline/prospects/{prospectID}/permits
       Body: { permit_type, jurisdiction, application_number, submitted_date, fee_cents, fee_currency_code }
       Response: 201 { permit: Permit }

PUT    /api/v1/org/{orgID}/pipeline/permits/{permitID}
       Body: { status, actual_issue_date?, notes? }
       Response: { permit: Permit }

# Pipeline Analytics
GET    /api/v1/org/{orgID}/pipeline/analytics
       Response: { by_currency: [{ currency_code, stages: [{ stage, count, weighted_revenue_cents }] }] }
       Note: Revenue forecasting grouped by currency_code (no cross-currency sums)
```

### 5.7 A2A Webhook Receiver

```
POST /api/v1/a2a/webhook
     Headers: X-JWS-Signature (JWS detached signature)
     Body: { event_type, payload, trace_id, timestamp }
     Response: 200 OK | 401 Invalid Signature | 409 Duplicate

     Supported event_types:
       - review_material_quote  (payload includes currency_code)
       - review_labor_bid       (payload includes currency_code)
       - update_schedule
       - delivery_confirmation
       - create_feed_card
```

---

## 6. Physics Engine Architecture

### 6.1 Preserved From Reference Vault

The physics engine is extracted directly from the reference vault with no algorithmic changes:

| File | Lines | Function |
|------|-------|----------|
| `cpm.go` | ~725 | BuildDependencyGraph, ForwardPass, BackwardPass, TopologicalSort, DetectCycle |
| `dhsm.go` | ~293 | Duration/Hours State Machine — integer nanosecond arithmetic |
| `swim.go` | ~67 | Surface Weather Impact — multipliers for WBS 7.0-9.6 |
| `scoping.go` | ~399 | WBS template hydration and task scoping |
| `equipment_validator.go` | ~117 | Equipment constraint validation for earthwork |

### 6.2 Key Function Signatures

```go
// Build DAG from tasks and dependencies
func BuildDependencyGraph(tasks []models.ProjectTask, deps []models.TaskDependency) *DependencyGraph

// Forward pass: compute EarlyStart/EarlyFinish with material constraints
func ForwardPass(g *DependencyGraph, projectStart time.Time, cal Calendar,
    materialConstraints map[uuid.UUID]time.Time) (map[uuid.UUID]TaskSchedule, error)

// Backward pass: compute LateStart/LateFinish, identify critical path
func BackwardPass(g *DependencyGraph, schedule map[uuid.UUID]TaskSchedule,
    cal Calendar, config *SchedulingConfig) ([]string, error)
```

### 6.3 Performance Budgets (CI Hard Gates)

```makefile
# Makefile target: make audit
audit:
    go test -bench=BenchmarkCPM -benchtime=10x ./internal/physics/... \
        -run=^$$ | go run ./tools/bench-gate/main.go \
        --cpm80=200ms --cpm200=500ms --dhsm=1ms --swim=5ms --graph=50ms
    @echo "Physics engine benchmarks: PASSED"
```

---

## 7. Flutter Offline Architecture

### 7.1 Drift Database Schema

```dart
// Drift table definitions
class Tasks extends Table {
  UuidColumn get id => uuid()();
  TextColumn get projectId => text()();
  TextColumn get wbsCode => text()();
  TextColumn get name => text()();
  IntColumn get percentComplete => integer().withDefault(const Constant(0))();
  TextColumn get priority => text().withDefault(const Constant('normal'))();
  TextColumn get status => text().withDefault(const Constant('pending'))();
  DateTimeColumn get scheduledDate => dateTime().nullable()();
  DateTimeColumn get lastSyncedAt => dateTime().nullable()();
}

class Outbox extends Table {
  UuidColumn get id => uuid()();
  TextColumn get actionType => text()();        // task_progress, crew_checkin, photo_upload
  TextColumn get payloadJson => text()();
  TextColumn get idempotencyKey => text()();     // UUID v7
  DateTimeColumn get createdAt => dateTime()();
  TextColumn get syncStatus => text().withDefault(const Constant('pending'))();
  // pending -> syncing -> synced | failed
  IntColumn get retryCount => integer().withDefault(const Constant(0))();
  DateTimeColumn get lastAttemptAt => dateTime().nullable()();
}

class CachedBriefings extends Table {
  UuidColumn get id => uuid()();
  TextColumn get projectId => text()();
  TextColumn get cardsJson => text()();          // Serialized feed cards
  DateTimeColumn get generatedAt => dateTime()();
  DateTimeColumn get cachedAt => dateTime()();
}
```

### 7.2 Sync Service Flow

```
Device Online:
  1. Pull: GET /api/v1/field/sync?since={lastSyncTimestamp}
  2. Update local Drift tables with server data
  3. Push: Drain outbox entries in FIFO order
     - For each entry: POST to appropriate endpoint with idempotency_key
     - 201: Mark synced, delete from outbox
     - 409: Already processed, mark synced
     - 5xx: Increment retry_count, backoff (1s, 2s, 4s, 8s, max 5min)
  4. Update lastSyncTimestamp

Device Offline:
  1. All writes go to Outbox table
  2. UI reads from local Drift tables
  3. Connectivity indicator shows Amber + pending count
  4. workmanager monitors connectivity_plus for restore event
```

---

## 8. Lit Component Hierarchy

```
fb-org-shell (organism — app shell)
├── fb-nav-sidebar (organism — navigation)
│   ├── fb-nav-item (molecule — nav links)
│   └── fb-avatar (atom — user avatar)
│
├── Portfolio Dashboard
│   ├── fb-financials-view (page)
│   │   ├── fb-budget-summary (organism — 3 stat cards)
│   │   │   └── fb-stat-card (molecule — single KPI)
│   │   ├── fb-ar-aging-chart (organism — D3 stacked bar)
│   │   └── fb-data-table (organism — project financials)
│   │       └── fb-data-cell (molecule — typed cell with JetBrains Mono)
│   ├── fb-fleet-view (page)
│   └── fb-hr-view (page)
│
├── Pre-Construction Pipeline
│   ├── fb-pipeline-view (page)
│   │   ├── fb-pipeline-kanban (organism — 6-stage drag board)
│   │   │   └── fb-pipeline-card (molecule — prospect summary)
│   │   └── fb-pipeline-summary (organism — weighted revenue by currency)
│   ├── fb-prospect-detail (page)
│   │   ├── fb-estimate-form (organism — line-item editor)
│   │   └── fb-permit-tracker (organism — permit status timeline)
│   └── fb-pipeline-analytics (page — revenue forecasting charts)
│
├── Agent Command Center
│   ├── fb-briefing-view (page)
│   │   └── fb-feed-list (organism)
│   │       └── fb-feed-card (molecule — action card)
│   ├── fb-schedule-view (page)
│   │   └── fb-gantt-chart (organism — CPM visualization)
│   └── fb-procurement-view (page)
│
└── Settings
    └── fb-settings-view (page)
```

---

## 9. CI/CD Pipeline

```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  backend:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_DB: fbos_test
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }

      # SQL Migration Linter (HARD FAIL — no exemptions)
      # Checks: (1) No DECIMAL/NUMERIC/FLOAT/MONEY on monetary columns
      #          (2) No orphan amount_cents without currency_code
      - name: Composite Currency Pattern Enforcement
        run: ./scripts/lint-migrations.sh

      # Unit + integration tests
      - name: Test
        run: go test ./...

      # Physics engine benchmark gates
      - name: Audit (make audit)
        run: make audit

      # Lint
      - name: Lint
        run: golangci-lint run ./...

  frontend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
      - run: cd frontend && npm ci && npm run lint && npm run build
      - name: Lighthouse CI
        run: npx lhci autorun

  mobile:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: subosito/flutter-action@v2
      - run: cd mobile && flutter analyze && flutter test
```

---

## 10. Cross-System Integration

### 10.1 JWT Validation (The Brain -> BuildOS)

```go
// JWKS-based JWT validation middleware
type JWKSValidator struct {
    jwksURL     string              // The Brain's /.well-known/jwks.json
    cachedKeys  *jwk.Set            // Cached JWKS key set
    refreshedAt time.Time
    mu          sync.RWMutex
}

// Validates JWT on every request; caches JWKS with 1-hour refresh
func (v *JWKSValidator) Middleware(next http.Handler) http.Handler
```

### 10.2 A2A Webhook Verification

```go
// Verifies JWS detached signature on inbound A2A webhooks
func VerifyA2AWebhook(r *http.Request, brainPublicKey *rsa.PublicKey) error {
    signature := r.Header.Get("X-JWS-Signature")
    body, _ := io.ReadAll(r.Body)
    // Verify RS256 JWS signature using go-jose/v4
    jws, err := jose.ParseDetached(signature, body)
    // Validate claims: iss=fb-brain, aud=fb-os, exp not passed
    return err
}
```

### 10.3 External Service Integrations

| Service | Purpose | Auth | Client Package |
|---------|---------|------|---------------|
| Tomorrow.io | Weather forecast (SWIM v2) | API Key | `internal/service/weather.go` |
| Twilio | SMS for SubLiaison | API Key | `internal/service/notification.go` |
| Firebase (FCM) | Push notifications | Service Account | `internal/service/notification.go` |
| Anthropic Claude | AI agent reasoning | API Key | `internal/agents/*.go` |

---

## 11. Observability Stack

```
                    ┌──────────────┐
                    │   Grafana    │ ◄── Dashboards
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────┐
        │Prometheus│ │  Sentry  │ │ Firebase │
        │ Metrics  │ │  Errors  │ │Analytics │
        └────┬─────┘ └────┬─────┘ └────┬─────┘
             │             │            │
        Chi middleware  Go panic   Flutter app
        River metrics   handler    Crashlytics
        Physics bench   Frontend   Usage data
```

| Layer | Tool | Data |
|-------|------|------|
| API Metrics | Prometheus + OpenTelemetry | Request latency, error rate, throughput |
| Queue Metrics | River built-in | Job states, durations, queue depth |
| Physics Benchmarks | `make audit` | CPM/DHSM/SWIM computation times |
| Frontend | Web Vitals (LCP, CLS, FID) | Sentry Performance |
| Mobile | Firebase Analytics + Crashlytics | Crash rate, session data |
| Business | PostgreSQL queries | Feed card rates, procurement automation |
