---
description: Prepare a release by verifying all work is complete, running full quality checks, generating changelog, bumping version, creating a git tag, and producing release notes. Requires explicit user approval before any deployment action.
inputs: PROJECT_STATE.md, EXECUTION_PLAN.md, TESTING_STRATEGY.md, TECH_STACK.md, existing CHANGELOG.md (if any)
outputs: Updated CHANGELOG.md, RELEASE_NOTES.md (in .agents/handoff/), git tag, version bump
---

# Workflow: Release Cut

This workflow prepares a release for deployment. It is the final quality gate before code goes to production. Every check must pass, every test must be green, and the user must explicitly approve the release before any deployment action occurs.

---

## Step 1: Verify All Work Is Complete

### Read PROJECT_STATE.md
Verify that all planned work for this release is complete:

1. **Check Completed Steps:**
   - [ ] Every step in EXECUTION_PLAN.md for this release is marked complete in PROJECT_STATE.md.
   - [ ] No steps are marked "In Progress."
   - [ ] No steps are marked "Pending" that are within the release scope.

2. **Check Known Issues:**
   - [ ] No critical or high severity issues are open.
   - [ ] Any deferred issues are documented with justification.
   - [ ] No open items in ESCALATION_LOG.md that block this release.

3. **Check Architecture Deviations:**
   - [ ] All deviations from ARCHITECTURE.md are documented and user-approved.
   - [ ] No unauthorized deviations exist.

### Verify Against EXECUTION_PLAN.md
Cross-reference every milestone and step:

```markdown
### Release Readiness Checklist
| Milestone | Steps Complete | Steps Total | Status |
|-----------|---------------|-------------|--------|
| [name] | [N] | [N] | READY / BLOCKED |

### Blocking Items (if any)
| Item | Description | Owner | Resolution |
|------|-------------|-------|-----------|
```

**If any blocking items exist, STOP.** Inform the user and do not proceed until all blockers are resolved. Do not cut a release with known blocking issues.

---

## Step 2: Run Full Test Suite

Execute the complete test suite -- every test type defined in TESTING_STRATEGY.md:

### Unit Tests
```bash
make test
```
- [ ] All unit tests pass.
- [ ] Coverage meets or exceeds the threshold in TESTING_STRATEGY.md.
- [ ] No skipped tests (if tests are skipped, document why).

### Integration Tests
```bash
make test-int
```
- [ ] All integration tests pass.
- [ ] Database operations verified against current schema.
- [ ] External service mocks/sandboxes are up to date.

### E2E Tests
```bash
make test-e2e
```
- [ ] All E2E tests pass.
- [ ] All critical user journeys (from TESTING_STRATEGY.md) are covered.
- [ ] Screenshots/recordings captured for review (if configured).

### Full Suite Summary
```markdown
| Test Type | Total | Passed | Failed | Skipped | Coverage |
|-----------|-------|--------|--------|---------|----------|
| Unit | [N] | [N] | 0 | [N] | [X]% |
| Integration | [N] | [N] | 0 | [N] | -- |
| E2E | [N] | [N] | 0 | [N] | -- |
| **Total** | **[N]** | **[N]** | **0** | **[N]** | -- |
```

**If any test fails, STOP.** Fix the failure and re-run the full suite. Do not proceed with a release that has failing tests.

---

## Step 3: Run Zero-Trust Audit

Perform the comprehensive zero-trust audit from TESTING_STRATEGY.md against the entire codebase:

### Security Audit
- [ ] Run dependency vulnerability scan (`npm audit` / `govulncheck` / equivalent).
- [ ] No critical or high vulnerabilities in dependencies.
- [ ] Run secrets scanner (gitleaks/detect-secrets) against the entire repo.
- [ ] No hardcoded secrets, API keys, tokens, or passwords.
- [ ] All user inputs are validated server-side.
- [ ] Authentication is enforced on all protected endpoints.
- [ ] No SQL injection, XSS, or CSRF vulnerabilities.
- [ ] Sensitive data is not logged.
- [ ] HTTPS is enforced.
- [ ] CORS is configured correctly.
- [ ] Rate limiting is in place.
- [ ] CSP headers are configured (if web application).

### Architecture Audit
- [ ] All code matches ARCHITECTURE.md interfaces.
- [ ] No circular dependencies.
- [ ] Package boundaries are respected.
- [ ] Data flows through defined layers only.
- [ ] No unauthorized external dependencies.
- [ ] SOLID principles are followed.

### Stability Audit
- [ ] All error paths return appropriate errors (no panics/crashes).
- [ ] Edge cases are handled (empty inputs, boundary values, null values).
- [ ] Timeouts are configured for all external calls.
- [ ] Retry logic has backoff and maximum attempts.
- [ ] Graceful shutdown is implemented.
- [ ] Health check endpoint works correctly.

### Performance Audit
- [ ] No N+1 queries.
- [ ] No unbounded list operations.
- [ ] Database queries use indexes (verify with EXPLAIN).
- [ ] Bundle size is within budget (if web application).
- [ ] No memory leaks (verify with profiling if applicable).
- [ ] Caching is implemented where specified in ARCHITECTURE.md.

### Accessibility Audit (if web application)
- [ ] ARIA attributes are correct.
- [ ] Keyboard navigation works for all interactive elements.
- [ ] Color contrast meets WCAG AA.
- [ ] Screen reader compatibility verified.
- [ ] Focus management is correct for modals and dynamic content.

### Audit Results
```markdown
| Category | Status | Findings |
|----------|--------|----------|
| Security | PASS/FAIL | [details] |
| Architecture | PASS/FAIL | [details] |
| Stability | PASS/FAIL | [details] |
| Performance | PASS/FAIL | [details] |
| Accessibility | PASS/FAIL | [details] |
```

**If any audit category fails, STOP.** Fix the findings and re-run the audit. Do not proceed with a release that has audit failures.

---

## Step 4: Determine Version Number

Follow Semantic Versioning (semver) to determine the new version:

### Read Current Version
Check the current version from:
- `package.json` (Node)
- `version.go` or `go.mod` (Go)
- `pyproject.toml` or `__version__` (Python)
- `Cargo.toml` (Rust)
- Whatever version source is defined in TECH_STACK.md.

### Apply Semver Rules
| Change Type | Version Bump | Examples |
|-------------|-------------|---------|
| Bug fixes, patches, no API changes | PATCH (0.0.X) | Security fix, typo fix, performance improvement |
| New features, backward-compatible | MINOR (0.X.0) | New endpoint, new UI feature, new config option |
| Breaking changes, API changes | MAJOR (X.0.0) | Removed endpoint, changed response format, renamed config |

### Pre-1.0 Projects
For projects that have not reached 1.0:
- MINOR bumps may contain breaking changes.
- Document this clearly in the release notes.
- Once the project is stable and in production, cut 1.0.0.

### Determine the Bump
Review all changes since the last release (from git log and CHANGELOG.md):
- If any change is breaking: MAJOR bump.
- If any change adds features (no breaking): MINOR bump.
- If all changes are fixes: PATCH bump.

**Present the proposed version to the user for confirmation.**

---

## Step 5: Generate / Update CHANGELOG.md

### Read Git History Since Last Release
```bash
git log [last-tag]..HEAD --oneline --no-merges
```

### Organize Changes by Category
Follow the Keep a Changelog format (https://keepachangelog.com):

```markdown
# Changelog

## [X.Y.Z] - YYYY-MM-DD

### Added
- [New feature or capability]
- [New feature or capability]

### Changed
- [Change to existing functionality]
- [Change to existing functionality]

### Deprecated
- [Feature that will be removed in a future release]

### Removed
- [Feature that was removed]

### Fixed
- [Bug fix]
- [Bug fix]

### Security
- [Security fix or improvement]
```

### Changelog Rules
1. **Every user-facing change must be listed.** Internal refactoring without user impact can be omitted.
2. **Link to issues/PRs** where applicable: `Fixed login timeout issue (#42)`.
3. **Use clear, user-understandable language.** Not "refactored auth module" but "Fixed intermittent login failures under high load."
4. **List breaking changes prominently** with migration instructions.
5. **Preserve all previous changelog entries.** Only add the new version section at the top.

---

## Step 6: Bump Version

Update the version number in all relevant files:

### Files to Update (varies by stack)
| File | Field | Example |
|------|-------|---------|
| `package.json` | `version` | `"1.2.3"` |
| `package-lock.json` | `version` | `"1.2.3"` |
| `go.mod` / version constant | version string | `v1.2.3` |
| `pyproject.toml` | `version` | `"1.2.3"` |
| `Cargo.toml` | `version` | `"1.2.3"` |
| API docs / OpenAPI spec | `info.version` | `"1.2.3"` |
| Docker image tags | tag | `app:1.2.3` |

### Verify Version Consistency
```bash
# Ensure all version references match
grep -r "version" [relevant files] | grep [old-version]   # should return nothing
grep -r "version" [relevant files] | grep [new-version]   # should return all version locations
```

### Build with New Version
```bash
make build
```
Verify the build output includes the correct version:
```bash
./app --version   # or equivalent
```

---

## Step 7: Create Git Tag

### Commit the Version Bump and Changelog
```bash
git add CHANGELOG.md [version files]
git commit -m "release: v[X.Y.Z]

Bump version to [X.Y.Z] and update changelog."
```

### Create Annotated Tag
```bash
git tag -a v[X.Y.Z] -m "Release v[X.Y.Z]

[One-line summary of the release]"
```

### Verify the Tag
```bash
git tag -l "v[X.Y.Z]"        # tag exists
git show v[X.Y.Z]             # tag points to correct commit
git log --oneline -1 v[X.Y.Z] # verify commit message
```

**Do NOT push the tag yet.** Wait for user approval in the next step.

---

## Step 8: Produce RELEASE_NOTES.md

Write `.agents/handoff/RELEASE_NOTES.md`:

```markdown
# Release Notes: v[X.Y.Z]

## Date: [ISO date]

## Summary
[2-3 sentence summary of what this release delivers. Focus on user value.]

## Highlights
- [Most important change 1]
- [Most important change 2]
- [Most important change 3]

## What's New
### Features
- [Feature 1]: [brief description]
- [Feature 2]: [brief description]

### Improvements
- [Improvement 1]: [brief description]
- [Improvement 2]: [brief description]

### Bug Fixes
- [Fix 1]: [brief description]
- [Fix 2]: [brief description]

### Security
- [Security fix/improvement]: [brief description]

## Breaking Changes
[If none: "No breaking changes in this release."]

[If any:]
### [Breaking change description]
**Before:** [old behavior]
**After:** [new behavior]
**Migration:** [steps to migrate]

## Upgrade Instructions
1. [Step-by-step upgrade instructions]
2. [Include any migration commands]
3. [Note any configuration changes needed]

## Known Issues
[If none: "No known issues."]
| Issue | Description | Workaround |
|-------|-------------|-----------|

## Quality Report
### Test Results
| Suite | Total | Passed | Failed | Coverage |
|-------|-------|--------|--------|----------|
| Unit | [N] | [N] | 0 | [X]% |
| Integration | [N] | [N] | 0 | -- |
| E2E | [N] | [N] | 0 | -- |

### Zero-Trust Audit
| Category | Status |
|----------|--------|
| Security | PASS |
| Architecture | PASS |
| Stability | PASS |
| Performance | PASS |
| Accessibility | PASS |

### Dependency Status
- Vulnerabilities: [0 critical, 0 high, N medium, N low]
- Last audit: [date]

## Deployment
- **Git tag:** v[X.Y.Z]
- **Commit:** [full SHA]
- **Branch:** [branch name]
- **Artifact:** [build artifact location/identifier]

## Contributors
[List of contributors for this release, from git log]
```

---

## Step 9: User Approval Gate

**STOP.** Present the release summary to the user and wait for explicit approval:

```markdown
## Release v[X.Y.Z] Ready for Review

### Changes
[Count] features, [count] improvements, [count] fixes, [count] security updates

### Quality
- All [count] tests pass
- Zero-trust audit: ALL PASS
- No critical/high vulnerabilities

### Files Modified
- CHANGELOG.md (updated)
- [version files] (version bumped to [X.Y.Z])
- RELEASE_NOTES.md (created)

### Pending Actions (require your approval)
1. Push git tag v[X.Y.Z] to remote
2. Push release commit to remote
3. Trigger deployment pipeline (if configured)

**Please confirm:**
- [ ] Changelog is accurate
- [ ] Version number is correct
- [ ] Release notes are ready
- [ ] Approved to push tag and trigger deployment
```

**Do not push the tag, push to remote, or trigger any deployment until the user explicitly approves.**

---

## Step 10: Execute Release (After Approval Only)

Only after the user confirms:

### Push Release Commit and Tag
```bash
git push origin [branch]
git push origin v[X.Y.Z]
```

### Verify Deployment Pipeline Triggered (if applicable)
- [ ] CI pipeline started for the tag.
- [ ] Build stage passed.
- [ ] Deployment to staging initiated (if configured for tag triggers).

### Create GitHub/GitLab Release (if applicable)
```bash
# GitHub
gh release create v[X.Y.Z] --title "v[X.Y.Z]" --notes-file .agents/handoff/RELEASE_NOTES.md

# GitLab (via API or UI -- inform user)
```

### Post-Release
- [ ] Verify the deployment completed successfully.
- [ ] Run smoke tests against the deployed environment.
- [ ] Monitor for errors in the first 15 minutes (logs, error tracking).

---

## Step 11: Update PROJECT_STATE.md

Update `.agents/handoff/PROJECT_STATE.md`:

```markdown
## Version: v[X.Y.Z]
## Last Updated: [ISO timestamp]
## Phase: release-prep -> maintenance

## Release History
| Version | Date | Tag | Commit | Status |
|---------|------|-----|--------|--------|
| v[X.Y.Z] | [date] | v[X.Y.Z] | [SHA] | Released |
```

Reset the "In Progress" and "Completed Steps" sections for the next development cycle, preserving the release history.

Append to `.agents/handoff/AUDIT_LOGS.md`:

```markdown
## Release Audit: v[X.Y.Z] -- [ISO timestamp]

### Pre-Release Verification
- All EXECUTION_PLAN.md steps complete: YES
- Full test suite: PASS ([count] tests)
- Zero-trust audit: ALL PASS
- Dependency vulnerabilities: [summary]
- Version bump: [old] -> [new]
- Git tag: v[X.Y.Z]
- Changelog updated: YES
- Release notes produced: YES
- User approved: YES

### Result: RELEASED
```
