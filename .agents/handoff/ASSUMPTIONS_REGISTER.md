# Assumptions Register

**Date:** 2026-04-02
**Pipeline Stage:** 00 - Vision Intake
**Status:** COMPLETE

---

## Critical Assumptions

| ID | Assumption | Risk if Wrong | Validation Method | Status |
|----|-----------|---------------|-------------------|--------|
| A1 | The gonum DependencyGraph is correct and complete for all four dependency types (FS, SS, FF, SF) | Schedule corruption, incorrect critical paths | Legacy cpm_determinism_test.go golden master tests | VALIDATED by vault review |
| A2 | Integer nanosecond math (AddWorkDuration) eliminates IEEE 754 drift | Cross-architecture schedule divergence | Tests prove minute-precision truncation works | VALIDATED by vault review |
| A3 | SWIM weather model multipliers (1.15, 1.25, 1.10) are empirically accurate for residential construction | Over/under-estimated task durations | Compare against historical project data; evolve to Tomorrow.io predictive | ASSUMED -- needs calibration data |
| A4 | WBS scope < 10.0 OR 13.x for weather sensitivity is correct | Missing weather adjustments on outdoor tasks | Cross-reference with construction best practices | ASSUMED |
| A5 | The Brain will continue as the upstream OIDC provider and webhook source | Authentication architecture collapse | Confirmed by TECH_STACK.md delegation model | VALIDATED |
| A6 | PostgreSQL 16+ with pgvector handles both relational and vector workloads at scale | Performance degradation at scale | Benchmark with 50+ concurrent projects | ASSUMED |
| A7 | Asynq (Redis-backed) remains viable for 16+ cron task types | Job queue reliability at scale | Evaluate River (PostgreSQL-native) as alternative | NEEDS EVALUATION |
| A8 | Lit 3.0+ is suitable for data-dense Industrial Dark dashboard | Performance issues with complex financial tables | Prototype with 1000+ row data grids | NEEDS PROTOTYPE |
| A9 | Flutter offline-first with Drift provides reliable field sync | Data loss on poor connectivity | Test with intermittent network simulation | NEEDS PROTOTYPE |
| A10 | Construction market will adopt AI-native scheduling (vs manual Gantt charts) | Low adoption, wasted investment | User research with 20+ builders | NEEDS VALIDATION |
| A11 | CorporateFinancialsServicer, EmployeeServicer, FleetServicer interfaces are sufficient scope for Organization-level modules | Missing critical back-office functions | Map against Buildertrend/Procore feature sets | NEEDS VALIDATION |
| A12 | The existing 80-task WBS template (WBS 5.2-15.4) covers standard residential construction | Missing tasks for specialty homes (custom, luxury, modular) | Builder interviews | ASSUMED |
| A13 | Autonomous procurement can achieve 80% hands-off ordering | Legal/liability concerns with auto-ordering | Regulatory research, insurance requirements | HIGH RISK |
| A14 | Google A2A protocol (v0.3) is stable enough for production agent communication | Protocol breaking changes | Monitor Linux Foundation governance | MEDIUM RISK |
| A15 | BIGINT cents for all monetary values is sufficient precision | Edge cases with foreign currency or micro-transactions | Not applicable for US residential construction | VALIDATED |

---

## Technical Assumptions

| ID | Assumption | Impact |
|----|-----------|--------|
| T1 | Go 1.24 remains the backend language (NO Python) | All physics engine, agents, services in Go |
| T2 | Raw SQL via pgx (NO ORMs) for all database access | Higher development cost, maximum query control |
| T3 | TypeScript strict mode for all frontend code | Compile-time type safety, Rosetta Stone contract tests |
| T4 | Docker multi-stage builds for deployment | CI/CD pipeline structure |
| T5 | Railway for production hosting | Deployment constraints and scaling limits |
| T6 | Clerk removed, JWT delegation to The Brain | Auth migration required |
| T7 | Redis 7 for Asynq task queue | Infrastructure dependency |

---

## Business Assumptions

| ID | Assumption | Impact |
|----|-----------|--------|
| B1 | Target market is US residential construction (custom homes, 1500-6000 GSF) | WBS scope, regulatory compliance, weather model |
| B2 | Users are small-to-mid builders (2-50 projects/year) | Pricing, feature complexity, mobile-first priority |
| B3 | The competitive gap is between Procore (too complex, too expensive) and spreadsheets (too manual) | Product positioning |
| B4 | 2030-ready means autonomous agents replace 60%+ of admin tasks | Architecture must support agent autonomy levels |
| B5 | Site superintendents are the primary daily users, not office staff | Mobile-first design, simplified workflows |
