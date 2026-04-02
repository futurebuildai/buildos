---
description: Generate and interpret test coverage reports. Runs the project's coverage tool, maps uncovered lines to architectural components, flags critical uncovered paths (auth, data mutation, payment, user input handling), and reports total, per-module, and critical path coverage with delta tracking.
invoked_from:
  - workflows/claude_code/04_quality_gates.md
  - workflows/claude_code/03_implement.md
  - After writing or modifying tests
  - Before any production release
produces:
  - Coverage summary (total lines, branches, functions, statements)
  - Per-module coverage breakdown mapped to ARCHITECTURE.md components
  - Critical path coverage report (auth, payments, data mutation, input handling)
  - Coverage delta from last run
  - List of uncovered lines in critical modules with recommended test additions
---

# Skill: Test Coverage Report

Generate, analyze, and interpret test coverage data. Coverage numbers alone are not the goal — the goal is to ensure that critical code paths are tested. A project with 60% coverage where all auth and payment paths are tested is safer than one with 90% coverage that skips auth logic.

---

## Prerequisites

Before running this skill, ensure the following exist:

- `TECH_STACK.md` — for test framework and coverage tooling
- `ARCHITECTURE.md` — for mapping modules to architectural components and identifying critical paths
- A working test suite that can be executed
- All project dependencies installed

---

## Step 1: Run Coverage Tool

Execute the test suite with coverage instrumentation.

### Actions

1. **Identify and run the correct coverage tool:**

   **JavaScript / TypeScript (Jest):**
   ```bash
   npx jest --coverage --coverageReporters=json-summary --coverageReporters=text --coverageReporters=lcov
   ```

   **JavaScript / TypeScript (Vitest):**
   ```bash
   npx vitest run --coverage
   ```

   **JavaScript / TypeScript (c8 / Istanbul / nyc):**
   ```bash
   npx c8 npm test
   # Or
   npx nyc npm test
   ```

   **Python (pytest-cov):**
   ```bash
   pytest --cov=src --cov-report=term-missing --cov-report=json --cov-report=html
   ```

   **Python (coverage.py):**
   ```bash
   coverage run -m pytest
   coverage report --show-missing
   coverage json -o coverage.json
   ```

   **Go:**
   ```bash
   go test -coverprofile=coverage.out -covermode=atomic ./...
   go tool cover -func=coverage.out
   go tool cover -html=coverage.out -o coverage.html
   ```

   **Rust:**
   ```bash
   # Using cargo-tarpaulin
   cargo tarpaulin --out json --out html --output-dir coverage/
   # Or using llvm-cov
   cargo llvm-cov --json --output-path coverage.json
   ```

2. **Verify tests pass before analyzing coverage:**
   - If any tests fail, fix them first. Coverage data from a failing test run is unreliable.
   - Record the number of tests run, passed, failed, and skipped.

   | Metric | Count |
   |--------|-------|
   | Tests run | [n] |
   | Tests passed | [n] |
   | Tests failed | [n] |
   | Tests skipped | [n] |

---

## Step 2: Extract Coverage Summary

Parse the coverage output into a standardized summary.

### Actions

1. **Extract top-level metrics:**

   | Metric | Percentage | Covered | Total |
   |--------|-----------|---------|-------|
   | Statements | [%] | [n] | [n] |
   | Branches | [%] | [n] | [n] |
   | Functions | [%] | [n] | [n] |
   | Lines | [%] | [n] | [n] |

2. **Parse JSON coverage report (if available):**

   **Jest / Istanbul / c8:**
   ```bash
   cat coverage/coverage-summary.json | python3 -c "
   import json, sys
   data = json.load(sys.stdin)
   total = data['total']
   for metric in ['statements', 'branches', 'functions', 'lines']:
     m = total[metric]
     print(f'{metric}: {m[\"pct\"]}% ({m[\"covered\"]}/{m[\"total\"]})')
   "
   ```

   **Python (coverage.py):**
   ```bash
   cat coverage.json | python3 -c "
   import json, sys
   data = json.load(sys.stdin)
   totals = data['totals']
   print(f'Lines: {totals[\"percent_covered\"]:.1f}% ({totals[\"covered_lines\"]}/{totals[\"num_statements\"]})')
   print(f'Branches: {totals.get(\"percent_covered_branches\", \"N/A\")}')
   "
   ```

   **Go:**
   ```bash
   go tool cover -func=coverage.out | tail -1
   ```

3. **Compare against coverage thresholds:**

   | Metric | Threshold | Actual | Status |
   |--------|----------|--------|--------|
   | Total line coverage | >= [target from config or 80%] | [measured] | PASS/FAIL |
   | Total branch coverage | >= [target from config or 70%] | [measured] | PASS/FAIL |
   | Total function coverage | >= [target from config or 80%] | [measured] | PASS/FAIL |

---

## Step 3: Per-Module Coverage Breakdown

Map coverage data to architectural components from ARCHITECTURE.md.

### Actions

1. **Read ARCHITECTURE.md to identify modules/components:**
   - List each architectural layer (e.g., API handlers, services, repositories, utilities).
   - Map file paths to architectural components.

2. **Extract per-file or per-directory coverage:**

   **Jest / Istanbul:**
   ```bash
   cat coverage/coverage-summary.json | python3 -c "
   import json, sys
   data = json.load(sys.stdin)
   for path, metrics in sorted(data.items()):
     if path == 'total':
       continue
     lines = metrics['lines']
     print(f'{path}: {lines[\"pct\"]}% ({lines[\"covered\"]}/{lines[\"total\"]})')
   "
   ```

   **Go:**
   ```bash
   go tool cover -func=coverage.out | head -50
   ```

3. **Group by architectural component:**

   | Component (from ARCHITECTURE.md) | Directory/Files | Line Coverage | Branch Coverage | Status |
   |----------------------------------|----------------|---------------|-----------------|--------|
   | API Handlers / Controllers | `src/api/` or `src/handlers/` | [%] | [%] | PASS/FAIL |
   | Business Logic / Services | `src/services/` or `src/domain/` | [%] | [%] | PASS/FAIL |
   | Data Access / Repositories | `src/repositories/` or `src/db/` | [%] | [%] | PASS/FAIL |
   | Authentication / Authorization | `src/auth/` | [%] | [%] | PASS/FAIL |
   | Utilities / Helpers | `src/utils/` or `src/lib/` | [%] | [%] | PASS/FAIL |
   | Middleware | `src/middleware/` | [%] | [%] | PASS/FAIL |
   | Configuration | `src/config/` | [%] | [%] | PASS/FAIL |

4. **Identify the lowest-coverage modules:**
   - List the bottom 5 modules by coverage percentage.
   - For each, list the specific uncovered line ranges.

---

## Step 4: Critical Path Coverage Analysis

Identify and verify coverage of the most important code paths.

### Actions

1. **Define critical paths (from ARCHITECTURE.md and threat model):**

   | Critical Path Category | Files/Functions | Why Critical |
   |-----------------------|----------------|-------------|
   | Authentication | Login, logout, session management, token refresh | Security boundary |
   | Authorization | Permission checks, role verification, IDOR prevention | Access control |
   | Data Mutation | Create, update, delete operations on core entities | Data integrity |
   | Payment Processing | Payment initiation, webhook handling, refund logic | Financial correctness |
   | User Input Handling | Form validation, sanitization, file upload | Injection prevention |
   | Error Handling | Global error handlers, fallback logic, retry mechanisms | Reliability |
   | Data Export / PII | Any code that touches or exports personal data | Privacy compliance |

2. **For each critical path, check coverage:**

   ```bash
   # Example: Check coverage of auth module specifically
   # Jest
   npx jest --coverage --collectCoverageFrom='src/auth/**/*.ts' --coverageReporters=text

   # Python
   pytest --cov=src/auth --cov-report=term-missing tests/test_auth*.py

   # Go
   go test -coverprofile=auth-coverage.out ./internal/auth/...
   go tool cover -func=auth-coverage.out
   ```

3. **Report critical path coverage:**

   | Critical Path | Line Coverage | Branch Coverage | Uncovered Lines | Risk Level |
   |--------------|---------------|-----------------|-----------------|------------|
   | Authentication | [%] | [%] | [file:lines] | HIGH if < 90% |
   | Authorization | [%] | [%] | [file:lines] | HIGH if < 90% |
   | Data Mutation | [%] | [%] | [file:lines] | HIGH if < 80% |
   | Payment Processing | [%] | [%] | [file:lines] | CRITICAL if < 95% |
   | User Input Handling | [%] | [%] | [file:lines] | HIGH if < 85% |
   | Error Handling | [%] | [%] | [file:lines] | MEDIUM if < 70% |

4. **For each uncovered critical line, document:**
   - What the line does.
   - What test would cover it.
   - Why it matters (what could go wrong if this line has a bug).

---

## Step 5: Coverage Delta

Compare current coverage against the previous run to detect regressions.

### Actions

1. **Check for previous coverage data:**
   ```bash
   # Look for stored coverage data
   ls coverage/coverage-summary.json 2>/dev/null
   git stash list | grep coverage 2>/dev/null
   # Or check CI artifacts for previous coverage
   ```

2. **Calculate delta (if previous data exists):**

   | Metric | Previous | Current | Delta | Status |
   |--------|---------|---------|-------|--------|
   | Statements | [%] | [%] | [+/- %] | Improved/Regressed/Same |
   | Branches | [%] | [%] | [+/- %] | Improved/Regressed/Same |
   | Functions | [%] | [%] | [+/- %] | Improved/Regressed/Same |
   | Lines | [%] | [%] | [+/- %] | Improved/Regressed/Same |

3. **If coverage decreased:**
   - Identify which files lost coverage.
   - Determine if the decrease is from new code without tests or from removed tests.
   - Flag any critical path coverage decrease as a blocker.

4. **If no previous data exists:**
   - Note this as the baseline run.
   - Recommend storing coverage data in CI for future delta tracking.
   - Suggest adding coverage thresholds to CI configuration:
     ```json
     // Jest example in package.json
     "jest": {
       "coverageThreshold": {
         "global": {
           "branches": 70,
           "functions": 80,
           "lines": 80,
           "statements": 80
         }
       }
     }
     ```

---

## Step 6: Recommend Missing Tests

For every uncovered critical path, provide specific test recommendations.

### Actions

1. **For each uncovered line in a critical module, write a test description:**

   ```markdown
   ### Recommended Tests

   1. **File:** `src/auth/login.ts:45-52`
      **Uncovered path:** Error handling when database connection fails during login
      **Recommended test:**
      ```
      test('returns 503 when database is unavailable during login', () => {
        // Mock database to throw connection error
        // Call login endpoint
        // Assert 503 status and appropriate error message
      })
      ```

   2. **File:** `src/services/payment.ts:112-118`
      **Uncovered path:** Webhook signature verification failure
      **Recommended test:**
      ```
      test('rejects webhook with invalid signature', () => {
        // Send webhook payload with tampered signature
        // Assert 401 status and event is not processed
      })
      ```
   ```

2. **Prioritize test recommendations:**
   - P0: Uncovered authentication/authorization paths.
   - P0: Uncovered payment/financial paths.
   - P1: Uncovered data mutation error paths.
   - P1: Uncovered input validation paths.
   - P2: Uncovered utility functions.
   - P3: Uncovered configuration/setup code.

---

## Output Format

```markdown
## Test Coverage Report

**Date:** [timestamp]
**Test framework:** [framework and version]
**Coverage tool:** [tool and version]
**Tests executed:** [passed]/[total] ([skipped] skipped, [failed] failed)

### Coverage Summary

| Metric | Threshold | Actual | Delta | Status |
|--------|----------|--------|-------|--------|
| Statements | [target]% | [actual]% | [delta] | PASS/FAIL |
| Branches | [target]% | [actual]% | [delta] | PASS/FAIL |
| Functions | [target]% | [actual]% | [delta] | PASS/FAIL |
| Lines | [target]% | [actual]% | [delta] | PASS/FAIL |

### Per-Module Coverage

| Module | Line Coverage | Branch Coverage | Status |
|--------|-------------|----------------|--------|
| [module] | [%] | [%] | PASS/FAIL |

### Critical Path Coverage

| Critical Path | Coverage | Required | Status | Risk |
|--------------|----------|----------|--------|------|
| Authentication | [%] | >= 90% | PASS/FAIL | [risk] |
| Authorization | [%] | >= 90% | PASS/FAIL | [risk] |
| Data Mutation | [%] | >= 80% | PASS/FAIL | [risk] |
| Payment | [%] | >= 95% | PASS/FAIL | [risk] |
| Input Handling | [%] | >= 85% | PASS/FAIL | [risk] |

### Uncovered Critical Lines

| File | Lines | Description | Priority | Recommended Test |
|------|-------|-------------|----------|-----------------|
| [path] | [lines] | [what is uncovered] | P0/P1/P2 | [test description] |

### Coverage Delta

[delta table if previous data exists, or "Baseline run — no previous data"]

**Overall: PASS / FAIL**
```

Coverage regressions in critical paths always fail the report, regardless of total coverage percentage.
