---
description: Design validation experiments including A/B tests, fake door tests, concierge tests, Wizard of Oz, and landing page tests. Structures hypothesis, test design, success criteria, sample size, duration, measurement plan, and go/no-go criteria.
invoked_from:
  - workflows/antigravity/03_solution_design.md
  - workflows/antigravity/04_scope_and_prioritization.md
  - skills/antigravity/assumption_mapping.md
  - On-demand for experiment planning
produces: Experiment plan embedded in calling artifact or standalone experiment brief
browser_usage: Light (sample size calculators, benchmark lookups)
---

# Skill: Experiment Design

Design a rigorous validation experiment to test a product hypothesis before committing to full build. The goal is to learn the maximum amount with the minimum investment.

---

## Input

- **Hypothesis to test** — from assumption mapping, opportunity scoring, or stakeholder question
- **Target metric** — what we expect to change if the hypothesis is true
- **Available resources** — time budget, engineering effort, user access, tools
- **Risk tolerance** — how confident do we need to be? (Higher stakes = more rigor)

---

## Step 1: Formulate the Hypothesis

Write a falsifiable hypothesis using this template:

```markdown
## Hypothesis

**We believe that** [change / intervention]
**for** [target user segment]
**will result in** [expected outcome / metric change]
**because** [rationale — why we think this will work]

**We will know this is true when** [specific, measurable success criterion]
**We will know this is false when** [specific, measurable failure criterion]
```

### Hypothesis Quality Rules

1. **Must be falsifiable.** If no result could disprove it, it is not a hypothesis.
2. **Must be specific.** Not "improve engagement" but "increase 7-day retention rate by at least 5 percentage points."
3. **Must include a mechanism.** The "because" clause explains WHY you expect the result. This is what you are actually testing.
4. **Must have a time bound.** When will we evaluate?

---

## Step 2: Select Experiment Type

Choose the experiment type based on what you need to learn and what you have available:

### Experiment Type Selector

| What You Need to Learn | Best Experiment Type | Effort | Timeline |
|------------------------|---------------------|--------|----------|
| "Do users want this feature?" (demand signal) | Fake Door Test | Low | 1-2 weeks |
| "Will users sign up for this product?" (market validation) | Landing Page Test | Low | 1-3 weeks |
| "Does this feature improve metrics?" (impact validation) | A/B Test | Medium-High | 2-6 weeks |
| "Can we deliver this value manually?" (value validation) | Concierge Test | Medium | 1-4 weeks |
| "Does the UX concept work?" (usability validation) | Wizard of Oz | Medium | 1-3 weeks |
| "Will users pay for this?" (willingness to pay) | Pricing Test / Pre-order | Low-Medium | 1-3 weeks |
| "Is the core mechanic fun/useful?" (experience validation) | Prototype Test | Medium | 1-2 weeks |

---

## Experiment Type 1: A/B Test

The gold standard for measuring causal impact of a change.

```markdown
### A/B Test Design

**Test Name:** [descriptive name]
**Hypothesis:** [from Step 1]

**Variants:**
- **Control (A):** [current experience — describe exactly what users see]
- **Treatment (B):** [new experience — describe exactly what changes]
- **[Treatment C]:** [optional additional variant if testing multiple approaches]

**Randomization Unit:** [user / session / device / organization]
**Allocation:** [e.g., 50/50, or 90/10 for risky changes]
**Targeting:** [which users are eligible — all, new, segment X]
**Exclusions:** [which users are excluded and why]

**Primary Metric:** [single metric that determines success/failure]
**Secondary Metrics:** [additional metrics to monitor]
**Guardrail Metrics:** [metrics that must NOT degrade — e.g., error rate, latency]

**Minimum Detectable Effect (MDE):** [smallest change worth detecting]
**Statistical Significance Level:** [typically alpha = 0.05]
**Statistical Power:** [typically 80%]
```

### Sample Size Calculation

**Browser: Use a sample size calculator if needed**

```markdown
**Sample Size Calculation:**
- Baseline conversion rate: [current rate]
- Minimum detectable effect: [absolute or relative change]
- Significance level (alpha): 0.05
- Power (1 - beta): 0.80
- **Required sample per variant:** [N]
- **Total required sample:** [N × number of variants]
- **Estimated daily traffic:** [N users/day in eligible population]
- **Estimated test duration:** [total sample / daily traffic] = [N] days

**Duration constraints:**
- Minimum: [N] days (to capture full weekly cycle — always run at least 7 days)
- Maximum: [N] days (diminishing returns; peeking risk increases)
- Include at least one full business cycle (weekday + weekend)
```

### Analysis Plan

```markdown
**Analysis Plan:**
1. Run test for predetermined duration — NO peeking at results early to make decisions
2. Check guardrail metrics first — if any are degraded, investigate before declaring success
3. Compare primary metric between variants using [test method — e.g., two-proportion z-test, t-test]
4. Check for novelty effects by analyzing metric over time within the test period
5. Segment analysis: check if effect varies by [user segment, platform, geography]
6. Document results regardless of outcome — negative results are valuable
```

---

## Experiment Type 2: Fake Door Test

Measure demand by offering a feature that does not yet exist and tracking interest.

```markdown
### Fake Door Test Design

**Test Name:** [descriptive name]
**Hypothesis:** [from Step 1]

**The "Door":**
- **Placement:** [where in the product the entry point appears — e.g., button, menu item, banner]
- **Trigger:** [what the user sees — exact copy and design description]
- **Click-through experience:** [what happens when they click — e.g., waitlist form, "coming soon" modal, survey]

**Measurement:**
- **Primary metric:** Click-through rate on the door
- **Secondary metric:** Completion rate of the follow-up action (waitlist signup, survey completion)
- **Denominator:** Users exposed to the door placement

**Success Criteria:**
- **Go:** Click-through rate >= [X%] (benchmark: [comparable feature engagement rates])
- **No-go:** Click-through rate < [Y%]
- **Investigate:** Between Y% and X% — run follow-up qualitative research

**Duration:** [N] days / [N] unique exposures
**User communication:** [How to handle users who click — set expectations, collect feedback]

**Ethical Considerations:**
- The "door" must not mislead users into thinking the feature exists
- The click-through page must clearly communicate the feature is planned/in development
- Offer a way to express interest (waitlist, feedback form)
```

---

## Experiment Type 3: Concierge Test

Deliver the feature's value manually to validate that users want it before building automation.

```markdown
### Concierge Test Design

**Test Name:** [descriptive name]
**Hypothesis:** [from Step 1]

**Manual Service:**
- **What we deliver:** [the value proposition, delivered by a human instead of software]
- **How we deliver it:** [process — email, Slack, video call, manual data entry, etc.]
- **Who delivers it:** [team member role]

**Participants:**
- **Target segment:** [who]
- **Recruitment method:** [how we find participants]
- **Sample size:** [5-15 users is typical for concierge]

**Test Protocol:**
1. [Step 1: How participants are onboarded]
2. [Step 2: How the service is delivered]
3. [Step 3: How feedback is collected]
4. [Step 4: How we measure success]

**Success Metrics:**
- **Primary:** [e.g., task completion rate, user satisfaction score, willingness to pay]
- **Secondary:** [e.g., time saved, repeat usage, referral intent]
- **Qualitative:** [interview questions to ask after the test]

**Go/No-Go Criteria:**
- **Go:** [N] of [M] participants [specific positive signal]
- **No-go:** Fewer than [N] participants show interest or derive value
- **Pivot:** Users want the value but the delivery mechanism is wrong — redesign approach

**Duration:** [N] weeks
**Cost:** [estimated time cost for manual delivery]
```

---

## Experiment Type 4: Wizard of Oz

The product appears automated to the user, but a human operates it behind the scenes.

```markdown
### Wizard of Oz Test Design

**Test Name:** [descriptive name]
**Hypothesis:** [from Step 1]

**Front Stage (what the user sees):**
- [Description of the interface — looks like a real feature]
- [User interactions — what they do]

**Back Stage (what we do):**
- [How the "wizard" fulfills the request manually]
- [Tools and processes used behind the scenes]
- [Response time expectations]

**Fidelity Level:**
- UI: [High / Medium / Low — how polished is the interface?]
- Response quality: [High — human responses should match or exceed what automation could do]
- Response time: [Target — should approximate automated speed where possible]

**Participants:** [N] users, [segment]
**Duration:** [N] weeks

**Measurement:**
- **Primary metric:** [user behavior metric — e.g., usage frequency, completion rate]
- **Secondary:** [satisfaction, NPS, qualitative feedback]
- **Operational:** [wizard time per request — informs build decision]

**Go/No-Go Criteria:**
- **Go:** Users engage with the feature at rate >= [X] and wizard effort per interaction is < [Y] minutes (making automation viable)
- **No-go:** Users do not engage, or wizard effort is so high that automation is impractical
```

---

## Experiment Type 5: Landing Page Test

Validate product-market fit by measuring conversion on a page describing the product.

```markdown
### Landing Page Test Design

**Test Name:** [descriptive name]
**Hypothesis:** [from Step 1]

**Page Content:**
- **Headline:** [value proposition — test multiple variants if possible]
- **Body:** [key benefits, social proof, differentiation]
- **CTA:** [what we ask visitors to do — e.g., "Join waitlist", "Get early access"]
- **Form fields:** [what info we collect — minimize to reduce friction]

**Traffic Source:**
- **Channel:** [paid ads, social media, community posts, email]
- **Targeting:** [demographic, interest, keyword targeting]
- **Budget:** [$N over N days]
- **Expected traffic:** [N visitors based on budget and channel benchmarks]

**Measurement:**
- **Primary metric:** CTA conversion rate (visitors who click the button)
- **Secondary metric:** Form completion rate (of those who click)
- **Tertiary:** Demographic/firmographic data of signups

**Variants (if A/B testing the page):**
- **Variant A:** [headline/positioning approach 1]
- **Variant B:** [headline/positioning approach 2]

**Success Criteria:**
- **Go:** Conversion rate >= [X%] (benchmark: landing page benchmarks for [product type] are typically [range])
- **No-go:** Conversion rate < [Y%]
- **Investigate:** Moderate conversion but unexpected demographics

**Duration:** [N] days or until [N] visitors
```

---

## Step 3: Define Measurement Plan

Regardless of experiment type, define how data will be collected and analyzed.

```markdown
## Measurement Plan

### Data Collection
| Metric | Event/Trigger | Data Source | Tool | Owner |
|--------|--------------|-------------|------|-------|
| [primary metric] | [what triggers measurement] | [where the data lives] | [analytics tool] | [who monitors] |
| [secondary metric] | ... | ... | ... | ... |
| [guardrail metric] | ... | ... | ... | ... |

### Analysis Timeline
- **Day 0:** Launch experiment, verify data collection is working
- **Day 1:** Sanity check — are users being allocated correctly? Is data flowing?
- **Day [mid]:** Mid-point check — guardrail metrics only (NO peeking at primary)
- **Day [end]:** Close experiment, run full analysis
- **Day [end+2]:** Document results and decision

### Reporting
- **Dashboard:** [where results will be visible]
- **Analysis document:** [where the full analysis will be written]
- **Decision forum:** [where the go/no-go decision will be made]
```

---

## Step 4: Define Go/No-Go Decision Framework

```markdown
## Decision Framework

### Outcomes and Actions

| Outcome | Criteria | Action |
|---------|----------|--------|
| **Strong Go** | Primary metric exceeds success criteria by >50% AND no guardrail violations | Proceed to full build with confidence |
| **Go** | Primary metric meets success criteria AND no guardrail violations | Proceed to build |
| **Conditional Go** | Primary metric meets criteria BUT guardrail metric degraded | Investigate guardrail issue; proceed only if resolvable |
| **Inconclusive** | Results between go and no-go thresholds | Extend test duration or redesign experiment |
| **No-Go** | Primary metric below failure threshold | Kill the initiative or pivot approach |
| **Strong No-Go** | Primary metric significantly below threshold OR guardrail violation | Kill and reallocate resources |

### Post-Experiment Actions
Regardless of outcome:
1. Document full results in [location]
2. Update ASSUMPTIONS_REGISTER.md with validated/invalidated status
3. Share learnings with team
4. Archive experiment artifacts
```

---

## Quality Checklist

Before launching an experiment, verify:
- [ ] Hypothesis is falsifiable and specific (includes numbers)
- [ ] Success and failure criteria are defined BEFORE the experiment starts (no moving goalposts)
- [ ] Sample size is calculated (for A/B tests) or justified (for qualitative experiments)
- [ ] Duration accounts for weekly cycles (minimum 7 days for A/B tests)
- [ ] Guardrail metrics are defined to catch unintended negative effects
- [ ] No-peeking rule is established (for A/B tests)
- [ ] Measurement plan confirms data collection is feasible before launch
- [ ] Go/no-go criteria are documented and agreed upon by stakeholders
- [ ] Ethical considerations addressed (no deception, clear communication)
- [ ] Post-experiment documentation plan is in place
