# Research Agenda

**Date:** 2026-04-02
**Pipeline Stage:** 00 - Vision Intake
**Status:** COMPLETE

---

## Research Domains

### Domain 1: CPM Engine Evolution
- Compare legacy gonum-based DependencyGraph with alternative graph libraries
- Resource leveling algorithms for construction scheduling
- Multi-project CPM with shared resource pools
- Deterministic scheduling guarantees (integer math, cross-platform reproducibility)

### Domain 2: Weather Integration (SWIM v2)
- Tomorrow.io Weather API -- hourly forecasts, historical data, construction-specific parameters
- NOAA weather data for US construction sites
- OpenWeatherMap vs Visual Crossing for cost-effective alternatives
- Predictive weather models for 5-14 day construction planning windows

### Domain 3: Competitive Landscape
- Procore -- commercial focus, pricing, residential gaps
- Buildertrend -- residential focus, CoConstruct acquisition impact
- Knowify -- trade contractor niche, QuickBooks integration
- Emerging AI-native competitors (inBuild, Projul, Planera)

### Domain 4: Construction Procurement Automation
- GEP SMART, Archdesk, Precoro -- enterprise procurement platforms
- AI-driven estimation and bid automation (40-60% time reduction reported)
- Material price tracking and supplier network optimization
- Integration with equipment allocation (Fleet module linkage)

### Domain 5: Fleet Management
- Samsara (G2 #1), Verizon Connect, Motive -- pricing and API access
- GPS Trackit merged into Zonar Ignition (Aug 2025)
- Construction-specific equipment tracking (excavators, compactors, graders per legacy equipment_validator.go)
- Telematics API integration patterns

### Domain 6: Industrial Dark UI Systems
- Bloomberg Terminal UX design principles (conceal complexity)
- GableLBM token system evolution to Lit Web Components
- Glassmorphism implementation with CSS custom properties
- Data-dense dashboard patterns for financial/operational data

### Domain 7: Background Job Architecture
- Asynq (current) vs River (PostgreSQL-native) vs Temporal (workflow engine)
- River's transactional guarantees align with "database is source of truth" philosophy
- Migration path from Redis-backed to PostgreSQL-backed queues
- Cron scheduling patterns for 16+ task types

### Domain 8: AI Agent Frameworks
- Google A2A protocol (v0.3, Linux Foundation) -- webhook-driven async
- LangGraph graph-based orchestration (v1.0, late 2025)
- CrewAI role-based multi-agent systems
- Evolution of legacy DailyFocus/Procurement/SubLiaison agents to A2A-compatible

### Domain 9: Mobile Field Applications
- Flutter offline-first with Drift (local SQLite) + background sync
- workmanager for background task execution
- Conflict resolution strategies for multi-device field updates
- Construction-specific mobile UX (gloved hands, outdoor visibility)

### Domain 10: Construction Financial Management
- AIA billing (G702/G703 forms) automation
- Job costing by WBS phase (aligns with legacy PROJECT_BUDGETS table)
- Progress billing with retention tracking
- AR aging (legacy ARAgingSnapshot model already exists)

### Domain 11: HR & Workforce Management
- Certified payroll (WH-347 form) for government projects
- Prevailing wage compliance (Davis-Bacon Act)
- Construction-specific time tracking with geolocation
- Certification expiration tracking (legacy already has TypeCertificationAlerts)

### Domain 12: Document Intelligence
- pgvector for construction document semantic search (already using vector(768))
- RAG pipeline with chunking strategies for construction plans
- Invoice extraction accuracy improvements (legacy InvoiceServicer exists)
- Blueprint/plan sheet analysis with Gemini Vision
