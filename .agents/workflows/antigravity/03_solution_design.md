---
description: Generate, evaluate, and select solution concepts through structured ideation, technical feasibility assessment, and build-vs-buy analysis with browser research.
stage: "03"
inputs: All Stage 00-02 artifacts
outputs: SOLUTION_CANDIDATES.md, FEASIBILITY_ASSESSMENT.md, BUILD_VS_BUY.md
browser_usage: Moderate-heavy (library research, API discovery, SaaS evaluation)
---

# Stage 03: Solution Design

Transform validated user needs into concrete solution concepts. This stage bridges "what users need" to "what we will build."

---

## Step 1: Frame the Solution Space

Read all upstream artifacts. Synthesize the design constraints:

```markdown
## Solution Constraints
- Must address top [N] underserved outcomes from JTBD_ANALYSIS.md
- Must serve [primary persona] first
- Must differentiate from [competitors] in [whitespace areas]
- Tech stack: [from TECH_STACK.md]
- Known feasibility constraints: [from RESEARCH_FINDINGS.md technical section]
```

## Step 2: Divergent Ideation

Generate **minimum 5 solution concepts** using different innovation lenses:

1. **Direct solution** — the most obvious approach that addresses the top outcomes head-on
2. **Analogy** — how does a completely different industry solve a similar problem? Search: "[different industry] + [core job]"
3. **Inversion** — what if we removed a step entirely? What if we did the opposite of competitors?
4. **Combination** — merge two existing approaches or tools into something new
5. **Simplification** — what is the absolute simplest thing that could work? What if we eliminated 80% of features?
6. **(Bonus) Platform play** — what if this was a platform others build on instead of an end-user tool?
7. **(Bonus) AI-native** — what if AI/ML was a core capability rather than an add-on?

For each concept, write:
- **Name** — a memorable handle
- **One-liner** — what it is in one sentence
- **How it works** — 3-5 bullet description
- **Key differentiator** — why this approach is unique
- **Which outcomes it addresses** — map to JTBD opportunity scores
- **Initial risk assessment** — what could go wrong

## Step 3: Technical Feasibility Research

**Browser: 5-15 searches per concept**

For each promising concept (top 3-4):

1. **Library/framework research:**
   - Search for "[capability] [tech stack language] library [current year]"
   - For each candidate library: check GitHub (stars, last commit, open issues, license)
   - Read the README and quickstart docs
   - Check for known issues: "[library] problems", "[library] alternatives"

2. **API/service availability:**
   - Search for "[capability] API", "[capability] SaaS"
   - Check pricing, rate limits, reliability
   - Read API documentation to assess integration complexity

3. **Architecture feasibility:**
   - Search for "[pattern] architecture [tech stack]"
   - Find examples of similar systems built with the same tech stack
   - Identify potential performance bottlenecks or scaling challenges

4. **Risk research:**
   - Search for "[approach] pitfalls", "[approach] lessons learned"
   - Look for postmortems from teams that tried similar approaches

**Invoke skill:** `.agents/skills/antigravity/risk_assessment.md` for each concept.

Write `.agents/handoff/FEASIBILITY_ASSESSMENT.md`:
```markdown
# Technical Feasibility Assessment

## Per-Concept Assessment

### [Concept Name]
**Overall Feasibility:** 🟢 Green / 🟡 Yellow / 🔴 Red

| Capability | Feasibility | Approach | Libraries/Tools | Risks | Sources |
|-----------|------------|---------|----------------|-------|---------|

**Unknowns Requiring Spikes:**
- [Unknown 1]: Time-boxed to [X hours], go/no-go criteria: [criteria]

**Technical Constraints:**
- [Constraint 1 that architecture must respect]

## Cross-Concept Comparison
| Dimension | Concept A | Concept B | Concept C |
|-----------|----------|----------|----------|
| Complexity | | | |
| Time to MVP | | | |
| Scalability | | | |
| Maintainability | | | |
| Risk Level | | | |
```

## Step 4: Build-vs-Buy Analysis

For each major capability identified in the concepts:

**Browser: 3-5 searches per capability**

- Search for "[capability] SaaS", "[capability] open source", "[capability] managed service"
- For each option: check pricing, features, limitations, lock-in risk
- Read comparison articles: "[option A] vs [option B]"

**Invoke skill:** `.agents/skills/antigravity/tradeoff_analysis.md` for significant decisions.

Write `.agents/handoff/BUILD_VS_BUY.md`:
```markdown
# Build vs. Buy Analysis

| Capability | Build | Buy (Product) | OSS | Recommendation | Rationale |
|-----------|-------|---------------|-----|----------------|-----------|

## Detailed Analysis

### [Capability 1]
**Options:**
- Build: [effort estimate, ongoing maintenance]
- [Product A]: [price, features, limitations, lock-in]
- [OSS Option]: [maturity, community, maintenance burden]
**5-Year TCO:** Build: $X | Buy: $Y | OSS: $Z
**Recommendation:** [choice] because [evidence-based rationale]
```

## Step 5: Evaluate and Select

Score each concept against the solution criteria:

**Invoke skill:** `.agents/skills/antigravity/feature_prioritization.md` (using RICE framework on concepts)

| Criteria | Weight | Concept A | Concept B | Concept C |
|----------|--------|----------|----------|----------|
| Addresses top JTBD outcomes | 30% | | | |
| Technical feasibility | 20% | | | |
| Differentiation from competitors | 20% | | | |
| Time to MVP | 15% | | | |
| Scalability potential | 10% | | | |
| Team/stack fit | 5% | | | |
| **Weighted Score** | | | | |

## Step 6: Write Recommendation

Write `.agents/handoff/SOLUTION_CANDIDATES.md`:
```markdown
# Solution Candidates

## Problem Restatement
[From VISION_BRIEF + validated by research]

## Solution Criteria
[What a good solution must do, derived from JTBD + competitive analysis]

## Evaluated Concepts
### Concept 1: [Name]
...
### Concept 2: [Name]
...

## Evaluation Matrix
[Scoring table from Step 5]

## Recommended Approach: [Concept Name]
**Rationale:**
- Addresses [X/Y] of the top underserved outcomes
- Technically feasible because [evidence from feasibility research]
- Differentiates through [whitespace from competitive analysis]
- Achievable as MVP in [estimated timeframe] because [build-vs-buy decisions]

**Key Risks and Mitigations:**
| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|

**What We Are NOT Building:**
[Explicitly rejected approaches and why]
```

## Step 7: TECH_STACK.md Check

If `TECH_STACK.md` is still empty, this is the point where it MUST be filled in. Based on the feasibility assessment and build-vs-buy decisions, either:
- Ask the user to fill in `TECH_STACK.md`
- Propose a tech stack with rationale and ask for approval

## Step 8: Update Pipeline State

Update `.agents/handoff/PIPELINE_STATE.md`:
```markdown
## Stage 03 — Solution Design ✅
- SOLUTION_CANDIDATES.md produced ([N] concepts evaluated)
- FEASIBILITY_ASSESSMENT.md produced
- BUILD_VS_BUY.md produced
- Recommended approach: [Concept Name]
## Next Stage: 04 — Scope & Prioritization
```

Proceed to Stage 04.
