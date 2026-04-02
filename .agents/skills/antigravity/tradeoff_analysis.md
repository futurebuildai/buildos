---
description: Structure decisions between competing options using weighted criteria scoring, sensitivity analysis, and reversibility assessment. Identifies one-way vs two-way door decisions and produces a defensible recommendation.
invoked_from:
  - workflows/antigravity/03_solution_design.md
  - workflows/antigravity/07_architecture_spec.md
  - On-demand for decision-making
produces: Decision record embedded in the calling artifact or standalone analysis
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: Tradeoff Analysis

When facing a decision between competing options (architectures, frameworks, approaches, vendors, features), use this structured process to make a defensible choice. The goal is not just to pick a winner — it is to document the reasoning so the decision can be revisited intelligently if circumstances change.

---

## Input

- **Decision to make** — a clear statement of what is being decided
- **Options** — at least 2, ideally 3-5 alternatives (including "do nothing" if applicable)
- **Context** — constraints, priorities, stakeholder concerns
- **Evaluation criteria** — what matters in this decision (or derive them in Step 1)

---

## Step 1: Frame the Decision

Clearly define the decision and its boundaries.

```markdown
## Decision Record: [Short Title]

**Decision Statement:** [What exactly are we deciding? Be specific.]
**Decision Date:** [When this decision is being made]
**Decision Maker:** [Who has the final call]
**Stakeholders:** [Who is affected by or cares about this decision]
**Context:** [Why is this decision needed now? What triggered it?]
**Constraints:**
- [Time constraint]
- [Budget constraint]
- [Technical constraint]
- [Organizational constraint]

**Options Under Consideration:**
1. [Option A: brief description]
2. [Option B: brief description]
3. [Option C: brief description]
4. [Status quo / do nothing: brief description] (if applicable)
```

---

## Step 2: Define Weighted Criteria

Identify the criteria that matter for this decision, then weight them by importance.

### Criteria Identification

Draw criteria from these categories:

| Category | Example Criteria |
|----------|-----------------|
| **User Impact** | UX quality, performance, accessibility, learning curve |
| **Technical** | Scalability, maintainability, complexity, security, reliability |
| **Business** | Cost, time-to-market, vendor risk, strategic alignment |
| **Team** | Skill availability, hiring market, learning curve for team |
| **Operational** | Monitoring, debugging, deployment complexity, SLA support |

### Weighting Rules

1. **Use 100-point budget.** Distribute 100 points across all criteria. This forces explicit trade-offs — you cannot make everything important.
2. **No criterion should exceed 30 points** unless the decision is truly single-dimensional.
3. **No criterion should be below 5 points.** If it is that unimportant, remove it.
4. **Document why** each criterion got its weight.

```markdown
## Evaluation Criteria

| # | Criterion | Weight | Rationale for Weight |
|---|-----------|--------|---------------------|
| C1 | [criterion] | [N]/100 | [why this matters this much] |
| C2 | [criterion] | [N]/100 | [why] |
| C3 | [criterion] | [N]/100 | [why] |
| C4 | [criterion] | [N]/100 | [why] |
| C5 | [criterion] | [N]/100 | [why] |
| | **Total** | **100** | |
```

---

## Step 3: Score Options

Rate each option against each criterion on a 1-5 scale.

### Scoring Scale

| Score | Meaning |
|-------|---------|
| 5 | Excellent — significantly better than alternatives on this criterion |
| 4 | Good — above average, clear strength |
| 3 | Adequate — meets the bar, no significant advantage or disadvantage |
| 2 | Below average — noticeable weakness, manageable |
| 1 | Poor — significant weakness, may require workaround or supplementation |

### Scoring Rules

1. **Score with evidence.** Every score of 5 or 1 must have a cited reason. Extreme scores without justification are suspect.
2. **Be relative, not absolute.** A score of 3 means "about as good as the other options" — it is the baseline.
3. **Invite devil's advocacy.** After scoring, ask: "Am I being fair to Option B? What would someone who prefers B say about my scores?"

### Raw Score Matrix

```markdown
## Option Scoring

| Criterion (Weight) | Option A | Option B | Option C | Notes |
|-------------------|----------|----------|----------|-------|
| C1: [name] ([N]) | [1-5] | [1-5] | [1-5] | [justification for any 1 or 5] |
| C2: [name] ([N]) | [1-5] | [1-5] | [1-5] | |
| C3: [name] ([N]) | [1-5] | [1-5] | [1-5] | |
| C4: [name] ([N]) | [1-5] | [1-5] | [1-5] | |
| C5: [name] ([N]) | [1-5] | [1-5] | [1-5] | |
```

---

## Step 4: Calculate Weighted Scores

```
Weighted Score = Sum of (Criterion Weight × Option Score for that Criterion) / 100
```

This produces a score on a 1-5 scale for each option.

```markdown
## Weighted Scores

| Criterion (Weight) | Option A (raw × weight) | Option B (raw × weight) | Option C (raw × weight) |
|-------------------|----------------------|----------------------|----------------------|
| C1 ([N]) | [score] × [weight] = [N] | ... | ... |
| C2 ([N]) | ... | ... | ... |
| C3 ([N]) | ... | ... | ... |
| C4 ([N]) | ... | ... | ... |
| C5 ([N]) | ... | ... | ... |
| **Total** | **[N]** | **[N]** | **[N]** |
| **Weighted Average** | **[N/100]** | **[N/100]** | **[N/100]** |
```

---

## Step 5: Sensitivity Analysis

Test how robust the ranking is by varying the weights.

### Weight Shift Scenarios

For each criterion, test what happens if its weight increases by 10 points (reducing the top-weighted criterion by 10):

```markdown
## Sensitivity Analysis

| Scenario | Weight Change | Option A Score | Option B Score | Option C Score | Winner Changes? |
|----------|--------------|---------------|---------------|---------------|----------------|
| Base case | — | [N] | [N] | [N] | — |
| C1 +10 | C1: [N]→[N+10], C[top]: [N]→[N-10] | [N] | [N] | [N] | Yes/No |
| C2 +10 | C2: [N]→[N+10], C[top]: [N]→[N-10] | [N] | [N] | [N] | Yes/No |
| C3 +10 | ... | ... | ... | ... | ... |
```

### Interpretation

- **If the winner never changes:** Decision is robust. High confidence.
- **If the winner changes in 1-2 scenarios:** Decision is moderately sensitive. Note the conditions under which the recommendation would flip.
- **If the winner changes frequently:** Decision is fragile. The options are very close; other factors (team preference, strategic alignment, gut feeling) should break the tie.

```markdown
### Sensitivity Assessment
**Robustness:** High / Moderate / Low
**The recommendation changes if:** [describe the scenario(s)]
**Most influential criterion:** [the one that, when shifted, most changes the ranking]
```

---

## Step 6: Reversibility Assessment

Classify the decision by how easily it can be reversed.

### One-Way Door vs Two-Way Door

| Type | Definition | Examples | Approach |
|------|-----------|----------|----------|
| **One-Way Door** | Difficult or impossible to reverse. High switching cost. | Database technology, public API contract, pricing model, core architecture | Invest heavily in analysis. Get it right. Take time. |
| **Two-Way Door** | Easily reversible. Low switching cost. | UI framework, internal tool choice, feature flag rollout, experiment design | Bias toward action. Pick the best option now, iterate. |

```markdown
## Reversibility Assessment

**Decision type:** One-Way Door / Two-Way Door
**Switching cost if we change later:** [Low / Medium / High / Prohibitive]
**Time to switch:** [hours / days / weeks / months]
**What we lose if we switch:** [sunk cost, migration effort, user disruption]
**Reversibility factors:**
- [Factor 1: e.g., "API contract is public — changing breaks clients"]
- [Factor 2: e.g., "Data format is internal — migration is straightforward"]

**Implication for decision rigor:**
[If one-way: "This is a high-stakes decision. The analysis must be thorough."
If two-way: "This is a low-stakes decision. Pick the current best option and move fast."]
```

---

## Step 7: Qualitative Factors

Some factors resist numerical scoring. Document them:

```markdown
## Qualitative Considerations

### Strategic Alignment
- Option A: [how it fits or conflicts with long-term strategy]
- Option B: [how it fits]
- Option C: [how it fits]

### Team Sentiment
- [Which option does the team prefer and why?]
- [Are there strong opinions that should be acknowledged?]

### Ecosystem Momentum
- [Which option has a growing vs shrinking ecosystem?]
- [Community trends, hiring market, investment signals]

### Risk Profile
- Option A: [primary risk — e.g., "less mature, higher uncertainty"]
- Option B: [primary risk — e.g., "vendor lock-in"]
- Option C: [primary risk — e.g., "team lacks expertise"]

### Gut Check
- [Does the quantitative winner feel right? If not, articulate why.]
- [What would a smart critic say about this choice?]
```

---

## Step 8: Produce Recommendation

```markdown
## Recommendation

**Recommended Option:** [Option N]
**Weighted Score:** [N] (vs [M] for runner-up)
**Decision Confidence:** High / Medium / Low
**Decision Type:** One-Way Door / Two-Way Door

### Why This Option
[2-3 sentences explaining the core reasoning. Lead with the strongest argument.]

### Why Not [Runner-Up Option]
[1-2 sentences explaining what it does worse. Be fair.]

### Why Not [Other Options]
[Brief elimination rationale for each]

### Key Trade-offs Accepted
By choosing [Option N], we accept:
1. [Trade-off 1 — what we give up] — Mitigation: [how we manage this]
2. [Trade-off 2] — Mitigation: [how]
3. [Trade-off 3] — Mitigation: [how]

### Conditions for Revisiting This Decision
Revisit if:
- [Condition 1 — e.g., "the library is deprecated"]
- [Condition 2 — e.g., "user research invalidates assumption X"]
- [Condition 3 — e.g., "team composition changes significantly"]

### Action Items
1. [Next step to implement the decision]
2. [Follow-up research or validation needed]
3. [Communication plan — who needs to know]
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] Decision statement is specific and unambiguous
- [ ] At least 3 options were considered (including status quo if applicable)
- [ ] Criteria weights sum to exactly 100
- [ ] Every extreme score (1 or 5) has a documented justification
- [ ] Sensitivity analysis tests at least 3 weight-shift scenarios
- [ ] Reversibility is explicitly assessed
- [ ] Recommendation includes trade-offs accepted and conditions for revisiting
- [ ] Devil's advocate perspective is represented (fairness to non-chosen options)
