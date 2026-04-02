---
description: Synthesize evidence-based personas, apply Jobs-to-Be-Done analysis, and map user journeys from research findings and competitor analysis.
stage: "02"
inputs: VISION_BRIEF.md, RESEARCH_FINDINGS.md, COMPETITIVE_LANDSCAPE.md, ASSUMPTIONS_REGISTER.md
outputs: PERSONAS.md, JTBD_ANALYSIS.md, USER_JOURNEYS.md
browser_usage: Moderate (forum/review research for user sentiment)
---

# Stage 02: User Research Synthesis

Transform raw research into structured user understanding. This stage produces the user models that drive every downstream product decision.

---

## Step 1: Gather User Signal from the Web

**Browser: 5-10 searches**

Search for real user voices related to the problem space:
- Reddit: search "[problem domain] reddit", "best [product type] reddit"
- Stack Overflow / domain-specific forums: search for questions people ask
- App store reviews of competitors (if applicable)
- Twitter/X: search for complaints or praise about existing solutions
- Product Hunt: read launch comments for competitors
- Support forums: search "[competitor] help", "[competitor] frustrating"

For each source, extract:
- **Behavioral patterns** — what are people actually doing?
- **Pain points** — what language do they use to describe frustrations?
- **Workarounds** — what hacks have people invented?
- **Wish list** — what do people explicitly ask for?

## Step 2: Build Evidence-Based Personas

Using research findings + user signals, construct personas grounded in behavioral evidence, not demographics.

### Process:
1. **Identify behavioral variables** from research: technical sophistication, use frequency, primary goal, organization size, decision-making authority, risk tolerance.
2. **Cluster patterns** — group observed behaviors into 2-3 distinct clusters.
3. **Construct personas** — one per cluster.

### Per-Persona Template:
```markdown
### [Persona Name] — [Archetype Title]
**Context:** [Role, environment, daily workflow]
**Primary Goal:** [What they're trying to achieve]
**Key Behaviors:**
- [Observed behavior 1 — evidence source]
- [Observed behavior 2 — evidence source]
**Pain Points:**
- [Pain 1 — quoted user language if available]
- [Pain 2]
**Current Tools/Workarounds:** [What they use today]
**Technical Sophistication:** [Low / Medium / High]
**Decision Factors:** [What drives their choices: price, speed, reliability, etc.]
**Representative Quote:** "[Actual quote from research if available]"
```

4. **Define anti-personas** — who is explicitly NOT the target user and why.

Write `.agents/handoff/PERSONAS.md`.

## Step 3: Jobs-to-Be-Done Analysis

Apply the JTBD framework to understand what users are trying to accomplish independent of any specific solution.

### 3a: Identify Core Jobs

For each persona, identify:
- **Core Functional Job:** "When I [situation], I want to [action], so I can [outcome]."
- **Emotional Jobs:** How they want to feel (confident, in control, not anxious)
- **Social Jobs:** How they want to be perceived (competent, modern, efficient)
- **Related Jobs:** Adjacent jobs that happen before, during, or after the core job

### 3b: Map the Job Process

For the core functional job, break it into 8 universal process steps:
1. **Define** — determine what needs to be done
2. **Locate** — find inputs or resources needed
3. **Prepare** — set up what's needed
4. **Confirm** — verify readiness
5. **Execute** — perform the core action
6. **Monitor** — track progress/results
7. **Modify** — make adjustments
8. **Conclude** — finalize and move on

### 3c: Extract Desired Outcomes

For each job step, define outcomes users want:
- Use format: "Minimize the [time/likelihood/effort] of [undesirable outcome]" or "Increase the [speed/accuracy/completeness] of [desirable outcome]"

### 3d: Score Opportunities

**Invoke skill:** `.agents/skills/antigravity/opportunity_scoring.md`

For each outcome:
- Rate **Importance** (1-10): How critical is this to the user?
- Rate **Current Satisfaction** (1-10): How well do existing solutions address this?
- Calculate **Opportunity Score** = Importance + (Importance - Satisfaction)
- Scores > 12 are high-opportunity. Scores > 15 are critical underserved needs.

Rank outcomes by opportunity score. The top outcomes become your product's focus.

Write `.agents/handoff/JTBD_ANALYSIS.md`:
```markdown
# Jobs-to-Be-Done Analysis

## Core Jobs by Persona
### [Persona 1]
- Functional: ...
- Emotional: ...
- Social: ...

## Job Map: [Core Job]
| Step | User Action | Desired Outcomes |
|------|------------|-----------------|

## Opportunity Scoring
| # | Outcome | Importance | Satisfaction | Opportunity Score |
|---|---------|-----------|-------------|------------------|
[Sorted by score descending]

## Top Underserved Outcomes
[Top 5-8 with analysis of why they're underserved]

## Competitive Hiring/Firing
[What solutions users currently "hire" for this job and why they "fire" them]
```

## Step 4: Map User Journeys

For the primary persona, map the end-to-end journey across the core job.

### Per-Journey Template:
```markdown
## Journey: [Persona] — [Job/Scenario]

| Phase | User Action | Touchpoint | Thoughts/Feelings | Pain Points | Opportunities |
|-------|------------|------------|-------------------|-------------|---------------|
| Awareness | ... | ... | ... | ... | ... |
| Consideration | ... | ... | ... | ... | ... |
| Onboarding | ... | ... | ... | ... | ... |
| Core Use | ... | ... | ... | ... | ... |
| Return/Expand | ... | ... | ... | ... | ... |
```

Map these journey types:
1. **Happy path** — everything goes right
2. **First-time user** — discovery through activation
3. **Error/recovery** — something goes wrong, user recovers
4. **Power user** — advanced usage patterns

Identify **Moments of Truth** — the 3-5 make-or-break moments in the journey.

Write `.agents/handoff/USER_JOURNEYS.md`.

## Step 5: Cross-Reference and Validate

- Do the personas' goals align with the JTBD outcomes?
- Do the journey pain points match the opportunity scores?
- Do the competitive weaknesses (from Stage 01) align with the top underserved outcomes?
- Update `ASSUMPTIONS_REGISTER.md` with any new assumptions or validations.

## Step 6: Update Pipeline State

Update `.agents/handoff/PIPELINE_STATE.md`:
```markdown
## Stage 02 — User Research Synthesis ✅
- PERSONAS.md produced ([N] personas + anti-personas)
- JTBD_ANALYSIS.md produced ([N] outcomes scored)
- USER_JOURNEYS.md produced ([N] journey maps)
## Next Stage: 03 — Solution Design
```

Proceed to Stage 03.
