---
description: Synthesize all upstream research into a Google L8-quality Product Requirements Document with traced requirements, detailed user stories, and Given/When/Then acceptance criteria.
stage: "06"
inputs: All Stage 00-05 artifacts
outputs: PRD.md (in handoff/)
browser_usage: Light (verify claims, NFR benchmarks)
---

# Stage 06: Product Specification

This is the synthesis stage. Every decision made here must trace back to evidence from upstream research. The PRD is the single source of truth for WHAT to build.

---

## Step 1: Read All Upstream Artifacts

Before writing a single line, read and internalize:
- `VISION_BRIEF.md` — the original intent
- `RESEARCH_FINDINGS.md` — market, competitive, and technical evidence
- `COMPETITIVE_LANDSCAPE.md` — what exists and where the whitespace is
- `PERSONAS.md` — who we're building for
- `JTBD_ANALYSIS.md` — what they need and what's underserved
- `USER_JOURNEYS.md` — how they'll experience the product
- `SOLUTION_CANDIDATES.md` — the chosen approach and why
- `FEASIBILITY_ASSESSMENT.md` — what's technically possible
- `BUILD_VS_BUY.md` — make/buy decisions
- `SCOPE_DEFINITION.md` — what's in MVP, what's deferred
- `METRICS_FRAMEWORK.md` — how success is measured
- `INFORMATION_ARCHITECTURE.md` — product structure
- `DESIGN_SYSTEM.md` — visual and interaction language

## Step 2: Write the PRD

Write `.agents/handoff/PRD.md` with the following structure:

```markdown
# Product Requirements Document

## 1. Overview

### 1.1 Product Vision
[One paragraph — from VISION_BRIEF.md, refined by research]

### 1.2 Problem Statement
[From VISION_BRIEF.md, validated by RESEARCH_FINDINGS.md]
Evidence: [cite specific research findings]

### 1.3 Target Users
[From PERSONAS.md — reference primary persona by name]

### 1.4 Success Metrics
[From METRICS_FRAMEWORK.md — north star + input metrics]

### 1.5 Scope
- **In scope (MVP):** [from SCOPE_DEFINITION.md Must Haves]
- **Out of scope:** [from SCOPE_DEFINITION.md Won't Haves with rationale]

---

## 2. User Stories

[Organized by user journey phase from USER_JOURNEYS.md]

### 2.1 [Journey Phase 1: e.g., Onboarding]

#### US-001: [Story Title]
**As a** [persona name],
**I want to** [capability],
**so that** [outcome — mapped to specific JTBD outcome and opportunity score].

**Evidence:** This addresses JTBD outcome [#] (opportunity score: [X]) and competitive gap [Y from COMPETITIVE_LANDSCAPE.md].

**Acceptance Criteria:**

```gherkin
Scenario: [Happy path]
  Given [precondition]
  When [action]
  Then [expected result]

Scenario: [Edge case]
  Given [precondition]
  When [action]
  Then [expected result]

Scenario: [Error case]
  Given [precondition]
  When [action]
  Then [expected error handling]
```

**UI Reference:** Screen [X] from INFORMATION_ARCHITECTURE.md
**Priority:** Must Have | Should Have
**Dependencies:** [US-XXX if any]

---
[Repeat for all user stories]
```

## Step 3: Non-Functional Requirements

**Browser: 2-3 searches for benchmarks**
- Search for "[product type] performance benchmarks"
- Search for "[product type] security requirements"

```markdown
## 3. Non-Functional Requirements

### 3.1 Performance
| Metric | Target | Benchmark Source |
|--------|--------|-----------------|
| Page load (LCP) | < [X]s | [source] |
| API response (p95) | < [X]ms | [source] |
| Time to interactive | < [X]s | [source] |
| Bundle size | < [X]KB | [standard] |

### 3.2 Security
- Authentication: [method from TECH_STACK.md]
- Authorization: [model]
- Data encryption: [at rest, in transit]
- Input validation: [all user inputs, server-side]
- OWASP Top 10 compliance: Required
[Reference RESEARCH_FINDINGS.md security section]

### 3.3 Accessibility
- WCAG 2.1 Level AA compliance: Required
- Keyboard navigation: All interactive elements
- Screen reader support: All content and actions
[Reference DESIGN_SYSTEM.md accessibility section]

### 3.4 Scalability
- Concurrent users: [target]
- Data volume: [expected growth]
- Scaling strategy: [horizontal/vertical, from FEASIBILITY_ASSESSMENT.md]

### 3.5 Reliability
- Uptime target: [e.g., 99.9%]
- Error budget: [e.g., 43.8 minutes/month]
- Recovery time objective: [RTO]
- Recovery point objective: [RPO]

### 3.6 Compatibility
- Browsers: [list with versions]
- Devices: [responsive breakpoints from DESIGN_SYSTEM.md]
- OS: [if mobile, from TECH_STACK.md]
```

## Step 4: Data Requirements

```markdown
## 4. Data Requirements

### 4.1 Data Objects
[From INFORMATION_ARCHITECTURE.md content inventory]
| Object | Key Attributes | Relationships | Lifecycle |
|--------|---------------|---------------|-----------|

### 4.2 Data Validation Rules
| Field | Type | Required | Constraints | Error Message |
|-------|------|----------|-------------|---------------|

### 4.3 Data Retention & Privacy
- PII fields: [list]
- Retention policy: [duration, deletion strategy]
- Export capability: [required formats]
- GDPR/privacy compliance: [requirements]
```

## Step 5: Integration Requirements

```markdown
## 5. Integrations
[From BUILD_VS_BUY.md — bought/OSS components that require integration]

| Integration | Purpose | API Type | Auth Method | Data Flow |
|-------------|---------|----------|-------------|-----------|
```

## Step 6: Cross-Reference Audit

Before finalizing, verify:
- [ ] Every Must Have capability from SCOPE_DEFINITION.md has at least one user story
- [ ] Every user story maps to a JTBD outcome
- [ ] Every user story has acceptance criteria with happy path, edge case, and error scenario
- [ ] Every user story references a screen from INFORMATION_ARCHITECTURE.md
- [ ] NFR targets have benchmark sources
- [ ] No capabilities snuck in that aren't in SCOPE_DEFINITION.md

## Step 7: Update Pipeline State

```markdown
## Stage 06 — Product Specification ✅
- PRD.md produced ([N] user stories, [M] NFRs)
- All stories traced to JTBD outcomes
- All stories have Given/When/Then acceptance criteria
## Next Stage: 07 — Architecture Specification
```

Proceed to Stage 07.
