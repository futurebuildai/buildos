---
description: Execute heavy browser-based research across market, competitive, technical, and design domains. The core research stage that grounds all downstream specs in evidence.
stage: "01"
inputs: RESEARCH_AGENDA.md, VISION_BRIEF.md, ASSUMPTIONS_REGISTER.md
outputs: RESEARCH_FINDINGS.md, COMPETITIVE_LANDSCAPE.md
browser_usage: HEAVY (20-50+ searches)
---

# Stage 01: Deep Research

This is the **highest-effort research stage**. The quality of all downstream specs depends on the depth and rigor of research conducted here. Use browser tools extensively.

---

## Step 1: Prepare Research Plan

Read `RESEARCH_AGENDA.md`. For each question, determine the best research approach:
- **WebSearch** — for broad discovery queries
- **WebFetch** — for reading specific pages in depth (docs, blog posts, pricing pages)
- **Browser automation** — for walking through competitor products, analyzing their UX flows

Prioritize questions by impact: which answers will most influence product decisions?

## Step 2: Market Research

**Browser: 5-10 searches**

- Search for the market this product operates in. Look for:
  - Market size estimates (TAM/SAM/SOM if available)
  - Growth trends and industry reports
  - Key players and market segments
  - Recent funding, acquisitions, or exits in the space
- Search for "state of [domain] [current year]" and "[domain] market trends [current year]"
- Read 2-3 industry overview articles or reports

**Document in RESEARCH_FINDINGS.md under `## Market Research`:**
- Market size and growth (with source URLs)
- Key trends affecting the space
- Market segments and where our product fits

## Step 3: Competitive Analysis

**Browser: 10-20 searches + page reads**

Identify 3-5 direct competitors and 2-3 indirect competitors/substitutes.

For each competitor:
1. **Search** for "[competitor] review", "[competitor] vs alternatives", "[competitor] pricing"
2. **Visit their website.** Read their homepage, features page, pricing page, docs.
3. **Read user reviews.** Search G2, Capterra, Product Hunt, Reddit, app store reviews. Focus on complaints and praise.
4. **Analyze their approach:** What is their core value prop? Who do they target? What do they charge? What do users love/hate?

If the competitor has a public product:
5. **Walk through their product** using browser automation. Document:
   - Onboarding flow
   - Core interaction patterns
   - Information architecture
   - Key UX decisions (good and bad)
   - Feature inventory

**Invoke skill:** For each top competitor, use `.agents/skills/antigravity/competitive_teardown.md` for structured deep-dive analysis.

**Document in COMPETITIVE_LANDSCAPE.md:**
```markdown
# Competitive Landscape

## Market Overview
[Summary paragraph]

## Competitor Matrix
| Competitor | Target Segment | Core Value Prop | Pricing | Key Strength | Key Weakness |
|-----------|---------------|-----------------|---------|-------------|-------------|

## Detailed Competitor Analyses

### [Competitor 1]
- **What they do:** ...
- **Target user:** ...
- **Pricing:** ...
- **Key features:** ...
- **UX observations:** ...
- **User sentiment:** (from reviews) ...
- **Strengths to learn from:** ...
- **Weaknesses to exploit:** ...
- **Sources:** [URLs]

### [Competitor 2]
...

## Feature Parity Matrix
| Feature | Us (planned) | Comp 1 | Comp 2 | Comp 3 |
|---------|-------------|--------|--------|--------|

## Whitespace Analysis
[Capabilities no competitor offers well — our opportunity]
```

## Step 4: Technical Research

**Browser: 5-10 searches + doc reads**

Based on the technical questions in `RESEARCH_AGENDA.md`:

- Search for "[domain] architecture best practices [current year]"
- Search for "[tech from TECH_STACK.md] [domain] tutorial" or "[tech] [domain] example"
- Find and read 2-3 technical blog posts or case studies of similar systems being built
- Search for relevant libraries, SDKs, APIs:
  - Check their GitHub repos: stars, last commit, open issues
  - Read their README and getting-started docs
  - Check for breaking changes or deprecation notices
- Search for known pitfalls: "[domain] common mistakes", "[domain] lessons learned"

**Document findings:**
- Recommended libraries/tools with evidence (repo health, community size, last update)
- Architecture patterns observed in similar products
- Known pitfalls and how others avoided them
- Third-party services/APIs available (with pricing if relevant)

## Step 5: Design & UX Pattern Research

**Browser: 5-10 searches**

- Search for "[product type] UX patterns", "[product type] UI design inspiration"
- Look at design galleries: Dribbble, Mobbin, Page Flows, UI Patterns
- Analyze how best-in-class products in adjacent spaces handle similar UX challenges
- Search for relevant design system documentation (Material, Radix, Shadcn, etc.)
- Look for accessibility patterns specific to the domain

**Document findings:**
- UX patterns that work well in this domain (with examples/screenshots noted)
- Design system approaches used by competitors
- Accessibility considerations specific to the use case
- Interaction patterns that users expect in this domain

## Step 6: Prior Art & Case Studies

**Browser: 3-5 searches**

- Search for "[domain] case study", "building [product type]", "[domain] tech stack"
- Look for conference talks, blog posts, or postmortems from teams that built similar products
- Search Hacker News, Dev.to, Medium for experience reports

**Document findings:**
- Lessons learned from similar projects
- Architecture decisions others made and outcomes
- Common failure modes

## Step 7: Validate Assumptions

Review `ASSUMPTIONS_REGISTER.md`. For each assumption marked for browser research validation:
- Search specifically to confirm or deny the assumption
- Update the assumption status: Validated / Invalidated / Partially Validated
- Document the evidence

## Step 8: Synthesize Findings

Compile all research into `.agents/handoff/RESEARCH_FINDINGS.md`:

```markdown
# Research Findings

## Executive Summary
[3-5 bullet synthesis of the most important findings]

## Market Research
### Market Size & Trends
### Key Players
### Sources: [URLs]

## Competitive Intelligence
[Reference COMPETITIVE_LANDSCAPE.md for details]
### Key Takeaways
### Differentiation Opportunities

## Technical Landscape
### Recommended Architecture Patterns
### Libraries & Tools (with evidence)
### APIs & Third-Party Services
### Known Pitfalls
### Sources: [URLs]

## Design & UX Patterns
### Common Patterns in This Domain
### Best-in-Class Examples
### Accessibility Considerations
### Sources: [URLs]

## Prior Art & Lessons Learned
### Case Studies
### Common Failure Modes
### Sources: [URLs]

## Assumption Validation Results
| # | Assumption | Original Confidence | Evidence | New Status |
|---|-----------|-------------------|----------|------------|
```

## Step 9: Update Pipeline State

Update `.agents/handoff/PIPELINE_STATE.md`:
```markdown
## Stage 01 — Deep Research ✅
- RESEARCH_FINDINGS.md produced
- COMPETITIVE_LANDSCAPE.md produced
- [X] assumptions validated, [Y] invalidated
## Next Stage: 02 — User Research Synthesis
```

Proceed to Stage 02.
