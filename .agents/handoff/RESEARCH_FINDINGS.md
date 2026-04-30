# Research Findings

**Date:** 2026-04-02
**Pipeline Stage:** 01 - Deep Research
**Status:** COMPLETE

---

## Legacy Vault Analysis

### Physics Engine Deep Dive

The FutureBuild physics engine (cpm.go, dhsm.go, swim.go, scoping.go, equipment_validator.go) represents a rare implementation of a deterministic CPM scheduler in Go, using gonum's graph library for DAG operations. This is production-grade code with explicit engineering decisions documented in comments.

**Architecture Strengths:**
1. **gonum integration** -- Using `simple.DirectedGraph` with `topo.Sort()` gives battle-tested graph algorithms (topological sort, cycle detection) without reimplementation. The UUID-to-int64 mapping pattern (`NodeMap`/`TaskMap`) is a clean adapter between domain IDs and gonum's int64 node requirement.
2. **Determinism obsession** -- The codebase went through a P1 Correctness Fix cycle that replaced float64 arithmetic with int64 nanosecond math. `AddWorkDuration()` uses `time.Duration` throughout and truncates to minute precision for cross-architecture reproducibility.
3. **Four dependency types** -- Full FS/SS/FF/SF support with separate forward (`calculateConstraintDate`) and backward (`calculateBackwardConstraintDate`) constraint functions. This matches professional PM tools like Primavera P6.
4. **Material constraints** -- The `materialConstraints map[uuid.UUID]time.Time` parameter in ForwardPass() implements a hard "Start No Earlier Than" constraint, enabling the MRP feedback loop from the ProcurementAgent.

**Architecture Gaps for 2030:**
1. **No resource leveling** -- The CPM solver assumes unlimited resources. Adding crew/equipment constraints requires extending ForwardPass with resource availability checks.
2. **Single-project scope** -- Each CPM run operates on one project's tasks. Multi-project scheduling needs a cross-project DependencyGraph or resource pool arbitrator.
3. **SWIM model is simplistic** -- Three static multipliers (precipitation, cold, heat) without temporal granularity, confidence intervals, or hourly forecasting. The `isWeatherSensitive()` function uses string-based WBS code parsing that could break with non-standard codes.
4. **No Monte Carlo simulation** -- No probabilistic schedule analysis for risk assessment. Duration ranges (optimistic/pessimistic/most likely) are not modeled.

### Agent Architecture Analysis

The three agents (DailyFocus, Procurement, SubLiaison) follow a consistent pattern:
- **Dependency injection** via constructor + builder pattern (WithFeedWriter, WithClaudeRunner, WithDistributedMutex)
- **Clock injection** for deterministic time simulation (pkg/clock.Clock interface)
- **Repository abstraction** (ProcurementRepository, ProjectRepository) for testability
- **Dual AI path** -- Claude primary with Gemini fallback
- **Feed card generation** for the portfolio view

The ProcurementAgent is the most production-hardened, with distributed mutex (Redis), batch processing, heartbeat renewal, and batch error tracking. This pattern should be the template for new agents.

### Design System Analysis

The GableLBM tokens are currently CSS custom properties mapped to Material Design token names (--md-sys-color-*). The glassmorphism approach (60% opacity Slate Steel, 24px blur) creates a distinctive dark aesthetic. Typography choices (Outfit + JetBrains Mono) are excellent for construction data dashboards.

Gap: The legacy design system spec is a static document. It needs to become a living component library with Lit Web Components that enforce the token system at the component level.

### Worker Pattern Analysis

The 16 Asynq task types show organic growth from 5 initial types (briefing, procurement, hydrate, skill execution, PR review) to 16 types spanning ERP, fleet, voice, and A2A domains. The `payloads.go` file uses typed structs with JSON marshaling -- a clean pattern.

Concern: Asynq's maintenance activity has declined. River (PostgreSQL-native job queue) eliminates the Redis dependency and provides transactional job enqueuing that aligns with the "database is source of truth" philosophy.

---

## Web Research Findings

### 1. CPM Software Implementations

**Finding:** No widely-adopted open-source CPM scheduler exists in Go beyond FutureBuild's implementation. Most CPM tools are either commercial (Primavera P6, Microsoft Project) or web-based SaaS (Asana, Monday.com) that implement simplified critical path calculation without full FS/SS/FF/SF support.

**Recommendation:** The gonum-based approach is uniquely differentiated. Preserve and extend it with resource leveling rather than adopting an external library.

Sources: [Wrike CPM Guide](https://www.wrike.com/blog/critical-path-is-easy-as-123/), [Asana CPM](https://asana.com/resources/critical-path-method)

### 2. Construction Weather APIs

**Finding:** Tomorrow.io is the strongest option for construction-specific weather data:
- Operates its own forecasting model (1F) with probabilistic outcomes in 7 percentiles
- 500+ weather parameters including precipitation intensity, wind gust, soil moisture
- Free tier: 500 API calls/day; Enterprise: flexible pricing
- Deterministic forecasts up to 14 days

OpenWeatherMap is cheaper but lacks construction-specific parameters. NOAA provides free US data but requires more processing.

**Recommendation:** Evolve SWIM from static multipliers to Tomorrow.io integration. Use Tomorrow.io's hourly precipitation probability to calculate task-specific weather delay risk rather than blanket multipliers.

Sources: [Tomorrow.io vs OpenWeatherMap](https://www.tomorrow.io/blog/tomorrow-vs-openweathermap/), [Top Weather APIs 2026](https://www.xweather.com/blog/article/top-weather-apis-for-production-2026), [Meteomatics Comparison](https://www.meteomatics.com/en/weather-api/best-weather-apis/)

### 3. Construction PM Software Market

**Finding:** Construction PM software market grew from $2.59B (2024) to $2.95B (2025), projected to reach $7.32B by 2032 (13.87% CAGR).

Key competitive dynamics:
- **Procore** (4.5/5 Capterra): Commercial-focused, "black box" pricing, 3-6 month onboarding. Residential contractors report half the features collect dust. Major complaints: bugs/crashes, pricing opacity, poor small-company support.
- **Buildertrend** (residential focus): Pricing hikes after initial period, data portability concerns, CoConstruct acquisition produced no meaningful feature updates. Good for residential workflows but limited automation.
- **Knowify** ($149/mo): Trade contractor niche, strong QuickBooks integration, AIA billing. Not designed for residential builders/GCs. Handles multi-phase jobs and service work.

**Recommendation:** FutureBuild's AI-native positioning fills the gap between Procore (too complex/expensive) and Buildertrend (limited automation). The deterministic CPM engine is a genuine differentiator -- no competitor offers transparent, auditable scheduling algorithms.

Sources: [Buildertrend vs Procore 2026](https://buildern.com/resources/blog/buildertrend-vs-procore/), [Construction PM Market](https://www.researchandmarkets.com/report/construction-project-management-software), [Procore Reviews G2](https://www.g2.com/products/procore/reviews), [Procore Reviews Capterra](https://www.capterra.com/p/56250/Procore/reviews/)

### 4. Construction Procurement Automation

**Finding:** Market valued at $1.5B (2025), projected $2.2B by 2030 (7.9% CAGR). AI-powered estimation and bidding report 40-60% reduction in estimation time. Key trend: unified platforms connecting procurement with ERP, accounting, and project management.

Palantir entered construction with AIP for construction companies. GEP SMART offers sourcing and contract management. Archdesk provides real-time data and automated workflows for construction procurement.

**Recommendation:** The legacy ProcurementAgent's MRP feedback loop (NeedByDate = EarlyStart - stagingBuffer, MustOrderDate = NeedByDate - leadTime - weatherBuffer) is architecturally sound. Evolve with supplier API integrations and Tribunal-governed autonomous ordering.

Sources: [Construction Procurement Outlook 2026](https://archdesk.com/blog/construction-procurement-trends-in-2026), [AI-Driven Procurement](https://assembly-industries.com/feeds/blog/ai-driven-procurement-platforms-construction-industry), [Palantir Construction](https://www.palantir.com/offerings/construction/)

### 5. Fleet Management

**Finding:** Samsara ranks #1 on G2 (99/100 score). GPS Trackit merged into Zonar Ignition (Aug 2025). Pricing: $8.95-$45/vehicle/month depending on provider and contract length. Key feature: unified platform integrating vehicle tracking, AI dash cams, and site visibility.

**Recommendation:** Build lightweight fleet management in-house (legacy FleetServicer already has the interface). Integrate via API with Samsara or Verizon Connect for GPS tracking/telematics rather than building hardware integration. Focus on equipment allocation scheduling that feeds into CPM material constraints.

Sources: [Samsara G2 #1](https://www.samsara.com/company/news/press-releases/Samsara-Ranks-No-1-in-Fleet-Management-on-G2-for-2025), [Fleet Tracking Pricing 2026](https://spytec.com/blogs/news/fleet-tracking-pricing-comparison), [Fleet GPS Comparison 2026](https://www.responsiblefleet.com/post/best-fleet-gps-tracking-solutions-in-2026-complete-comparison-guide)

### 6. Industrial Dark UI Design

**Finding:** Bloomberg Terminal's UX design philosophy is about concealing complexity -- hundreds of features accessible through a dense but learnable interface. Dark mode has evolved from trend to fundamental expectation. Key principles for 2026: purpose-built design systems with calibrated grayscale palettes (not pure black), WCAG 2.1 contrast ratios, and data-dense layouts that reduce cognitive load.

Glassmorphism remains popular but requires careful implementation to maintain readability. The shadcn ecosystem demonstrates that token-based design systems with zero hardcoded colors are the current best practice.

**Recommendation:** Evolve GableLBM tokens into a Lit Web Component library. Maintain the Deep Space (#0A0B10) base with Gable Green (#00FFA3) accent. Use JetBrains Mono for all financial/numerical data. Implement glassmorphism sparingly -- cards and modals only, not data tables.

Sources: [Bloomberg Terminal UX](https://www.bloomberg.com/company/stories/how-bloomberg-terminal-ux-designers-conceal-complexity/), [Dark Mode 2026](https://kyady.com/en/blog/dark-mode-2026-best-practices-elegant-interfaces), [Bloomberg Color Accessibility](https://www.bloomberg.com/company/stories/designing-the-terminal-for-color-accessibility/)

### 7. Go Background Job Queues

**Finding:** Three main options in the Go ecosystem:
- **Asynq** (current): Redis-backed, full-featured (scheduling, retries, priorities, Prometheus, web UI). Low recent commit activity raises maintenance concerns.
- **River**: PostgreSQL-native, transactional job enqueuing, atomic guarantees. Eliminates Redis dependency. Active development.
- **Temporal**: Heavy-duty workflow orchestration, complex setup, steep learning curve. Overkill for current use case.

**Recommendation:** Migrate from Asynq to River over 2-3 sprints. River's PostgreSQL-native approach aligns with the "database is source of truth" constraint and eliminates the Redis infrastructure dependency. The transactional enqueuing guarantees simplify the ProcurementAgent's batch flush pattern.

Sources: [Asynq vs Machinery vs Work](https://medium.com/@geisonfgfg/task-queues-in-go-asynq-vs-machinery-vs-work-powering-background-jobs-in-high-throughput-systems-45066a207aa7), [River Queue](https://riverqueue.com/), [River HN Discussion](https://news.ycombinator.com/item?id=38349716)

### 8. AI Agent Frameworks & A2A Protocol

**Finding:** 68% of production AI agents are built on open-source frameworks. LangGraph (v1.0, graph-based orchestration) and CrewAI (role-based multi-agent) lead the market. Google A2A protocol (v0.3, now under Linux Foundation) provides standardized agent interop via HTTP, SSE, JSON-RPC with webhook push notifications for async workflows.

Key trend: convergence toward graph-based orchestration. All major frameworks now adopt graph/workflow execution models.

**Recommendation:** Keep agents in Go (not Python) to maintain the "NO Python" constraint. Adopt A2A protocol for external agent communication (The Brain integration). Internal agents remain Go services with the existing builder pattern. The Tribunal ConsensusEngine already implements the multi-model pattern -- evolve it into the decision authority for autonomous actions.

Sources: [A2A Protocol IBM](https://www.ibm.com/think/topics/agent2agent-protocol), [Google A2A Announcement](https://developers.googleblog.com/en/a2a-a-new-era-of-agent-interoperability/), [A2A v0.3 Upgrade](https://cloud.google.com/blog/products/ai-machine-learning/agent2agent-protocol-is-getting-an-upgrade), [AI Agent Framework Comparison 2026](https://o-mega.ai/articles/langgraph-vs-crewai-vs-autogen-top-10-agent-frameworks-2026)

### 9. pgvector for Construction Documents

**Finding:** pgvector has matured significantly by 2026. HNSW index supports fast approximate nearest neighbor search up to tens of millions of vectors. IVFFlat scales to hundreds of millions. Key advancement: combining vector search with traditional SQL filters enables hybrid queries (semantic similarity + structured metadata filtering).

**Recommendation:** Preserve the existing pgvector setup (vector(768) on document_chunks). Expand to index construction material specs, supplier catalogs, and inspection checklists. Use hybrid search (vector + SQL WHERE clauses) for contextual document retrieval.

Sources: [pgvector 2026 Guide](https://www.instaclustr.com/education/vector-database/pgvector-key-features-tutorial-and-pros-and-cons-2026-guide/), [PostgreSQL Vector Search 2026](https://calmops.com/database/postgresql/postgresql-vector-search-pgvector-complete-guide-2026/)

### 10. Flutter Offline-First

**Finding:** Flutter's official documentation now includes an offline-first architecture guide. The Drift library provides local SQLite storage with sync mechanisms. The workmanager package enables background task execution. Key challenge: conflict resolution when multiple field workers update the same task.

**Recommendation:** Use Drift for local storage + custom sync engine with last-write-wins conflict resolution (construction tasks rarely have concurrent edits). Background sync via workmanager with retry logic. Target Android-first (field crews predominantly use Android).

Sources: [Flutter Offline-First Docs](https://docs.flutter.dev/app-architecture/design-patterns/offline-first), [Flutter Sync Engine](https://medium.com/@pravinkunnure9/building-a-flutter-offline-first-sync-engine-flutter-sync-engine-with-conflict-resolution-5a087f695104), [Offline-First AI Flutter](https://dasroot.net/posts/2026/03/building-offline-first-ai-applications-flutter/)

### 11. Construction Financial Management

**Finding:** AIA billing (G702/G703) automation is a critical feature. Foundation Software provides certified payroll, job costing, and AIA billing in one platform. NetSuite supports job costing by work code. Key trend: cloud-native solutions with AI-powered expense categorization and predictive profitability.

The legacy system already has PROJECT_BUDGETS (estimated/committed/actual per WBS phase), CorporateBudget (int64 cents), ARAgingSnapshot, and GLSyncLog models. This is a strong foundation.

**Recommendation:** Build AIA billing as a first-class feature (G702/G703 PDF generation from PROJECT_BUDGETS data). Implement job costing views that roll up from WBS task level to phase level to project level to organization level (the corporate rollup already exists as TypeCorporateRollup cron).

Sources: [AIA Billing Procore](https://www.procore.com/library/aia-billing), [Construction Billing Software 2026](https://projul.com/blog/construction-billing-software/), [ERP for Contractors 2026](https://appficiency.com/the-ultimate-guide-to-erp-for-specialty-contractors-2026-edition/)

### 12. Construction HR & Payroll

**Finding:** Certified payroll compliance (WH-347 form, Davis-Bacon Act) is non-negotiable for government projects. DOL recovered $273M+ in back wages in 2023 alone. Leading platforms: Hammr (time-tracking + geolocation + certified payroll), eBacon (Davis-Bacon compliance), Foundation (full construction accounting). Miter is an emerging construction-first HCM platform.

The legacy system has EmployeeServicer with time logging, certification tracking, and labor burden calculation.

**Recommendation:** Do NOT build payroll processing in-house. Integrate with Hammr or similar for actual payroll execution. Focus on time tracking with geolocation (feeds into job costing), certification expiration alerts (already have TypeCertificationAlerts), and labor allocation to projects.

Sources: [Certified Payroll 2026](https://www.hh2.com/construction-human-resources/certified-payroll-requirements-for-construction-2026), [Hammr Prevailing Wage](https://www.hammr.com/prevailing-wage-software-for-construction), [Construction Payroll Software 2025](https://hcm.sage.com/blog/best-construction-payroll-software)

### 13. Construction Pain Points & Technology Adoption

**Finding:** 52% of rework in construction is caused by poor communication ($31.3B annually). 48% of construction leaders cite training costs as the biggest tech adoption barrier. Field crews want simple, integrated tools with no steep learning curves. Superintendents report daily battles of workforce shortages, supply chain chaos, and communication breakdowns.

Key insight: technology must reduce admin burden without adding complexity. The most adopted tools are those that require less than 1 day of training.

**Recommendation:** The agent-first architecture (agents surface priorities, users approve/dismiss) is the right approach. Minimize manual data entry. Maximize automated feed cards. Flutter field app must be usable with work gloves and in direct sunlight.

Sources: [Construction Pain Points](https://premiercs.com/blog/construction-pain-points-essential-guide-for-project-managers), [Construction Tech Adoption](https://www.truelook.com/blog/construction-technology-slow-in-adoption-how-do-we-bridge-the-gap), [Superintendent Productivity](https://safesitecheckin.com/a-helpful-productivity-tool-for-overwhelmed-construction-site-superintendents/)
