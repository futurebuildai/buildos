# Information Architecture

**Document ID:** AG-05-IA
**System:** FutureBuild OS (System of Execution)
**Created:** 2026-04-02
**Pipeline Stage:** 05 - Design System
**Status:** COMPLETE

---

## 1. Workspace Model

FutureBuild OS organizes all user experience into **three workspaces**, each optimized for a distinct user archetype and interaction pattern.

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        FutureBuild OS                                   │
├─────────────────┬─────────────────────────┬─────────────────────────────┤
│  PORTFOLIO      │  AGENT COMMAND CENTER   │  FIELD PORTAL               │
│  Dashboard      │  (Chat + Feed Cards)    │  (Flutter Mobile)           │
│                 │                          │                             │
│  Tom (Owner)    │  Mike (Superintendent)   │  Carlos (Field Worker)      │
│  Sarah (Admin)  │  Sarah (Admin)           │  Subcontractors             │
│                 │  AI Agents               │                             │
│                 │                          │                             │
│  Web Desktop    │  Web Desktop + Mobile    │  Mobile Only (Offline)      │
├─────────────────┼─────────────────────────┼─────────────────────────────┤
│  Financial KPIs │  Morning Briefing       │  Task List                  │
│  Project Grid   │  Procurement Alerts     │  Daily Log                  │
│  HR Dashboard   │  Schedule (Gantt)       │  Photo Progress             │
│  Fleet Assets   │  Agent Activity         │  Crew Check-in              │
│  AR Aging       │  Chat + Artifacts       │  Offline Queue              │
└─────────────────┴─────────────────────────┴─────────────────────────────┘
```

### Workspace Definitions

| Workspace | Primary Users | Access | Purpose |
|-----------|--------------|--------|---------|
| **Portfolio Dashboard** | Tom (Owner), Sarah (Admin) | Web (desktop ≥1200px) | Financial oversight, multi-project KPIs, HR/Fleet management, approval workflows |
| **Agent Command Center** | Mike (Superintendent), Sarah (Admin), AI Agents | Web (desktop + tablet) | AI-driven chat, morning briefing, schedule management, procurement, real-time agent activity |
| **Field Portal** | Carlos (Field Worker), Subcontractors | Flutter mobile (offline-first) | Task execution, daily logs, photo progress, crew check-in, bilingual support |

---

## 2. Navigation Model

### 2.1 Primary Navigation (Left Sidebar)

The left sidebar persists across all web workspaces. It collapses to a hamburger on tablet/mobile.

```
┌─────────────────────┐
│ ⬡ FutureBuild OS    │  ← Logo + workspace switcher
├─────────────────────┤
│                     │
│ 📊 Portfolio        │  ← Workspace: Portfolio Dashboard
│   ├─ Financials     │
│   ├─ Projects       │
│   ├─ Fleet          │
│   └─ HR             │
│                     │
│ 🤖 Command Center   │  ← Workspace: Agent Command Center
│   ├─ Briefing       │
│   ├─ Schedule       │
│   ├─ Procurement    │
│   └─ Chat           │
│                     │
│ ⚡ Activity          │  ← Agent Activity Log (cross-workspace)
│                     │
├─────────────────────┤
│ ⚙ Settings          │  ← Org settings, Brain integration
│ 👤 Profile          │  ← User profile, preferences
└─────────────────────┘
```

### 2.2 Workspace Switcher

Users switch between Portfolio and Command Center via the top-level nav items. The sidebar context updates (sub-items change) but the shell structure remains constant.

### 2.3 Contextual Navigation

Within each workspace, secondary navigation uses:
- **Tabs:** For sub-views within a page (e.g., Financials: Summary | AR Aging | By Project)
- **Breadcrumbs:** For drill-down paths (Portfolio > Sunrise Villa > Phase 7 > Roofing)
- **Project selector:** Dropdown or card grid for project-level context switching

### 2.4 Field Portal Navigation (Flutter)

```
┌───────────────────────────────┐
│  [Tasks]  [Log]  [Photos]  [≡]│  ← Bottom tab bar
└───────────────────────────────┘
```

- **Tasks:** Feed card list with completion actions
- **Log:** Daily work log entry
- **Photos:** Photo progress capture with annotation
- **More:** Profile, settings, sync status

---

## 3. Screen Inventory

### 3.1 Portfolio Dashboard Workspace

| Screen | Route | Components | Data Source |
|--------|-------|------------|------------|
| **Financial Summary** | `/portfolio/financials` | `<fb-budget-summary>`, `<fb-ar-aging-chart>`, `<fb-project-financials-table>` | `GET /api/v1/org/{orgID}/financials/summary`, `/ar-aging`, `/projects` |
| **Project Grid** | `/portfolio/projects` | `<fb-project-card>[]` in responsive grid | `GET /api/v1/org/{orgID}/projects` |
| **Project Detail** | `/portfolio/projects/:id` | Tabbed view: Overview, Budget, Schedule, Team | `GET /api/v1/projects/:id` |
| **Fleet Management** | `/portfolio/fleet` | `<fb-fleet-grid>` with status badges, maintenance alerts | `GET /api/v1/org/{orgID}/fleet` |
| **HR Dashboard** | `/portfolio/hr` | `<fb-employee-table>` with cert expiration banner | `GET /api/v1/org/{orgID}/employees` |

### 3.2 Agent Command Center Workspace

| Screen | Route | Components | Data Source |
|--------|-------|------------|------------|
| **Morning Briefing** | `/command/briefing` | `<fb-feed-list>` with priority-sorted feed cards, weather card | `GET /api/v1/briefing/today` |
| **Schedule** | `/command/schedule` | `<fb-gantt-chart>`, CPM data panel, resource conflict list | `GET /api/v1/projects/:id/schedule/gantt` |
| **Procurement** | `/command/procurement` | `<fb-procurement-feed>`, Tribunal recommendation cards | `GET /api/v1/procurement/alerts` |
| **Chat** | `/command/chat` | 3-panel: project tree (left) + chat (center) + artifacts (right) | `POST /api/v1/chat/stream` (SSE) |
| **Chat Thread** | `/command/chat/:projectId/:threadId` | Message list + inline action cards + artifact panel | SSE stream |

### 3.3 Field Portal Workspace (Flutter)

| Screen | Route | Components | Data Source |
|--------|-------|------------|------------|
| **Task List** | `/field/tasks` | Task cards with completion slider, offline indicator | `GET /api/v1/field/tasks` + Drift cache |
| **Daily Log** | `/field/log` | Form: work description, crew count, hours, weather notes | `POST /api/v1/field/daily-log` |
| **Photo Progress** | `/field/photos` | Camera capture + annotation + WBS tag | `POST /api/v1/field/photos` |
| **Crew Check-in** | `/field/checkin` | Worker list with one-tap attendance buttons | `POST /api/v1/field/crew-checkin` |
| **Sync Status** | `/field/sync` | Outbox queue count, last sync time, connectivity indicator | Local Drift DB query |

### 3.4 Shared Screens

| Screen | Route | Components |
|--------|-------|------------|
| **Login** | `/login` | FB-Brain OIDC redirect (no local login form) |
| **Settings** | `/settings` | Org config, Brain integration status, notification preferences |
| **Profile** | `/profile` | User info, role display, notification settings |
| **Not Found** | `/404` | Empty state with navigation back |

---

## 4. Content Inventory

### 4.1 Data Objects

| Object | Source | Display Context | Typography |
|--------|--------|----------------|-----------|
| Project | PostgreSQL | Project cards, detail views | Title: Outfit; Budget/dates: JetBrains Mono |
| Task (WBS) | PostgreSQL | Gantt bars, task lists, feed cards | ID: JetBrains Mono; Name: Outfit |
| Budget | PostgreSQL (BIGINT cents) | Financial tables, stat cards | All amounts: JetBrains Mono |
| Invoice | PostgreSQL | AR aging, financial detail | Amounts: JetBrains Mono; Vendor: Outfit |
| Feed Card | River queue → API | Briefing list, procurement feed | Priority badge: Outfit; Data: JetBrains Mono |
| Weather Forecast | Tomorrow.io cache | Briefing weather card | Temps/wind: JetBrains Mono; Conditions: Outfit |
| Fleet Asset | PostgreSQL | Asset grid cards | ID/hours: JetBrains Mono; Name: Outfit |
| Employee | PostgreSQL | HR table rows | Name: Outfit; Cert dates: JetBrains Mono |
| Schedule (CPM) | Physics engine | Gantt chart, critical path | Dates/float: JetBrains Mono |

### 4.2 Actions

| Action | Trigger | Workspace | Permission |
|--------|---------|-----------|-----------|
| Approve procurement | Feed card button | Command Center | Owner, Admin |
| Dismiss alert | Feed card button | Command Center | All roles |
| Mark task complete | Slider + confirm | Field Portal | Field Worker |
| Submit daily log | Form submit | Field Portal | Field Worker |
| Recalculate schedule | Button or chat command | Command Center | Owner, Admin |
| View project detail | Card click → drill-down | Portfolio | All roles |
| Export PDF (AIA G702) | Button in financials | Portfolio | Owner, Admin |
| Trigger rollup | Button in financials | Portfolio | Admin |

### 4.3 System Messages

| Message Type | Display | Auto-dismiss |
|-------------|---------|--------------|
| Success | Green toast, glow flash | 5s |
| Error | Red toast, persists | Manual dismiss |
| Warning | Amber toast | 10s |
| Info | Blue toast | 8s |
| Agent action | Activity log entry | Persists in log |
| Offline queue | Amber status bar at top | Persists until online |

---

## 5. URL Structure

### 5.1 Web Routes

```
/                                    → Redirect to /portfolio/financials (default)
/login                               → FB-Brain OIDC redirect

# Portfolio Workspace
/portfolio                           → Redirect to /portfolio/financials
/portfolio/financials                → Financial summary dashboard
/portfolio/projects                  → Project grid
/portfolio/projects/:id              → Project detail (tabbed: overview, budget, schedule, team)
/portfolio/projects/:id/budget       → Project budget detail
/portfolio/projects/:id/schedule     → Project schedule (Gantt)
/portfolio/fleet                     → Fleet asset management
/portfolio/hr                        → HR & certifications

# Agent Command Center
/command                             → Redirect to /command/briefing
/command/briefing                    → Morning briefing feed
/command/schedule                    → Schedule view with project selector
/command/procurement                 → Procurement alerts + Tribunal cards
/command/chat                        → Chat interface (project tree + conversation)
/command/chat/:projectId             → Chat for specific project
/command/chat/:projectId/:threadId   → Specific thread

# Settings
/settings                            → Organization settings
/settings/brain                      → FB-Brain integration config
/settings/notifications              → Notification preferences
/profile                             → User profile
```

### 5.2 Flutter Routes

```
/field/tasks                         → Task list (offline-capable)
/field/log                           → Daily log entry
/field/photos                        → Photo progress capture
/field/checkin                       → Crew check-in
/field/sync                          → Sync status & outbox
/field/profile                       → User profile
```

### 5.3 API Routes (Backend)

```
# Organization
GET    /api/v1/org/{orgID}/financials/summary
GET    /api/v1/org/{orgID}/financials/ar-aging
GET    /api/v1/org/{orgID}/financials/projects
GET    /api/v1/org/{orgID}/projects
GET    /api/v1/org/{orgID}/fleet
GET    /api/v1/org/{orgID}/employees

# Project
GET    /api/v1/projects/{id}
GET    /api/v1/projects/{id}/schedule/gantt
POST   /api/v1/projects/{id}/schedule/recalculate

# Agent
GET    /api/v1/briefing/today
GET    /api/v1/procurement/alerts
POST   /api/v1/chat/stream

# Field
GET    /api/v1/field/tasks
POST   /api/v1/field/daily-log
POST   /api/v1/field/photos
POST   /api/v1/field/crew-checkin
GET    /api/v1/field/sync?since={timestamp}

# A2A
POST   /api/v1/a2a/webhook
GET    /api/v1/a2a/agent-card
```

---

## 6. Taxonomy & Naming Conventions

### 6.1 User-Facing Vocabulary

| Internal Term | User-Facing Term | Context |
|--------------|-----------------|---------|
| CorporateBudget | Financial Summary | Portfolio dashboard |
| ARAgingSnapshot | AR Aging | Financial charts |
| DailyFocusBriefing | Morning Briefing | Command Center |
| ProcurementAgent alert | Procurement Alert | Feed cards |
| TypeCorporateRollup | Financial Rollup | Settings/triggers |
| CPM ForwardPass | Schedule Recalculation | Command Center |
| WBS Code | Task ID | All contexts |
| FleetAsset | Equipment | Portfolio fleet |
| Tribunal recommendation | AI Recommendation | Feed cards |

### 6.2 Component Naming Convention

- **Tag prefix:** `fb-` (e.g., `fb-budget-summary`)
- **File naming:** PascalCase matching class (e.g., `FBBudgetSummary.ts`)
- **Event naming:** `fb-{noun}-{verb}` (e.g., `fb-task-completed`, `fb-card-dismissed`)
- **CSS class naming:** Kebab-case (e.g., `glass-card`, `hover-lift`, `data-mono`)

### 6.3 Status Vocabulary

| Status | Badge Color | Usage |
|--------|------------|-------|
| Active | Gable Green | Running projects, online assets |
| Warning | Amber | Approaching deadlines, cert expiring |
| Critical | Safety Red | Overdue, over-budget, equipment down |
| Complete | Gable Green (dimmed) | Finished tasks, paid invoices |
| Pending | Blueprint Blue | Awaiting approval, in queue |
| Offline | Gray (#5A5B66) | Disconnected devices, unavailable |

---

## 7. Information Hierarchy

### 7.1 Progressive Disclosure Pattern

```
Level 0: Workspace Selection
  └─ Portfolio | Command Center | Field Portal

Level 1: Dashboard View (aggregated KPIs)
  └─ 3-5 stat cards with summary metrics

Level 2: List/Grid View (all items of a type)
  └─ Sortable table or card grid

Level 3: Detail View (single item)
  └─ Tabbed detail with full data

Level 4: Action Modal (modify state)
  └─ Confirmation dialog or form
```

### 7.2 Data Density by Workspace

| Workspace | Density | Rationale |
|-----------|---------|-----------|
| Portfolio | **High** | Owner/Admin needs multi-project financial overview. Tables with 10+ columns, charts with 4+ data series. |
| Command Center | **Medium** | Superintendent needs prioritized feed. Cards with 3-5 key data points each. Chat has no density constraint. |
| Field Portal | **Low** | Field workers need one-tap actions. Cards with 1-2 data points. Large touch targets. Minimal text. |

---

## 8. Cross-Workspace Interactions

| Interaction | Source | Target | Mechanism |
|------------|--------|--------|-----------|
| "View project" from feed card | Command Center | Portfolio project detail | Router navigation |
| "Approve" procurement alert | Command Center | Triggers River job → PO creation | API call + toast confirmation |
| Task completion in field | Field Portal | Updates Command Center briefing | Drift outbox → API sync → Signals update |
| Schedule recalculation | Command Center (chat) | Updates Portfolio budget impact | CPM engine → API → Signals |
| A2A webhook from Brain | External | Creates feed card in Command Center | River queue → push notification |

---

## 9. Offline Architecture (Field Portal)

```
┌─────────────────────────────────┐
│         Flutter App              │
│                                  │
│  ┌──────────┐  ┌──────────────┐ │
│  │  UI Layer │  │ Drift Local  │ │
│  │  (Cards)  │──│ SQLite DB    │ │
│  └──────────┘  └──────┬───────┘ │
│                       │          │
│  ┌────────────────────▼───────┐ │
│  │     Outbox Table            │ │
│  │  id | action | payload     │ │
│  │  idempotency_key | status  │ │
│  └────────────────────┬───────┘ │
│                       │          │
│  ┌────────────────────▼───────┐ │
│  │  workmanager Background    │ │
│  │  + connectivity_plus       │ │
│  │  Drains outbox on connect  │ │
│  └────────────────────┬───────┘ │
│                       │          │
└───────────────────────┼──────────┘
                        │ HTTPS
┌───────────────────────▼──────────┐
│         FB-OS Server              │
│  Validates idempotency_key        │
│  Returns missed notifications     │
│  via /api/v1/field/sync           │
└───────────────────────────────────┘
```

- **Online indicator:** Green dot in header when connected; amber with pending count when offline
- **Queue visibility:** Sync Status screen shows outbox item count and last successful sync timestamp
- **Conflict resolution:** Server-wins strategy; client re-fetches after sync
