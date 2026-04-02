# Pipeline State

**Date:** 2026-04-02
**System:** FutureBuild OS -- 2030-Ready AI-Native Revamp
**Orchestrator:** Antigravity Pipeline (Stages 00-04)
**Status:** PAUSED AT APPROVAL GATE 1

---

## Stage Status

| Stage | Name | Status | Artifacts | Notes |
|-------|------|--------|-----------|-------|
| 00 | Vision Intake | COMPLETE | VISION_BRIEF.md, RESEARCH_AGENDA.md, ASSUMPTIONS_REGISTER.md | Vault read complete: 15 file categories ingested from reference-vault |
| 01 | Deep Research | COMPLETE | RESEARCH_FINDINGS.md, COMPETITIVE_LANDSCAPE.md | 20+ web searches across 12 research domains; legacy vault analysis of physics engine, agents, design system, worker patterns |
| 02 | User Research | COMPLETE | PERSONAS.md, JTBD_ANALYSIS.md, USER_JOURNEYS.md | 4 personas, 8 JTBD, 5 user journeys mapped to legacy system components |
| 03 | Solution Design | COMPLETE | SOLUTION_CANDIDATES.md, FEASIBILITY_ASSESSMENT.md, BUILD_VS_BUY.md | 6 solution concepts, all assessed FEASIBLE, 25+ build/buy decisions |
| 04 | Scope & Prioritization | COMPLETE | SCOPE_DEFINITION.md, METRICS_FRAMEWORK.md | Walking skeleton (4 weeks), MVP (16 weeks), 5 post-MVP phases |
| 05 | Design System | COMPLETE | DESIGN_SYSTEM.md, INFORMATION_ARCHITECTURE.md | GableLBM Industrial Dark tokens, FBBaseElement, Tailwind CSS 4, 3 workspaces (Portfolio, Agent Command Center, Field Portal) |
| 06 | Product Specification | COMPLETE | PRD.md | 22 user stories across 5 journeys, Given/When/Then acceptance criteria, 7 NFR categories, full traceability matrix |
| 07 | Architecture Spec | COMPLETE | ARCHITECTURE.md | Go package structure, PostgreSQL schema (20+ tables), River job definitions, API route contracts, physics engine preservation, Flutter Drift schema, Lit hierarchy, CI/CD pipeline |

---

## Approval Gate 1

**Gate Type:** Human approval required before proceeding to execution stages (05+)

**What is being approved:**

1. **Walking Skeleton scope** (Week 1-4): River queue migration, JWT auth from FB-Brain, fb-org-shell with GableLBM tokens, Flutter scaffold with Drift, GitHub Actions CI/CD
2. **MVP feature set** (Week 5-20): 10 features (M1-M10) with P0/P1/P2 prioritization
3. **Build vs Buy decisions**: 25+ decisions including BUILD for CPM, dashboard, agents, Tribunal; BUY for Tomorrow.io, River, Samsara, payroll integration
4. **Post-MVP phasing**: 4 phases through Week 53+ covering operations, billing, advanced AI, and scale
5. **Metrics framework**: North star metrics, feature-level KPIs, graduation criteria for Tribunal autonomy levels
6. **Out-of-scope items**: Pre-permit phases, in-house payroll, GPS hardware, commercial construction, React, Python, ORMs

**Decisions requiring owner input:**

| Decision | Options | Recommendation | Impact |
|----------|---------|---------------|--------|
| Asynq -> River migration | (a) Migrate now, (b) Keep Asynq for MVP, migrate later | (a) Migrate now -- eliminates Redis dependency, aligns with "database is source of truth" | Walking skeleton architecture |
| Tribunal Level 1 legal review | (a) Proceed with recommend-only, (b) Wait for legal opinion before building | (a) Proceed -- Level 1 is recommendation only, no autonomous action | M5 timeline |
| Tomorrow.io tier | (a) Free tier for MVP, (b) Enterprise from start | (a) Free tier -- 500 calls/day sufficient for <10 projects | M3 cost |
| Flutter vs Progressive Web App | (a) Flutter native, (b) PWA for mobile | (a) Flutter -- offline-first with Drift requires native SQLite | M4 architecture |
| GableLBM design implementation | (a) Build tokens from spec, (b) Wait for Figma designs | (a) Build from spec -- GABLE_LBM_DESIGN_SYSTEM.md is detailed enough | Walking skeleton frontend |

---

## Blocking Dependencies

| Dependency | Blocker For | Owner | Status | Action Needed |
|-----------|-------------|-------|--------|---------------|
| FB-Brain JWT issuer endpoint | Walking skeleton auth | FB-Brain team | NEEDED | Coordinate with FB-Brain team to expose /auth/token endpoint |
| Tomorrow.io API key | M3: SWIM v2 | DevOps | NEEDED | Sign up for free tier, store key in environment config |
| River library evaluation | Walking skeleton queue | Backend team | NEEDED | Benchmark River vs Asynq: job throughput, failure modes, observability |
| Firebase project setup | M4: Flutter push | DevOps | NEEDED | Create Firebase project, configure FCM, add google-services.json |
| GableLBM Figma designs | M1: Dashboard polish | Design team | NEEDED | Can proceed with spec-based tokens; Figma needed for pixel-perfect layouts |
| Legal review: autonomous ordering | M5: Tribunal Level 1 | Legal | NEEDED | Review: can AI recommend purchase orders? Liability for Level 1 (recommend only) |

---

## Artifact Inventory

All artifacts are located in `/home/colton/Desktop/futurebuild-ecosystem/futurebuild-os/.agents/handoff/`.

### Stage 00: Vision Intake
| File | Size | Description |
|------|------|-------------|
| VISION_BRIEF.md | 11.6 KB | Vision statement, legacy foundation analysis (10 subsystems), preserve vs evolve decisions, target outcomes, tech stack confirmation |
| RESEARCH_AGENDA.md | 3.7 KB | 12 research domains with specific topics |
| ASSUMPTIONS_REGISTER.md | 4.6 KB | 15 critical assumptions, 7 technical assumptions, 5 business assumptions |

### Stage 01: Deep Research
| File | Size | Description |
|------|------|-------------|
| RESEARCH_FINDINGS.md | 18.5 KB | Legacy vault analysis (4 sections) + 13 web research findings with citations |
| COMPETITIVE_LANDSCAPE.md | 7.0 KB | 4-tier competitor analysis, feature comparison, user sentiment, strategic opportunities |

### Stage 02: User Research
| File | Size | Description |
|------|------|-------------|
| PERSONAS.md | 7.9 KB | 4 personas (Mike, Sarah, Tom, Carlos) with demographics, workflows, pain points |
| JTBD_ANALYSIS.md | 7.2 KB | 8 jobs-to-be-done mapped to legacy components with priority matrix |
| USER_JOURNEYS.md | 7.9 KB | 5 journeys (morning briefing, procurement, financial review, field progress, sub coordination) |

### Stage 03: Solution Design
| File | Size | Description |
|------|------|-------------|
| SOLUTION_CANDIDATES.md | 13.1 KB | 6 concepts with architecture, code signatures, trade-offs, recommendations |
| FEASIBILITY_ASSESSMENT.md | 7.4 KB | Technical feasibility for all 6 concepts, prototype validation plans, infrastructure migration |
| BUILD_VS_BUY.md | 5.9 KB | 25+ capability decisions, cost implications, risk assessment |

### Stage 04: Scope & Prioritization
| File | Size | Description |
|------|------|-------------|
| SCOPE_DEFINITION.md | 8.0 KB | Walking skeleton (4 weeks), MVP (10 features, 16 weeks), post-MVP phases, exclusions, dependencies |
| METRICS_FRAMEWORK.md | ~10 KB | North star metrics, feature-level KPIs, technical health metrics, measurement infrastructure, graduation criteria |
| PIPELINE_STATE.md | This file | Pipeline status, approval gate, blocking dependencies, artifact inventory |

---

## Vault Files Consumed

The following reference-vault files were read and analyzed during Stages 00-01:

**Physics Engine (5 files):**
- `reference-vault/futurebuild-os/internal/physics/cpm.go` (725 lines)
- `reference-vault/futurebuild-os/internal/physics/dhsm.go` (293 lines)
- `reference-vault/futurebuild-os/internal/physics/swim.go` (67 lines)
- `reference-vault/futurebuild-os/internal/physics/scoping.go` (399 lines)
- `reference-vault/futurebuild-os/internal/physics/equipment_validator.go` (117 lines)

**Tests (1 file):**
- `reference-vault/futurebuild-os/internal/physics/cpm_determinism_test.go` (160 lines)

**Architecture (1 file):**
- `reference-vault/futurebuild-os/CLAUDE.md` (284 lines)

**Agents (3 files):**
- `reference-vault/futurebuild-os/internal/agents/daily_focus.go` (~500 lines)
- `reference-vault/futurebuild-os/internal/agents/procurement.go` (~549 lines)
- `reference-vault/futurebuild-os/internal/agents/sub_liaison.go` (~742 lines)

**Workers (2 files):**
- `reference-vault/futurebuild-os/internal/worker/payloads.go` (261 lines)
- `reference-vault/futurebuild-os/internal/worker/scheduler.go` (56 lines)

**Service Layer (1 file):**
- `reference-vault/futurebuild-os/internal/service/interfaces.go` (213 lines)

**Models (2 files):**
- `reference-vault/futurebuild-os/internal/models/project.go`
- `reference-vault/futurebuild-os/internal/models/corporate_financials.go`

**FutureShade/Tribunal (4 files):**
- `reference-vault/futurebuild-os/internal/futureshade/service.go`
- `reference-vault/futurebuild-os/internal/futureshade/types.go`
- `reference-vault/futurebuild-os/internal/futureshade/tribunal/engine.go` (~260 lines)
- `reference-vault/futurebuild-os/internal/futureshade/tribunal/types.go` (~142 lines)

**Specs (5 files):**
- `reference-vault/futurebuild-os/specs/CPM_RES_MODEL_SPEC.md`
- `reference-vault/futurebuild-os/specs/GABLE_LBM_DESIGN_SYSTEM.md`
- `reference-vault/futurebuild-os/specs/BACKEND_SCOPE.md`
- `reference-vault/futurebuild-os/specs/DATA_SPINE_SPEC.md`
- `reference-vault/futurebuild-os/specs/FRONTEND_SCOPE.md`

**Cross-System (2 files):**
- `reference-vault/FB-Brain/CLAUDE.md`
- `.agents/TECH_STACK.md`

**Total: 26 vault files consumed, ~4,000+ lines of legacy code and specification analyzed.**

---

## Next Steps (Post-Approval)

Upon approval of Gate 1, the pipeline proceeds to execution stages:

1. **Stage 05: Architecture Blueprint** -- Detailed technical architecture for walking skeleton, including River queue setup, JWT middleware, Lit component hierarchy, Flutter project structure
2. **Stage 06: Implementation Planning** -- Sprint breakdown for walking skeleton (4 weeks) and MVP features (16 weeks), dependency ordering, parallel workstream identification
3. **Stage 07-10: Execution** -- Code implementation following the blueprints

**The pipeline is PAUSED. No code will be written until Gate 1 is approved.**
