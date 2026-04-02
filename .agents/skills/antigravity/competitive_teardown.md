---
description: Deep-dive structured analysis of a single competitor product. Walk through their product using browser tools, document UX flows, feature inventory, design patterns, pricing, onboarding, strengths, and weaknesses. Produces a structured teardown document.
invoked_from:
  - workflows/antigravity/01_deep_research.md
  - workflows/antigravity/03_solution_design.md
  - On-demand when user requests competitor analysis
produces: Structured teardown section for COMPETITIVE_LANDSCAPE.md or standalone analysis
browser_usage: Heavy (5-15 searches + product walkthrough per competitor)
---

# Skill: Competitive Teardown

Perform a thorough, structured analysis of a single competitor product. This goes far beyond reading their marketing page — you walk through the actual product experience and extract actionable intelligence.

---

## Input

- **Competitor name / URL** (required)
- **Our product context** — what we are building and why (from VISION_BRIEF.md if available)
- **Specific questions** — anything the invoking workflow wants answered about this competitor

---

## Step 1: Reconnaissance

**Browser: 2-3 searches**

Before touching the product, gather background:

1. **Search** `"[competitor]"` — read their homepage, about page, and any recent press.
2. **Search** `"[competitor] funding"` or `"[competitor] crunchbase"` — understand their scale, funding stage, team size.
3. **Search** `"[competitor] review [current year]"` — get user sentiment from G2, Capterra, Product Hunt, Reddit, app store reviews.

### Capture

```markdown
### Background
- **Founded:** [year]
- **Funding:** [amount, stage, key investors]
- **Team size:** [estimate]
- **Target market:** [who they sell to]
- **Positioning:** [their stated value prop — from their homepage/tagline]
- **Pricing model:** [free/freemium/paid, price points]
```

---

## Step 2: Pricing & Packaging Analysis

**Browser: Visit pricing page**

Use WebFetch or browser automation to read their pricing page in detail.

### Capture

```markdown
### Pricing & Packaging
| Tier | Price | Key Inclusions | Key Limitations |
|------|-------|---------------|----------------|
| Free | $0 | ... | ... |
| Pro | $X/mo | ... | ... |
| Enterprise | Custom | ... | ... |

**Pricing model:** Per-seat / Per-usage / Flat rate / Hybrid
**Free trial:** Yes/No, [duration]
**Notable pricing tactics:** [e.g., usage caps, feature gating, seat minimums]
**Price anchoring:** [How they frame value — ROI calculators, competitor comparison tables, etc.]
```

---

## Step 3: Onboarding Experience

**Browser: Walk through signup and onboarding**

If the product has a free tier or trial, walk through the onboarding flow using browser automation. If not, search for screenshots/videos of the onboarding (`"[competitor] onboarding flow"`, `"[competitor] getting started"`).

### Evaluate

1. **Signup friction:** How many steps? What information required? Social login available?
2. **Time to value:** How quickly does a new user experience the core value?
3. **Activation sequence:** What does the product guide users to do first?
4. **Empty states:** How does the product handle the "blank canvas" problem?
5. **Progressive disclosure:** Does it overwhelm with features or reveal gradually?
6. **Onboarding patterns:** Checklist? Tutorial? Interactive walkthrough? Video? None?

### Capture

```markdown
### Onboarding Teardown
- **Signup steps:** [count and description]
- **Time to first value:** [estimate — seconds/minutes]
- **Activation trigger:** [what the product considers "activated"]
- **Onboarding pattern:** [checklist / tutorial / interactive / none]
- **Empty state handling:** [description]
- **Strengths:** [what they do well]
- **Weaknesses:** [what is confusing or creates friction]
- **Screenshots/notes:** [key observations from walkthrough]
```

---

## Step 4: Core Product Walkthrough

**Browser: Navigate the product systematically**

Walk through the core user flows. For each flow:

1. **Identify the primary action.** What is the main thing a user does in this product?
2. **Trace the flow end-to-end.** From trigger to completion.
3. **Note every interaction.** Clicks, inputs, waits, transitions.
4. **Evaluate the UX.** Is it intuitive? Efficient? Frustrating?

### Feature Inventory

Build a comprehensive list of features:

```markdown
### Feature Inventory
| Category | Feature | Description | Maturity | Notes |
|----------|---------|-------------|----------|-------|
| Core | [feature] | [what it does] | Polished / Basic / Beta | [observations] |
| Core | [feature] | ... | ... | ... |
| Secondary | [feature] | ... | ... | ... |
| Integration | [feature] | ... | ... | ... |
| Admin | [feature] | ... | ... | ... |
```

### UX Flow Documentation

For each core flow:

```markdown
### Flow: [Name — e.g., "Create a new project"]
1. [Step] — [observation about UX]
2. [Step] — [observation]
3. [Step] — [observation]
...
**Total steps:** [count]
**Estimated time:** [seconds/minutes]
**Friction points:** [list any confusing or slow steps]
**Clever patterns:** [anything well-designed worth noting]
```

---

## Step 5: Design & UX Pattern Analysis

Analyze the design decisions:

### Visual Design
- **Design language:** Modern/dated, minimal/complex, playful/professional
- **Color palette:** Primary, accent, semantic colors
- **Typography:** Font choices, hierarchy
- **Density:** Information-dense vs spacious
- **Consistency:** Is the design language applied uniformly?

### Interaction Patterns
- **Navigation model:** Sidebar / top nav / command palette / breadcrumb
- **Data presentation:** Tables, cards, lists, dashboards
- **Feedback mechanisms:** Loading states, success/error messages, toasts, modals
- **Keyboard support:** Shortcuts, accessibility
- **Responsiveness:** Mobile experience, responsive breakpoints

### Information Architecture
- **Top-level structure:** How is the product organized?
- **Hierarchy depth:** How many levels deep does navigation go?
- **Discoverability:** Can users find features without being told where they are?
- **Mental model:** Does the IA match how users think about the domain?

---

## Step 6: User Sentiment Analysis

**Browser: 2-3 searches for reviews**

Search for user reviews and community discussion:

- `"[competitor] review" site:g2.com`
- `"[competitor]" site:reddit.com`
- `"[competitor] alternative"` (reveals what users wish was different)
- `"[competitor] problems"` or `"[competitor] frustrating"`

### Capture

```markdown
### User Sentiment
**Overall rating:** [G2/Capterra score if available]
**Review volume:** [approximate count]

**Top praise themes (what users love):**
1. [Theme] — mentioned in [N] reviews. Example: "[brief quote]"
2. [Theme] — ...
3. [Theme] — ...

**Top complaint themes (what users hate):**
1. [Theme] — mentioned in [N] reviews. Example: "[brief quote]"
2. [Theme] — ...
3. [Theme] — ...

**Churn signals (why users leave):**
- [Reason 1]
- [Reason 2]

**Sources:** [URLs to review pages]
```

---

## Step 7: Strategic Assessment

Synthesize all observations into strategic intelligence:

```markdown
### Strategic Assessment

**Core value proposition (actual, not marketing):**
[What users actually get value from, based on reviews and product walkthrough]

**Moat / defensibility:**
[What makes them hard to displace — network effects, data, integrations, brand, switching cost]

**Strengths to learn from:**
1. [Strength] — Why it works: [explanation]. How we can adopt: [specific suggestion]
2. [Strength] — ...
3. [Strength] — ...

**Weaknesses to exploit:**
1. [Weakness] — Evidence: [from reviews/walkthrough]. Our opportunity: [specific suggestion]
2. [Weakness] — ...
3. [Weakness] — ...

**Underserved segments:**
[User segments or use cases they handle poorly or ignore]

**Likely roadmap direction:**
[Based on recent releases, job postings, blog posts — where are they heading?]

**Threat level:** High / Medium / Low
**Rationale:** [Why this threat level]
```

---

## Output

Compile all sections into a single structured teardown. This gets embedded in `COMPETITIVE_LANDSCAPE.md` under the competitor's heading, or delivered as a standalone analysis if invoked on-demand.

### Quality Checklist

Before finalizing, verify:
- [ ] All sections are populated with specific observations, not generic statements
- [ ] Feature inventory is comprehensive (not just what their marketing page highlights)
- [ ] User sentiment includes actual review data with source URLs
- [ ] Strengths and weaknesses are actionable (each includes a "so what" for our product)
- [ ] Pricing is documented with actual numbers, not just "they have a free tier"
- [ ] Onboarding evaluation is based on actual walkthrough or documented evidence
- [ ] All source URLs are included
