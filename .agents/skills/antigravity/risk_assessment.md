---
description: Systematic risk identification and assessment across product, technical, business, and security dimensions. Rates likelihood and impact, calculates severity, proposes mitigation strategies and contingency plans.
invoked_from:
  - workflows/antigravity/04_scope_and_prioritization.md
  - workflows/antigravity/08_testing_strategy.md
  - workflows/antigravity/10_spec_review.md
  - On-demand for risk analysis
produces: Risk register section in SCOPE_DEFINITION.md, ARCHITECTURE.md, or standalone risk analysis
browser_usage: None (analytical skill — operates on existing data)
---

# Skill: Risk Assessment

Systematically identify, rate, and plan mitigations for risks across all dimensions of the product. This is not a checkbox exercise — the goal is to surface risks that could kill the project and ensure there is a plan for each.

---

## Input

- **Artifact(s) to assess** — the scope, architecture, or plan being evaluated
- **Context** — what stage of the project (pre-build risks differ from post-launch risks)
- **Known constraints** — timeline, budget, team size, tech stack

---

## Step 1: Risk Identification

Systematically walk through each risk category and brainstorm risks. Use the prompts below as a checklist — not every prompt will apply, but each should be considered.

### Category 1: Product Risks

Risks that the product does not deliver the intended value.

| Prompt | Think About |
|--------|------------|
| Are we solving the right problem? | Problem validation, user research quality |
| Will users adopt this? | Activation barriers, switching costs, habit formation |
| Is the UX intuitive enough? | Complexity, learning curve, accessibility |
| Will the MVP be too thin? | Minimum bar for value delivery |
| Will users churn after initial adoption? | Engagement depth, ongoing value |
| Are we targeting the right persona? | Market segment, persona accuracy |
| Is the value proposition clear? | Messaging, positioning, differentiation |
| Feature creep risk? | Scope discipline, stakeholder pressure |

### Category 2: Technical Risks

Risks that the system cannot be built as designed or will not perform.

| Prompt | Think About |
|--------|------------|
| Can we build this with the chosen stack? | Stack maturity, team expertise |
| Will it scale to projected usage? | Load projections, bottlenecks |
| Are there single points of failure? | Redundancy, failover |
| Third-party dependency risks? | API stability, vendor lock-in, deprecation |
| Data integrity risks? | Consistency, migration, backup/recovery |
| Integration complexity? | Number of integrations, API quality, versioning |
| Performance risks? | Latency targets, compute costs at scale |
| Technical debt risk? | Shortcuts in MVP that compound later |
| Platform/browser compatibility? | Target environments, polyfills, testing matrix |

### Category 3: Business Risks

Risks to the business model, timeline, or viability.

| Prompt | Think About |
|--------|------------|
| Will users pay for this? | Willingness to pay, pricing model validation |
| Can we acquire users cost-effectively? | CAC projections, channel strategy |
| Competitive response? | Incumbent retaliation, fast followers |
| Regulatory or legal risks? | Compliance, data privacy, IP |
| Timeline risks? | Deadline pressure, dependency delays |
| Team risks? | Key person dependency, skill gaps, availability |
| Budget risks? | Infrastructure costs, third-party service costs at scale |
| Market timing? | Too early, too late, market shifts |

### Category 4: Security Risks

Risks of security breach, data loss, or vulnerability.

| Prompt | Think About |
|--------|------------|
| Authentication vulnerabilities? | Auth flow, session management, token handling |
| Authorization gaps? | Role-based access, privilege escalation |
| Data exposure risks? | PII handling, encryption, data leakage |
| Input validation? | Injection attacks, XSS, CSRF |
| Dependency vulnerabilities? | Known CVEs, supply chain attacks |
| Infrastructure security? | Network exposure, secrets management, logging |
| Compliance requirements? | GDPR, SOC2, HIPAA, PCI-DSS (if applicable) |
| Incident response readiness? | Detection, alerting, recovery procedures |

---

## Step 2: Rate Each Risk

For every identified risk, rate two dimensions on a 1-5 scale:

### Likelihood (How probable is this?)

| Score | Level | Definition |
|-------|-------|-----------|
| 1 | Rare | Requires extraordinary circumstances; < 5% probability |
| 2 | Unlikely | Could happen but not expected; 5-20% probability |
| 3 | Possible | Reasonable chance; 20-50% probability |
| 4 | Likely | More probable than not; 50-80% probability |
| 5 | Almost Certain | Would be surprised if it did NOT happen; > 80% probability |

### Impact (How bad is it if this happens?)

| Score | Level | Definition |
|-------|-------|-----------|
| 1 | Negligible | Minor inconvenience; self-correcting; no user impact |
| 2 | Minor | Small delay or degraded experience; quickly recoverable |
| 3 | Moderate | Noticeable user impact; requires significant effort to address |
| 4 | Major | Feature or system failure; significant rework or data loss |
| 5 | Critical | Project failure, security breach, regulatory violation, or business-ending |

### Severity Score

```
Severity = Likelihood × Impact
```

| Severity Range | Classification | Action Required |
|---------------|----------------|-----------------|
| 20-25 | Critical | Must mitigate before proceeding. Escalate to stakeholders. |
| 12-19 | High | Mitigation plan required. Active monitoring. |
| 6-11 | Medium | Mitigation plan recommended. Periodic monitoring. |
| 2-5 | Low | Accept or monitor. Document and move on. |
| 1 | Negligible | Accept. No action needed. |

---

## Step 3: Build the Risk Register

```markdown
## Risk Register

| ID | Risk | Category | Likelihood (1-5) | Impact (1-5) | Severity | Classification | Owner |
|----|------|----------|------------------|--------------|----------|----------------|-------|
| R1 | [risk description] | Product | [N] | [N] | [L×I] | Critical/High/Medium/Low | [role] |
| R2 | [risk description] | Technical | [N] | [N] | [L×I] | ... | [role] |
| R3 | [risk description] | Business | [N] | [N] | [L×I] | ... | [role] |
| R4 | [risk description] | Security | [N] | [N] | [L×I] | ... | [role] |

*Sorted by Severity descending.*
```

---

## Step 4: Risk Matrix Visualization

```markdown
## Risk Matrix

                         Impact →
                    1-Negligible  2-Minor  3-Moderate  4-Major  5-Critical
Likelihood ↓
5-Almost Certain  |    [IDs]    | [IDs]  |   [IDs]   | [IDs]  |  [IDs]   |
4-Likely          |    [IDs]    | [IDs]  |   [IDs]   | [IDs]  |  [IDs]   |
3-Possible        |    [IDs]    | [IDs]  |   [IDs]   | [IDs]  |  [IDs]   |
2-Unlikely        |    [IDs]    | [IDs]  |   [IDs]   | [IDs]  |  [IDs]   |
1-Rare            |    [IDs]    | [IDs]  |   [IDs]   | [IDs]  |  [IDs]   |

Legend: Green (1-5) | Yellow (6-11) | Orange (12-19) | Red (20-25)
```

---

## Step 5: Mitigation Strategies

For every risk with Severity >= 6 (Medium and above), define a mitigation strategy.

### Mitigation Types

| Strategy | Definition | When to Use |
|----------|-----------|------------|
| **Avoid** | Eliminate the risk by changing the approach | When an alternative approach removes the risk entirely |
| **Reduce** | Lower the likelihood or impact | When the risk cannot be avoided but can be contained |
| **Transfer** | Shift the risk to a third party | Insurance, SLAs, managed services, outsourcing |
| **Accept** | Acknowledge and prepare for consequences | When mitigation cost exceeds the risk cost |

### Mitigation Plan Template

For each risk requiring mitigation:

```markdown
### R[ID]: [Risk Name]

**Risk:** [description]
**Severity:** [N] ([classification])
**Strategy:** Avoid / Reduce / Transfer / Accept

**Mitigation Actions:**
1. [Specific action to take] — Owner: [role] — Timeline: [when]
2. [Specific action to take] — Owner: [role] — Timeline: [when]
3. [Specific action to take] — Owner: [role] — Timeline: [when]

**Contingency Plan (if risk materializes despite mitigation):**
1. [Immediate response action]
2. [Recovery steps]
3. [Communication plan — who to notify and how]

**Early Warning Indicators:**
- [Signal that this risk is becoming more likely]
- [Metric to monitor]

**Residual Risk After Mitigation:**
- Likelihood: [new score] (was [old score])
- Impact: [new score] (was [old score])
- Residual Severity: [new calculation]
```

---

## Step 6: Contingency Plans for Critical Risks

For any risk classified as Critical (Severity 20-25), develop a detailed contingency plan:

```markdown
### Contingency Plan: R[ID]

**Trigger:** [What event or metric threshold activates this plan]

**Immediate Actions (0-24 hours):**
1. [Action] — Responsible: [role]
2. [Action] — Responsible: [role]
3. [Stakeholder communication] — Template: [brief]

**Short-term Response (1-7 days):**
1. [Action]
2. [Action]

**Recovery Path:**
- **Option A:** [approach] — Timeline: [estimate] — Trade-off: [what we lose]
- **Option B:** [approach] — Timeline: [estimate] — Trade-off: [what we lose]
- **Recommended option:** [A or B] — Rationale: [why]

**Post-Incident:**
- Root cause analysis
- Update risk register
- Adjust mitigation strategies
```

---

## Step 7: Risk Summary and Recommendations

```markdown
## Risk Summary

### Risk Profile
- **Total risks identified:** [N]
- **Critical:** [N] | **High:** [N] | **Medium:** [N] | **Low:** [N]
- **Top risk category:** [Product / Technical / Business / Security]

### Top 5 Risks (by Severity)
1. R[ID]: [name] — Severity [N] — Mitigation: [brief summary]
2. R[ID]: [name] — Severity [N] — Mitigation: [brief summary]
3. R[ID]: [name] — Severity [N] — Mitigation: [brief summary]
4. R[ID]: [name] — Severity [N] — Mitigation: [brief summary]
5. R[ID]: [name] — Severity [N] — Mitigation: [brief summary]

### Systemic Risk Patterns
[Are there patterns? Multiple technical risks suggesting architecture fragility?
Multiple business risks suggesting market uncertainty? Call these out.]

### Risk Budget
[Total effort needed for mitigation activities. If mitigation requires >15% of project
effort, flag this as a risk itself — the project may be too risky to proceed as planned.]

### Recommendations
1. [Recommendation tied to highest risks]
2. [Recommendation]
3. [Recommendation]

### Risks Accepted
[List risks explicitly accepted with rationale for acceptance]
```

---

## Quality Checklist

Before finalizing, verify:
- [ ] All four risk categories (Product, Technical, Business, Security) have at least 2 risks identified
- [ ] Likelihood and Impact ratings are justified, not arbitrary
- [ ] Every risk with Severity >= 6 has a mitigation plan
- [ ] Every Critical risk has a detailed contingency plan
- [ ] Mitigation actions are specific (not "monitor the situation" without defining what to monitor)
- [ ] Early warning indicators are defined for High and Critical risks
- [ ] Residual risk is calculated after mitigation
- [ ] Risk summary identifies systemic patterns
- [ ] Accepted risks are explicitly documented with rationale
