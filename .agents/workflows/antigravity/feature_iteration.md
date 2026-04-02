---
description: Specify a new feature or enhancement for an existing post-v1 product. Reads current state, researches the feature domain, and produces a surgical delta spec that Claude Code can execute against the existing codebase.
stage: "post-v1"
inputs: PROJECT_STATE.md, all existing handoff/ blueprints, user feature request
outputs: FEATURE_DELTA.md, updated EXECUTION_PLAN.md, updated TESTING_STRATEGY.md
browser_usage: Moderate-heavy (feature domain research, library discovery, UX patterns)
approval_gate: true
---

# Post-v1 Workflow: Feature Iteration

The user wants to add a feature or enhance the existing product. Your job is to understand the current system deeply, research the feature domain, and produce a precise delta specification that Claude Code can execute without breaking existing functionality.

**This workflow ends with an APPROVAL GATE.** Present the feature delta for user review before handoff.

---

## Step 1: Load Current System State

Read ALL of the following to build a complete mental model of the existing system:

1. **`.agents/handoff/PROJECT_STATE.md`** — what has been built, what step execution is at
2. **`.agents/handoff/ARCHITECTURE.md`** — current system architecture, module boundaries, data models
3. **`.agents/handoff/API_CONTRACT.md`** — existing API endpoints and contracts
4. **`.agents/handoff/PRD.md`** — original product requirements (to understand original intent)
5. **`.agents/handoff/DESIGN_SYSTEM.md`** — existing component library and design tokens
6. **`.agents/handoff/TESTING_STRATEGY.md`** — current test coverage and approach
7. **`.agents/handoff/EXECUTION_PLAN.md`** — completed and remaining milestones
8. **`.agents/handoff/SCOPE_DEFINITION.md`** — what was explicitly deferred (the new feature may overlap)
9. **`.agents/TECH_STACK.md`** — technology constraints

Produce a brief summary:
```markdown
## Current System Understanding
- **Architecture style:** [monolith/modular/etc.]
- **Key modules:** [list with one-line descriptions]
- **API surface:** [N endpoints across M resource groups]
- **Data entities:** [list core entities]
- **Test coverage:** [unit/integration/e2e status]
- **Known technical debt:** [from PROJECT_STATE.md]
- **Deferred features:** [from SCOPE_DEFINITION.md — check for overlap with this request]
```

## Step 2: Parse the Feature Request

Analyze the user's feature request and extract:

1. **Feature name** — a clear, concise label
2. **Problem statement** — what user problem does this solve? Use: *"Users currently [pain point], this feature enables [outcome]."*
3. **Scope boundaries** — what is IN scope and what is explicitly OUT
4. **User stories** — write Given/When/Then acceptance criteria for each story
5. **Affected components** — which existing modules, endpoints, and UI components are touched
6. **New components** — what entirely new modules, endpoints, or UI components are needed
7. **Data model impact** — new entities, modified entities, new relationships, migrations needed
8. **Deferred feature overlap** — does this feature overlap with anything in SCOPE_DEFINITION.md's "Won't Have" or "Should Have" lists?

## Step 3: Research the Feature Domain

**Browser: 10-20 searches**

### 3a: UX Pattern Research
- Search for "[feature type] UX patterns [current year]"
- Search for "[feature type] UI best practices"
- Look at how competitors or best-in-class products implement this feature
- Search for accessibility considerations specific to this feature type
- Note interaction patterns, edge cases, and error states that users expect

### 3b: Technical Implementation Research
- Search for "[feature capability] [tech stack from TECH_STACK.md] implementation"
- Search for "[feature capability] library [language] [current year]"
- For each candidate library:
  - Check GitHub: stars, last commit date, open issues, license
  - Read README and quickstart documentation
  - Verify version compatibility with existing dependencies in TECH_STACK.md
  - Check for known issues: "[library] problems", "[library] breaking changes"
- Search for "[feature pattern] architecture patterns"
- Search for "[feature domain] edge cases", "[feature domain] common mistakes"

### 3c: Integration Risk Research
- Search for "[new library] + [existing library] compatibility"
- Search for "[feature pattern] migration strategies" (if modifying existing data)
- Look for blog posts or case studies of teams adding this feature to existing systems

Document all findings with source URLs.

## Step 4: Impact Analysis

Map the feature against every existing artifact to identify all touchpoints:

### Architecture Impact
| Existing Module | Change Type | Description |
|----------------|-------------|-------------|
| [module] | UNCHANGED | No modifications needed |
| [module] | MODIFIED | [what changes and why] |
| [new module] | NEW | [purpose and responsibilities] |

### API Impact
| Endpoint | Change Type | Description |
|----------|-------------|-------------|
| [existing endpoint] | UNCHANGED | No modifications |
| [existing endpoint] | MODIFIED | [new params, changed response, etc.] |
| [new endpoint] | NEW | [purpose, method, resource] |

### Data Model Impact
| Entity | Change Type | Description | Migration Required |
|--------|-------------|-------------|-------------------|
| [entity] | UNCHANGED | No changes | No |
| [entity] | MODIFIED | [new fields, changed constraints] | Yes |
| [new entity] | NEW | [purpose, key fields] | Yes |

### UI Impact
| Component/Screen | Change Type | Description |
|-----------------|-------------|-------------|
| [component] | UNCHANGED | No changes |
| [component] | MODIFIED | [what changes] |
| [new component] | NEW | [purpose, where it appears] |

### Dependency Impact
| Existing Dependency | Status | Notes |
|--------------------|--------|-------|
| [package] | Compatible | No version change needed |
| [package] | Needs Upgrade | [from version X to Y, reason] |
| [new package] | NEW | [purpose, version, license] |

## Step 5: Produce FEATURE_DELTA.md

Write `.agents/handoff/FEATURE_DELTA.md`:

```markdown
# Feature Delta: [Feature Name]

## Summary
[One paragraph describing the feature and its value]

## User Stories
### US-F1: [Story Title]
**As a** [user type], **I want to** [action], **so that** [outcome].
**Given** [precondition], **When** [action], **Then** [result].
**Priority:** Must Have / Should Have / Nice to Have

[Repeat for all stories]

## Research Findings
### UX Patterns
[Key patterns discovered, with source URLs]
### Technical Approach
[Recommended implementation approach based on research]
### Libraries/Tools
| Library | Purpose | Version | Evidence |
|---------|---------|---------|----------|
### Risks Identified
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|

---

## Architecture Delta

### UNCHANGED Modules
[List modules that require no changes — this confirms you've audited them]

### MODIFIED Modules

#### [Module Name]
**Current behavior:** [what it does now]
**New behavior:** [what it will do after the change]
**Interface changes:**
```
// BEFORE
[current interface/type signature]

// AFTER
[new interface/type signature]
```
**Internal changes:** [what implementation changes are needed]
**Risk:** [what could break]

[Repeat for each modified module]

### NEW Modules

#### [New Module Name]
**Responsibility:** [single responsibility description]
**Interfaces:**
```
[interface/type definitions]
```
**Dependencies:** [which existing modules it depends on]
**Depended on by:** [which existing modules will use it]

---

## API Delta

### UNCHANGED Endpoints
[List — confirms audit]

### MODIFIED Endpoints

#### [METHOD /path]
**Current contract:** [current request/response]
**New contract:** [new request/response]
**Breaking change:** Yes/No
**Migration strategy:** [if breaking]

### NEW Endpoints

#### [METHOD /path]
**Description:** [what it does]
**Request:**
```json
{ "field": "type — constraints" }
```
**Response (status):**
```json
{ "data": { ... } }
```
**Error Responses:**
| Status | Code | When |
|--------|------|------|
**Auth:** [required permissions]

---

## Data Model Delta

### UNCHANGED Entities
[List — confirms audit]

### MODIFIED Entities

#### [Entity Name]
**New fields:**
| Field | Type | Constraints | Default | Migration |
|-------|------|------------|---------|-----------|

**Modified fields:**
| Field | Old Definition | New Definition | Migration |
|-------|---------------|----------------|-----------|

**Migration script requirements:**
- [Step-by-step migration logic]
- [Rollback procedure]
- [Data backfill needed? Y/N + approach]

### NEW Entities

#### [Entity Name]
| Field | Type | Constraints | Index |
|-------|------|------------|-------|
**Relationships:** [FK references to existing entities]

---

## UI Delta

### UNCHANGED Components
[List — confirms audit]

### MODIFIED Components

#### [Component Name]
**Current:** [what it renders/does now]
**After:** [what changes]
**New props/state:** [additions]
**Accessibility impact:** [any a11y changes needed]

### NEW Components

#### [Component Name]
**Purpose:** [what it renders/does]
**Props:** [interface definition]
**States:** [loading, empty, error, populated]
**Accessibility:** [ARIA roles, keyboard behavior, screen reader text]
**Design tokens used:** [from DESIGN_SYSTEM.md]

---

## Dependency Delta
| Package | Action | Version | Purpose | License |
|---------|--------|---------|---------|---------|
| [pkg] | ADD | X.Y.Z | [why] | [license] |
| [pkg] | UPGRADE | X.Y.Z -> A.B.C | [why] | [license] |
| [pkg] | REMOVE | - | [no longer needed because] | - |
```

## Step 6: Update EXECUTION_PLAN.md

Append a new milestone to `.agents/handoff/EXECUTION_PLAN.md`:

```markdown
## Milestone F-[N]: [Feature Name]

### Step F-[N].1: Database Migration
- [ ] Write migration for [changes from Data Model Delta]
- [ ] Write rollback migration
- [ ] Test migration on copy of current seed data
- **Audit:** Migration applies and rolls back cleanly, existing data preserved

### Step F-[N].2: Backend — Modified Modules
- [ ] Update [module] interfaces per FEATURE_DELTA.md
- [ ] Update [module] implementation
- [ ] Update existing unit tests for modified behavior
- [ ] Add new unit tests for new behavior
- **Audit:** All existing tests still pass, new tests pass, no regressions

### Step F-[N].3: Backend — New Modules
- [ ] Implement [new module] per FEATURE_DELTA.md interfaces
- [ ] Write unit tests (target: same coverage as existing modules)
- **Audit:** New module tests pass, integrates with existing modules

### Step F-[N].4: API Layer Changes
- [ ] Update modified endpoints per API Delta
- [ ] Implement new endpoints per API Delta
- [ ] Write integration tests for all changed/new endpoints
- [ ] Verify existing endpoint integration tests still pass
- **Audit:** Full API integration test suite passes

### Step F-[N].5: Frontend — Modified Components
- [ ] Update components per UI Delta
- [ ] Update component tests
- **Audit:** Component tests pass, visual regression clean

### Step F-[N].6: Frontend — New Components
- [ ] Implement new components per UI Delta
- [ ] Write component tests
- [ ] Integrate into existing pages/routes
- **Audit:** Component tests pass, accessibility checks pass

### Step F-[N].7: E2E Integration
- [ ] Write E2E tests for new user stories
- [ ] Verify existing E2E tests still pass
- **Audit:** Full E2E suite passes

### Step F-[N].8: Regression Verification
- [ ] Run full test suite (unit + integration + E2E)
- [ ] Verify no performance degradation on existing flows
- [ ] Run accessibility audit on modified/new screens
- **Audit:** Zero regressions, all acceptance criteria from user stories met
```

## Step 7: Update TESTING_STRATEGY.md

Append to `.agents/handoff/TESTING_STRATEGY.md`:

```markdown
## Feature Addition: [Feature Name]

### New Unit Tests
| Module | Functions to Test | Edge Cases |
|--------|------------------|-----------|
[For each modified and new module]

### New Integration Tests
| Endpoint | Happy Path | Auth | Validation | Not Found | Conflict |
|----------|-----------|------|-----------|-----------|---------|
[For each modified and new endpoint]

### New E2E Tests
| User Story | Steps | Assertions | Priority |
|-----------|-------|-----------|----------|
[For each new user story]

### Regression Tests
| Existing Flow | Why It Might Break | Test to Verify |
|--------------|-------------------|---------------|
[For each existing flow that touches modified components]

### Performance Baseline
| Metric | Pre-Feature Value | Acceptable Post-Feature Value |
|--------|------------------|------------------------------|
[For each critical performance metric that might be affected]
```

## Step 8: APPROVAL GATE

**PAUSE HERE.** Present to the user:

1. **Feature summary** — what will be built and why
2. **Scope** — what is in and out
3. **Impact assessment** — how many modules/endpoints/components are touched
4. **New dependencies** — any new libraries or services
5. **Risk assessment** — what could go wrong and mitigations
6. **Execution plan** — the milestone steps Claude Code will follow
7. **Breaking changes** — any API or data model breaking changes

Ask the user: **"Feature delta spec is ready. Should I proceed with handoff to Claude Code?"**

## Step 9: Update Pipeline State

Update `.agents/handoff/PROJECT_STATE.md` to reflect the new feature work:
```markdown
## Active Work: Feature Iteration — [Feature Name]
- FEATURE_DELTA.md produced
- EXECUTION_PLAN.md updated with Milestone F-[N]
- TESTING_STRATEGY.md updated with regression criteria
- ⏸️ APPROVAL GATE — awaiting user confirmation
```
