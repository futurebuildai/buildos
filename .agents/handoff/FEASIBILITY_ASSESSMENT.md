# Feasibility Assessment

**Date:** 2026-04-02
**Pipeline Stage:** 03 - Solution Design
**Status:** COMPLETE

---

## Assessment Matrix

| Concept | Technical Feasibility | Business Viability | Risk Level | Timeline Estimate | Verdict |
|---------|----------------------|-------------------|------------|-------------------|---------|
| C1: CPM Resource Leveling | HIGH -- gonum foundation solid | HIGH -- #1 gap vs competitors | LOW | 6-8 weeks | GO |
| C2: SWIM v2 (Tomorrow.io) | HIGH -- REST API integration | HIGH -- unique differentiator | MEDIUM (API dependency) | 8-12 weeks | GO |
| C3: Industrial Dark Dashboard | HIGH -- Lit/Vite proven | HIGH -- addresses admin/owner gap | LOW | 12-16 weeks | GO |
| C4: A2A Agent Protocol | MEDIUM -- protocol v0.3, evolving | MEDIUM -- future-proofing | MEDIUM | 6-8 weeks | GO (incremental) |
| C5: Corporate Modules | HIGH -- service interfaces exist | HIGH -- revenue-enabling | LOW-MEDIUM | 16-20 weeks (3 modules) | GO (phased) |
| C6: Tribunal Procurement | MEDIUM -- AI decision accuracy | HIGH -- key differentiator | HIGH (liability) | 8-12 weeks | GO (graduated) |

---

## Detailed Technical Feasibility

### C1: CPM Resource Leveling

**Existing Foundation:**
- DependencyGraph, ForwardPass, BackwardPass are production-tested
- EquipmentValidator already checks WBS 7.x equipment constraints
- FleetServicer.CheckEquipmentAvailability provides availability data
- TypeResourceConflictScan cron task exists

**Technical Challenges:**
1. Resource leveling is NP-hard in general; need heuristic approach (priority-based, not optimal)
2. Multi-project scheduling requires cross-org data access (security implications)
3. Must not break determinism guarantee -- resource conflicts as warnings, not CPM inputs

**Prototype Validation:** Implement resource leveling for a single resource type (equipment) on a single project. Measure performance with 100-task graph.

**Verdict: FEASIBLE.** The hard parts (graph topology, CPM passes) are done. Resource leveling is additive.

---

### C2: SWIM v2 (Tomorrow.io)

**Existing Foundation:**
- SWIM model with isWeatherSensitive() and WBS code parsing
- WeatherServicer interface defined
- ProcurementAgent already uses weather forecasts for buffer calculation
- DailyFocusAgent fetches geocoded weather

**Technical Challenges:**
1. Tomorrow.io API rate limits (500 calls/day free tier) -- need caching strategy
2. Historical weather data for calibration requires storage (new table or extend project_context)
3. Confidence intervals add complexity to DHSM calculation
4. Must maintain fallback to legacy static multipliers when API unavailable

**Prototype Validation:** Integrate Tomorrow.io for one project. Compare predicted vs actual delays over 4-week period.

**Verdict: FEASIBLE.** API integration is straightforward. Calibration accuracy improves with data volume.

---

### C3: Industrial Dark Dashboard

**Existing Foundation:**
- GableLBM design tokens fully specified (colors, typography, glassmorphism)
- Lit 3.0 with Shadow DOM is the established frontend framework
- Vite build system configured
- FBElement base class with shared styles and emit() helper
- CorporateBudget, ARAgingSnapshot, GLSyncLog models exist
- Signals-based reactive state (@lit-labs/preact-signals)

**Technical Challenges:**
1. Data-dense tables (1000+ rows) require virtual scrolling in Lit
2. D3.js integration with Shadow DOM needs careful coordination
3. Two UI paradigms (chat + dashboard) increases routing complexity
4. Financial data requires real-time updates (SSE or polling)

**Prototype Validation:** Build fb-budget-summary card with mock data. Validate glassmorphism rendering, JetBrains Mono alignment, and dark mode contrast ratios.

**Verdict: FEASIBLE.** No technical blockers. Primary risk is development velocity for three full dashboard views.

---

### C4: A2A Agent Protocol

**Existing Foundation:**
- pkg/a2a/ package already exists
- TypeA2AWebhookDispatch task type defined
- A2AServicer interface with LogExecution, GetExecutionLogs, GetActiveAgents, PauseAgent, ResumeAgent
- FB-Brain already communicates via webhooks

**Technical Challenges:**
1. A2A v0.3 is pre-1.0 -- breaking changes possible
2. gRPC support in A2A v0.3 adds complexity if adopted
3. Agent Card discovery requires new endpoint (/a2a/agent-card)
4. Must maintain backward compatibility with existing cron triggers

**Prototype Validation:** Publish Agent Card for ProcurementAgent. Test FB-Brain invoking "analyze_procurement" skill via A2A task endpoint.

**Offline Field Sync (L8 Requirement):**
- Brain→OS A2A webhooks are server-to-server and always online — no delivery gap
- The gap is OS→Field device notifications and Flutter→OS API calls made while the device is offline on a job site
- **Client-side solution:** Drift-backed outbox table with `workmanager` background sync. Uses `connectivity_plus` for network state detection. Actions queue locally and drain with exponential backoff on reconnect. Server enforces idempotency via UUID v7 keys.
- **Server-side solution:** River retry queue (`TypeFieldNotificationRetry`) with exponential backoff (30s → 1hr, 6 retries). Dead letter queue (`field_notification_dlq`) after exhaustion. Pull-based sync endpoint (`/api/v1/field/sync?since=<timestamp>`) for devices that miss push notifications.
- **Deterministic guarantee:** Construction triggers (e.g., "Inspection Task Ready") are NEVER lost — they persist in River until acknowledged or DLQ'd for manual review.

**Verdict: FEASIBLE but requires monitoring of protocol evolution.** Start with HTTP/JSON-RPC only, defer gRPC.

---

### C5: Corporate Modules

**Existing Foundation:**
- CorporateFinancialsServicer: RollupCorporateBudget, GetCorporateBudget, CalculateARAging, CreateGLSyncLog
- EmployeeServicer: CRUD, LogTime, ApproveTimeLog, CalculateLaborBurden, AddCertification, GetExpiringCertifications
- FleetServicer: CreateFleetAsset, AllocateEquipment, CheckEquipmentAvailability, LogMaintenance, GetUpcomingMaintenance
- Cron tasks: TypeCorporateRollup, TypeCertificationAlerts, TypeMaintenanceReminders

**Technical Challenges:**
1. AIA billing (G702/G703) PDF generation requires template engine (Go template + wkhtmltopdf or Chrome headless)
2. QuickBooks integration for GL sync requires OAuth2 flow
3. Payroll compliance (certified payroll, prevailing wage) is regulatory minefield
4. Telematics API integration (Samsara) requires vendor partnership

**Prototype Validation:** Build Financials dashboard with mock data. Generate one AIA G702 PDF from real project budget data.

**Verdict: FEASIBLE for Financials and Fleet. HR should INTEGRATE (payroll) not BUILD.**

---

### C6: Tribunal Procurement

**Existing Foundation:**
- ConsensusEngine with Architect (Claude) + Historian (Gemini) + Coordinator (Gemini Flash)
- Fail-closed default (JSON parse failure -> REJECTED)
- Decision persistence with votes and consensus scores
- ProcurementAgent already calculates MustOrderDate and detects CRITICAL items

**Technical Challenges:**
1. AI decision accuracy for financial commitments must be very high (>95%)
2. Legal liability for autonomous purchasing needs legal review
3. Vendor API integration for price checking and PO generation
4. Audit trail must be immutable and legally defensible
5. Builder trust must be earned gradually (Level 1 -> Level 2 -> Level 3)

**Prototype Validation:** Run Tribunal on 50 historical procurement decisions. Measure consensus accuracy against what humans actually decided.

**Verdict: FEASIBLE at Level 1 (recommend). Level 2+ requires extended validation period.**

---

## Infrastructure Feasibility

| Component | Current State | 2030 Target | Migration Effort |
|-----------|-------------|-------------|------------------|
| Go 1.24 backend | Production | Continue | None |
| PostgreSQL 16 + pgvector | Production | Continue | Minimal |
| Redis / Asynq | Production | Migrate to River (PostgreSQL-native) | 2-3 sprints |
| Lit 3.0 frontend | Production | Lit 4.0 | Minor upgrade |
| Flutter mobile | Planned | Offline-first field app | New development |
| Clerk auth | Production | Remove, delegate to FB-Brain JWT | 1-2 sprints |
| Docker / Railway | Production | Continue | None |
| Tomorrow.io API | Not integrated | SWIM v2 source | New integration |
| Samsara API | Not integrated | Fleet telematics | New integration |
