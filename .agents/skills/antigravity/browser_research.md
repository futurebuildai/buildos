---
description: Meta-skill for structured web research. Defines the protocol for searching effectively, evaluating sources, synthesizing findings, and citing evidence. Invoked whenever a workflow or skill requires browser-based research.
invoked_from:
  - workflows/antigravity/01_deep_research.md
  - workflows/antigravity/02_user_research.md
  - workflows/antigravity/03_solution_design.md
  - workflows/antigravity/07_architecture_spec.md
  - Any skill or workflow requiring evidence gathering
produces: Research findings with sourced citations embedded in the calling artifact
browser_usage: Variable (depends on invoking context — 1-50+ searches)
---

# Skill: Browser Research Protocol

This is the foundational research skill. Every time you use browser tools to gather evidence, follow this protocol. Quality research is the single biggest differentiator between generic output and L8-quality specs.

---

## Phase 1: Query Formulation

Before searching, plan your queries. Poor queries waste cycles and return shallow results.

### Query Strategy Rules

1. **Start broad, then narrow.** First search establishes the landscape; subsequent searches drill into specifics.
2. **Use multiple query formulations** for the same question:
   - Direct: `[topic] [current year]`
   - Comparative: `[topic] vs [alternative]`
   - Problem-oriented: `[topic] challenges` or `[topic] common mistakes`
   - Evidence-seeking: `[topic] benchmark data` or `[topic] case study`
   - Community: `[topic] site:reddit.com` or `[topic] site:news.ycombinator.com`
3. **Include the current year** in queries where recency matters (market data, library status, best practices).
4. **Use domain-specific terminology.** If researching a technical topic, use the precise terms practitioners use — not layperson descriptions.
5. **Quote exact phrases** when searching for specific concepts: `"jobs to be done"` not `jobs to be done`.

### Query Planning Template

Before starting research, write a brief plan:

```
Research Question: [What are we trying to learn?]
Query 1 (broad): [query]
Query 2 (specific): [query]
Query 3 (community): [query]
Fallback query: [alternative if first three yield poor results]
Target sources: [types of sources we want — docs, blogs, reports, forums]
```

---

## Phase 2: Source Evaluation

Not all sources are equal. Apply the CRAAP framework to every source.

### CRAAP Evaluation Criteria

| Criterion | Question | Red Flags |
|-----------|----------|-----------|
| **Currency** | When was this published/updated? | No date visible; content references outdated versions |
| **Relevance** | Does this address our specific question? | Generic overview when we need specifics; different domain |
| **Authority** | Who wrote this? What are their credentials? | Anonymous; no author bio; content farm indicators |
| **Accuracy** | Is this supported by evidence? Can claims be verified? | No citations; contradicts multiple other sources; anecdotal only |
| **Purpose** | Why does this exist? Is there bias? | Vendor marketing disguised as analysis; affiliate content; SEO bait |

### Source Tier System

Prioritize sources in this order:

- **Tier 1 (Primary):** Official documentation, peer-reviewed research, published benchmarks, company-published data, regulatory filings
- **Tier 2 (Authoritative Secondary):** Well-known industry analysts (Gartner, Forrester), established tech publications (InfoQ, Martin Fowler, etc.), conference talks from recognized practitioners
- **Tier 3 (Community):** Stack Overflow (high-vote answers), Reddit threads with substantive discussion, Hacker News threads, well-maintained GitHub repos
- **Tier 4 (Use with caution):** Blog posts from unknown authors, Medium articles, marketing whitepapers, undated content

### Cross-Reference Rule

**Any claim that influences a product decision must be corroborated by at least two independent sources.** If only one source supports a claim, note it as "single-source — low confidence" in the output.

---

## Phase 3: Information Extraction

When reading a source, extract structured information — do not passively skim.

### For Each Source, Capture

1. **Key finding:** The specific fact, data point, or insight relevant to your question.
2. **Context:** Conditions under which this finding applies (e.g., "for B2B SaaS with >1000 users").
3. **Strength of evidence:** Strong (data-backed, peer-reviewed), Moderate (practitioner experience, case study), Weak (opinion, anecdotal).
4. **Publication date:** Exact date if available, otherwise approximate.
5. **Source URL:** Full URL for citation.

### Data Extraction Priorities

When you find quantitative data, always capture:
- The exact number/range
- The methodology (how was it measured?)
- The sample size or scope
- The date of measurement
- Conditions or caveats

Example (good): "Average onboarding completion rate for developer tools: 23-34% (Source: ProductLed 2024 benchmark report, n=127 SaaS products, URL)"

Example (bad): "Onboarding rates are generally low in this space."

---

## Phase 4: Synthesis

Raw findings are not useful. Synthesize them into actionable intelligence.

### Synthesis Rules

1. **Connect findings to decisions.** Every finding should link to a product, design, or architecture decision it informs.
2. **Identify patterns across sources.** If three competitors all use the same approach, that is a strong signal.
3. **Note contradictions.** When sources disagree, document both sides and assess which is more credible and why.
4. **Distinguish facts from interpretations.** "The market is $4.2B (Gartner 2024)" is a fact. "This means there is room for a new entrant" is your interpretation — label it as such.
5. **Quantify when possible.** Replace "many users complained" with "14 of the top 20 G2 reviews mention this issue."

### Synthesis Output Structure

```markdown
### [Research Question]

**Finding:** [1-2 sentence summary of what we learned]

**Evidence:**
- [Source 1 — key data point] (Source: [URL], [date])
- [Source 2 — corroborating or contrasting point] (Source: [URL], [date])

**Confidence:** High / Medium / Low
**Implication for our product:** [How this finding should influence our decisions]
```

---

## Phase 5: Citation Standards

Every research-backed claim in every artifact must be cited. No exceptions.

### Citation Format

Inline citations in artifacts:

```markdown
The average time-to-value for developer tools is 5-15 minutes
(Source: [ProductLed Benchmarks 2024](https://example.com/benchmarks), measured across 127 SaaS products).
```

### Citation Rules

1. **Always include the full URL.** Not just the domain — the full page URL.
2. **Include the publication date** if available.
3. **Note the access date** for rapidly changing content (pricing pages, documentation).
4. **If a page requires login or is paywalled,** note this so others can verify.
5. **For competitor product observations** (from browser walkthroughs), cite as: `(Observed: [Product Name] [page/feature], accessed [date])`.

### Source List

At the end of every research-heavy artifact, include a consolidated source list:

```markdown
## Sources
| # | Source | URL | Accessed | Type |
|---|--------|-----|----------|------|
| 1 | [title] | [url] | [date] | Primary / Secondary / Community |
| 2 | ... | ... | ... | ... |
```

---

## Anti-Patterns to Avoid

1. **Surface-level research.** Reading only the first search result. Always check 3-5 sources minimum per question.
2. **Confirmation bias.** Only searching for evidence that supports your hypothesis. Actively search for counterarguments.
3. **Recency bias.** Assuming the newest source is the best. Sometimes a 2-year-old deep-dive is more valuable than yesterday's hot take.
4. **Authority fallacy.** Trusting a big-name source without checking their methodology. Even Gartner gets things wrong.
5. **Copy-paste findings.** Pasting raw quotes instead of synthesizing. Your job is to extract insights, not transcribe.
6. **Missing negative results.** If you searched for something and found nothing, document that too — the absence of evidence is itself informative.
7. **Unsourced claims.** Writing "studies show" or "research indicates" without a specific citation. Always be specific.
