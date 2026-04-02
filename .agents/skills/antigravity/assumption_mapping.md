---
description: Extract assumptions from any artifact, categorize by type (Desirability, Viability, Feasibility), rate confidence levels, plot on a risk matrix, and design lightweight validation experiments for high-risk assumptions.
invoked_from:
  - workflows/antigravity/00_vision_intake.md
  - workflows/antigravity/03_solution_design.md
  - workflows/antigravity/10_spec_review.md
  - On-demand for assumption auditing
produces: ASSUMPTIONS_REGISTER.md or assumption section in calling artifact
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: Assumption Mapping

Every product decision rests on assumptions. This skill systematically surfaces them, rates their risk, and designs experiments to validate the riskiest ones before committing resources.

---

## Input

- **Artifact(s) to audit** — any document containing decisions, strategies, or plans (e.g., VISION_BRIEF.md, SOLUTION_CANDIDATES.md, SCOPE_DEFINITION.md)
- **Context** — stage of the pipeline (earlier stages have more assumptions; later stages should have fewer)

---

## Step 1: Extract Assumptions

Read the input artifact(s) and identify every assumption. An assumption is any belief that must be true for a decision to be correct, but which has not been validated with evidence.

### Where Assumptions Hide

Look for these linguistic signals:

| Signal | Example | Hidden Assumption |
|--------|---------|------------------|
| "Users will..." | "Users will prefer a CLI interface" | Users actually want a CLI over a GUI |
| "The market is..." | "The market is growing" | Growth rate and trajectory are as estimated |
| "We can..." | "We can integrate with their API" | The API exists, is stable, and has the needed endpoints |
| "It should take..." | "It should take 2 weeks to build" | Scope is well-understood, no unknowns, team has skills |
| "This will improve..." | "This will improve retention by 15%" | Causal link between feature and retention exists |
| "Based on..." | "Based on competitor analysis" | Competitor data is current and representative |
| Absence of alternatives | Only one solution considered | There is no better approach |
| Unstated dependencies | "We will use Stripe" | Stripe supports our use case, pricing works at our scale |

### Extraction Rules

1. **Be aggressive.** Extract more assumptions than you think exist. It is better to surface 50 assumptions and discard 30 than to miss 5 critical ones.
2. **Make implicit assumptions explicit.** If the artifact says "we will build a real-time dashboard," the implicit assumptions include: users need real-time (not near-real-time), the data sources support real-time, the infrastructure can handle real-time.
3. **Check every number.** Every metric, estimate, timeline, and cost figure has assumptions behind it.
4. **Question the problem definition.** The biggest assumption is often "this is the right problem to solve."

---

## Step 2: Categorize Assumptions

Classify each assumption into one of three categories:

### Desirability Assumptions
"Do users actually want this?"

Examples:
- Users have this problem frequently enough to seek a solution
- Users would switch from their current solution to ours
- The target persona accurately represents real users
- Users value [feature] enough to pay for it
- Users prefer [approach A] over [approach B]

### Viability Assumptions
"Can this sustain a business?"

Examples:
- Users will pay $X/month for this
- The market is large enough to sustain the business
- Customer acquisition cost will be less than lifetime value
- The pricing model aligns with how users perceive value
- Revenue will cover infrastructure costs at scale
- There are no regulatory barriers to this approach

### Feasibility Assumptions
"Can we actually build this?"

Examples:
- The technology stack can support this at scale
- Third-party APIs/services will remain available and stable
- The team has the skills to build this in the estimated time
- Performance requirements are achievable with the chosen architecture
- Data is available in the format and quality needed
- No patents or IP issues block this approach

---

## Step 3: Rate Confidence

For each assumption, rate two dimensions:

### Confidence Level (How sure are we this is true?)

| Level | Score | Description |
|-------|-------|-------------|
| Validated | 5 | Proven with direct evidence (data, user testing, prototype) |
| High | 4 | Strong indirect evidence (research, analogies, expert opinion) |
| Medium | 3 | Some supporting evidence but gaps remain |
| Low | 2 | Mostly gut feeling; minimal evidence |
| Unknown | 1 | Pure speculation; no evidence at all |

### Impact If Wrong (What happens if this assumption is false?)

| Level | Score | Description |
|-------|-------|-------------|
| Fatal | 5 | Product/business fails entirely |
| Severe | 4 | Major rework needed; significant time/money loss |
| Significant | 3 | Feature fails or requires substantial modification |
| Moderate | 2 | Suboptimal outcome but recoverable with minor changes |
| Minimal | 1 | Barely noticeable; easy to adjust |

### Risk Score

```
Risk Score = Impact If Wrong × (6 - Confidence Level)
```

This formula ensures that low-confidence, high-impact assumptions surface as the riskiest. Maximum risk score: 25 (Impact=5, Confidence=1).

---

## Step 4: Build the Assumptions Register

```markdown
## Assumptions Register

| # | Assumption | Category | Confidence | Impact | Risk Score | Status | Validation Method |
|---|-----------|----------|-----------|--------|-----------|--------|------------------|
| A1 | [assumption] | Desirability | [1-5] | [1-5] | [calculated] | Unvalidated | [method] |
| A2 | [assumption] | Viability | [1-5] | [1-5] | [calculated] | Unvalidated | [method] |
| A3 | [assumption] | Feasibility | [1-5] | [1-5] | [calculated] | Unvalidated | [method] |

*Sorted by Risk Score descending.*

### Status Legend
- **Unvalidated** — Not yet tested
- **In Validation** — Experiment in progress
- **Validated** — Confirmed true with evidence
- **Invalidated** — Confirmed false; requires pivot
- **Partially Validated** — Partially true; needs refinement
```

---

## Step 5: Plot Risk Matrix

Create a visual risk matrix grouping assumptions by urgency:

```markdown
## Risk Matrix

                    Impact If Wrong →
                    Low(1)    Moderate(2)    Significant(3)    Severe(4)    Fatal(5)
Confidence  ↓
Unknown (1)         |  Monitor  |   Research   |    VALIDATE     |  VALIDATE   | VALIDATE NOW |
Low (2)             |  Monitor  |   Research   |    VALIDATE     |  VALIDATE   | VALIDATE NOW |
Medium (3)          |  Accept   |   Monitor    |    Research     |  VALIDATE   | VALIDATE NOW |
High (4)            |  Accept   |   Accept     |    Monitor      |  Research   |   VALIDATE   |
Validated (5)       |  Accept   |   Accept     |    Accept       |  Accept     |   Monitor    |
```

### Action Categories

| Action | Meaning | When |
|--------|---------|------|
| **VALIDATE NOW** | Stop and test before proceeding | Risk Score >= 15 |
| **VALIDATE** | Design experiment, run before major investment | Risk Score 10-14 |
| **Research** | Gather more evidence via browser research | Risk Score 6-9 |
| **Monitor** | Note it, revisit if new info surfaces | Risk Score 3-5 |
| **Accept** | Low risk, move forward | Risk Score 1-2 |

---

## Step 6: Design Validation Experiments

For every assumption in the VALIDATE NOW or VALIDATE category, design a lightweight experiment.

### Experiment Design Template

```markdown
### Experiment: Validate Assumption [#]

**Assumption:** [statement]
**Risk Score:** [N]
**Category:** Desirability / Viability / Feasibility

**Experiment Type:** [see options below]
**Hypothesis:** If [assumption] is true, then [observable outcome]
**Method:** [step-by-step what to do]
**Success Criteria:** [specific, measurable threshold]
**Failure Criteria:** [what tells us the assumption is wrong]
**Time Required:** [hours/days]
**Resources Required:** [tools, people, money]
**Decision:** If validated → [next action]. If invalidated → [pivot plan]
```

### Experiment Types by Category

**Desirability Validation:**
- Fake door test — build a button/page for the feature, measure click-through rate
- Landing page test — describe the product, measure signup intent
- User interviews — 5-8 target users, ask about the problem (not the solution)
- Survey — quantitative validation with larger sample
- Concierge test — manually deliver the value to 5-10 users, observe response
- Search volume analysis — use Google Trends / keyword tools to measure demand

**Viability Validation:**
- Pricing page test — present pricing, measure conversion
- Pre-order / waitlist — gauge willingness to pay before building
- Letter of intent — get prospective customers to commit (B2B)
- Unit economics model — calculate CAC, LTV, margins with best estimates
- Competitive pricing analysis — compare willingness to pay against market rates

**Feasibility Validation:**
- Technical spike — build the riskiest piece in a time-boxed prototype
- API/library evaluation — test the actual integration, not just the docs
- Load test — simulate expected traffic against a prototype
- Expert review — have a domain expert assess the approach
- Proof of concept — minimal working demo of the core technical challenge

---

## Step 7: Validation Prioritization

Sequence the experiments by risk score and dependency:

```markdown
## Validation Sequence

### Immediate (Before Proceeding)
| Priority | Assumption | Risk Score | Experiment | Duration |
|----------|-----------|-----------|------------|----------|
| 1 | [assumption] | [N] | [experiment type] | [time] |
| 2 | [assumption] | [N] | [experiment type] | [time] |

### Before Major Investment (Before Build Starts)
| Priority | Assumption | Risk Score | Experiment | Duration |
|----------|-----------|-----------|------------|----------|

### During Build (Can Validate in Parallel)
| Priority | Assumption | Risk Score | Experiment | Duration |
|----------|-----------|-----------|------------|----------|
```

---

## Step 8: Link to Decisions

Map high-risk assumptions back to the decisions they underpin:

```markdown
## Assumption-Decision Map

| Decision | Depends On Assumptions | Max Risk Score | Confidence in Decision |
|----------|----------------------|---------------|----------------------|
| [decision from artifact] | A1, A3, A7 | [highest risk among them] | Low / Medium / High |
| [decision] | A2, A5 | [N] | ... |

### Decisions at Risk
[List any decisions that depend on assumptions with Risk Score >= 15.
These decisions should be flagged as provisional until assumptions are validated.]
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] At least 15-30 assumptions extracted (fewer suggests insufficient rigor)
- [ ] Assumptions span all three categories (Desirability, Viability, Feasibility)
- [ ] No assumption is both high-confidence AND has "no evidence" noted
- [ ] Risk scores are correctly calculated
- [ ] Every VALIDATE NOW assumption has an experiment designed
- [ ] Experiments have specific success/failure criteria (not vague)
- [ ] Validation sequence accounts for dependencies
- [ ] High-risk assumptions are linked back to the decisions they affect
