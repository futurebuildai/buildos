---
description: Define comprehensive zero-trust testing strategy covering unit, integration, e2e, security, performance, and accessibility testing aligned to architecture and PRD.
stage: "08"
inputs: PRD.md, ARCHITECTURE.md, API_CONTRACT.md, TESTING_STRATEGY from TECH_STACK.md
outputs: TESTING_STRATEGY.md (in handoff/)
browser_usage: Moderate (testing framework research, best practices)
---

# Stage 08: Testing Strategy

Define the complete testing plan that Claude Code will execute. Every component in the architecture must have a testing approach. Every PRD acceptance criterion must map to a test.

---

## Step 1: Research Testing Best Practices

**Browser: 5-8 searches**

- Search for "[tech stack] testing best practices [current year]"
- Search for "[test framework from TECH_STACK.md] setup guide"
- Search for "[tech stack] integration testing patterns"
- Search for "[tech stack] e2e testing with [e2e framework from TECH_STACK.md]"
- Search for "[API style] API testing best practices"
- Look for testing strategy examples in well-maintained open-source projects using the same stack

## Step 2: Write Testing Strategy

Write `.agents/handoff/TESTING_STRATEGY.md`:

```markdown
# Testing Strategy

## 1. Test Pyramid

### 1.1 Coverage Targets
| Layer | Target Coverage | Framework | Rationale |
|-------|----------------|-----------|-----------|
| Unit | [e.g., 80%+] | [from TECH_STACK.md] | [why this target] |
| Integration | [e.g., critical paths] | [framework] | [why] |
| E2E | [e.g., core user journeys] | [from TECH_STACK.md] | [why] |

### 1.2 What Gets Tested Where
| Concern | Unit | Integration | E2E |
|---------|------|-------------|-----|
| Business logic | ✅ Primary | | |
| Data validation | ✅ | | |
| API contracts | | ✅ Primary | |
| Database operations | | ✅ Primary | |
| Auth flows | | ✅ | ✅ |
| User journeys | | | ✅ Primary |
| Error handling | ✅ | ✅ | |
| Edge cases | ✅ Primary | | |

---

## 2. Unit Testing

### 2.1 Per-Module Test Plan
[For each module/package in ARCHITECTURE.md]

| Module | Key Functions to Test | Edge Cases | Mocking Required |
|--------|---------------------|-----------|-----------------|

### 2.2 Mocking Strategy
- External APIs: [mock approach — in-memory fakes, recorded fixtures, mock server]
- Database: [in-memory DB, test containers, repository interface mocks]
- Time/randomness: [injectable clock, seeded RNG]
- File system: [in-memory FS, temp directories]

### 2.3 Test Data Strategy
- Factories/fixtures: [approach — factory functions, fixture files, builders]
- Seeding: [how test data is created and cleaned up]
- Isolation: [each test gets fresh data / shared read-only fixtures]

---

## 3. Integration Testing

### 3.1 API Integration Tests
For each endpoint in API_CONTRACT.md:

| Endpoint | Happy Path | Auth Failure | Validation Error | Not Found | Conflict |
|----------|-----------|-------------|-----------------|-----------|---------|
| POST /resource | ✅ | ✅ | ✅ | N/A | ✅ |
| GET /resource/:id | ✅ | ✅ | N/A | ✅ | N/A |
[Complete matrix]

### 3.2 Database Integration Tests
| Operation | Test | Isolation |
|-----------|------|-----------|
| Migrations | Up and down, idempotent | Clean DB per test |
| Queries | Correct results, performance | Seeded data |
| Constraints | FK, unique, check | Violation scenarios |
| Indexes | Used by query planner | EXPLAIN ANALYZE |

### 3.3 External Service Integration
For each integration in BUILD_VS_BUY.md:
| Service | Test Approach | Failure Modes to Test |
|---------|--------------|----------------------|
| [Service] | [Contract test / Sandbox / Mock] | [timeout, 5xx, rate limit, invalid response] |

---

## 4. E2E Testing

### 4.1 Critical User Journeys
[From USER_JOURNEYS.md — map each journey to an E2E test]

| Journey | Steps | Assertions | Priority |
|---------|-------|-----------|----------|

### 4.2 E2E Test Environment
- Browser/device matrix: [from PRD compatibility requirements]
- Test data: [seeded state required]
- External services: [mocked/sandboxed/live]

---

## 5. Security Testing

### 5.1 OWASP Top 10 Coverage
| Vulnerability | Test Approach | Tools |
|---------------|--------------|-------|
| Injection (SQL, NoSQL, OS) | Parameterized query verification, fuzzing | [tool] |
| Broken Authentication | Session fixation, brute force, token expiry | [tool] |
| Sensitive Data Exposure | TLS verification, response header audit | [tool] |
| XML External Entities | N/A or [test] | |
| Broken Access Control | Role escalation tests, IDOR tests | [tool] |
| Security Misconfiguration | Header audit, default credential check | [tool] |
| XSS | Input reflection tests, CSP verification | [tool] |
| Insecure Deserialization | [test if applicable] | |
| Known Vulnerabilities | Dependency audit | [tool from TECH_STACK.md] |
| Insufficient Logging | Audit log verification | Manual |

### 5.2 Auth-Specific Tests
| Test | Description | Expected Result |
|------|------------|----------------|
| Token expiry | Access with expired token | 401 |
| Invalid token | Access with malformed token | 401 |
| Role escalation | User A accesses User B's resource | 403 |
| CSRF | Submit form without CSRF token | 403 |
| Rate limiting | Exceed rate limit | 429 |

### 5.3 Secrets Detection
- Pre-commit hook: scan for API keys, tokens, passwords
- CI check: [tool] scans all committed code
- No secrets in: environment variables committed to repo, client-side code, logs

---

## 6. Performance Testing

### 6.1 Load Test Scenarios
[Based on PRD NFRs]
| Scenario | Concurrent Users | Duration | Target Response Time | Tool |
|----------|-----------------|----------|---------------------|------|

### 6.2 Frontend Performance
| Metric | Target | Test Method |
|--------|--------|------------|
| LCP | [from PRD] | Lighthouse CI |
| FID/INP | [from PRD] | Lighthouse CI |
| CLS | [from PRD] | Lighthouse CI |
| Bundle size | [from PRD] | Build analysis |

### 6.3 Database Performance
| Query | Expected Time | Test Method |
|-------|--------------|------------|
| [Critical queries from ARCHITECTURE.md] | < [X]ms | EXPLAIN ANALYZE |

---

## 7. Accessibility Testing

### 7.1 Automated Checks
| Tool | Scope | CI Integration |
|------|-------|---------------|
| [axe-core/equivalent] | All pages | Per-PR |

### 7.2 Manual Checks
| Check | Pages | Frequency |
|-------|-------|-----------|
| Keyboard navigation | All interactive screens | Per feature |
| Screen reader | Core journeys | Per milestone |
| Color contrast | All UI | Per design system change |
| Focus management | Modals, forms, navigation | Per feature |

---

## 8. Zero-Trust Audit Checklist

After EVERY implementation step, Claude Code must verify:

### Code Quality
- [ ] All imports resolve to real packages
- [ ] No `any` types or untyped constructs
- [ ] Error paths return appropriate errors (not panics/crashes)
- [ ] No hardcoded secrets, URLs, or magic numbers
- [ ] Naming follows project conventions

### Architecture
- [ ] Code matches ARCHITECTURE.md interfaces
- [ ] No circular dependencies
- [ ] Package boundaries respected
- [ ] Data flows through defined layers

### Security
- [ ] All user inputs validated server-side
- [ ] Auth checked on every protected endpoint
- [ ] No SQL injection vectors
- [ ] No XSS vectors in rendered content
- [ ] Sensitive data not logged

### Tests
- [ ] New code has unit tests
- [ ] Tests actually assert meaningful behavior (not just "runs without error")
- [ ] Edge cases covered
- [ ] Existing tests still pass
```

## Step 3: Update Pipeline State

```markdown
## Stage 08 — Testing Strategy ✅
- TESTING_STRATEGY.md produced
- [N] unit test areas defined
- [N] integration test scenarios
- [N] E2E journeys covered
- Security testing matrix complete
## Next Stage: 09 — Execution Plan
```

Proceed to Stage 09.
