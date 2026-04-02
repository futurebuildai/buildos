---
description: Decompose an epic or feature into INVEST-compliant user stories with Given/When/Then acceptance criteria. Produces a story map showing the narrative flow and a dependency graph for implementation ordering.
invoked_from:
  - workflows/antigravity/06_product_spec.md
  - workflows/antigravity/feature_iteration.md
  - On-demand for story breakdown
produces: User stories section in PRD or standalone story map document
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: User Story Decomposition

Take an epic or feature description and break it down into small, well-formed user stories that a development team can estimate, build, and verify independently.

---

## Input

- **Epic or feature description** — from SCOPE_DEFINITION.md, PRD, or user-provided description
- **Personas** — from PERSONAS.md (who is performing the actions)
- **User journeys** — from USER_JOURNEYS.md (context for the stories)
- **Acceptance criteria context** — any non-functional requirements, edge cases, or constraints

---

## Step 1: Understand the Epic

Before decomposing, fully understand the scope:

1. **Identify the job-to-be-done** this epic addresses.
2. **Identify the persona(s)** who will use this capability.
3. **Map the end-to-end flow** at a high level (what happens from start to finish).
4. **Identify the boundaries** — what is in scope and what is explicitly out of scope.
5. **List known constraints** — technical limitations, design system rules, regulatory requirements.

```markdown
## Epic Understanding

**Epic:** [name]
**Job:** [JTBD outcome this addresses]
**Primary Persona:** [who]
**End-to-end flow:** [high-level sequence]
**In scope:** [explicit inclusions]
**Out of scope:** [explicit exclusions]
**Constraints:** [list]
```

---

## Step 2: Identify Story Slices

Decompose the epic into vertical slices — each slice delivers a thin but complete piece of user value.

### Slicing Strategies

Apply these strategies in order of preference:

1. **Workflow steps.** If the epic has a multi-step flow, each step can be a story.
2. **Business rules.** If different rules apply to different cases, each rule variation is a story.
3. **Data variations.** If the feature handles different data types or input formats, each is a story.
4. **Interfaces/platforms.** If the feature works on multiple interfaces (web, mobile, API), each is a story.
5. **CRUD operations.** Create, Read, Update, Delete as separate stories.
6. **Happy path first, then edge cases.** The happy path is story 1; error handling, validation, edge cases are subsequent stories.
7. **Performance tiers.** Basic implementation first, then optimization as a separate story.

### Anti-Patterns to Avoid

- **Horizontal slicing** (e.g., "build the database layer," "build the API," "build the UI") — these are tasks, not stories. Each story must deliver end-to-end value.
- **Stories that are just tasks** (e.g., "Set up the CI pipeline") — rephrase in terms of user value or classify as technical enabler stories.
- **Mega-stories** that take more than a few days — decompose further.
- **Micro-stories** that have no independently testable behavior — merge with related stories.

---

## Step 3: Write User Stories (INVEST Compliant)

Each story must satisfy the INVEST criteria:

| Criterion | Definition | Test |
|-----------|-----------|------|
| **I**ndependent | Can be developed in any order relative to other stories | No required sequence (though some ordering may be preferred) |
| **N**egotiable | Details can be discussed and adjusted | Story is not over-specified; leaves room for implementation decisions |
| **V**aluable | Delivers value to a user or stakeholder | A user can DO something they could not do before |
| **E**stimable | Team can estimate the effort | Scope is clear enough to size; no major unknowns |
| **S**mall | Can be completed within a single sprint/iteration | 1-5 days of work for a developer |
| **T**estable | Has clear acceptance criteria that can be verified | Given/When/Then scenarios are defined |

### Story Format

```markdown
### Story [ID]: [Short descriptive title]

**As a** [persona],
**I want to** [action/capability],
**So that** [benefit/outcome].

**Size:** XS / S / M / L (if L, consider further decomposition)
**Priority:** Must / Should / Could (from MoSCoW)
**JTBD Outcome:** [reference to opportunity score if available]
```

### Story Numbering Convention

Use the format: `[EPIC_PREFIX]-[NUMBER]`
- Example: `AUTH-001`, `AUTH-002`, `DASH-001`

---

## Step 4: Write Acceptance Criteria (Given/When/Then)

Every story gets acceptance criteria in Gherkin format. These become the test specifications.

### Format

```gherkin
**Acceptance Criteria:**

Scenario 1: [descriptive name — happy path]
  Given [precondition / initial state]
  And [additional precondition if needed]
  When [action the user takes]
  And [additional action if needed]
  Then [expected outcome]
  And [additional expected outcome if needed]

Scenario 2: [descriptive name — alternate path]
  Given [precondition]
  When [action]
  Then [expected outcome]

Scenario 3: [descriptive name — error case]
  Given [precondition]
  When [action that should fail]
  Then [expected error behavior]
  And [system state after error]
```

### Acceptance Criteria Rules

1. **Every story needs at least 2 scenarios:** the happy path and at least one error/edge case.
2. **Be specific in Given/When/Then.** Not "Given a user is logged in" but "Given a user with role 'editor' is authenticated and has at least one project."
3. **Include boundary conditions.** Empty states, maximum limits, concurrent access, timeout behavior.
4. **Include non-functional criteria where relevant.** "Then the results appear within 200ms" or "Then the action is logged in the audit trail."
5. **Do not over-specify implementation.** Say "Then the user sees a confirmation" not "Then a green toast notification appears in the top-right corner for 3 seconds." (Unless UI specs dictate this.)

### Non-Functional Acceptance Criteria

For stories with performance, security, or accessibility requirements, add:

```markdown
**Non-Functional Criteria:**
- Performance: [specific threshold — e.g., "response time < 500ms at p95"]
- Accessibility: [WCAG level and specific requirements]
- Security: [authentication, authorization, data handling requirements]
- Data: [validation rules, format constraints]
```

---

## Step 5: Build the Story Map

Arrange stories into a visual story map that shows the narrative flow.

### Story Map Structure

```markdown
## Story Map

### Backbone (User Activities — left to right)
[Activity 1] → [Activity 2] → [Activity 3] → [Activity 4]

### Walking Skeleton (Minimum viable flow — one story per activity)
| Activity 1 | Activity 2 | Activity 3 | Activity 4 |
|-----------|-----------|-----------|-----------|
| [EPIC-001] | [EPIC-003] | [EPIC-006] | [EPIC-009] |

### Release 1 (MVP)
| Activity 1 | Activity 2 | Activity 3 | Activity 4 |
|-----------|-----------|-----------|-----------|
| [EPIC-001] | [EPIC-003] | [EPIC-006] | [EPIC-009] |
| [EPIC-002] | [EPIC-004] | [EPIC-007] | [EPIC-010] |
|            | [EPIC-005] |            |            |

### Release 2
| Activity 1 | Activity 2 | Activity 3 | Activity 4 |
|-----------|-----------|-----------|-----------|
|            |            | [EPIC-008] | [EPIC-011] |
|            |            |            | [EPIC-012] |
```

### Story Map Rules

1. **Read left-to-right for user flow.** The backbone tells the story of how a user accomplishes the job.
2. **Read top-to-bottom for priority.** Higher stories are more important.
3. **Draw the release line.** Everything above the line ships in that release; below defers.
4. **The walking skeleton is the absolute minimum** — one story per activity that makes the flow complete end-to-end.

---

## Step 6: Dependency Graph

Map dependencies between stories to identify implementation order and parallelization opportunities.

```markdown
## Dependency Graph

\`\`\`mermaid
graph TD
    EPIC-001[AUTH-001: User registration] --> EPIC-002[AUTH-002: Email verification]
    EPIC-001 --> EPIC-003[AUTH-003: Login flow]
    EPIC-003 --> EPIC-004[DASH-001: View dashboard]
    EPIC-004 --> EPIC-005[DASH-002: Filter dashboard]
    EPIC-004 --> EPIC-006[DASH-003: Export dashboard data]
    EPIC-001 --> EPIC-007[PROF-001: Edit profile]

    style EPIC-001 fill:#ff9999
    style EPIC-003 fill:#ff9999
    style EPIC-004 fill:#ff9999
\`\`\`

### Critical Path
The longest dependency chain (highlighted in red above):
AUTH-001 → AUTH-003 → DASH-001 → [DASH-002 | DASH-003]

**Critical path length:** [N] stories
**Parallelization opportunities:**
- After AUTH-001: AUTH-002, AUTH-003, and PROF-001 can proceed in parallel
- After DASH-001: DASH-002 and DASH-003 can proceed in parallel

### Dependency Types
| From | To | Type | Notes |
|------|----|------|-------|
| AUTH-001 | AUTH-003 | Hard (functional) | Login requires registration to exist |
| DASH-001 | DASH-002 | Soft (convenience) | Filtering could use mock data initially |
```

---

## Step 7: Technical Enabler Stories

Some work does not deliver direct user value but is necessary to enable user stories. Track these separately:

```markdown
## Technical Enabler Stories

| # | Enabler | Enables Stories | Rationale |
|---|---------|---------------|-----------|
| TE-001 | Set up authentication middleware | AUTH-001, AUTH-002, AUTH-003 | Required infrastructure for all auth stories |
| TE-002 | Create database schema for [domain] | Multiple | Shared data layer |
| TE-003 | Configure CI/CD pipeline | All | Required for deployment |
```

Technical enablers should be:
- Tied to specific user stories they enable
- Sized and prioritized like regular stories
- Completed just-in-time (not all up front)

---

## Output Compilation

Compile all stories into a structured document:

```markdown
# Story Decomposition: [Epic Name]

## Epic Summary
[From Step 1]

## Story Map
[From Step 5]

## User Stories

### [Story ID]: [Title]
As a [persona], I want to [action], so that [benefit].
Size: [XS/S/M/L] | Priority: [Must/Should/Could] | Depends on: [story IDs]

Acceptance Criteria:
[Gherkin scenarios from Step 4]

---
[Repeat for each story]

## Dependency Graph
[From Step 6]

## Technical Enablers
[From Step 7]

## Summary Statistics
- Total stories: [N]
- Must Have: [N] | Should Have: [N] | Could Have: [N]
- Critical path length: [N] stories
- Estimated parallelism: [N] parallel tracks possible
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] Every story satisfies all INVEST criteria
- [ ] Every story has at least 2 Given/When/Then scenarios (happy path + error case)
- [ ] No story is larger than L (if so, decompose further)
- [ ] The story map walking skeleton forms a complete end-to-end user flow
- [ ] Dependency graph has no circular dependencies
- [ ] Critical path is identified and highlighted
- [ ] Technical enablers are linked to the user stories they enable
- [ ] All stories trace back to the original epic scope (no scope creep)
- [ ] Out-of-scope items from Step 1 are not represented as stories
