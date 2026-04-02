---
description: Apply multiple prioritization frameworks (RICE, ICE, Kano, MoSCoW, WSJF) to a set of features or capabilities. Triangulate across frameworks to produce a combined, evidence-backed ranking with explicit rationale.
invoked_from:
  - workflows/antigravity/04_scope_and_prioritization.md
  - workflows/antigravity/feature_iteration.md
  - On-demand for feature prioritization
produces: Combined prioritization ranking embedded in SCOPE_DEFINITION.md or standalone analysis
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: Feature Prioritization (Multi-Framework)

Apply five complementary prioritization frameworks to the same feature set, then triangulate to produce a final ranking. No single framework is sufficient — each reveals different aspects of priority.

---

## Input

- **Feature/capability list** — from solution design, decomposition, or user-provided list
- **JTBD opportunity scores** — if available (from opportunity_scoring.md)
- **Competitive landscape data** — to inform table-stakes assessment
- **Technical feasibility estimates** — if available (from FEASIBILITY_ASSESSMENT.md)
- **User research data** — personas, journey maps, pain points

---

## Framework 1: MoSCoW Classification

Start here. MoSCoW establishes the categorical boundaries before you score.

### Definitions

| Category | Definition | Test |
|----------|-----------|------|
| **Must Have** | The product is unusable or unsaleable without this. | "If we ship without this, would users consider the product broken?" |
| **Should Have** | Important, delivers significant value, but the product functions without it. | "Would absence notably reduce satisfaction or adoption?" |
| **Could Have** | Desirable, improves experience, but easy to live without. | "Would users notice its absence in V1?" |
| **Won't Have (this time)** | Explicitly out of scope for this release. Acknowledged, not forgotten. | "Can this wait until after we validate the core?" |

### Classification Rules

1. **Must Haves should be <=40% of total effort.** If more, you are not scoping ruthlessly enough.
2. **Every Must Have needs a justification.** "Users expect it" is not enough — cite evidence (JTBD outcome, competitive parity, regulatory requirement).
3. **Won't Haves need a reason and a target phase.** This prevents "we will get to it" syndrome.
4. **Challenge every Must Have.** Ask: "What happens if we ship without this? Can we do a simpler version?" If the answer is "yes, a simpler version works," downgrade to Should Have with a scoped-down variant as the Must Have.

### Output

```markdown
## MoSCoW Classification

### Must Have
| # | Feature | Justification | JTBD Outcome Addressed |
|---|---------|--------------|----------------------|
| 1 | [feature] | [evidence-backed reason] | [outcome reference] |

### Should Have
| # | Feature | Justification | Target Phase |
|---|---------|--------------|-------------|

### Could Have
| # | Feature | Value | Target Phase |
|---|---------|-------|-------------|

### Won't Have (This Time)
| # | Feature | Rationale for Deferral | Target Phase |
|---|---------|----------------------|-------------|
```

---

## Framework 2: RICE Scoring

RICE quantifies priority by combining four dimensions. Apply to all Must Have and Should Have features.

### Dimensions

| Dimension | Definition | Scale | How to Estimate |
|-----------|-----------|-------|-----------------|
| **Reach** | How many users will this affect per quarter? | Absolute number or % of users | Use persona data, funnel estimates, or comparable metrics |
| **Impact** | How much will this move the target metric for each user reached? | 0.25 (Minimal), 0.5 (Low), 1 (Medium), 2 (High), 3 (Massive) | Use JTBD opportunity scores as input |
| **Confidence** | How sure are we about Reach and Impact estimates? | 100% (High), 80% (Medium), 50% (Low) | Based on evidence quality — data = 100%, research = 80%, gut = 50% |
| **Effort** | Person-weeks to implement. | Number (person-weeks) | Use T-shirt sizing converted to weeks; include design, dev, QA |

### Formula

```
RICE Score = (Reach × Impact × Confidence) / Effort
```

### Scoring Rules

1. **Be honest about Confidence.** If your Reach estimate is a guess, use 50%. Overconfident scoring defeats the purpose.
2. **Effort includes everything.** Design, development, testing, documentation, deployment. Not just "coding time."
3. **Use consistent scales.** If Reach is "% of users" for one feature, use % for all.
4. **Document assumptions** behind Reach and Effort estimates.

### Output

```markdown
## RICE Scoring

| # | Feature | Reach | Impact | Confidence | Effort (pw) | RICE Score |
|---|---------|-------|--------|-----------|-------------|------------|
| 1 | [feature] | [N] | [0.25-3] | [50-100%] | [N] | [calculated] |

*Sorted by RICE Score descending.*

### Key Assumptions
- Reach basis: [how Reach was estimated]
- Effort basis: [how Effort was estimated]
```

---

## Framework 3: ICE Scoring

ICE is faster and more intuitive than RICE. Use it as a cross-check.

### Dimensions

| Dimension | Definition | Scale |
|-----------|-----------|-------|
| **Impact** | How much will this improve the key metric? | 1-10 |
| **Confidence** | How sure are we that this will have the predicted impact? | 1-10 |
| **Ease** | How easy is this to implement? (Inverse of effort) | 1-10 |

### Formula

```
ICE Score = Impact × Confidence × Ease
```

Maximum score: 1000. In practice, top features score 300-600.

### Output

```markdown
## ICE Scoring

| # | Feature | Impact (1-10) | Confidence (1-10) | Ease (1-10) | ICE Score |
|---|---------|--------------|-------------------|-------------|-----------|
| 1 | [feature] | [N] | [N] | [N] | [calculated] |

*Sorted by ICE Score descending.*
```

---

## Framework 4: Kano Model Classification

Kano reveals the relationship between investment and user satisfaction — not all features create satisfaction linearly.

### Categories

| Category | Definition | Investment Strategy |
|----------|-----------|-------------------|
| **Basic (Must-be)** | Expected. Absence causes dissatisfaction, but presence does not delight. | Invest enough to meet expectations, no more. |
| **Performance (One-dimensional)** | More is better. Satisfaction scales linearly with execution quality. | Invest proportional to competitive differentiation goals. |
| **Delighter (Attractive)** | Unexpected. Absence is fine, but presence creates disproportionate delight. | Invest selectively — small effort, big wow factor. |
| **Indifferent** | Users do not care either way. | Do not invest. |
| **Reverse** | Some users actively dislike this feature. | Avoid or make optional. |

### Classification Method

For each feature, ask two questions:
1. "If this feature is present, how would users feel?" (Functional)
2. "If this feature is absent, how would users feel?" (Dysfunctional)

Use the Kano evaluation matrix:

| | Absent: Like | Absent: Expect | Absent: Neutral | Absent: Tolerate | Absent: Dislike |
|---|---|---|---|---|---|
| **Present: Like** | Questionable | Attractive | Attractive | Attractive | Performance |
| **Present: Expect** | Reverse | Indifferent | Indifferent | Indifferent | Basic |
| **Present: Neutral** | Reverse | Indifferent | Indifferent | Indifferent | Basic |
| **Present: Tolerate** | Reverse | Indifferent | Indifferent | Indifferent | Basic |
| **Present: Dislike** | Reverse | Reverse | Reverse | Reverse | Questionable |

### Grounding Rule

Base classifications on evidence:
- **Basic:** Identified through competitive parity analysis — every competitor does this.
- **Performance:** Identified through JTBD scoring — high importance outcomes.
- **Delighter:** Identified through whitespace analysis — no competitor does this, but user research suggests desire.

### Output

```markdown
## Kano Classification

| # | Feature | Category | Evidence | Investment Strategy |
|---|---------|----------|----------|-------------------|
| 1 | [feature] | Basic / Performance / Delighter / Indifferent | [evidence] | [strategy] |
```

---

## Framework 5: WSJF (Weighted Shortest Job First)

WSJF prioritizes by the cost of delay relative to job size. Ideal for time-sensitive features.

### Dimensions

| Dimension | Definition | Scale |
|-----------|-----------|-------|
| **User-Business Value** | Revenue impact, user satisfaction, strategic alignment | 1, 2, 3, 5, 8, 13 (Fibonacci) |
| **Time Criticality** | How much value decays with delay? Market window, competitor moves, seasonal | 1, 2, 3, 5, 8, 13 |
| **Risk Reduction / Opportunity Enablement** | Does this reduce risk or unlock future capabilities? | 1, 2, 3, 5, 8, 13 |
| **Job Size** | Relative effort to implement | 1, 2, 3, 5, 8, 13 |

### Formula

```
Cost of Delay = User-Business Value + Time Criticality + Risk Reduction
WSJF = Cost of Delay / Job Size
```

### Output

```markdown
## WSJF Scoring

| # | Feature | Business Value | Time Criticality | Risk Reduction | CoD | Job Size | WSJF |
|---|---------|---------------|-----------------|---------------|-----|----------|------|
| 1 | [feature] | [1-13] | [1-13] | [1-13] | [sum] | [1-13] | [calculated] |

*Sorted by WSJF descending.*
```

---

## Triangulation: Combined Ranking

Now combine all five frameworks into a single ranking.

### Step 1: Normalize Scores

Convert each framework's scores to a 1-10 scale:
```
Normalized = ((Score - Min) / (Max - Min)) × 9 + 1
```

### Step 2: Assign Framework Weights

Default weights (adjust based on context):

| Framework | Weight | Rationale |
|-----------|--------|-----------|
| MoSCoW | 30% | Categorical boundaries are the strongest signal |
| RICE | 25% | Most rigorous quantitative framework |
| Kano | 20% | Ensures you invest correctly in each category |
| WSJF | 15% | Captures time sensitivity |
| ICE | 10% | Cross-check on the quantitative scores |

For MoSCoW, convert to numeric: Must=10, Should=7, Could=4, Won't=1.

### Step 3: Calculate Combined Score

```
Combined = (MoSCoW_norm × 0.30) + (RICE_norm × 0.25) + (Kano_norm × 0.20) + (WSJF_norm × 0.15) + (ICE_norm × 0.10)
```

### Step 4: Produce Final Ranking

```markdown
## Combined Feature Priority Ranking

| Rank | Feature | MoSCoW | RICE | ICE | Kano | WSJF | Combined Score | Recommendation |
|------|---------|--------|------|-----|------|------|---------------|----------------|
| 1 | [feature] | Must | [N] | [N] | Perf | [N] | [N] | MVP — build first |
| 2 | ... | ... | ... | ... | ... | ... | ... | ... |

### Confidence Assessment
| Feature | Ranking Confidence | Key Uncertainty | What Would Change It |
|---------|-------------------|-----------------|---------------------|
| [feature] | High/Medium/Low | [what we're unsure about] | [what evidence would shift priority] |
```

---

## Disagreement Analysis

When frameworks disagree, it reveals important nuances:

```markdown
## Framework Disagreements

| Feature | Framework A says | Framework B says | Resolution |
|---------|-----------------|-----------------|------------|
| [feature] | RICE: high (broad reach) | Kano: Basic (not differentiating) | Build it, but do not over-invest — meet expectations only |
| [feature] | MoSCoW: Should | WSJF: top (time-critical) | Elevate to Must — market window justifies urgency |
```

Document every disagreement and your resolution rationale. Disagreements are where the most important strategic thinking happens.

---

## Quality Checklist

Before finalizing, verify:
- [ ] All Must Haves are <=40% of total estimated effort
- [ ] Every Must Have has evidence-backed justification
- [ ] RICE Confidence scores honestly reflect evidence quality
- [ ] Kano classifications are grounded in competitive/user research data
- [ ] WSJF Time Criticality scores reflect real deadlines, not artificial urgency
- [ ] Combined ranking resolves framework disagreements explicitly
- [ ] Each feature's ranking confidence is assessed
- [ ] Won't Haves have documented rationale and target phase
