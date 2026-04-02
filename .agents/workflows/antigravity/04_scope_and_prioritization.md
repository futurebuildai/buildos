---
description: Define MVP scope, prioritize features using evidence-based frameworks, define success metrics and phased roadmap. Final pre-design gate.
stage: "04"
inputs: SOLUTION_CANDIDATES.md, FEASIBILITY_ASSESSMENT.md, BUILD_VS_BUY.md, JTBD_ANALYSIS.md, COMPETITIVE_LANDSCAPE.md
outputs: SCOPE_DEFINITION.md, METRICS_FRAMEWORK.md
browser_usage: Light-moderate (benchmarks, comparable launch strategies)
approval_gate: true
---

# Stage 04: Scope & Prioritization

Take the recommended solution and ruthlessly scope it into a shippable MVP. Every cut must be evidence-backed. Every inclusion must be justified.

**This stage ends with an APPROVAL GATE.** Present the scope and metrics to the user before proceeding to design.

---

## Step 1: Decompose the Solution

Break the recommended solution (from SOLUTION_CANDIDATES.md) into discrete capabilities:

1. List every feature/capability implied by the solution concept.
2. For each, note which JTBD outcome it addresses and its opportunity score.
3. Map dependencies between capabilities (which ones require others to exist first).

**Invoke skill:** `.agents/skills/antigravity/dependency_mapping.md`

## Step 2: Prioritize with Evidence

**Invoke skill:** `.agents/skills/antigravity/feature_prioritization.md`

Apply multiple frameworks and triangulate:

### MoSCoW Classification
For each capability:
- **Must Have** — MVP fails without it. Users cannot complete the core job.
- **Should Have** — Significant value, addresses a top-5 JTBD outcome, reasonable effort.
- **Could Have** — Nice-to-have, addresses lower-ranked outcomes.
- **Won't Have (this time)** — Explicitly deferred. Document the rationale.

### RICE Scoring
| Capability | Reach | Impact | Confidence | Effort | RICE Score |
|-----------|-------|--------|-----------|--------|------------|

### Kano Classification
For each Must/Should capability:
- **Basic** — expected, absence causes dissatisfaction (table stakes from competitive analysis)
- **Performance** — more is better, linear satisfaction
- **Delighter** — unexpected, creates disproportionate satisfaction

## Step 3: Define the Walking Skeleton

The MVP is not "all Must Haves." It is the **thinnest possible end-to-end slice** that delivers the core value:

1. Identify the single most important user journey (from USER_JOURNEYS.md).
2. Identify the minimum capabilities needed to complete that journey end-to-end.
3. That is the walking skeleton. Everything else layers on top.

```markdown
## Walking Skeleton
**Core Journey:** [Persona] → [Job] → [Outcome]
**Minimum Capabilities:**
1. [Capability] — enables [journey step]
2. [Capability] — enables [journey step]
...
**What the skeleton deliberately lacks:** [list with rationale]
```

## Step 4: Define Success Metrics

**Browser: 3-5 searches for benchmarks**

- Search for "[product type] benchmark metrics [current year]"
- Search for "[domain] activation rate benchmark", "[domain] retention benchmark"
- Look for industry reports with baseline metrics

### North Star Metric
Define the single metric that best captures value delivery:
- **Metric:** [definition]
- **Why this metric:** [rationale tied to JTBD core job]
- **Measurement:** [how to calculate]

### Input Metrics Tree
Decompose the north star:
```
North Star: [metric]
  ├── [Input 1]: [definition]
  │   ├── [Sub-input 1a]
  │   └── [Sub-input 1b]
  ├── [Input 2]: [definition]
  └── [Input 3]: [definition]
```

### Feature-Level Metrics
For each MVP capability:
| Capability | Adoption Metric | Engagement Metric | Success Target | Benchmark Source |
|-----------|----------------|-------------------|---------------|-----------------|

### Counter-Metrics (Guardrails)
For each optimization target, define what must NOT degrade:
| Optimization | Counter-Metric | Threshold |
|-------------|----------------|-----------|

### Instrumentation Plan
For each metric:
| Metric | Event/Data Point | Where Captured | Tool |
|--------|-----------------|----------------|------|

Write `.agents/handoff/METRICS_FRAMEWORK.md`.

## Step 5: Define Phased Roadmap

```markdown
## Phase 1: Walking Skeleton (MVP)
**Goal:** Prove the core value proposition
**Capabilities:** [list]
**Success Criteria:** [from metrics framework]
**Estimated Scope:** [T-shirt size]

## Phase 2: V1 (Complete Experience)
**Goal:** Full feature set for primary persona
**Capabilities:** [list — Must + Should Haves]
**Success Criteria:** [metrics targets]

## Phase 3: V2 (Growth & Expansion)
**Goal:** Expand to secondary personas, advanced features
**Capabilities:** [list — Could Haves + new ideas from V1 learnings]
```

## Step 6: Scope Risk Assessment

**Invoke skill:** `.agents/skills/antigravity/risk_assessment.md`

For each Must Have capability:
| Capability | Risk | Likelihood | Impact | Mitigation | Fallback if Late |
|-----------|------|-----------|--------|------------|-----------------|

## Step 7: Write Scope Definition

Write `.agents/handoff/SCOPE_DEFINITION.md`:
```markdown
# Scope Definition

## MVP Boundary

### Must Have (In Scope)
| # | Capability | JTBD Outcome | MoSCoW | RICE | Rationale |
|---|-----------|-------------|--------|------|-----------|

### Should Have (V1)
| # | Capability | JTBD Outcome | MoSCoW | RICE | Rationale |
|---|-----------|-------------|--------|------|-----------|

### Won't Have (Deferred)
| # | Capability | Rationale for Deferral | Target Phase |
|---|-----------|----------------------|-------------|

## Walking Skeleton
[From Step 3]

## Dependency Graph
[From Step 1]

## Phased Roadmap
[From Step 5]

## Risk Register
[From Step 6]

## Scope Lock Statement
The following capabilities constitute the MVP. Any additions require a scope renegotiation with the user. Removals require documented rationale.
[Explicit list of what is in the MVP]
```

## Step 8: APPROVAL GATE

**PAUSE HERE.** Present to the user:

1. The recommended solution (from Stage 03)
2. The MVP scope (Must Haves with rationale)
3. What is explicitly out of scope and why
4. The success metrics
5. The phased roadmap

Ask the user to review and approve before proceeding to design.

## Step 9: Update Pipeline State

Update `.agents/handoff/PIPELINE_STATE.md`:
```markdown
## Stage 04 — Scope & Prioritization ✅
- SCOPE_DEFINITION.md produced
- METRICS_FRAMEWORK.md produced
- MVP: [N] capabilities, [M] deferred
- ⏸️ APPROVAL GATE — awaiting user review
## Next Stage: 05 — Design System (after approval)
```
