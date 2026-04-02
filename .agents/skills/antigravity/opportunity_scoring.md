---
description: Apply the Jobs-to-be-Done (JTBD) opportunity scoring algorithm to rank user outcomes by underservedness. Calculates Opportunity Score from Importance and Satisfaction ratings to identify where the biggest product opportunities lie.
invoked_from:
  - workflows/antigravity/02_user_research.md
  - workflows/antigravity/04_scope_and_prioritization.md
  - On-demand for opportunity analysis
produces: Ranked opportunity scores embedded in JTBD_ANALYSIS.md or standalone analysis
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: Opportunity Scoring (JTBD)

Apply the Ulwick Opportunity Scoring algorithm to a set of user outcomes. This identifies which outcomes are most underserved — the biggest gaps between what users need and what current solutions deliver.

---

## Input

- **A list of desired outcomes** — from JTBD analysis, user research, or user-provided list
- **Context on current solutions** — what exists today that addresses these outcomes (from competitive analysis)
- Optional: **Survey data or review evidence** to ground the ratings

Each outcome should follow the JTBD outcome format:
`[Direction] + [metric] + [object of control] + [contextual clarifier]`

Example: "Minimize the time it takes to find relevant search results when exploring a new topic"

---

## Step 1: Define the Outcome Set

List all outcomes to be scored. If outcomes are not yet in JTBD format, rewrite them:

### Outcome Formatting Rules

- Start with a direction verb: Minimize, Maximize, Increase, Reduce, Avoid, Ensure
- Include a measurable metric: time, likelihood, number, frequency, effort
- Specify the object of control: what the user is acting on
- Add context: when/where this outcome matters

**Good:** "Minimize the time it takes to set up a new project with the correct configuration"
**Bad:** "Easy project setup" (vague, not measurable, no context)

```markdown
## Outcome Set
| # | Outcome Statement | Job Step | Persona |
|---|-------------------|----------|---------|
| 1 | [JTBD-formatted outcome] | [which step in the job] | [which persona] |
| 2 | ... | ... | ... |
```

---

## Step 2: Rate Importance (1-10)

For each outcome, rate how important it is to the user on a 1-10 scale.

### Rating Guidance

| Score | Meaning | Evidence Indicators |
|-------|---------|-------------------|
| 9-10 | Critical — job fails without this | Users mention this unprompted; top review theme; workaround attempts are desperate |
| 7-8 | Very important — significant pain if unmet | Frequently mentioned in reviews; users pay premiums for solutions addressing this |
| 5-6 | Moderately important — noticeable but livable | Mentioned occasionally; some users care, others do not |
| 3-4 | Low importance — nice to have | Rarely mentioned; only power users care |
| 1-2 | Minimal importance — barely registers | Almost never comes up; users do not think about this |

### Grounding Rules

- **Use evidence, not intuition.** Ground ratings in user research, review analysis, forum discussions.
- **Rate from the user's perspective,** not yours. What matters to them, not what you think should matter.
- **Consider frequency.** An outcome that matters every time the job is performed rates higher than one that matters occasionally.
- **Document your reasoning** for any score above 7 or below 4 — these are the extremes that drive decisions.

---

## Step 3: Rate Satisfaction (1-10)

For each outcome, rate how well current solutions (competitors, workarounds, status quo) satisfy this outcome on a 1-10 scale.

### Rating Guidance

| Score | Meaning | Evidence Indicators |
|-------|---------|-------------------|
| 9-10 | Fully satisfied — existing solutions nail this | No complaints in reviews; competitors execute flawlessly |
| 7-8 | Well satisfied — minor gaps only | Occasional complaints; solutions work but are not perfect |
| 5-6 | Moderately satisfied — noticeable gaps | Regular complaints; workarounds needed sometimes |
| 3-4 | Poorly satisfied — significant gaps | Major complaint theme; users actively seek alternatives |
| 1-2 | Barely/not satisfied — wide open | No viable solution exists; users are desperate or resigned |

### Grounding Rules

- **Evaluate the best available solution,** not the average. If one competitor satisfies this well, satisfaction is high even if others do not.
- **Include non-obvious solutions.** Spreadsheets, email, manual processes, and "just don't do it" are all competing solutions.
- **Weight by market share.** If the dominant solution does poorly but a niche product does well, satisfaction should be moderate (most users experience the dominant solution).

---

## Step 4: Calculate Opportunity Scores

For each outcome, calculate:

```
Opportunity Score = Importance + (Importance - Satisfaction)
```

This formula gives extra weight to outcomes that are both important AND unsatisfied. The maximum possible score is 20 (Importance=10, Satisfaction=0).

### Score Interpretation

| Score Range | Classification | Interpretation |
|------------|----------------|----------------|
| 15-20 | Critical Opportunity | Extremely underserved. Build this — it is the core differentiator. |
| 12-14.9 | High Opportunity | Significantly underserved. Strong candidate for MVP inclusion. |
| 10-11.9 | Moderate Opportunity | Worth addressing but not urgent. Good for V1 or V2. |
| 6-9.9 | Low Opportunity | Adequately served or low importance. Deprioritize unless table stakes. |
| < 6 | Over-Served / Irrelevant | Current solutions already satisfy this. Do not invest here unless for parity. |

### Edge Cases

- **High Importance + High Satisfaction (e.g., I=9, S=8 -> Score=10):** This is table stakes. Users expect it but competitors already do it well. You must match, but it will not differentiate.
- **Low Importance + Low Satisfaction (e.g., I=3, S=2 -> Score=4):** Nobody cares and nobody does it. Ignore.
- **High Importance + Low Satisfaction (e.g., I=9, S=2 -> Score=16):** The sweet spot. This is where you win.

---

## Step 5: Produce the Scoring Table

```markdown
## Opportunity Scoring Results

| # | Outcome | Importance | Satisfaction | Opp Score | Classification | Rationale |
|---|---------|-----------|-------------|-----------|---------------|-----------|
| 1 | [outcome] | [1-10] | [1-10] | [calculated] | Critical/High/Moderate/Low | [brief evidence note] |
| 2 | ... | ... | ... | ... | ... | ... |

*Sorted by Opportunity Score descending.*
```

---

## Step 6: Identify Opportunity Clusters

Group high-scoring outcomes by theme or job step:

```markdown
## Opportunity Clusters

### Cluster 1: [Theme — e.g., "Initial Setup Experience"]
- Outcome #X (Score: [N])
- Outcome #Y (Score: [N])
- **Combined signal:** [What this cluster tells us about where to focus]

### Cluster 2: [Theme]
- ...
```

Clusters reveal strategic themes, not just individual features. A cluster of 3 outcomes scoring 12-14 is often more important than a single outcome at 16.

---

## Step 7: Sensitivity Check

For your top 5 outcomes, test how sensitive the ranking is to rating changes:

```markdown
## Sensitivity Analysis

| Outcome | Current Score | If Importance +/-1 | If Satisfaction +/-1 | Robust? |
|---------|--------------|--------------------|--------------------|---------|
| [outcome] | [score] | [range] | [range] | Yes/No |
```

If a small rating change would significantly alter the ranking, flag the outcome as "rating-sensitive" and note that additional user research is needed to validate the score.

---

## Step 8: Recommendations

```markdown
## Recommendations

### Must Address (Critical + High Opportunity)
These outcomes should directly shape the MVP feature set:
1. [Outcome] (Score: [N]) — Suggests we need: [feature/capability implication]
2. ...

### Monitor (Moderate Opportunity)
Worth addressing in V1/V2 but not MVP-critical:
1. [Outcome] (Score: [N])
2. ...

### Table Stakes (High Importance, High Satisfaction)
Competitors already do this well — we must match but not over-invest:
1. [Outcome] (I: [N], S: [N])
2. ...

### Deprioritize (Low Opportunity / Over-Served)
Explicitly do NOT invest in these:
1. [Outcome] (Score: [N]) — Why: [reason]
2. ...
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] All outcomes are in proper JTBD format (direction + metric + object + context)
- [ ] Importance and Satisfaction ratings are grounded in evidence, not guesses
- [ ] Scores above 7 and below 4 have documented rationale
- [ ] Opportunity Scores are correctly calculated (Importance + (Importance - Satisfaction))
- [ ] Results are sorted by score descending
- [ ] Clusters are identified and interpreted
- [ ] Sensitivity analysis is done for top 5 outcomes
- [ ] Recommendations link outcomes to concrete feature implications
