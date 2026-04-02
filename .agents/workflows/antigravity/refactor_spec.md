---
description: Specify a code refactoring that improves internal structure without changing observable behavior. Documents current vs target structure, migration path, and defines invariants that must not change.
stage: "post-v1"
inputs: PROJECT_STATE.md, ARCHITECTURE.md, TESTING_STRATEGY.md, user refactoring request
outputs: REFACTOR_SPEC.md, updated ARCHITECTURE.md, updated TESTING_STRATEGY.md
browser_usage: Moderate (best practices, target pattern research, migration strategies)
---

# Post-v1 Workflow: Refactor Specification

The user wants to restructure code without changing behavior. Your job is to precisely document the current structure, research the target pattern, define the migration path, and establish invariants that must remain unchanged throughout the refactoring.

**Critical principle:** A refactor is successful if and only if all existing tests pass without modification after the refactor is complete. If tests need to change, the refactoring scope may be expanding into behavior changes -- flag this explicitly.

---

## Step 1: Understand the Refactoring Goal

Parse the user's refactoring request and extract:

1. **What to refactor** — which modules, files, patterns, or code areas
2. **Why** — the motivation (readability, performance, maintainability, removing duplication, preparing for a future feature, addressing tech debt)
3. **Target state** — what the code should look like after (if the user has a vision)
4. **Constraints** — timeline, files that must not be touched, features that are actively in development

If the user's request is vague (e.g., "clean up the backend"), ask clarifying questions:
- "Which specific modules or patterns do you want to refactor?"
- "What is the main pain point with the current structure?"
- "Is there a specific pattern or architecture you want to move toward?"

## Step 2: Load System Context

Read ALL of the following:

1. **`.agents/handoff/PROJECT_STATE.md`** — current state, recent changes, known tech debt
2. **`.agents/handoff/ARCHITECTURE.md`** — current architecture (this is your source of truth for current structure)
3. **`.agents/handoff/API_CONTRACT.md`** — API contracts (these are invariants)
4. **`.agents/handoff/TESTING_STRATEGY.md`** — current test coverage (these tests must continue to pass)
5. **`.agents/handoff/EXECUTION_PLAN.md`** — any in-progress work that might conflict
6. **`.agents/TECH_STACK.md`** — technology constraints and conventions

## Step 3: Document Current Structure

For the code area being refactored, create a detailed map of the current state:

```markdown
## Current Structure

### Module Map
[For each module/file in the refactoring scope]

#### [Module/Package Name]
**Path:** [file path from project root]
**Responsibility:** [what it does — single or multiple responsibilities]
**Public interface:**
```
[All exported functions, types, interfaces — exact signatures]
```
**Internal dependencies:** [what it imports from within the project]
**External dependencies:** [what it imports from third-party packages]
**Consumed by:** [what other modules import from this one]
**Lines of code:** [approximate — for scope estimation]
**Test coverage:** [covered / partially covered / uncovered]
**Known issues:**
- [Code smell, duplication, complexity metric if available]

### Dependency Graph
```
[Module A] --> [Module B] --> [Module C]
                          --> [Module D]
[Module E] --> [Module B]
```

### Current Pain Points
1. [Pain point 1: e.g., "Module X has 3 responsibilities"]
2. [Pain point 2: e.g., "Functions A and B in different modules duplicate logic"]
3. [Pain point 3: e.g., "Circular dependency between Module Y and Z"]
```

## Step 4: Research Target Patterns

**Browser: 8-15 searches**

### 4a: Pattern Best Practices
- Search for "[target pattern] [tech stack from TECH_STACK.md] best practices [current year]"
- Search for "[target pattern] [language] example project"
- Search for "[current pattern] to [target pattern] migration"
- Read official framework documentation on recommended project structure
- Find 2-3 well-structured open-source projects using the target pattern

### 4b: Refactoring Techniques
- Search for "[refactoring type] refactoring techniques [language]"
- Search for "[refactoring type] step by step migration"
- Search for "strangler fig pattern" or "branch by abstraction" if doing incremental migration
- Search for "[specific code smell being addressed] refactoring"
- Look for Martin Fowler's catalog entries or similar authoritative refactoring references

### 4c: Risk Research
- Search for "[refactoring type] pitfalls", "[refactoring type] lessons learned"
- Search for "[target pattern] common mistakes [tech stack]"
- Look for blog posts about failed refactoring attempts for cautionary patterns

Document findings:
```markdown
## Research Findings
### Target Pattern Evidence
| Source | Pattern Recommended | Key Insight | URL |
|--------|-------------------|-------------|-----|

### Migration Strategies Found
| Strategy | Applicability | Risk Level | URL |
|----------|--------------|-----------|-----|

### Pitfalls to Avoid
| Pitfall | How to Avoid | Source |
|---------|-------------|--------|
```

## Step 5: Define Target Structure

Based on research, define the exact target state:

```markdown
## Target Structure

### Module Map (Post-Refactor)

#### [Module/Package Name] (NEW / MODIFIED / RENAMED / SPLIT / MERGED)
**Path:** [new file path]
**Responsibility:** [single, clear responsibility]
**Public interface:**
```
[All exported functions, types, interfaces — exact signatures]
```
**Internal dependencies:** [imports — should be cleaner than before]
**External dependencies:** [unchanged unless cleaning up unused imports]

### Target Dependency Graph
```
[Cleaner dependency graph — no cycles, clear layer boundaries]
```

### Structural Improvements
| Before | After | Improvement |
|--------|-------|-------------|
| [Module X: 3 responsibilities] | [Module X + Module Y: 1 each] | Single responsibility |
| [Duplicated logic in A and B] | [Shared module C] | DRY |
| [Circular dep X <-> Y] | [X -> Z <- Y] | Acyclic dependency graph |
```

## Step 6: Define Invariants

This is the most critical section. These are the things that MUST NOT change:

```markdown
## Invariants — What Must NOT Change

### Observable Behavior
- [ ] All existing API endpoints return identical responses for identical requests
- [ ] All user-facing UI behavior is identical
- [ ] All error messages and error codes are identical
- [ ] All side effects (emails, notifications, webhook calls) fire identically
- [ ] All performance characteristics are equivalent or better

### API Contracts
[List every endpoint from API_CONTRACT.md that is in the refactoring scope]
| Endpoint | Method | Request Schema | Response Schema | Must Be Identical |
|----------|--------|---------------|-----------------|-------------------|
| [path] | [GET/POST/etc.] | [unchanged] | [unchanged] | YES |

### Public Interfaces
[List every public interface/export that other modules consume]
| Module | Export | Signature | Consumed By | Must Be Identical |
|--------|--------|-----------|-------------|-------------------|
| [module] | [function/type] | [signature] | [consumers] | YES / CHANGED (see migration) |

### Database Schema
- [ ] No schema changes (migrations) required
- [ ] All queries return identical results
- [ ] All data relationships preserved

### Test Results
- [ ] ALL existing unit tests pass WITHOUT modification
- [ ] ALL existing integration tests pass WITHOUT modification
- [ ] ALL existing E2E tests pass WITHOUT modification

**If any test needs to change:** This means the refactoring is changing behavior, not just structure. Document WHY the test change is safe and get user approval.

### Configuration
- [ ] Environment variables unchanged
- [ ] Configuration files unchanged
- [ ] Build output unchanged (or functionally equivalent)
```

## Step 7: Produce REFACTOR_SPEC.md

Write `.agents/handoff/REFACTOR_SPEC.md`:

```markdown
# Refactor Specification: [Refactoring Title]

## Summary
**Motivation:** [why this refactoring is needed]
**Scope:** [which modules/files are affected]
**Target pattern:** [what pattern we are moving toward]
**Risk level:** Low / Medium / High

## Current vs Target
| Aspect | Current | Target |
|--------|---------|--------|
| Module count | [N] | [M] |
| Dependency graph | [description] | [description] |
| Primary code smell | [description] | Eliminated |
| Testability | [description] | [description] |

---

## Invariants
[Full invariant list from Step 6 — this is the refactoring contract]

---

## Migration Path

The refactoring MUST be executed in this order. Each phase is independently safe — if execution stops at any phase boundary, the system is in a valid state.

### Phase 1: Preparation
**Goal:** Set up target structure alongside existing code

- [ ] Create new directory/file structure per Target Structure
- [ ] Add any new interfaces/types that will be needed
- [ ] Do NOT modify existing code yet
- **Verification:** All existing tests pass, no behavior changes

### Phase 2: Introduce Abstraction Layer (if applicable)
**Goal:** Create seam between old and new code

- [ ] Introduce interface/abstraction that both old and new implementations can satisfy
- [ ] Wire existing code through the new abstraction
- **Verification:** All existing tests pass, no behavior changes
- **Rollback:** Remove abstraction, revert to direct calls

### Phase 3: Migrate [Component/Module A]
**Goal:** Move first component to new structure

**Files affected:**
| File | Action | Description |
|------|--------|-------------|
| [old path] | MODIFY/DELETE | [what changes] |
| [new path] | CREATE/MODIFY | [what is created/changed] |

**Step-by-step:**
1. [Precise instruction 1]
2. [Precise instruction 2]
3. [Precise instruction N]

**Verification:**
- [ ] All existing tests pass
- [ ] [Specific check for this component]

**Rollback:** [How to undo this phase specifically]

### Phase 4: Migrate [Component/Module B]
[Same structure as Phase 3]

### Phase [N]: Cleanup
**Goal:** Remove old code, dead imports, temporary abstractions

- [ ] Remove old files/modules that are now unused
- [ ] Remove temporary abstraction layers (if no longer needed)
- [ ] Remove unused imports across the codebase
- [ ] Update any internal documentation or comments
- **Verification:** All tests pass, no dead code, linter clean

---

## Files Affected (Complete Inventory)

| File Path | Action | Phase | Description |
|-----------|--------|-------|-------------|
| [path] | CREATE | 1 | [new module] |
| [path] | MODIFY | 3 | [extract function to new module] |
| [path] | DELETE | N | [old module, fully migrated] |
| [path] | RENAME | 3 | [renamed for clarity] |

---

## Interface Changes

### Interfaces That Change (Internal Only)
| Interface | Current Signature | New Signature | Migration |
|-----------|------------------|---------------|-----------|
| [interface] | [old] | [new] | Phase [N]: update all callers |

### Interfaces That Must NOT Change (Public/External)
| Interface | Signature | Reason |
|-----------|-----------|--------|
| [interface] | [signature] | API contract / consumed by external modules |

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| [Subtle behavior change missed by tests] | [L/M/H] | [L/M/H] | [Run full E2E suite, manual verification of edge cases] |
| [Circular dependency introduced] | [L/M/H] | [L/M/H] | [Verify dependency graph after each phase] |
| [Performance regression] | [L/M/H] | [L/M/H] | [Benchmark before and after] |

---

## Research References
| Topic | Source | Key Takeaway | URL |
|-------|--------|-------------|-----|
```

## Step 8: Update ARCHITECTURE.md

Write the post-refactor target architecture as an addendum to `.agents/handoff/ARCHITECTURE.md`:

```markdown
## Refactor Target: [Refactoring Title]

### Post-Refactor Module Structure
[Updated module structure reflecting the target state]

### Post-Refactor Dependency Graph
[Updated dependency graph]

### Changes from Current Architecture
| Section | Change | Rationale |
|---------|--------|-----------|
| [section] | [what changed] | [why] |

**Note:** This section becomes the active architecture documentation once the refactor is complete. The previous structure documentation should be archived.
```

## Step 9: Update TESTING_STRATEGY.md

Append to `.agents/handoff/TESTING_STRATEGY.md`:

```markdown
## Refactoring: [Refactoring Title]

### Pre-Refactor Test Baseline
- Unit tests: [N passing]
- Integration tests: [N passing]
- E2E tests: [N passing]
- **All of these must pass identically after refactoring**

### Verification Approach per Phase
| Phase | Test Suite to Run | Expected Result |
|-------|------------------|-----------------|
| Phase 1 | Full suite | All pass, zero changes |
| Phase 2 | Full suite | All pass, zero changes |
| Phase 3 | Full suite | All pass, zero changes |
| Phase N | Full suite | All pass, zero changes |
| Final | Full suite + linter + dead code check | All pass, zero warnings |

### New Tests Added by Refactoring
| Test | Purpose | Module |
|------|---------|--------|
| [test] | [verify new module boundary works correctly] | [module] |
| [test] | [verify new abstraction layer is transparent] | [module] |

### Behavioral Equivalence Tests
[Tests that explicitly verify old behavior equals new behavior]
| Test | What It Verifies | Approach |
|------|-----------------|----------|
| [test] | [API response identical pre/post] | Snapshot comparison |
| [test] | [Query results identical pre/post] | Data comparison |

### Performance Baseline Tests
| Metric | Pre-Refactor Baseline | Acceptable Post-Refactor |
|--------|----------------------|--------------------------|
| [endpoint response time] | [Xms] | [<= Xms] |
| [bundle size] | [X KB] | [<= X KB] |
| [memory usage] | [X MB] | [<= X MB] |
```

## Step 10: Present Refactor Specification

Present to the user:

1. **Refactoring summary** — what is being restructured and why
2. **Scope** — number of files affected, modules changed
3. **Migration path** — number of phases, each independently safe
4. **Invariants** — what will NOT change (behavior, APIs, test results)
5. **Risk assessment** — what could go wrong and mitigations
6. **Research backing** — key patterns and best practices found

Ask the user: **"Refactor specification is ready. Each phase is independently safe with rollback points. Should I hand off to Claude Code?"**

## Step 11: Update Pipeline State

Update `.agents/handoff/PROJECT_STATE.md`:
```markdown
## Active Work: Refactoring — [Refactoring Title]
- REFACTOR_SPEC.md produced ([N] phases, [M] files affected)
- ARCHITECTURE.md updated with post-refactor target
- TESTING_STRATEGY.md updated with verification approach
- Invariant contract defined: [N] behavioral invariants, [M] interface invariants
- Awaiting handoff to Claude Code
```
