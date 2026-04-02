---
description: Systematically triage a bug report, trace it through the architecture, research known issues, and produce a surgical fix specification with regression tests for Claude Code to execute.
stage: "post-v1"
inputs: Bug description, PROJECT_STATE.md, ARCHITECTURE.md, TESTING_STRATEGY.md
outputs: BUG_REPORT.md, FIX_SPEC.md, updated TESTING_STRATEGY.md
browser_usage: Moderate (library issue trackers, known bugs, Stack Overflow)
---

# Post-v1 Workflow: Bug Triage

The user has reported a bug or unexpected behavior. Your job is to systematically understand the bug, trace it through the architecture to identify root cause, research whether it is a known issue, and produce a precise fix specification that Claude Code can execute without introducing regressions.

---

## Step 1: Gather Bug Context

Read the user's bug description and extract all available information:

1. **What happened** — the observed behavior
2. **What was expected** — the correct behavior
3. **How to reproduce** — steps the user took (or can be inferred)
4. **When it started** — after a deploy, a specific action, or always existed
5. **Frequency** — always, intermittent, only under specific conditions
6. **Environment** — browser, OS, device, user account type, data state
7. **Error messages** — exact error text, console output, log entries
8. **Screenshots/logs** — any visual evidence the user provided

If the bug description is vague, list what information is missing and ask the user before proceeding. Do NOT guess at reproduction steps.

## Step 2: Load System Context

Read the following to understand the system the bug lives in:

1. **`.agents/handoff/PROJECT_STATE.md`** — current system state, recent changes
2. **`.agents/handoff/ARCHITECTURE.md`** — system architecture and module boundaries
3. **`.agents/handoff/API_CONTRACT.md`** — API contracts for affected endpoints
4. **`.agents/handoff/TESTING_STRATEGY.md`** — what tests exist and what gaps there might be
5. **`.agents/TECH_STACK.md`** — libraries and frameworks in use

Note: Pay special attention to recent changes in PROJECT_STATE.md. Bugs frequently correlate with the most recent modifications.

## Step 3: Trace Through Architecture

Starting from the user-facing symptom, trace the bug through every architectural layer it touches:

### 3a: Identify Affected Components
```markdown
## Affected Component Trace
1. **UI Layer:** [component/page where bug manifests]
   - Component: [name from DESIGN_SYSTEM.md]
   - State management: [what state is involved]
   - User interaction: [what triggers the bug]

2. **API Layer:** [endpoint(s) involved]
   - Endpoint: [METHOD /path from API_CONTRACT.md]
   - Request: [what the frontend sends]
   - Response: [what comes back — correct or incorrect]

3. **Service Layer:** [business logic module(s)]
   - Module: [name from ARCHITECTURE.md]
   - Function: [specific function/method suspected]
   - Dependencies: [what this module calls]

4. **Data Layer:** [database operations involved]
   - Entity: [which data entity]
   - Operation: [read/write/update/delete]
   - Query: [suspected problematic query or data state]

5. **External Services:** [third-party integrations involved]
   - Service: [name]
   - API call: [what call is made]
   - Failure mode: [timeout, bad response, rate limit]
```

### 3b: Narrow Root Cause Hypothesis
Based on the trace, form 1-3 root cause hypotheses:

| # | Hypothesis | Layer | Evidence For | Evidence Against | Confidence |
|---|-----------|-------|-------------|-----------------|------------|
| H1 | [description] | [layer] | [why likely] | [why maybe not] | High/Medium/Low |
| H2 | [description] | [layer] | [why likely] | [why maybe not] | High/Medium/Low |

## Step 4: Research Known Issues

**Browser: 5-15 searches**

### 4a: Library-Specific Research
For each library or framework involved in the affected component trace:

- Search for "[library name] [error message or symptom]"
- Search for "[library name] [version from TECH_STACK.md] known issues"
- Search for "[library name] [version] bug"
- Check the library's GitHub Issues: search for the error message or symptom
- Check if a newer version fixes the issue: "[library name] changelog [version]"
- Search Stack Overflow: "[library name] [symptom or error]"

### 4b: Pattern Research
- Search for "[symptom description] [tech stack]"
- Search for "[error message exact text]" (in quotes)
- Search for "[domain] common bugs [tech stack]"
- Look for similar bug reports in open-source projects using the same stack

### 4c: Regression Research
If the bug was introduced by a recent change:
- Search for "[pattern used in recent change] side effects"
- Search for "[library changed] migration issues"

Document all findings:
```markdown
## Research Findings
### Known Issues Found
| Source | Description | Relevant? | URL |
|--------|-------------|-----------|-----|

### Library Bugs
| Library | Version | Known Bug | Fix Available | URL |
|---------|---------|-----------|--------------|-----|

### Stack Overflow / Community
| Question | Answer Summary | Applicable? | URL |
|----------|---------------|-------------|-----|

### Root Cause Verdict
Based on research: [Confirm or update hypothesis, cite evidence]
```

## Step 5: Severity Classification

Classify the bug using a P0-P3 scale:

| Severity | Criteria | Response Time |
|----------|----------|---------------|
| **P0 — Critical** | System down, data loss, security vulnerability, all users affected | Immediate fix |
| **P1 — High** | Core feature broken, significant user impact, no workaround | Fix in current sprint |
| **P2 — Medium** | Feature degraded, workaround exists, subset of users affected | Fix in next sprint |
| **P3 — Low** | Cosmetic issue, edge case, minimal user impact | Backlog |

```markdown
## Severity Assessment
**Classification:** P[0-3]
**Justification:**
- User impact: [who is affected and how severely]
- Frequency: [how often it occurs]
- Workaround: [exists / does not exist — describe if exists]
- Data impact: [any data corruption or loss risk]
- Security impact: [any security implications]
```

## Step 6: Produce BUG_REPORT.md

Write `.agents/handoff/BUG_REPORT.md`:

```markdown
# Bug Report: [Short Title]

## Summary
[One sentence describing the bug]

## Severity: P[0-3]
**Justification:** [from Step 5]

## Reproduction Steps
1. **Preconditions:** [system state, user type, data required]
2. [Step 1]
3. [Step 2]
4. [Step N]
5. **Observed result:** [what happens]
6. **Expected result:** [what should happen]

**Frequency:** Always / Intermittent ([X]% of attempts) / Conditional (only when [condition])
**Environment:** [browser, OS, device specifics if relevant]

## Affected Components
| Layer | Component | Role in Bug |
|-------|-----------|-------------|
| UI | [component] | [symptom manifests here] |
| API | [endpoint] | [incorrect response / missing validation] |
| Service | [module.function] | [root cause logic error] |
| Data | [entity/query] | [incorrect data state / bad query] |
| External | [service] | [unreliable response / timeout] |

## Root Cause Analysis
**Root cause:** [precise technical description of what is wrong]
**Layer:** [which architectural layer contains the defect]
**Introduced by:** [known change / original implementation / library bug / unknown]

### Evidence
- [Evidence 1: from architecture trace]
- [Evidence 2: from browser research]
- [Evidence 3: from code analysis]

### Root Cause Chain
1. [Triggering condition]
2. [What happens at layer 1]
3. [What happens at layer 2]
4. [Resulting in the observed bug]

## What Tests Should Have Caught This
- [Test type] testing [scenario] — [was this test missing or incorrect?]
- [Why existing tests did not catch this]

## Research Findings
[Summarize relevant browser research results with source URLs]
```

## Step 7: Produce FIX_SPEC.md

Write `.agents/handoff/FIX_SPEC.md`:

```markdown
# Fix Specification: [Bug Title]

## Fix Summary
[One paragraph describing the fix approach]

## Surgical Changes

### Change 1: [Module/File Description]
**File:** [path relative to project root from ARCHITECTURE.md]
**Module:** [from ARCHITECTURE.md]
**Change type:** Bug fix / Missing validation / Error handling / Data migration / Dependency update

**Current behavior:**
```
[Pseudocode or description of current broken logic]
```

**Fixed behavior:**
```
[Pseudocode or description of correct logic]
```

**Why this fixes it:** [explanation of how this change addresses the root cause]

**Side effects:** [NONE — or list any potential side effects and why they are acceptable]

[Repeat for each change needed]

---

## Interface Impact

### Interfaces That Change
| Interface | Change | Backward Compatible | Consumers Affected |
|-----------|--------|--------------------|--------------------|
| [interface] | [description] | Yes/No | [list of callers] |

### Interfaces That Must NOT Change
| Interface | Reason |
|-----------|--------|
| [interface] | [this is a public API / consumed by N modules / contract with frontend] |

---

## Data Migration (if applicable)
**Required:** Yes / No

If yes:
```markdown
### Migration Steps
1. [Step 1]
2. [Step 2]

### Rollback Steps
1. [Step 1]
2. [Step 2]

### Data Validation
- [ ] [Check that validates migration succeeded]
```

---

## Dependency Changes (if applicable)
| Package | Current Version | New Version | Reason | Breaking Changes |
|---------|----------------|-------------|--------|-----------------|

---

## Regression Tests to Add

### Unit Tests
| Test | Module | Asserts | Priority |
|------|--------|---------|----------|
| [test name] | [module] | [what it verifies] | Must Have |
| [test name] | [module] | [edge case from bug] | Must Have |

### Integration Tests
| Test | Endpoint/Flow | Asserts | Priority |
|------|--------------|---------|----------|
| [test name] | [endpoint] | [what it verifies] | Must Have |

### E2E Tests (if applicable)
| Test | Journey | Asserts | Priority |
|------|---------|---------|----------|
| [test name] | [user flow] | [the bug no longer occurs] | Must Have |

---

## Execution Order

Claude Code must execute the fix in this order:

1. **Write regression tests FIRST** (they should FAIL against current code)
2. Apply fix changes in order:
   - [ ] Change 1: [description]
   - [ ] Change 2: [description]
3. **Run regression tests** (they should now PASS)
4. **Run full existing test suite** (must still pass — zero regressions)
5. Run data migration (if applicable)
6. Verify fix against original reproduction steps

## Verification Checklist
- [ ] Bug reproduction steps now produce expected result
- [ ] All new regression tests pass
- [ ] All existing tests still pass
- [ ] No new lint warnings or errors
- [ ] No performance degradation on affected endpoints
- [ ] Fix addresses root cause (not just symptom)
```

## Step 8: Update TESTING_STRATEGY.md

Append to `.agents/handoff/TESTING_STRATEGY.md`:

```markdown
## Bug Fix Regression: [Bug Title] (P[0-3])

### Gap Analysis
**Why existing tests did not catch this:**
- [Reason 1: missing test scenario]
- [Reason 2: incorrect assertion]
- [Reason 3: untested edge case]

### New Regression Tests
| Test ID | Type | Module/Endpoint | Scenario | Assertion |
|---------|------|----------------|----------|-----------|
| REG-[N] | Unit | [module] | [scenario from the bug] | [expected behavior] |
| REG-[N] | Integration | [endpoint] | [scenario from the bug] | [expected response] |
| REG-[N] | E2E | [journey] | [reproduction steps] | [bug does not occur] |

### Testing Strategy Amendments
[If this bug reveals a systemic testing gap, describe the broader change needed]
- [E.g., "Add negative test cases for all validation endpoints"]
- [E.g., "Add concurrent access tests for shared resources"]
- [E.g., "Add timeout handling tests for all external service calls"]
```

## Step 9: Present Fix Specification

Present to the user:

1. **Bug summary** — what is broken and severity
2. **Root cause** — what is wrong and where
3. **Fix approach** — summary of surgical changes
4. **Impact scope** — how many files/modules/endpoints are touched
5. **Regression coverage** — what new tests will prevent recurrence
6. **Risk assessment** — any risk the fix introduces new issues

Ask the user: **"Fix specification is ready. Should I hand off to Claude Code for execution?"**

## Step 10: Update Pipeline State

Update `.agents/handoff/PROJECT_STATE.md`:
```markdown
## Active Work: Bug Fix — [Bug Title] (P[0-3])
- BUG_REPORT.md produced (root cause identified)
- FIX_SPEC.md produced ([N] surgical changes specified)
- TESTING_STRATEGY.md updated with [N] regression tests
- Awaiting handoff to Claude Code
```
