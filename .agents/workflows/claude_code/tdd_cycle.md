---
description: Strict Red-Green-Refactor test-driven development protocol. Every implementation step follows this cycle to ensure correctness, prevent regressions, and maintain audit-ready test coverage.
inputs: TESTING_STRATEGY.md, EXECUTION_PLAN.md (current step), ARCHITECTURE.md (interfaces and contracts)
outputs: Test files, implementation files, updated AUDIT_LOGS.md
---

# Workflow: TDD Cycle

This workflow is invoked for **every implementation step** in EXECUTION_PLAN.md. It enforces the Red-Green-Refactor discipline: write the test first, make it pass with minimum code, then improve the code without breaking tests. No exceptions.

---

## Step 1: Read Testing Requirements for the Current Step

Before writing any code:

1. **Read TESTING_STRATEGY.md** -- identify which tests are required for this step:
   - What test layer? (unit, integration, e2e)
   - What module/component is being tested?
   - What edge cases are listed?
   - What mocking strategy applies?
   - What coverage threshold must be met?

2. **Read ARCHITECTURE.md** -- identify the interfaces, types, and contracts for the code being implemented:
   - Function signatures and return types.
   - Error types and error handling behavior.
   - Dependencies to mock/stub.
   - Package boundaries that must not be violated.

3. **Read API_CONTRACT.md** (if implementing an API endpoint) -- identify:
   - Request/response schemas.
   - Status codes for each scenario (success, validation error, not found, conflict, auth failure).
   - Error response format.

4. **Plan the tests** -- before writing code, list every test case:

```markdown
### Test Plan for [Step X.Y: Description]

#### Happy Path
- [ ] [Test: description of expected behavior]

#### Validation / Input Edge Cases
- [ ] [Test: empty input]
- [ ] [Test: boundary values]
- [ ] [Test: invalid types]

#### Error Paths
- [ ] [Test: expected error condition]
- [ ] [Test: dependency failure handling]

#### Security (if applicable)
- [ ] [Test: auth required]
- [ ] [Test: authorization check]
- [ ] [Test: input sanitization]
```

---

## Step 2: RED -- Write Failing Tests

Write all planned tests **before writing any implementation code**. Each test must:

### Test Structure
Follow the Arrange-Act-Assert (AAA) or Given-When-Then pattern:

```
Test: [descriptive name matching the behavior being tested]
  Arrange: Set up test data, mocks, and preconditions
  Act:     Call the function/endpoint under test
  Assert:  Verify the expected outcome
```

### Test Quality Requirements
- **Descriptive names** -- test names must describe the behavior, not the implementation. Use the pattern: `[Unit]_[Scenario]_[ExpectedResult]` or `should [expected behavior] when [condition]`.
- **One assertion per concept** -- each test should verify one logical behavior (multiple assertions are fine if they verify the same concept).
- **Independent** -- tests must not depend on execution order or shared mutable state.
- **Deterministic** -- no flaky tests. Mock time, randomness, and external services.
- **Fast** -- unit tests must run in milliseconds. Use mocks/stubs for slow dependencies.

### Mock and Stub Setup
Per TESTING_STRATEGY.md mocking strategy:
- Create interface-based mocks for dependencies.
- Use the mocking library specified in TECH_STACK.md.
- Set up test fixtures and factories for test data.
- Configure test database (if integration tests) per TESTING_STRATEGY.md.

### Run the Tests
```bash
make test  # or the appropriate test command for this test layer
```

**Every test must FAIL.** This is the Red phase. If a test passes before implementation:
- The test is not testing new behavior -- delete it or fix it.
- The behavior already exists -- verify this is expected and skip to Step 4.

Verify each test fails **for the right reason**:
- It should fail because the function/module does not exist yet, OR
- It should fail because the function returns wrong/default values.
- It should NOT fail because of syntax errors, import errors, or test setup bugs.

Fix any test infrastructure issues before proceeding.

---

## Step 3: GREEN -- Write Minimum Code to Pass

Now implement the code. The goal is to make all tests pass with the **simplest correct implementation**.

### Rules for the Green Phase
1. **Write only enough code to pass the tests.** Do not add features, optimizations, or patterns that are not required by a test.
2. **Follow ARCHITECTURE.md interfaces exactly.** The function signatures, types, and package locations must match the spec.
3. **Handle all error paths that have tests.** If a test asserts error behavior, the code must handle that error.
4. **Do not skip any failing test.** If a test is hard to make pass, the implementation may need a different approach -- not a skipped test.

### Implementation Checklist
- [ ] Function/type signatures match ARCHITECTURE.md.
- [ ] All imports are real packages (no hallucinated dependencies).
- [ ] Error handling follows the error taxonomy in ARCHITECTURE.md.
- [ ] No `any` types, no untyped maps, no type assertions without checks.
- [ ] Code is in the correct package/module per ARCHITECTURE.md boundaries.

### Run the Tests
```bash
make test
```

**Every test must PASS.** If tests still fail:
- Read the error message carefully.
- Fix the implementation (not the test, unless the test has a bug).
- Re-run until all tests are green.

---

## Step 4: REFACTOR -- Improve Code While Keeping Tests Green

With all tests passing, improve the code quality without changing behavior.

### Refactoring Targets
Look for and address:

1. **Duplication** -- extract shared logic into helper functions.
2. **Naming** -- rename variables, functions, types for clarity. Names should match domain language from PRD.md.
3. **Complexity** -- simplify nested conditionals, extract complex expressions into named variables.
4. **SOLID violations**:
   - Single Responsibility: does each function do one thing?
   - Open/Closed: can behavior be extended without modifying existing code?
   - Liskov Substitution: do implementations satisfy their interfaces fully?
   - Interface Segregation: are interfaces minimal and focused?
   - Dependency Inversion: do modules depend on abstractions, not concretions?
5. **Error handling** -- ensure errors are wrapped with context, not swallowed.
6. **Documentation** -- add doc comments to exported functions/types per project conventions.

### Refactoring Rules
- **Run tests after every change.** Not after a batch of changes -- after each individual refactoring move.
- **If a test breaks, undo the last change immediately.** Refactoring must not change behavior.
- **Do not add new features during refactoring.** No new tests, no new behavior. Only restructuring.
- **Do not change test code during implementation refactoring** (unless renaming to match updated code names).

### Run Tests After Refactoring
```bash
make test
```

All tests must still pass. If any test breaks, the refactoring changed behavior -- revert and try a different approach.

---

## Step 5: Run Full Test Suite for Regressions

After the TDD cycle for this step is complete, run the **entire** test suite, not just the tests for this step:

```bash
make test-all   # unit + integration + e2e (whatever exists so far)
```

### Regression Check
- [ ] All pre-existing tests still pass.
- [ ] No new warnings or deprecation notices introduced.
- [ ] Test coverage has not decreased (check with `make coverage`).
- [ ] No tests became flaky (if a test fails intermittently, investigate and fix).

### If Regressions Are Found
1. **Stop.** Do not proceed to the next step.
2. Identify which change in this step caused the regression.
3. Fix the regression while keeping the new tests green.
4. Re-run the full suite until everything passes.
5. If the regression is caused by a spec conflict (new behavior contradicts old behavior), escalate per the Escalation Protocol -- do not resolve spec conflicts unilaterally.

---

## Step 6: Log Results in AUDIT_LOGS.md

Append to `.agents/handoff/AUDIT_LOGS.md`:

```markdown
## [Step X.Y: Description] -- [ISO timestamp]

### TDD Cycle
- **Tests Written:** [count] ([count] unit, [count] integration, [count] e2e)
- **Red Phase:** All [count] tests failed as expected
- **Green Phase:** All [count] tests pass
- **Refactoring:** [summary of refactoring performed, or "No refactoring needed"]
- **Regression Check:** Full suite [count] tests -- ALL PASS

### Coverage
- **This Step:** [X]% line coverage for new code
- **Overall:** [X]% (threshold: [X]% per TESTING_STRATEGY.md)

### Test Cases
| Test Name | Layer | Status | Notes |
|-----------|-------|--------|-------|
| [test name] | Unit | PASS | |
| [test name] | Integration | PASS | |

### Code Quality
- Linter: PASS
- Formatter: PASS
- Type Check: PASS (if applicable)

### Findings
[Any issues discovered during the cycle, edge cases added, or concerns]
```

---

## Cycle Integrity Rules

These rules are non-negotiable:

1. **Never write implementation before tests.** If you catch yourself writing code first, stop, delete it, and write the test.
2. **Never skip the Red phase.** If tests pass immediately, they are not testing new behavior.
3. **Never skip the regression check.** A passing step that breaks previous work is a failing step.
4. **Never mark a step complete with failing tests.** Zero tolerance for broken tests.
5. **Never delete a test to make the suite pass.** If a test is wrong, fix it. If behavior changed intentionally, update the test and document why.
6. **Never mock what you own in integration tests.** Only mock external dependencies and boundaries you do not control.
7. **Test behavior, not implementation.** Tests should survive refactoring. If changing internal structure breaks tests, the tests are coupled to implementation details.
