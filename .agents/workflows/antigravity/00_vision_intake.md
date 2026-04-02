---
description: Parse a user's vision paragraph into a structured brief, research agenda, and assumptions register. Entry point for the autonomous pipeline.
stage: "00"
inputs: User vision (1-2 paragraphs)
outputs: VISION_BRIEF.md, RESEARCH_AGENDA.md, ASSUMPTIONS_REGISTER.md
browser_usage: Light
---

# Stage 00: Vision Intake

The user has provided a high-level vision for a feature or product. Your job is to transform this raw intent into structured artifacts that drive all downstream research and specification.

---

## Step 1: Parse the Vision

Read the user's vision paragraph carefully. Extract and document:

1. **Core Problem Statement** — What problem is being solved? Restate it in one sentence using: *"For [who], the problem of [what] results in [consequence]."*
2. **Target Users** — Who are the intended users? What do we know or assume about them?
3. **Desired Outcomes** — What does success look like from the user's perspective?
4. **Implied Capabilities** — What features or capabilities are implied by the vision?
5. **Constraints Mentioned** — Any explicit constraints (timeline, tech, budget, platform)?
6. **Tone & Positioning** — What quality/brand/experience level is implied?

## Step 2: Generate the Research Agenda

For every aspect of the vision that requires validation or deeper understanding, generate a specific research question. Organize into categories:

### Market & Competitive
- What existing products solve this or a similar problem?
- How big is the market for this type of solution?
- What do users of existing solutions complain about?
- What pricing models exist in this space?

### User & Problem
- Who exactly are the target users (demographics, behaviors, context)?
- How do users currently solve this problem (workarounds)?
- What is the frequency and severity of this problem?
- What adjacent problems exist that we should be aware of?

### Technical & Feasibility
- What tech stacks and architectures are commonly used for similar products?
- What third-party APIs, services, or libraries are available?
- What are the known technical challenges in this domain?
- Are there open-source solutions we could build on?

### Design & UX
- What UI/UX patterns do best-in-class products in this space use?
- What design systems or component libraries are common?
- What are the key user flows and interaction patterns?

### Business & Strategy
- What business models work in this space?
- What are the key metrics for similar products?
- What regulatory or compliance requirements exist?

**Browser research (light):** Do a quick search to validate the problem space is real and refine your research questions. 2-5 searches max at this stage.

## Step 3: Extract Assumptions

Review everything written so far. Every statement that is not a direct quote from the user is an assumption. Log each one:

| # | Assumption | Category | Confidence | Validation Method |
|---|-----------|----------|-----------|------------------|
| A1 | [statement] | Market / User / Technical / Design / Business | High / Medium / Low | [how to validate: browser research, user interview, prototype, spike] |

## Step 4: Check TECH_STACK.md

Read `.agents/TECH_STACK.md`. If the project has declared tech stack preferences, note them as constraints. If TECH_STACK.md is empty, flag this — it must be filled in before Stage 03.

## Step 5: Produce Artifacts

Write the following to `.agents/handoff/`:

### `VISION_BRIEF.md`
```markdown
# Vision Brief
## Problem Statement
[One sentence]
## Target Users
[Description]
## Desired Outcomes
[Bulleted list]
## Implied Capabilities
[Bulleted list with brief descriptions]
## Constraints
[Any mentioned constraints]
## Positioning
[Quality/brand/experience expectations]
```

### `RESEARCH_AGENDA.md`
```markdown
# Research Agenda
## Market & Competitive
- [ ] [Question 1]
- [ ] [Question 2]
...
## User & Problem
...
## Technical & Feasibility
...
## Design & UX
...
## Business & Strategy
...
```

### `ASSUMPTIONS_REGISTER.md`
```markdown
# Assumptions Register
| # | Assumption | Category | Confidence | Validation Method | Status |
|---|-----------|----------|-----------|-------------------|--------|
| A1 | ... | ... | ... | ... | Unvalidated |
```

## Step 6: Update Pipeline State

Write/update `.agents/handoff/PIPELINE_STATE.md`:
```markdown
# Pipeline State
## Current Stage: 00 — Vision Intake ✅
## Next Stage: 01 — Deep Research
## Artifacts Produced:
- VISION_BRIEF.md
- RESEARCH_AGENDA.md
- ASSUMPTIONS_REGISTER.md
```

Proceed immediately to Stage 01.
