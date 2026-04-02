---
description: Analyze dependencies between features, systems, or services. Produces a dependency graph in Mermaid format, identifies the critical path, bottlenecks, circular dependencies, and parallelization opportunities.
invoked_from:
  - workflows/antigravity/04_scope_and_prioritization.md
  - workflows/antigravity/07_architecture_spec.md
  - workflows/antigravity/09_execution_plan.md
  - skills/antigravity/user_story_decomposition.md
  - On-demand for dependency analysis
produces: Dependency graph and analysis embedded in SCOPE_DEFINITION.md, EXECUTION_PLAN.md, or standalone analysis
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: Dependency Mapping

Analyze a set of features, components, or services and map their dependencies to identify the critical path, bottlenecks, parallelization opportunities, and circular dependency risks. The output is a Mermaid dependency graph plus structured analysis.

---

## Input

- **Items to map** — features, user stories, components, services, or tasks
- **Known constraints** — team size, timeline, resource availability
- **Context** — what the dependency map will be used for (execution planning, risk assessment, architecture review)

---

## Step 1: Inventory the Items

List all items to be mapped with their key attributes:

```markdown
## Dependency Inventory

| ID | Item | Type | Estimated Effort | Owner/Team | Status |
|----|------|------|-----------------|------------|--------|
| F1 | [name] | Feature / Component / Service / Task | [T-shirt or days] | [team] | Planned / In Progress / Done |
| F2 | [name] | ... | ... | ... | ... |
| F3 | [name] | ... | ... | ... | ... |
```

---

## Step 2: Identify Dependencies

For each item, ask: "What must exist or be completed before this can start or function?"

### Dependency Types

| Type | Definition | Notation | Example |
|------|-----------|----------|---------|
| **Hard (Functional)** | Item B literally cannot work without Item A. Technical necessity. | A →→ B | "Login requires user registration to exist" |
| **Soft (Convenience)** | Item B could work without A, but building A first makes B easier or better. | A --> B | "Search filtering is easier to build after the search API exists" |
| **External** | Item depends on something outside the team's control. | ext→ B | "Payment integration requires merchant account approval" |
| **Data** | Item B needs data produced or structured by Item A. | A ==> B | "Dashboard requires analytics schema to be defined" |
| **API/Interface** | Item B consumes an interface defined by Item A. | A -.-> B | "Mobile app depends on REST API contract being stable" |

### Dependency Matrix

Build a matrix showing all relationships:

```markdown
## Dependency Matrix

| Item ↓ depends on → | F1 | F2 | F3 | F4 | F5 | F6 | F7 | F8 |
|---------------------|----|----|----|----|----|----|----|----|
| **F1** | — | | | | | | | |
| **F2** | H | — | | | | | | |
| **F3** | | S | — | | | | | |
| **F4** | H | | | — | | | | |
| **F5** | | | H | H | — | | | |
| **F6** | | | | | S | — | | |
| **F7** | | | | | | | — | E |
| **F8** | | | | | | | | — |

Legend: H = Hard, S = Soft, D = Data, A = API, E = External
```

---

## Step 3: Build the Dependency Graph

Produce a Mermaid diagram showing all dependencies:

```markdown
## Dependency Graph

\`\`\`mermaid
graph TD
    F1[F1: User Registration<br/>Effort: M] --> F2[F2: Email Verification<br/>Effort: S]
    F1 --> F4[F4: User Profile<br/>Effort: M]
    F2 -.-> F3[F3: Password Reset<br/>Effort: S]
    F4 --> F5[F5: Dashboard<br/>Effort: L]
    F3 --> F5
    F5 -.-> F6[F6: Dashboard Filters<br/>Effort: S]
    EXT1([Merchant Account]) --> F7[F7: Payment Integration<br/>Effort: L]
    F8[F8: Notification System<br/>Effort: M]

    classDef critical fill:#ff6b6b,stroke:#c92a2a,color:#fff
    classDef parallel fill:#69db7c,stroke:#2b8a3e,color:#fff
    classDef external fill:#ffd43b,stroke:#f08c00,color:#000
    classDef blocked fill:#ff8787,stroke:#e03131,color:#fff

    class F1,F4,F5 critical
    class F2,F3,F8 parallel
    class EXT1 external
\`\`\`

### Legend
- 🔴 Red: Critical path items
- 🟢 Green: Parallelizable items
- 🟡 Yellow: External dependencies
- Solid arrow (-->): Hard dependency
- Dashed arrow (-.->): Soft dependency
```

### Graph Construction Rules

1. **Direction is top-down or left-right.** Dependencies point from prerequisite to dependent item.
2. **Include effort estimates** in node labels to enable critical path analysis.
3. **Color-code** by critical path membership, parallelization opportunity, and external dependency status.
4. **Group related items** using Mermaid subgraphs when there are natural clusters.
5. **Keep it readable.** For more than 15 items, break into multiple focused graphs (by domain, by phase, by team).

---

## Step 4: Identify Critical Path

The critical path is the longest chain of dependent items from start to finish. It determines the minimum possible timeline.

### Critical Path Algorithm

1. **Start from items with no dependencies** (entry points).
2. **Trace forward** through hard dependencies, summing effort estimates.
3. **The longest path** from any entry point to any terminal item is the critical path.

```markdown
## Critical Path Analysis

### Critical Path
F1 (M: 5d) → F4 (M: 5d) → F5 (L: 10d) → F6 (S: 2d)
**Total critical path duration:** 22 days

### Near-Critical Paths
F1 (M: 5d) → F2 (S: 2d) → F3 (S: 2d) → F5 (L: 10d)
**Duration:** 19 days (3 days of slack)

### Implications
- **Minimum project duration:** 22 days (assuming one developer per item)
- **Items that cannot slip:** F1, F4, F5 (zero float)
- **Items with float:** F2 (3 days), F3 (3 days), F6 (0 days)
- **Bottleneck:** F5 (Dashboard) — longest single item on critical path
```

---

## Step 5: Detect Circular Dependencies

Circular dependencies are a structural problem that must be resolved.

### Detection

Walk the graph looking for cycles: A → B → C → A.

```markdown
## Circular Dependency Check

### Result: [No circular dependencies detected / Circular dependencies found]

### Cycles Found (if any)
| Cycle | Items | Resolution Strategy |
|-------|-------|-------------------|
| Cycle 1 | F3 → F7 → F3 | [Break by: defining a stable interface between F3 and F7, allowing parallel development against the interface] |
| Cycle 2 | ... | ... |

### Resolution Strategies
1. **Interface extraction:** Define a stable API contract between the circular items. Both develop against the contract independently.
2. **Merge:** If two items are truly circular, they may be one item artificially split. Consider merging.
3. **Stub/Mock:** Develop one item against a stub of the other, then integrate.
4. **Phased delivery:** Deliver a minimal version of Item A that unblocks Item B, then enhance Item A later.
```

---

## Step 6: Identify Bottlenecks

Bottlenecks are items that block disproportionately many downstream items.

```markdown
## Bottleneck Analysis

| Item | Blocks (directly) | Blocks (transitively) | Bottleneck Score | Risk |
|------|-------------------|----------------------|-----------------|------|
| F1 | F2, F4 | F2, F3, F4, F5, F6 | 5 items | High — delays here cascade to 5 downstream items |
| F5 | F6 | F6 | 1 item | Low |

### Bottleneck Mitigation Strategies

**F1 (User Registration) — blocks 5 downstream items:**
1. **Start first.** This must be the first item in development.
2. **Timebox aggressively.** Do not let scope expand. Ship the minimum that unblocks downstream.
3. **Define interface early.** Publish the API contract for F1 on day 1 so downstream teams can develop against stubs.
4. **Assign best developer.** Bottleneck items should not have uncertainty about capability.
5. **Have a fallback.** If F1 is delayed, what is the simplest possible shim that unblocks F2 and F4?
```

---

## Step 7: Parallelization Opportunities

Identify items that can proceed simultaneously:

```markdown
## Parallelization Opportunities

### Parallel Tracks

**Track 1 (Critical Path):**
F1 → F4 → F5 → F6

**Track 2 (Auth completion — after F1):**
F1 → F2 → F3

**Track 3 (Independent):**
F8 (Notification System — no dependencies, can start immediately)

**Track 4 (External dependency):**
F7 (Payment — blocked by external merchant account, start process immediately)

### Parallelism Summary
- **Maximum parallelism:** [N] tracks can proceed simultaneously
- **Minimum team size for full parallelism:** [N] developers
- **Duration with 1 developer:** [N] days (serial execution)
- **Duration with [N] developers:** [N] days (parallel execution)
- **Speedup factor:** [serial / parallel] = [N]x

### Recommended Sequencing
Given [N] available developers:

| Week | Developer 1 | Developer 2 | Developer 3 |
|------|------------|------------|------------|
| 1 | F1 (critical path) | F8 (independent) | External: merchant account |
| 2 | F4 (critical path) | F2 (after F1) | F7 prep (while waiting) |
| 3 | F5 (critical path) | F3 (after F2) | F7 (after approval) |
| 4 | F5 (continued) | F6 (after F5 — wait or help F5) | F7 (continued) |
```

---

## Step 8: External Dependency Plan

External dependencies require special handling because they are outside the team's control.

```markdown
## External Dependencies

| ID | External Dependency | Blocks | Expected Lead Time | Owner | Status | Mitigation |
|----|-------------------|--------|-------------------|-------|--------|-----------|
| EXT1 | Merchant account approval | F7 | 2-4 weeks | [person] | Not started | Start immediately; build against sandbox |
| EXT2 | [dependency] | [items] | [time] | [person] | [status] | [mitigation] |

### External Dependency Rules
1. **Start external dependencies first.** They have the least controllable timelines.
2. **Always have a workaround.** What can be built without the external dependency?
3. **Track actively.** Check status at least weekly.
4. **Escalation path.** Know who to contact if the dependency is delayed.
```

---

## Step 9: Summary and Recommendations

```markdown
## Dependency Map Summary

### Key Metrics
- **Total items:** [N]
- **Total dependencies:** [N] (H: [N], S: [N], E: [N], D: [N], A: [N])
- **Critical path length:** [N] items, [N] days
- **Maximum parallelism:** [N] concurrent tracks
- **External dependencies:** [N]
- **Circular dependencies:** [N] (must be resolved)
- **Primary bottleneck:** [item] (blocks [N] downstream items)

### Recommendations
1. **Start immediately:** [items with no dependencies — list them]
2. **Resolve first:** [circular dependencies or blockers — list them]
3. **Staff critical path:** [items on critical path that need the best developers]
4. **Initiate external dependencies now:** [external items with long lead times]
5. **Optimize for parallelism:** [specific suggestions for team allocation]

### Risk Flags
- [Any dependency chains that are fragile]
- [Any items with multiple hard dependencies (integration risk)]
- [Any external dependencies without fallback plans]
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] All items in the inventory are represented in the dependency graph
- [ ] Dependency types are correctly classified (hard vs soft vs external)
- [ ] The Mermaid graph renders correctly (no syntax errors)
- [ ] Critical path is identified and highlighted
- [ ] Circular dependencies are detected and have resolution plans
- [ ] Bottlenecks are identified with mitigation strategies
- [ ] Parallelization opportunities are mapped with team allocation suggestions
- [ ] External dependencies have lead times, owners, and fallback plans
- [ ] Effort estimates are included to enable timeline calculation
