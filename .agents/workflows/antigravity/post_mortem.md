---
description: Conduct a structured post-mortem analysis of an incident or failure. Builds a timeline, performs root cause analysis using the 5 Whys technique, researches industry prevention patterns, and produces actionable architecture and testing changes.
stage: "post-v1"
inputs: Incident description, PROJECT_STATE.md, ARCHITECTURE.md, TESTING_STRATEGY.md, AUDIT_LOGS.md (if available)
outputs: POST_MORTEM.md, PREVENTION_SPEC.md
browser_usage: Moderate (industry incident patterns, prevention best practices, reliability engineering)
---

# Post-v1 Workflow: Post-Mortem

The team experienced an incident, outage, failure, or significant unexpected behavior. Your job is to conduct a blameless, thorough post-mortem that identifies exactly what happened, why it happened, what the testing and monitoring gaps were, and what concrete changes will prevent recurrence.

**Principles:**
- **Blameless:** Focus on systems and processes, not individuals
- **Thorough:** Go beyond the immediate cause to systemic factors
- **Actionable:** Every finding must produce a concrete, implementable change
- **Evidence-based:** All claims must be traced to logs, timestamps, or research

---

## Step 1: Gather Incident Context

Collect all available information about the incident:

1. **User's incident description** — what they observed, when, impact
2. **`.agents/handoff/PROJECT_STATE.md`** — system state, recent deployments, active changes
3. **`.agents/handoff/ARCHITECTURE.md`** — system architecture (to understand failure domains)
4. **`.agents/handoff/TESTING_STRATEGY.md`** — current testing approach (to identify gaps)
5. **`.agents/handoff/AUDIT_LOGS.md`** (if exists) — any logged events from the system
6. **`.agents/TECH_STACK.md`** — technologies involved

Ask the user for any additional context:
- Exact times (when detected, when resolved, when incident started if known)
- Who was affected (all users, subset, internal only)
- What actions were taken to resolve (and when)
- Any error messages, log excerpts, screenshots, or monitoring alerts
- Whether this or a similar incident has happened before

If critical timeline information is missing, ask before proceeding. An accurate timeline is essential.

## Step 2: Build the Incident Timeline

Construct a precise, chronological timeline of the incident:

```markdown
## Incident Timeline

### Pre-Incident Context
- **Last deployment:** [date/time — what was deployed]
- **System state:** [normal / degraded / under load]
- **Recent changes:** [from PROJECT_STATE.md — anything changed recently]

### Timeline
| Time (UTC) | Event | Source | Category |
|------------|-------|--------|----------|
| [T-?] | [Last known good state / normal operation] | [source] | Baseline |
| [T-?] | [First triggering event — deploy, config change, traffic spike, etc.] | [source] | Trigger |
| [T-?] | [First system anomaly — error rate increase, latency spike, etc.] | [source] | Symptom |
| [T+0] | **Incident detected** — [how: alert, user report, monitoring] | [source] | Detection |
| [T+?] | [First response action — who did what] | [source] | Response |
| [T+?] | [Diagnosis steps taken] | [source] | Investigation |
| [T+?] | [Mitigation applied — what was done] | [source] | Mitigation |
| [T+?] | [Service restored / incident resolved] | [source] | Resolution |
| [T+?] | [Post-incident verification — how was recovery confirmed] | [source] | Verification |

### Key Metrics
- **Time to detection (TTD):** [how long from first anomaly to detection]
- **Time to mitigation (TTM):** [how long from detection to mitigation]
- **Time to resolution (TTR):** [how long from detection to full resolution]
- **Total incident duration:** [first anomaly to resolution]
- **User impact duration:** [how long users were affected]
```

## Step 3: Root Cause Analysis — 5 Whys

Starting from the incident symptom, ask "Why?" five times to drill to the root cause. Each answer must be supported by evidence.

```markdown
## Root Cause Analysis

### The 5 Whys

**Symptom:** [What the user/system experienced]

**Why 1:** Why did [symptom] occur?
→ Because [direct cause]. Evidence: [log entry, architecture trace, data]

**Why 2:** Why did [direct cause] happen?
→ Because [deeper cause]. Evidence: [log entry, code analysis, architecture gap]

**Why 3:** Why did [deeper cause] happen?
→ Because [systemic cause]. Evidence: [process gap, design decision, missing safeguard]

**Why 4:** Why did [systemic cause] exist?
→ Because [organizational/process cause]. Evidence: [missing test, missing monitoring, spec gap]

**Why 5:** Why did [organizational cause] persist?
→ Because [root cause]. Evidence: [fundamental gap in process, architecture, or testing]

### Root Cause Statement
[One clear sentence identifying the root cause]

### Contributing Factors
These did not directly cause the incident but made it worse or more likely:

| Factor | How It Contributed | Category |
|--------|-------------------|----------|
| [factor] | [explanation] | Architecture / Testing / Process / Monitoring / Documentation |
| [factor] | [explanation] | Architecture / Testing / Process / Monitoring / Documentation |
| [factor] | [explanation] | Architecture / Testing / Process / Monitoring / Documentation |
```

## Step 4: Analyze Detection and Response

Evaluate how the incident was detected and handled:

```markdown
## Detection Analysis

### How Was the Incident Detected?
- **Detection method:** [Automated alert / User report / Manual observation]
- **Detection latency:** [time from first anomaly to detection]
- **Should have been detected by:** [what monitoring should have caught this earlier]

### Why Was Detection Delayed? (if applicable)
- [Missing monitoring on [X]]
- [Alert threshold too permissive]
- [No health check for [component]]
- [Log level insufficient for [module]]

### Response Analysis
| Response Action | Time | Effectiveness | Could Be Faster? |
|----------------|------|---------------|-------------------|
| [action] | [time after detection] | [effective/ineffective] | [yes — how] |

### Escalation Path
- Was the right person/team engaged? [yes/no — who should have been]
- Was documentation sufficient for on-call to diagnose? [yes/no — what was missing]
```

## Step 5: Analyze Testing Gaps

Determine what the testing strategy missed:

```markdown
## Testing Gap Analysis

### What Tests Should Have Prevented This
| Test Type | Scenario | Should Have Caught | Why It Was Missing |
|-----------|----------|-------------------|-------------------|
| [unit] | [scenario] | [root cause defect] | [not tested / wrong assertion] |
| [integration] | [scenario] | [interaction failure] | [missing test / incomplete mock] |
| [e2e] | [scenario] | [user-facing symptom] | [not in test suite / environment differs] |
| [load] | [scenario] | [capacity issue] | [no load testing / unrealistic load profile] |
| [chaos] | [scenario] | [failure mode] | [no chaos testing] |

### Test Environment vs Production Differences
| Aspect | Test Environment | Production | Gap Impact |
|--------|-----------------|-----------|-----------|
| [data volume] | [small seed data] | [millions of records] | [query perf differs] |
| [concurrency] | [single user] | [hundreds concurrent] | [race conditions missed] |
| [infrastructure] | [local/single node] | [distributed/multi-node] | [network partitions missed] |
| [third-party services] | [mocked] | [real with latency/failures] | [timeout handling missed] |
```

## Step 6: Research Industry Patterns

**Browser: 10-20 searches**

### 6a: Similar Incidents
- Search for "[root cause type] incident post-mortem"
- Search for "[technology] [failure mode] post-mortem"
- Search for "[service type] outage analysis [current year]"
- Look for public post-mortems from companies using similar tech stacks (Google, Amazon, Cloudflare, GitHub, etc.)
- Search Hacker News for "[failure mode] post-mortem" discussion threads

### 6b: Prevention Patterns
- Search for "[root cause] prevention strategies"
- Search for "[failure mode] resilience patterns"
- Search for "[technology] reliability best practices [current year]"
- Search for circuit breaker, bulkhead, retry, and other resilience patterns relevant to the failure
- Search for "[monitoring/observability tool from TECH_STACK.md] alerting best practices"

### 6c: Industry Standards
- Search for "site reliability engineering [failure domain]"
- Search for "[industry] incident management best practices"
- Look for runbook templates and incident response frameworks
- Search for chaos engineering approaches relevant to the failure mode

Document findings:
```markdown
## Research Findings

### Similar Incidents in Industry
| Company/Project | Incident | Root Cause | Prevention Applied | Source URL |
|-----------------|----------|-----------|-------------------|-----------|

### Prevention Patterns Found
| Pattern | Description | Applicability | Source URL |
|---------|-------------|--------------|-----------|

### Reliability Engineering Practices
| Practice | Description | Implementation Effort | Source URL |
|----------|-------------|----------------------|-----------|
```

## Step 7: Produce POST_MORTEM.md

Write `.agents/handoff/POST_MORTEM.md`:

```markdown
# Post-Mortem: [Incident Title]

**Date:** [incident date]
**Severity:** P[0-3]
**Duration:** [total incident duration]
**User Impact:** [description of user-facing impact]
**Status:** [Resolved / Monitoring / Partially resolved]

---

## Executive Summary
[3-5 sentences: what happened, root cause, impact, resolution, key actions]

---

## Timeline
[Complete timeline from Step 2]

---

## Root Cause
**Root cause:** [one sentence from 5 Whys analysis]

### 5 Whys Analysis
[Full 5 Whys chain from Step 3]

### Contributing Factors
[Table from Step 3]

---

## Impact Assessment

### User Impact
| Metric | Value |
|--------|-------|
| Users affected | [number or percentage] |
| Duration of impact | [time] |
| Data loss | [none / description] |
| Revenue impact | [none / estimated if known] |
| SLA/SLO breach | [yes — which / no] |

### System Impact
| Component | Impact | Recovery Status |
|-----------|--------|----------------|
| [component] | [description] | Fully recovered / Degraded / Needs repair |

---

## Detection and Response
[Full detection and response analysis from Step 4]

---

## What Went Well
[Things that worked during the incident — be specific]
1. [E.g., "Alert triggered within 5 minutes of anomaly"]
2. [E.g., "Rollback procedure worked as documented"]
3. [E.g., "Database backups were current and restorable"]

## What Went Poorly
[Things that failed or were slower than needed — be specific]
1. [E.g., "No monitoring on the affected queue depth"]
2. [E.g., "Runbook did not cover this failure mode"]
3. [E.g., "Integration test mocked the service that actually failed"]

## Where We Got Lucky
[Things that could have been worse — be specific about the luck factor]
1. [E.g., "Low traffic period — in peak hours, impact would have been 10x"]
2. [E.g., "User reported within minutes — no automated alert would have caught this for hours"]

---

## Testing Strategy Gaps
[Full analysis from Step 5]

---

## Industry Research
[Key findings from Step 6 — patterns and practices others use to prevent this]

---

## Action Items

### Immediate (This Sprint)
| # | Action | Owner | Category | Prevents |
|---|--------|-------|----------|----------|
| 1 | [specific action] | [Antigravity/Claude Code] | Fix/Monitor/Test | Recurrence of this exact incident |

### Short-Term (Next 2 Sprints)
| # | Action | Owner | Category | Prevents |
|---|--------|-------|----------|----------|
| 2 | [specific action] | [Antigravity/Claude Code] | Architecture/Testing | Class of similar incidents |

### Long-Term (Backlog)
| # | Action | Owner | Category | Prevents |
|---|--------|-------|----------|----------|
| 3 | [specific action] | [Antigravity/Claude Code] | Process/Culture | Systemic risk |

---

## Lessons Learned
1. [Lesson 1: stated as a principle that should guide future decisions]
2. [Lesson 2]
3. [Lesson 3]
```

## Step 8: Produce PREVENTION_SPEC.md

Write `.agents/handoff/PREVENTION_SPEC.md` — the actionable specification that Claude Code will execute:

```markdown
# Prevention Specification: [Incident Title]

## Summary
This specification defines concrete architecture, testing, and monitoring changes to prevent recurrence of [incident title] and the class of incidents it belongs to.

---

## Architecture Changes

### Change A-1: [Title]
**Category:** Resilience / Error Handling / Data Integrity / Configuration / Deployment
**Prevents:** [specific failure mode]

**Current state:**
```
[Current architecture/code pattern that enabled the failure]
```

**Target state:**
```
[New architecture/code pattern that prevents the failure]
```

**Implementation:**
1. [Step-by-step implementation instructions for Claude Code]
2. [Step 2]
3. [Step N]

**Verification:**
- [ ] [Test that proves this change prevents the failure mode]
- [ ] [Existing tests still pass]

[Repeat for each architecture change]

---

## Testing Changes

### Change T-1: [Title]
**Category:** Unit / Integration / E2E / Load / Chaos / Monitoring
**Gap addressed:** [which testing gap from the post-mortem]

**New tests to add:**
| Test | Type | Module/Endpoint | Scenario | Assertion |
|------|------|----------------|----------|-----------|
| [test] | [type] | [module] | [scenario that would have caught the incident] | [expected behavior] |

**Existing tests to strengthen:**
| Test | Current Assertion | Improved Assertion | Why |
|------|------------------|-------------------|-----|
| [test] | [what it checks now] | [what it should check] | [gap it closes] |

**Test environment changes:**
| Aspect | Current | Proposed | Rationale |
|--------|---------|----------|-----------|
| [aspect] | [current setup] | [proposed setup] | [closer to production] |

[Repeat for each testing change]

---

## Monitoring and Alerting Changes

### Change M-1: [Title]
**Category:** Health Check / Alert / Dashboard / Log Level / Metric
**Detection gap addressed:** [what should have been detected earlier]

**New monitoring:**
| Monitor | Metric/Signal | Threshold | Alert Channel | Severity |
|---------|--------------|-----------|---------------|----------|
| [name] | [what to watch] | [when to alert] | [where to notify] | [P0-P3] |

**New health checks:**
| Service | Endpoint | Check | Interval | Failure Action |
|---------|----------|-------|----------|---------------|
| [service] | [path] | [what to verify] | [frequency] | [what happens on failure] |

**Logging improvements:**
| Module | Current Log Level | New Log Level | New Log Events |
|--------|------------------|--------------|---------------|
| [module] | [current] | [proposed] | [what new events to log] |

---

## Runbook Updates

### Runbook R-1: [Failure Scenario Title]
**Trigger:** [when to use this runbook — alert name, symptom observed]

**Diagnosis steps:**
1. [Check X — what to look for, where to look]
2. [Check Y — what to look for, where to look]
3. [Check Z — what to look for, where to look]

**Mitigation steps:**
1. [Action 1 — exact command or procedure]
2. [Action 2 — exact command or procedure]
3. [Escalation — when and to whom]

**Verification steps:**
1. [How to confirm the mitigation worked]
2. [How to confirm no secondary damage]

---

## Execution Order

Claude Code must implement prevention changes in this order:

### Phase 1: Immediate Prevention (This Sprint)
1. [ ] Add monitoring/alerting changes (M-1, M-2, ...)
2. [ ] Add regression tests that would have caught this incident (T-1, ...)
3. [ ] Apply critical architecture fix (A-1)
4. [ ] Verify: all new tests pass, all existing tests pass

### Phase 2: Hardening (Next Sprint)
5. [ ] Apply remaining architecture changes (A-2, A-3, ...)
6. [ ] Strengthen existing tests (T-2, ...)
7. [ ] Add runbook documentation (R-1, ...)
8. [ ] Verify: full test suite passes, monitoring operational

### Phase 3: Systemic Improvements (Backlog)
9. [ ] [Longer-term improvements from research findings]
10. [ ] [Process changes]

---

## Verification Checklist
- [ ] Original incident scenario no longer produces failure (tested)
- [ ] Similar scenarios in the same failure class are also prevented (tested)
- [ ] All new monitoring/alerting fires correctly when conditions are simulated
- [ ] All existing tests still pass (zero regressions)
- [ ] Runbook is complete and actionable
- [ ] ARCHITECTURE.md updated to reflect changes
- [ ] TESTING_STRATEGY.md updated with new test coverage
```

## Step 9: Present Post-Mortem

Present to the user:

1. **Incident summary** — what happened and impact
2. **Root cause** — the 5 Whys result in one sentence
3. **Key contributing factors** — top 3 systemic issues
4. **Testing gaps** — what the testing strategy missed
5. **Prevention plan** — summary of architecture, testing, and monitoring changes
6. **Industry context** — how others prevent similar incidents
7. **Execution phases** — immediate, short-term, and long-term actions

Ask the user: **"Post-mortem is complete. Should I hand off the prevention specification to Claude Code for implementation?"**

## Step 10: Update Pipeline State

Update `.agents/handoff/PROJECT_STATE.md`:
```markdown
## Active Work: Post-Mortem — [Incident Title]
- POST_MORTEM.md produced (root cause identified, [N] action items)
- PREVENTION_SPEC.md produced ([N] architecture changes, [M] testing changes, [K] monitoring changes)
- Execution phases: [N] immediate, [M] short-term, [K] long-term actions
- Awaiting handoff to Claude Code for prevention implementation
```
