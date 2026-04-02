# Vision Brief: FutureBuild OS 2030-Ready Revamp

**Date:** 2026-04-02
**Pipeline Stage:** 00 - Vision Intake
**Status:** COMPLETE

---

## 1. Vision Statement

Revamp FutureBuild OS into a 2030-ready AI-Native Operating System for residential construction. This is NOT a greenfield build -- it is the evolution of a working, production-deployed system with 82 database migrations, 18 completed roadmap phases, and battle-tested physics engine code. The revamp extracts the deterministic CPM-res1.0 physics engine and SWIM weather model from the legacy Go codebase, builds an Industrial Dark dashboard using Lit/Vite for Organization-level management (Corporate Financials, HR, Fleet), and replaces manual admin bandwidth with autonomous agents for procurement and site management.

---

## 2. Legacy Foundation (Critical Context)

### 2.1 CPM Physics Engine Architecture

From `reference-vault/futurebuild-os/internal/physics/cpm.go`:

**DependencyGraph struct** -- The crown jewel. Uses `gonum.org/v1/gonum/graph/simple.DirectedGraph` to model construction task dependencies as a DAG. Key fields:
- `Graph *simple.DirectedGraph` -- gonum directed graph for topological operations
- `NodeMap map[uuid.UUID]int64` -- UUID-to-int64 mapping for gonum compatibility
- `TaskMap map[int64]uuid.UUID` -- reverse mapping
- `Tasks map[uuid.UUID]models.ProjectTask` -- task data lookup
- `Deps map[int64]map[int64]models.TaskDependency` -- edge metadata with O(1) lookup

**BuildDependencyGraph()** -- Constructs the DAG from ProjectTask and TaskDependency slices. Sequential int64 node IDs starting at 1. Skips invalid dependencies silently (from/to not in task set).

**TopologicalSort()** -- Wraps `topo.Sort()` from gonum, converting node IDs back to UUIDs.

**DetectCycle()** -- Uses topo.Unorderable error type to extract WBS codes of cyclic tasks for human-readable error messages.

**ForwardPass()** -- Calculates Early Start (ES) and Early Finish (EF). Handles all four dependency types (FS, SS, FF, SF) with lag days. Supports material constraints as "Start No Earlier Than" dates (MRP feedback loop). Uses `Calendar.AddWorkDuration()` for deterministic integer math.

**BackwardPass()** -- Calculates Late Start (LS), Late Finish (LF), Total Float, and critical path. Uses configurable `CriticalPathThreshold` (default 0.001) for float comparison. Returns critical path as WBS codes in topological order.

**Calendar Interface** -- Two methods: deprecated `AddWorkingDays(float64)` and deterministic `AddWorkDuration(time.Duration)`. The P1 Correctness Fix eliminated IEEE 754 floating-point drift by using integer nanosecond math and truncating to minute precision.

**StandardCalendar** -- Configurable work week (defaults Mon-Fri), holiday support (month/day comparison ignoring year). `isNonWorkingDay()` skips weekends and holidays.

### 2.2 DHSM Duration Calculator

From `reference-vault/futurebuild-os/internal/physics/dhsm.go`:

**CalculateSAF()** -- Size Adjustment Factor: `(GSF / StandardHouseSizeSF) ^ SizeAdjustmentExponent`. Config-decoupled via `PhysicsConfig`. Inspections are locked to SAF=1.0.

**CalculateTaskDurationV2()** -- The deterministic variant using int64 nanoseconds. Flow:
1. Convert base duration days to nanoseconds
2. Apply SAF as scaled integer (SAFScaleFactor=1000, 3 decimal places)
3. Apply multipliers (linear, scaled, step formulas) via scaled integer math
4. Apply SWIM weather overlay
5. Quantize to 0.5-day (HalfDay=4 hours) increments via ceiling

**Key constants:** `WorkDayPrecision = 30 * time.Minute`, `HalfDay = 4 * time.Hour`, `FullDay = 8 * time.Hour`, `SAFScaleFactor = 1000`.

**Context variables:** `supply_chain_volatility`, `rough_inspection_latency`, `final_inspection_latency` -- extracted from `ProjectContext`.

### 2.3 SWIM Weather Model

From `reference-vault/futurebuild-os/internal/physics/swim.go`:

**ApplyWeatherAdjustment()** -- Simple multiplier-based model:
- Precipitation > 10mm: multiply by 1.15
- Low temp < 0C: multiply by 1.25 (frozen ground)
- High temp > 35C: multiply by 1.10 (heat restrictions)

**isWeatherSensitive()** -- WBS scope rule: WBS major < 10.0 (pre-dry-in) OR WBS 13.x (exterior finishes). Internal work bypasses weather adjustments.

### 2.4 Project Scoping Engine

From `reference-vault/futurebuild-os/internal/physics/scoping.go`:

**ApplyScope()** -- Deterministic (no AI, no randomness) adaptation of the WBS template based on:
- Foundation type (slab removes waterproofing; basement adds drain tile, damp proofing, egress)
- Story count (1-story removes second floor framing; 3+ adds engineered floor system)
- GSF (>4000 adds extended site prep)
- Topography (hillside adds retaining wall, extends foundation 40%)
- In-progress projects (marks completed tasks, supports phase wildcards like "8.x")

### 2.5 Autonomous Agents

From `reference-vault/futurebuild-os/internal/agents/`:

**DailyFocusAgent** -- Morning briefing generation. Streams active projects with worker pool (MaxConcurrentProjects=10). Geocodes project address for weather. Fetches schedule focus tasks. Generates briefing via Claude (primary) or Gemini (fallback). Writes feed cards. Detects upcoming inspections within 10 business days. Builder pattern for optional dependencies.

**ProcurementAgent** -- Long-lead item monitoring. Distributed mutex (Redis) prevents duplicate execution in Blue/Green deployments. Streaming iteration with batch flush (DefaultBatchSize=100). Calculates order dates: `NeedByDate = EarlyStart - stagingBuffer`, `MustOrderDate = NeedByDate - leadTime - weatherBuffer`. 72-hour notification dampening. Config-decoupled via ProcurementConfig.

**SubLiaisonAgent** -- Subcontractor coordination. Transactional Outbox pattern for at-most-once SMS delivery. Context binding (48h window) for inbound message parsing. Regex-based percentage extraction and delay indicator detection. Phase-to-trade name mapping.

### 2.6 FutureShade/Tribunal Intelligence Layer

From `reference-vault/futurebuild-os/internal/futureshade/tribunal/`:

**ConsensusEngine** -- Multi-model decision process:
- The Architect (Claude Opus) votes YEA/NAY/ABSTAIN
- The Historian (Gemini Code Assist) votes independently
- The Coordinator (Gemini Flash) synthesizes with consensus score
- Fail-closed default: JSON parse failure defaults to REJECTED
- Includes self-healing Diagnose() method for runtime errors

### 2.7 Asynq Worker/Cron Patterns

From `reference-vault/futurebuild-os/internal/worker/`:

**16 task types** including: daily_briefing, procurement_check, hydrate_project, drift_detection, delay_cascade, resource_conflict_scan, corporate_rollup, certification_alerts, maintenance_reminders, voice_transcription, a2a_webhook_dispatch.

**Cron schedules:** 05:00 UTC procurement, 06:00 UTC daily briefing, 07:00 UTC drift detection, 23:00 UTC expire actions.

**Patterns:** Typed payloads with JSON marshaling, asynq.Queue routing, MaxRetry/Timeout config, idempotency checks, circuit breakers, 72h notification dampening.

### 2.8 GableLBM Industrial Dark Design System

From `reference-vault/futurebuild-os/specs/GABLE_LBM_DESIGN_SYSTEM.md`:

**Color tokens (HSL):**
- Background: `230 20% 5%` (#0A0B10) -- Deep Space
- Primary: `158 100% 50%` (#00FFA3) -- Gable Green
- Secondary: `217 33% 17%` (#161821) -- Slate Steel
- Tertiary: `199 95% 60%` (#38BDF8) -- Blueprint Blue
- Error: `346 87% 60%` (#F43F5E) -- Safety Red

**Glassmorphism:** `rgba(22, 24, 33, 0.6)` background, 24px backdrop-filter blur, 1px solid rgba(255,255,255,0.05) border.

**Typography:** Outfit (sans-serif), JetBrains Mono (data/numerical).

**Radii:** 0.75rem standard, 1.5rem large.

### 2.9 Service Layer & Domain Models

From `reference-vault/futurebuild-os/internal/service/interfaces.go`:

**18 service interfaces** including: ProjectServicer, ScheduleServicer, InvoiceServicer, DocumentServicer, BudgetServicer, CorporateFinancialsServicer, EmployeeServicer, FleetServicer, A2AServicer, CalibrationServicer, ResourceConflictServicer, DelayCascadeServicer.

**Corporate models** already exist: CorporateBudget (int64 cents), GLSyncLog, ARAgingSnapshot -- proving the Organization-level financial layer has foundation.

### 2.10 Cross-System Context

From `reference-vault/FB-Brain/CLAUDE.md`:

FB-Brain is an upstream orchestration platform that sends webhooks to FB-OS. Separate Go + PostgreSQL system with MaterialsFlow and LaborFlow orchestrators. FB-OS receives JWTs issued by FB-Brain for authentication. A2A protocol bridges the two systems.

---

## 3. What Must Be Preserved vs. Evolved for 2030

### PRESERVE (Non-Negotiable)
1. **DependencyGraph + gonum DAG** -- The graph topology is correct and battle-tested
2. **Deterministic integer math** -- The P1 Correctness Fix (AddWorkDuration, SAFScaleFactor) eliminates IEEE 754 drift
3. **Four dependency types** (FS, SS, FF, SF) with lag days
4. **Calendar interface** with configurable work weeks and holidays
5. **Material constraints** as hard CPM inputs (MRP feedback loop)
6. **DHSM quantization** to 0.5-day increments
7. **Scoping engine** deterministic rules (no AI in scheduling)
8. **Fail-loudly approach** -- ErrInvalidTaskDuration halts CPM, not silently defaults
9. **Database as source of truth** -- agents are stateless calculators
10. **Monetary precision** -- int64 cents, no floating-point currency

### EVOLVE (2030 Targets)
1. **SWIM weather model** -- From simple multipliers to predictive model with Tomorrow.io API, hourly granularity, and confidence intervals
2. **Resource leveling** -- Add resource constraints to CPM (crew availability, equipment conflicts)
3. **Multi-project scheduling** -- Cross-project resource conflict detection (already has TypeResourceConflictScan)
4. **Autonomous agents** -- Evolve to A2A-compatible webhook-driven architecture per Google's Agent2Agent protocol
5. **Dashboard** -- From chat-first 3-panel to Organization-root Industrial Dark dashboard for corporate functions
6. **Corporate modules** -- Expand CorporateFinancialsServicer, EmployeeServicer, FleetServicer into full modules
7. **Tribunal** -- Evolve from consensus voting to decision engine for autonomous procurement execution
8. **Mobile** -- Flutter offline-first field app with background sync
9. **Auth** -- Centralized JWT delegation to FB-Brain (already in TECH_STACK.md)

---

## 4. Target Outcomes

| Outcome | Metric | Timeline |
|---------|--------|----------|
| Replace manual admin bandwidth | 60% reduction in back-office hours | 12 months |
| Autonomous procurement | 80% of orders placed without human intervention | 18 months |
| Multi-project visibility | Single dashboard for 50+ concurrent projects | 9 months |
| Weather accuracy | SWIM v2 predicts delays 5+ days ahead with >75% accuracy | 12 months |
| Field adoption | 90% daily log completion rate via Flutter app | 15 months |
| Schedule determinism | Zero floating-point drift across architectures | Preserved from legacy |

---

## 5. Technical Stack (Confirmed)

From `.agents/TECH_STACK.md`:

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.24, chi router, pgx/v5 (raw SQL) |
| Frontend | Vite + Lit (Web Components), TypeScript strict |
| Mobile | Flutter (field surfaces only, offline-first) |
| Database | PostgreSQL 16+ with pgvector |
| Queue | Redis 7 via Asynq |
| Auth | Centralized JWT delegation to FB-Brain |
| CI/CD | GitHub Actions, Docker, Railway |
| Observability | OpenTelemetry, Prometheus, Sentry |

**Hard Constraints:** NO React, NO ORMs, NO Python logic, ALL monetary values as BIGINT cents, JetBrains Mono for numerical data.
