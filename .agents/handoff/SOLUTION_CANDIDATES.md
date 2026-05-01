# Solution Candidates

**Date:** 2026-04-02
**Pipeline Stage:** 03 - Solution Design
**Status:** COMPLETE

---

## Concept 1: CPM-res1.0 Enhancement -- Resource Leveling + Multi-Project

### Problem
The legacy DependencyGraph (reference-vault/futurebuild-os/internal/physics/cpm.go) assumes unlimited resources. In practice, builders share crews, equipment, and inspectors across projects, creating invisible conflicts.

### Solution
Preserve the gonum DependencyGraph core. Add a resource leveling layer AFTER the standard CPM pass:

1. **Standard CPM Pass** (preserve): ForwardPass -> BackwardPass -> Critical Path
2. **Resource Leveling Pass** (new): Iterate non-critical tasks by float (lowest first). If a task's resource requirements conflict with another task on the same date, push the lower-priority task forward within its float window.
3. **Multi-Project Arbitration** (new): Create a cross-project resource pool. The existing TypeResourceConflictScan cron task becomes the trigger. Pool tracks crew types, equipment assets (from FleetServicer), and inspector availability.

### Architecture
```
ResourcePool {
    CrewAvailability map[string][]DateRange  // crewType -> available windows
    EquipmentPool    map[uuid.UUID][]DateRange // assetID -> allocated windows
    InspectorSlots   map[string]int           // inspectorType -> slots/day
}

func ResourceLeveledSchedule(
    g *DependencyGraph,
    schedule map[uuid.UUID]TaskSchedule,
    pool *ResourcePool,
    cal Calendar,
) (map[uuid.UUID]TaskSchedule, []ResourceConflict)
```

### Trade-offs
- (+) Preserves deterministic CPM core unchanged
- (+) Resource conflicts surfaced as warnings, not hard constraints (builder can override)
- (-) Increases scheduling complexity; O(n^2) worst case for conflict resolution
- (-) Multi-project requires cross-org data access patterns

### Recommendation: BUILD. The gonum foundation is solid. Resource leveling is the #1 feature gap vs. Primavera P6.

---

## Concept 2: SWIM v2 -- Predictive Weather Model with Tomorrow.io

### Problem
The legacy SWIM model (reference-vault/futurebuild-os/internal/physics/swim.go) uses three static multipliers: precipitation >10mm (1.15x), cold <0C (1.25x), heat >35C (1.10x). No temporal granularity, no forecast confidence, no regional calibration.

### Solution
Replace static multipliers with a predictive model consuming Tomorrow.io Weather API:

1. **Hourly Forecast Ingestion**: Fetch 14-day hourly forecasts for each project's geocoded location
2. **Task-Level Weather Risk**: For each weather-sensitive task (WBS < 10.0 OR 13.x), calculate delay probability based on hourly weather windows during the task's planned date range
3. **Confidence-Weighted Duration**: `AdjustedDuration = BaseDuration * (1 + RiskScore * WeatherImpactWeight)`
4. **Historical Calibration**: Compare predicted delays against actual delays from completed projects (feeds into Org_Bias[Phase] per CPM_RES_MODEL_SPEC.md Section 13.2)

### Architecture
```go
type SWIMv2Config struct {
    APIKey            string
    ForecastHorizonDays int    // default 14
    MinPrecipMM       float64 // threshold for delay risk
    MinTempC          float64 // cold threshold
    MaxTempC          float64 // heat threshold
    CalibrationWeight float64 // blend of prediction vs historical
}

func CalculateWeatherRisk(
    forecast []HourlyForecast,
    taskStart, taskEnd time.Time,
    cfg SWIMv2Config,
) WeatherRiskScore
```

### Trade-offs
- (+) 5-14 day predictive capability vs. current day-of-only
- (+) Regional calibration via project geocoding
- (-) API dependency (Tomorrow.io) introduces a failure point; need fallback to legacy multipliers
- (-) Historical calibration requires 10+ completed projects per builder

### Recommendation: BUILD iteratively. Phase 1: Tomorrow.io integration with legacy multiplier fallback. Phase 2: Historical calibration after sufficient project completions.

---

## Concept 3: Industrial Dark Dashboard -- Lit Web Components

### Problem
The legacy frontend is a chat-first 3-panel layout (reference-vault/futurebuild-os/specs/FRONTEND_SCOPE.md). Company owners and admins need data-dense dashboards for financial oversight, not conversational interfaces.

### Solution
Build Organization-root dashboard views using Lit Web Components with GableLBM Industrial Dark tokens:

1. **Corporate Financials View**: CorporateBudget rollup, AR aging, GL sync status. Portfolio-level P&L.
2. **HR/Workforce View**: Employee roster, certification tracking, time log approval queue, labor burden by project.
3. **Fleet Assets View**: Equipment inventory, allocation calendar, maintenance schedule, utilization metrics.
4. **Project Portfolio View**: Card grid of all projects with health indicators (schedule, budget, procurement).

### Component Architecture
```
<fb-org-shell>                    // Organization-level shell
├── <fb-org-nav>                  // Left nav: Financials, HR, Fleet, Projects
├── <fb-financials-view>
│   ├── <fb-budget-summary>       // Glassmorphism summary cards
│   ├── <fb-ar-aging-chart>       // D3.js stacked bar chart
│   ├── <fb-project-financials-table> // Data grid with JetBrains Mono
│   └── <fb-gl-sync-status>       // Last sync timestamp + status
├── <fb-hr-view>
│   ├── <fb-employee-roster>
│   ├── <fb-cert-tracker>
│   └── <fb-time-approval-queue>
├── <fb-fleet-view>
│   ├── <fb-asset-inventory>
│   ├── <fb-allocation-calendar>
│   └── <fb-maintenance-schedule>
└── <fb-portfolio-view>
    └── <fb-project-card>[]       // Grid of project health cards
```

### Design System
- Base: GableLBM tokens from GABLE_LBM_DESIGN_SYSTEM.md
- Background: #0A0B10 (Deep Space)
- Accent: #00FFA3 (Gable Green)
- Data font: JetBrains Mono
- Cards: Glassmorphism (60% opacity, 24px blur)
- Currency: Right-aligned, comma-separated, JetBrains Mono, green/red variance coloring

### Trade-offs
- (+) Addresses Sarah and Tom's JTBD directly
- (+) Lit Web Components with Shadow DOM provide style isolation
- (+) GableLBM tokens already defined, reducing design decisions
- (-) Complex data tables in Lit require careful virtualization for performance
- (-) Two UI paradigms (chat for project, dashboard for org) increases frontend complexity

### Recommendation: BUILD. The chat-first paradigm serves Mike (superintendent) well but fails Sarah (admin) and Tom (owner). Organization-level dashboard is the missing piece.

---

## Concept 4: Autonomous Agents -- A2A-Compatible Webhook-Driven Architecture

### Problem
The legacy agents (DailyFocus, Procurement, SubLiaison) are cron-triggered Go services. They cannot communicate with external systems (The Brain, supplier APIs) in a standardized way.

### Solution
Evolve agents to Google A2A protocol compatibility:

1. **Agent Card**: Each agent publishes an Agent Card (JSON manifest) describing its capabilities, skills, and authentication requirements
2. **Task Protocol**: Agents communicate via A2A tasks (JSON-RPC over HTTP) with states: submitted, working, input-required, completed, failed, canceled
3. **Webhook Push**: Long-running tasks push status updates to registered webhooks (replaces polling)
4. **Skill Registry**: Each agent registers skills that external agents (The Brain) can invoke

### Architecture
```go
type AgentCard struct {
    Name         string   `json:"name"`
    Description  string   `json:"description"`
    URL          string   `json:"url"`
    Skills       []Skill  `json:"skills"`
    AuthSchemes  []string `json:"auth_schemes"`
}

type Skill struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Description string   `json:"description"`
    InputSchema  json.RawMessage `json:"input_schema"`
    OutputSchema json.RawMessage `json:"output_schema"`
}
```

### Agent Evolution
- **DailyFocusAgent** -> A2A skill: "generate_briefing" (invokable by The Brain for on-demand briefings)
- **ProcurementAgent** -> A2A skill: "analyze_procurement", "place_order" (invokable by The Brain with supplier context)
- **SubLiaisonAgent** -> A2A skill: "confirm_sub", "parse_inbound" (invokable by SMS/email gateway)
- **New: ScheduleAgent** -> A2A skill: "recalculate_schedule", "what_if_analysis"

### Trade-offs
- (+) Standard protocol (Linux Foundation governance) for cross-system communication
- (+) The Brain can invoke OS agents via A2A instead of custom webhooks
- (+) Enables future marketplace of third-party construction agents
- (-) A2A v0.3 is still evolving; may require migration effort
- (-) Adds HTTP endpoint overhead to existing cron-based agents

### Recommendation: BUILD incrementally. Start with Agent Card publication and The Brain integration. Keep internal cron triggers alongside A2A endpoints.

---

## Concept 5: Corporate Modules (Financials, HR, Fleet)

### Problem
The legacy system has service interfaces for corporate functions (CorporateFinancialsServicer, EmployeeServicer, FleetServicer) but no dedicated UI or complete business logic.

### Solution
Build three Organization-level modules:

**5A. Corporate Financials**
- Budget rollup (daily TypeCorporateRollup cron)
- AR aging dashboard (ARAgingSnapshot model)
- AIA billing automation (G702/G703 PDF generation)
- GL sync for external accounting (QuickBooks, Sage)
- Job costing drill-down (Organization > Project > WBS Phase > Task)

**5B. HR / Workforce**
- Employee management (already have EmployeeServicer)
- Time tracking with geolocation (already have LogTime, ApproveTimeLog)
- Certification tracking with 30/14/7-day alerts (TypeCertificationAlerts)
- Labor burden calculation per project (already have CalculateLaborBurden)
- Payroll integration via API (Hammr or similar, NOT built in-house)

**5C. Fleet / Equipment**
- Asset inventory with type/status/condition (already have FleetServicer)
- Project allocation with availability checking (CheckEquipmentAvailability)
- Maintenance scheduling with reminders (TypeMaintenanceReminders)
- Equipment constraint validation in CPM (equipment_validator.go for WBS 7.x)
- Telematics integration via Samsara API (GPS, usage hours)

### Trade-offs
- (+) Service interfaces already defined, reducing design work
- (+) Cron tasks already exist for rollup, certification, maintenance
- (+) Directly addresses Tom (owner) and Sarah (admin) personas
- (-) Significant frontend work for three full modules
- (-) Payroll complexity makes in-house build risky (regulatory compliance)

### Recommendation: BUILD Financials first (highest impact), then Fleet (equipment-schedule linkage), then HR (integrate, do not build payroll).

---

## Concept 6: AI-Native -- Tribunal as Decision Engine for Autonomous Procurement

### Problem
The current Tribunal (reference-vault/futurebuild-os/internal/futureshade/tribunal/engine.go) handles ad-hoc decisions with YEA/NAY/ABSTAIN voting. It is not wired into the procurement execution flow.

### Solution
Evolve the Tribunal into the decision authority for autonomous operations:

1. **Procurement Auto-Ordering**: When ProcurementAgent detects a CRITICAL item, Tribunal evaluates: budget available? preferred vendor? historical price within range? If ConsensusScore > 0.8 AND amount < owner's auto-approve threshold -> auto-generate PO.
2. **Schedule Override Governance**: When a sub reports a delay, Tribunal evaluates cascade impact and recommends: absorb in float, reassign crew, or escalate to owner.
3. **Budget Variance Alerts**: When committed spend exceeds estimated by >10%, Tribunal evaluates root cause and recommends: re-estimate, find alternative vendor, or flag for owner review.

### Architecture
```go
type ProcurementDecisionContext struct {
    Item           ProcurementItem
    BudgetRemaining int64
    PreferredVendor *Vendor
    HistoricalPrice PriceRange
    OwnerThreshold  int64 // auto-approve below this amount
}

func (e *ConsensusEngine) ReviewProcurement(ctx context.Context, pdc ProcurementDecisionContext) (*TribunalResponse, error) {
    // Architect evaluates risk, Historian evaluates precedent, Coordinator synthesizes
}
```

### Autonomy Levels
| Level | Description | Tribunal Role | Example |
|-------|------------|--------------|---------|
| 0 | Fully manual | Not involved | Owner places all orders |
| 1 | Recommend | Suggests action, human decides | "I recommend ordering windows from VendorX" |
| 2 | Auto with approval | Executes if human approves in 24h | PO generated, Tom approves |
| 3 | Fully autonomous | Executes within guardrails | Orders under $500 auto-approved |

### Trade-offs
- (+) Differentiating feature -- no competitor has AI-governed procurement
- (+) Tribunal architecture already supports multi-model consensus
- (+) Graduated autonomy levels manage risk
- (-) Legal/liability questions around autonomous purchasing
- (-) Requires trust-building with builders (start at Level 1, earn Level 3)

### Recommendation: BUILD with Level 1 (recommend) as default. Level 2 opt-in after 30 days of accurate recommendations. Level 3 only for items under configurable threshold.
