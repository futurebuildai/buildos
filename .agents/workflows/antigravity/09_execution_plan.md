---
description: Break the architecture into a granular, dependency-ordered, phased implementation plan with quality gates per milestone. The final artifact Claude Code executes against.
stage: "09"
inputs: All handoff artifacts (PRD.md, ARCHITECTURE.md, API_CONTRACT.md, TESTING_STRATEGY.md, SCOPE_DEFINITION.md, DESIGN_SYSTEM.md)
outputs: EXECUTION_PLAN.md (in handoff/)
browser_usage: None (pure synthesis of existing artifacts)
---

# Stage 09: Execution Plan

This is the implementation blueprint that Claude Code follows step-by-step. It must be granular enough that each step is unambiguous, ordered by dependency, and grouped into deployable milestones.

---

## Step 1: Identify All Implementation Tasks

Read ARCHITECTURE.md and decompose it into discrete implementation tasks:

### Infrastructure & Scaffolding
- Project initialization (directory structure, config files, dependencies)
- Database setup (schema, migrations, seed data)
- CI/CD pipeline setup
- Development environment configuration
- Linting, formatting, git hooks

### Backend
- For each module/package in ARCHITECTURE.md:
  - Interface/type definitions
  - Implementation
  - Unit tests
- For each API endpoint in API_CONTRACT.md:
  - Handler/controller
  - Input validation
  - Integration tests
- Cross-cutting concerns:
  - Authentication middleware
  - Error handling middleware
  - Logging/observability setup
  - Rate limiting

### Frontend
- For each component in DESIGN_SYSTEM.md:
  - Component implementation
  - Component tests
- For each screen in INFORMATION_ARCHITECTURE.md:
  - Page/view implementation
  - Data fetching/state management
  - E2E tests for the journey this screen belongs to
- Cross-cutting:
  - Routing setup
  - Auth integration
  - Error boundary/handling
  - Loading/empty states

### Integration
- Frontend ↔ Backend integration
- External service integration (from BUILD_VS_BUY.md)
- E2E test suite

### Polish & Hardening
- Performance optimization
- Accessibility audit fixes
- Security audit fixes
- Documentation

## Step 2: Dependency Ordering

**Invoke skill:** `.agents/skills/antigravity/dependency_mapping.md`

Map dependencies between tasks:
```
[Scaffolding] → [Database schema] → [Backend core modules] → [API endpoints] → [Frontend components] → [Pages/Views] → [E2E tests] → [Polish]
```

Identify tasks that can be parallelized (e.g., independent API endpoints, independent UI components).

## Step 3: Group into Milestones

Each milestone must be:
- **Independently deployable** — it works on its own, even if incomplete
- **Testable** — has a clear quality gate
- **Valuable** — delivers user-visible progress (aligned to SCOPE_DEFINITION.md phases)

```markdown
## Milestone Structure

### Milestone 0: Project Foundation
**Goal:** Working development environment with CI/CD
**Tasks:** Scaffolding, config, CI/CD, database schema, seed data
**Quality Gate:** `make dev` starts the app, CI pipeline runs, migrations apply cleanly
**Estimated effort:** [T-shirt size]

### Milestone 1: Core Data Layer
**Goal:** Backend data layer functional with tests
**Tasks:** All repository/DAL implementations, database integration tests
**Quality Gate:** All data operations have passing integration tests, migration up/down works
**Estimated effort:** [T-shirt size]

### Milestone 2: API Layer
**Goal:** Complete API functional with auth
**Tasks:** All API endpoints, auth middleware, API integration tests
**Quality Gate:** All API endpoints return correct responses, auth enforced, rate limiting works
**Estimated effort:** [T-shirt size]

### Milestone 3: UI Foundation
**Goal:** Design system components built and tested
**Tasks:** All DESIGN_SYSTEM.md components, component tests
**Quality Gate:** All components render correctly, pass accessibility checks
**Estimated effort:** [T-shirt size]

### Milestone 4: Core User Journey
**Goal:** Primary user journey works end-to-end
**Tasks:** Pages for core journey, data fetching, state management, E2E tests
**Quality Gate:** E2E test for happy path passes, core journey completable
**Estimated effort:** [T-shirt size]

### Milestone 5: Complete MVP
**Goal:** All Must Have features functional
**Tasks:** Remaining pages/features, error handling, empty states, loading states
**Quality Gate:** All E2E tests pass, all acceptance criteria met, zero-trust audit clean
**Estimated effort:** [T-shirt size]

### Milestone 6: Hardening
**Goal:** Production-ready quality
**Tasks:** Performance optimization, accessibility fixes, security audit, documentation
**Quality Gate:** Lighthouse scores meet targets, security audit clean, WCAG AA compliant
**Estimated effort:** [T-shirt size]
```

## Step 4: Write Granular Steps

For each milestone, write numbered implementation steps:

Write `.agents/handoff/EXECUTION_PLAN.md`:

```markdown
# Execution Plan

## How to Use This Plan
Claude Code: Execute steps sequentially within each milestone. After each step, run the zero-trust audit checklist from TESTING_STRATEGY.md. Do not proceed to the next step until the current step passes audit.

## Milestone 0: Project Foundation

### Step 0.1: Initialize Project
- [ ] Create project directory structure per ARCHITECTURE.md Section 2.1
- [ ] Initialize package manager ([from TECH_STACK.md])
- [ ] Install core dependencies (list specific packages and versions)
- [ ] Configure linter ([from TECH_STACK.md]) with project rules
- [ ] Configure formatter ([from TECH_STACK.md])
- [ ] Create .gitignore, .env.example
- [ ] Create Makefile/Taskfile with: `dev`, `test`, `lint`, `build` targets
- **Audit:** Project builds, linter runs clean, formatter runs clean

### Step 0.2: Database Setup
- [ ] Create database migration tool configuration
- [ ] Write initial migration: [list all tables from ARCHITECTURE.md Section 3]
- [ ] Write seed data script for development
- [ ] Write migration rollback
- **Audit:** Migrations apply and rollback cleanly, seed data populates correctly

### Step 0.3: CI/CD Pipeline
- [ ] Create CI config ([from TECH_STACK.md])
- [ ] Configure stages: lint → test → build
- [ ] Configure test coverage reporting
- [ ] Configure dependency vulnerability scanning
- **Audit:** Pipeline runs successfully on push

[Continue with numbered steps for each milestone...]

## Rollback Points
| After Step | Can Rollback To | How |
|------------|----------------|-----|

## Critical Path
[The longest sequential chain — this determines minimum timeline]
Steps: 0.1 → 0.2 → 1.1 → ... → N

## Parallelization Opportunities
| Parallel Track A | Parallel Track B | Merge Point |
|-----------------|-----------------|-------------|
```

## Step 5: Cross-Reference Validation

Verify completeness:
- [ ] Every API endpoint in API_CONTRACT.md has a step
- [ ] Every component in DESIGN_SYSTEM.md has a step
- [ ] Every screen in INFORMATION_ARCHITECTURE.md has a step
- [ ] Every user story in PRD.md is covered by at least one step
- [ ] Every step has a clear audit criteria
- [ ] Dependencies are respected (no step references something not yet built)
- [ ] All test types from TESTING_STRATEGY.md are assigned to specific steps

## Step 6: Update Pipeline State

```markdown
## Stage 09 — Execution Plan ✅
- EXECUTION_PLAN.md produced
- [N] milestones, [M] steps total
- Critical path: [N] steps
- Parallelization opportunities: [N]
## Next Stage: 10 — Spec Review
```

Proceed to Stage 10.
