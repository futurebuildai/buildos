---
description: Fermi estimation for feature impact using funnel analysis, cohort modeling, or comparable benchmarks. Produces a range estimate (low/mid/high) with sensitivity analysis showing which assumptions drive the most variance.
invoked_from:
  - workflows/antigravity/04_scope_and_prioritization.md
  - workflows/antigravity/06_product_spec.md
  - workflows/antigravity/feature_iteration.md
  - On-demand for impact sizing
produces: Impact estimate section in PRD, SCOPE_DEFINITION.md, or standalone analysis
browser_usage: Light (benchmark data lookups when needed)
---

# Skill: Impact Estimation

Estimate the quantitative impact of a feature or change before building it. Use Fermi estimation techniques to produce a defensible range rather than a false-precision point estimate.

---

## Input

- **Feature or change description** — what is being estimated
- **Target metric** — what metric we expect to move (e.g., activation rate, revenue, retention)
- **Baseline data** — current metric values if available
- **User data** — user counts, segment sizes, funnel metrics if available
- **Comparable data** — benchmarks from competitors or analogous features if available

---

## Step 1: Define What We Are Estimating

Be precise about the impact claim:

```markdown
## Impact Estimation: [Feature Name]

**Claim:** [Feature X] will [increase/decrease] [metric] by [direction] for [segment].
**Target Metric:** [exact metric definition — e.g., "7-day retention rate for new users"]
**Current Baseline:** [current value if known, or "unknown — estimate needed"]
**Time Horizon:** [over what period — e.g., "within 90 days of launch"]
**Affected Population:** [who is impacted — all users, new users, segment X]
```

---

## Step 2: Choose Estimation Method

Select the most appropriate method based on available data:

### Method 1: Funnel Analysis
Best when: You can model the feature's impact as changes to funnel conversion steps.

### Method 2: Cohort Modeling
Best when: You can compare user segments (with/without the capability) to estimate lift.

### Method 3: Comparable Benchmarks
Best when: Similar features in similar products have published impact data.

### Method 4: Bottom-Up Fermi Estimation
Best when: No direct data exists; you must reason from first principles.

If uncertain, use multiple methods and triangulate.

---

## Method 1: Funnel Analysis

Model the user funnel and estimate how the feature changes each step.

### Step 2a: Map the Current Funnel

```markdown
### Current Funnel

| Step | Description | Current Rate | Users (of [N] total) |
|------|-------------|-------------|---------------------|
| 1 | [e.g., Visit landing page] | 100% (entry) | [N] |
| 2 | [e.g., Start signup] | [X%] | [N × X%] |
| 3 | [e.g., Complete signup] | [X%] | [calculated] |
| 4 | [e.g., First action] | [X%] | [calculated] |
| 5 | [e.g., Activation] | [X%] | [calculated] |
| 6 | [e.g., Retention (D7)] | [X%] | [calculated] |

**End-to-end conversion:** [product of all rates]
```

### Step 2b: Estimate Feature Impact on Each Step

For each funnel step the feature affects:

```markdown
### Impact on Funnel

| Step | Current Rate | Estimated New Rate | Basis for Estimate |
|------|-------------|-------------------|-------------------|
| [affected step] | [X%] | [Y%] | [why — benchmark, logic, comparable] |
| [affected step] | [X%] | [Y%] | [why] |

**Unchanged steps:** [list steps unaffected by this feature]
```

### Step 2c: Calculate Net Impact

```markdown
### Funnel Impact Calculation

| Scenario | Affected Step Changes | New End-to-End Rate | Absolute Lift | Relative Lift |
|----------|---------------------|--------------------|--------------|--------------|
| Low | [conservative changes] | [rate] | [+N%] | [+N%] |
| Mid | [expected changes] | [rate] | [+N%] | [+N%] |
| High | [optimistic changes] | [rate] | [+N%] | [+N%] |
```

---

## Method 2: Cohort Modeling

Compare user populations to estimate impact.

### Step 2a: Define Cohorts

```markdown
### Cohort Definition

| Cohort | Description | Size | Current Metric Value |
|--------|-------------|------|---------------------|
| Treatment (proxy) | [users who approximate having the feature — e.g., power users] | [N] | [value] |
| Control (proxy) | [users without the feature equivalent] | [N] | [value] |

**Observed difference:** [treatment metric] - [control metric] = [delta]
```

### Step 2b: Adjust for Confounders

The raw difference overestimates impact because treatment/control cohorts differ in ways beyond the feature:

```markdown
### Confounding Adjustments

| Confounder | Direction of Bias | Estimated Adjustment |
|-----------|------------------|---------------------|
| [e.g., Selection bias — power users are inherently more engaged] | Overstates impact | Reduce estimate by [X%] |
| [e.g., Novelty effect] | Overstates short-term impact | Reduce by [X%] for long-term |
| [e.g., Missing complementary features] | Understates impact | Increase by [X%] |

**Adjusted impact estimate:** [raw delta] × [adjustment factor] = [adjusted delta]
```

---

## Method 3: Comparable Benchmarks

Use published data from similar features in similar products.

### Step 2a: Find Comparables

**Browser: 2-4 searches** (if benchmarks are not already available)

Search for:
- `"[feature type] impact benchmark"`
- `"[feature type] conversion lift case study"`
- `"[product type] [metric] improvement"`

```markdown
### Comparable Benchmarks

| Source | Feature | Product Type | Metric | Impact Observed | Relevance |
|--------|---------|-------------|--------|----------------|-----------|
| [source + URL] | [comparable feature] | [product type] | [metric] | [+X%] | High/Medium/Low |
| [source + URL] | [comparable feature] | [product type] | [metric] | [+X%] | ... |
| [source + URL] | ... | ... | ... | ... | ... |
```

### Step 2b: Adjust for Context Differences

```markdown
### Context Adjustments

| Comparable | Raw Impact | Adjustment | Adjusted Impact | Rationale |
|-----------|-----------|------------|----------------|-----------|
| [comp 1] | [+X%] | [×0.7] | [+Y%] | [e.g., "Their product is more mature, larger user base"] |
| [comp 2] | [+X%] | [×1.2] | [+Y%] | [e.g., "Smaller product, our segment is more engaged"] |
```

---

## Method 4: Bottom-Up Fermi Estimation

When no funnel data or comparables exist, reason from first principles.

### Step 2a: Decompose into Estimable Components

Break the impact into a chain of factors you can individually estimate:

```markdown
### Estimation Chain

Impact = [Factor 1] × [Factor 2] × [Factor 3] × ... × [Factor N]

| Factor | Description | Low Estimate | Mid Estimate | High Estimate | Basis |
|--------|-------------|-------------|-------------|--------------|-------|
| F1 | [e.g., % of users who encounter the problem] | [X%] | [Y%] | [Z%] | [evidence or reasoning] |
| F2 | [e.g., % of those who try the new feature] | [X%] | [Y%] | [Z%] | [evidence] |
| F3 | [e.g., % improvement for users who use it] | [X%] | [Y%] | [Z%] | [evidence] |
```

### Step 2b: Calculate Range

```
Low estimate = F1_low × F2_low × F3_low × ...
Mid estimate = F1_mid × F2_mid × F3_mid × ...
High estimate = F1_high × F2_high × F3_high × ...
```

---

## Step 3: Produce the Range Estimate

Regardless of method, produce a three-point estimate:

```markdown
## Impact Estimate

| Scenario | Metric Change | Absolute Impact | User Impact | Revenue/Business Impact |
|----------|--------------|-----------------|-------------|----------------------|
| **Low** (conservative) | [metric: X → Y] | [+N absolute] | [N users affected] | [$N or qualitative] |
| **Mid** (expected) | [metric: X → Y] | [+N absolute] | [N users affected] | [$N or qualitative] |
| **High** (optimistic) | [metric: X → Y] | [+N absolute] | [N users affected] | [$N or qualitative] |

**Expected value:** [weighted — typically 20% low, 60% mid, 20% high]
**Estimation method:** [Funnel / Cohort / Comparable / Fermi]
**Confidence in estimate:** High / Medium / Low
```

---

## Step 4: Sensitivity Analysis

Identify which assumptions drive the most variance in the estimate.

### Tornado Analysis

For each input factor, fix all others at their mid value and vary the one factor between its low and high values:

```markdown
## Sensitivity Analysis

| Factor | Low Value → Impact | Mid Value → Impact | High Value → Impact | Swing |
|--------|-------------------|-------------------|--------------------|----- |
| [Factor 1] | [impact with F1 low] | [impact with F1 mid] | [impact with F1 high] | [high - low] |
| [Factor 2] | [impact] | [impact] | [impact] | [swing] |
| [Factor 3] | [impact] | [impact] | [impact] | [swing] |

*Sorted by Swing descending — the top factor drives the most uncertainty.*

### Tornado Chart (text representation)
[Factor with largest swing]  |============================| Swing: [N]
[Factor 2]                   |==================|          Swing: [N]
[Factor 3]                   |============|                Swing: [N]
[Factor 4]                   |=======|                     Swing: [N]

### Key Insight
The estimate is most sensitive to **[Factor X]**. If we can validate this assumption
(currently [confidence level]), the estimate range narrows significantly.
Recommended: [specific validation action for the top sensitivity driver].
```

---

## Step 5: Sanity Checks

Before finalizing, run these sanity checks:

```markdown
## Sanity Checks

| Check | Result | Pass? |
|-------|--------|-------|
| **Order of magnitude:** Does the mid estimate seem reasonable compared to total addressable impact? | [assessment] | Yes/No |
| **Comparison to history:** How does this compare to past feature impacts in this product? | [assessment] | Yes/No |
| **Competitor parallel:** When competitors launched similar features, did they see similar impact? | [assessment] | Yes/No |
| **Upper bound test:** Is the high estimate physically possible given the user base? | [assessment] | Yes/No |
| **Lower bound test:** Could the impact really be as low as the low estimate? What would cause that? | [assessment] | Yes/No |
| **Base rate:** What is the typical impact of a feature of this scope? | [assessment] | Yes/No |
```

---

## Step 6: Recommendation

```markdown
## Summary and Recommendation

**Feature:** [name]
**Expected Impact:** [mid estimate with metric and timeframe]
**Confidence Range:** [low] to [high]
**Estimation Confidence:** High / Medium / Low

**Key assumption to validate:** [the top sensitivity driver]
**If we are wrong about this assumption:** [what happens to the estimate]

**ROI Assessment:**
- Estimated effort to build: [person-weeks]
- Expected mid-case impact: [metric improvement]
- Impact per person-week: [metric improvement / effort]
- Comparison: [how this compares to other features in the backlog]

**Recommendation:** Build / Deprioritize / Validate assumption first
**Rationale:** [1-2 sentences]
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] Target metric is precisely defined (not vague like "engagement")
- [ ] Estimation method is appropriate for available data
- [ ] Three-point range (low/mid/high) is provided, not just a point estimate
- [ ] Each estimate in the range has a documented basis
- [ ] Sensitivity analysis identifies the top variance driver
- [ ] Sanity checks are passed (or failures are explained)
- [ ] Confidence level honestly reflects the evidence quality
- [ ] ROI comparison to alternatives is included
