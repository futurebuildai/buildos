---
description: Final self-audit of all specification artifacts for ambiguity, contradictions, completeness, and cross-reference integrity before handoff to Claude Code.
stage: "10"
inputs: All handoff artifacts
outputs: SPEC_REVIEW.md (in handoff/)
browser_usage: Light (verify library/API status)
approval_gate: true
---

# Stage 10: Spec Review

This is the quality gate before handing off to Claude Code. Every issue found here is cheaper to fix than discovering it during implementation.

**This stage ends with the FINAL APPROVAL GATE.**

---

## Step 1: Ambiguity Scan

Read every artifact in `.agents/handoff/`. For each requirement, spec, or instruction, ask: **Could an engineer interpret this two different ways?**

Flag ambiguities:

| # | Document | Section | Ambiguous Statement | Why It's Ambiguous | Resolution |
|---|----------|---------|--------------------|--------------------|------------|

Common ambiguity patterns to check:
- Vague quantities: "fast", "many", "several", "appropriate" — replace with numbers
- Undefined behavior: "handle gracefully" — specify what happens
- Implicit requirements: "standard error handling" — which standard?
- Unclear ownership: "the system should" — which component?
- Missing edge cases: "when the user submits" — what if they submit twice rapidly?

## Step 2: Contradiction Detection

Cross-reference all artifacts for conflicts:

### PRD ↔ Architecture
- Does every PRD user story have a corresponding architecture component?
- Do PRD NFR targets match architecture performance budgets?
- Does the PRD scope match what the architecture can deliver?

### Architecture ↔ API Contract
- Does every API endpoint in API_CONTRACT.md map to an architecture handler?
- Do request/response schemas match data model definitions?
- Is the auth model consistent between architecture and API contract?

### Architecture ↔ Design System
- Does the frontend architecture support all DESIGN_SYSTEM.md component patterns?
- Does the state management approach support all interaction patterns?
- Do responsive breakpoints align between design system and frontend architecture?

### PRD ↔ Scope Definition
- Are there any PRD user stories for capabilities NOT in SCOPE_DEFINITION.md?
- Are there any SCOPE_DEFINITION.md Must Haves without a PRD user story?

### Testing Strategy ↔ Architecture
- Does every architecture component have a testing approach?
- Do the testing tools align with TECH_STACK.md?
- Are all integration points covered by integration tests?

### Execution Plan ↔ Everything
- Does every step in EXECUTION_PLAN.md reference existing architecture/design specs?
- Are all PRD user stories covered by at least one execution step?
- Are all testing requirements assigned to specific execution steps?

Flag contradictions:

| # | Document A | Document B | Contradiction | Severity | Resolution |
|---|-----------|-----------|---------------|----------|------------|

## Step 3: Completeness Check

### User Story Completeness (INVEST)
For each user story in PRD.md:
- [ ] **Independent** — can be implemented without other stories (or dependencies are noted)
- [ ] **Negotiable** — not over-specified with implementation details
- [ ] **Valuable** — delivers user value (traced to JTBD outcome)
- [ ] **Estimable** — clear enough to estimate effort
- [ ] **Small** — completable in one execution step (or decomposed)
- [ ] **Testable** — has Given/When/Then acceptance criteria

### Edge Case Enumeration
For each user-facing feature:
| Feature | Empty State? | Max Data? | Concurrent Access? | Network Failure? | Permission Denied? | Invalid Input? |
|---------|-------------|-----------|-------------------|-----------------|-------------------|---------------|

### Missing Pieces Checklist
- [ ] Error messages defined for all error states?
- [ ] Loading states defined for all async operations?
- [ ] Empty states defined for all list/data views?
- [ ] Pagination defined for all list endpoints?
- [ ] Sort/filter options defined where needed?
- [ ] Confirmation flows for destructive actions?
- [ ] Undo/recovery paths where applicable?
- [ ] Offline behavior defined (if applicable)?
- [ ] Email/notification content defined (if applicable)?

## Step 4: Library & API Verification

**Browser: 3-5 searches**

For each external library, API, or service referenced in ARCHITECTURE.md:
- Verify it still exists and is actively maintained
- Check the version referenced is current (or note if a newer version is available)
- Verify the API/interface described in the spec matches current documentation
- Check for known security vulnerabilities

| Library/Service | Version in Spec | Latest Version | Status | Issues |
|----------------|----------------|----------------|--------|--------|

## Step 5: Assumption Closure

Read `ASSUMPTIONS_REGISTER.md`. Verify:
- [ ] All high-impact assumptions have been validated (or flagged as risks)
- [ ] No unvalidated assumptions are load-bearing in the architecture
- [ ] Remaining unvalidated assumptions have mitigation strategies

## Step 6: Write Review Report

Write `.agents/handoff/SPEC_REVIEW.md`:

```markdown
# Specification Review Report

## Review Summary
- **Artifacts Reviewed:** [list]
- **Issues Found:** [N] (Critical: [X], Major: [Y], Minor: [Z])
- **Issues Resolved:** [N]
- **Issues Remaining:** [N]
- **Overall Assessment:** [Ready for Handoff / Needs Revision]

## Ambiguities Found & Resolved
[Table from Step 1]

## Contradictions Found & Resolved
[Table from Step 2]

## Completeness Gaps & Resolutions
[Findings from Step 3]

## Library/API Verification
[Table from Step 4]

## Assumption Status
[Summary from Step 5]

## Unresolved Issues
[Any issues that need user input to resolve]

## Handoff Readiness Checklist
- [ ] PRD.md — complete, unambiguous, all stories have acceptance criteria
- [ ] ARCHITECTURE.md — complete, consistent with PRD, all components specified
- [ ] API_CONTRACT.md — complete, consistent with architecture, all endpoints defined
- [ ] DESIGN_SYSTEM.md — complete, accessible, all MVP components specified
- [ ] TESTING_STRATEGY.md — complete, all components covered, tools specified
- [ ] EXECUTION_PLAN.md — complete, dependency-ordered, all stories covered
- [ ] TECH_STACK.md — filled in with all technology choices
- [ ] All cross-references consistent
- [ ] All libraries/APIs verified current
```

## Step 7: Auto-Fix Minor Issues

If the review found minor issues (typos, missing details that have obvious answers), fix them directly in the source artifacts. Log the fixes in SPEC_REVIEW.md.

If the review found major issues, list them as unresolved and present to the user.

## Step 8: FINAL APPROVAL GATE

**PAUSE HERE.** Present to the user:

1. **Review summary** — how many issues found, how many resolved
2. **Unresolved issues** (if any) — items needing user decision
3. **Handoff readiness assessment**
4. **Complete artifact inventory** — list of all documents ready for Claude Code

Ask the user: **"All specs are ready for handoff to Claude Code. Should I proceed?"**

## Step 9: Update Pipeline State

```markdown
## Stage 10 — Spec Review ✅
- SPEC_REVIEW.md produced
- [N] issues found, [M] resolved, [K] remaining
- ⏸️ FINAL APPROVAL GATE — awaiting user confirmation for handoff
## Pipeline Status: COMPLETE — Ready for Claude Code Handoff

## Complete Artifact Inventory:
- VISION_BRIEF.md
- RESEARCH_AGENDA.md
- ASSUMPTIONS_REGISTER.md
- RESEARCH_FINDINGS.md
- COMPETITIVE_LANDSCAPE.md
- PERSONAS.md
- JTBD_ANALYSIS.md
- USER_JOURNEYS.md
- SOLUTION_CANDIDATES.md
- FEASIBILITY_ASSESSMENT.md
- BUILD_VS_BUY.md
- SCOPE_DEFINITION.md
- METRICS_FRAMEWORK.md
- INFORMATION_ARCHITECTURE.md
- DESIGN_SYSTEM.md
- PRD.md
- ARCHITECTURE.md
- API_CONTRACT.md
- TESTING_STRATEGY.md
- EXECUTION_PLAN.md
- SPEC_REVIEW.md
- PIPELINE_STATE.md
```
