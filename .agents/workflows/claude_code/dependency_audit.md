---
description: Audit all project dependencies for vulnerabilities, outdated versions, and license compliance. Apply updates incrementally with full test verification after each change. Produce a comprehensive audit report.
inputs: TECH_STACK.md (package manager and audit tools), project dependency files (package.json, go.mod, requirements.txt, Cargo.toml, etc.)
outputs: Updated dependency files, DEPENDENCY_AUDIT.md (in .agents/handoff/)
---

# Workflow: Dependency Audit

This workflow systematically audits all project dependencies for security vulnerabilities, outdated versions, and license issues. Updates are applied one at a time with full test suite verification after each change to isolate any breakage.

---

## Step 1: Identify Audit Tools and Dependencies

### Read TECH_STACK.md
Determine the appropriate audit tools for the project's stack:

| Stack | Audit Tool | Outdated Check | License Check |
|-------|-----------|----------------|---------------|
| Node/npm | `npm audit` | `npm outdated` | `license-checker` or `nlf` |
| Node/pnpm | `pnpm audit` | `pnpm outdated` | `license-checker` |
| Node/yarn | `yarn audit` | `yarn outdated` | `license-checker` |
| Go | `govulncheck` | `go list -m -u all` | `go-licenses` |
| Python/pip | `pip-audit` or `safety` | `pip list --outdated` | `pip-licenses` |
| Python/poetry | `poetry audit` | `poetry show --outdated` | `pip-licenses` |
| Rust | `cargo audit` | `cargo outdated` | `cargo-license` |
| Ruby | `bundle audit` | `bundle outdated` | `license_finder` |
| Java/Gradle | `gradle dependencyCheckAnalyze` | `gradle dependencyUpdates` | `gradle checkLicense` |
| Java/Maven | `mvn dependency-check:check` | `mvn versions:display-dependency-updates` | `mvn license:check` |

### Inventory Current Dependencies
Generate a complete dependency inventory:

```bash
# Example for Node
npm ls --all --json > dependency-tree.json

# Example for Go
go list -m all

# Example for Python
pip freeze
```

Record the total count of direct and transitive dependencies.

---

## Step 2: Run Vulnerability Audit

Execute the vulnerability audit tool:

```bash
# Run the appropriate audit command
npm audit --json    # or equivalent for the stack
```

### Parse and Classify Findings

Classify every finding by severity:

| Severity | Definition | Action Required |
|----------|-----------|-----------------|
| **Critical** | Remote code execution, data breach risk | Fix immediately, block release |
| **High** | Privilege escalation, significant data exposure | Fix before next release |
| **Medium** | Limited impact, requires specific conditions | Fix within current sprint |
| **Low** | Minimal impact, theoretical risk | Fix when convenient |
| **Info** | Best practice recommendation | Document and evaluate |

For each finding, record:

```markdown
### Finding: [CVE or advisory ID]
- **Package:** [name]@[version]
- **Severity:** [Critical/High/Medium/Low]
- **Description:** [brief description of the vulnerability]
- **Affected versions:** [range]
- **Fixed in:** [version]
- **Path:** [dependency path -- direct or transitive]
- **CVSS Score:** [if available]
- **Exploitability:** [network/local, complexity, privileges required]
```

---

## Step 3: Check for Outdated Dependencies

Run the outdated dependency check:

```bash
npm outdated        # or equivalent
```

### Categorize Updates

| Category | Description | Priority |
|----------|-----------|----------|
| **Patch** (1.0.0 -> 1.0.1) | Bug fixes only | Apply freely |
| **Minor** (1.0.0 -> 1.1.0) | New features, backward compatible | Review changelog, apply |
| **Major** (1.0.0 -> 2.0.0) | Breaking changes | Read migration guide, plan carefully |

Record all available updates:

```markdown
| Package | Current | Latest | Update Type | Has Vulnerability Fix |
|---------|---------|--------|-------------|----------------------|
| [name] | [current] | [latest] | Patch/Minor/Major | Yes/No |
```

---

## Step 4: Check Changelogs for Breaking Changes

**For every update that will be applied**, read the changelog or release notes:

### For Patch Updates
- Skim the changelog for unexpected breaking changes (some projects break semver).
- Note any deprecation warnings.

### For Minor Updates
- Read the changelog for new features that might affect existing usage.
- Check for deprecated APIs that the project currently uses.
- Look for new peer dependency requirements.

### For Major Updates
- **Read the full migration guide.**
- List every breaking change that affects the project.
- Identify code changes needed.
- If the migration is complex, flag it for a separate workflow/PR rather than bundling it with the audit.

### Research Approach
For each non-trivial update:
1. Check the package's GitHub releases page or CHANGELOG.md.
2. Search for "[package name] [version] breaking changes" if the changelog is unclear.
3. Check the package's migration guide (if major version bump).

Record findings:

```markdown
### [Package] [current] -> [target]
- **Breaking changes:** [none / list]
- **Deprecations:** [none / list]
- **Migration steps:** [none / list]
- **Risk:** [Low/Medium/High]
```

---

## Step 5: Apply Updates Incrementally

**CRITICAL: Apply updates one at a time (or one logical group at a time) with full test verification after each.** This isolates breakage to a specific update.

### Update Order
Apply updates in this order:
1. **Security patches** (critical/high severity) -- these are urgent.
2. **Patch updates** for direct dependencies.
3. **Minor updates** for direct dependencies.
4. **Transitive dependency updates** (via audit fix or lock file regeneration).
5. **Major updates** (only if approved by the user).

### For Each Update

#### 1. Apply the Update
```bash
# Example for Node
npm install [package]@[version]

# Example for Go
go get [package]@[version]

# Example for Python
pip install [package]==[version]
```

#### 2. Verify the Lock File
```bash
# Ensure the lock file is consistent
npm ls   # check for peer dependency warnings
```

#### 3. Run Full Test Suite
```bash
make test-all   # unit + integration + e2e
```

#### 4. Run Linter and Type Checker
```bash
make lint
make typecheck   # if applicable
```

#### 5. Run Build
```bash
make build
```

#### 6. Record Result
```markdown
| Package | From | To | Tests | Lint | Build | Status |
|---------|------|----|-------|------|-------|--------|
| [name] | [old] | [new] | PASS | PASS | PASS | Applied |
```

#### 7. If Tests Fail
1. **Read the error message carefully.** Determine if it is a genuine breaking change or a test that needs updating.
2. If the update introduced a breaking change that requires code modifications:
   - Make the minimum code changes needed to accommodate the update.
   - Re-run the full test suite.
   - Document what code changes were needed.
3. If the update breaks functionality that cannot be easily fixed:
   - **Revert the update immediately.**
   - Document why the update was reverted.
   - Flag it for future attention or escalation.

#### 8. Commit After Each Successful Update
```bash
git add [dependency files]
git commit -m "chore(deps): update [package] from [old] to [new]

[reason: security fix CVE-XXXX / routine update / etc.]"
```

Individual commits per update make it easy to revert a specific update if problems are discovered later.

---

## Step 6: License Compliance Check

Run the license checker:

```bash
license-checker --json   # or equivalent
```

### Classify Licenses

| License Category | Examples | Action |
|-----------------|----------|--------|
| **Permissive** | MIT, Apache-2.0, BSD-2/3, ISC | Allowed -- no action needed |
| **Copyleft (Weak)** | LGPL, MPL | Review -- may be acceptable depending on usage |
| **Copyleft (Strong)** | GPL, AGPL | Flag -- may require project to be open-sourced |
| **Proprietary** | Custom, Commercial | Flag -- may require license purchase |
| **Unknown** | No license, UNLICENSED | Flag -- cannot use safely |

### License Report
```markdown
### License Summary
| License | Count | Packages |
|---------|-------|----------|
| MIT | [N] | [list if small, or "see full report"] |
| Apache-2.0 | [N] | ... |

### Flagged Licenses
| Package | License | Concern | Action Needed |
|---------|---------|---------|---------------|
| [name] | [license] | [why flagged] | [recommended action] |
```

If any flagged licenses are found, **notify the user** and do not proceed with deployment until licensing is resolved.

---

## Step 7: Produce DEPENDENCY_AUDIT.md

Write `.agents/handoff/DEPENDENCY_AUDIT.md`:

```markdown
# Dependency Audit Report

## Date: [ISO timestamp]
## Auditor: Claude Code (automated)

## Summary
- **Total direct dependencies:** [count]
- **Total transitive dependencies:** [count]
- **Vulnerabilities found:** [count] ([critical], [high], [medium], [low])
- **Vulnerabilities fixed:** [count]
- **Dependencies updated:** [count]
- **License issues:** [count]

## Vulnerability Findings

### Critical
| CVE | Package | Affected | Fixed In | Status |
|-----|---------|----------|----------|--------|
| [CVE] | [pkg] | [ver] | [ver] | Fixed/Unfixable/Deferred |

### High
| CVE | Package | Affected | Fixed In | Status |
|-----|---------|----------|----------|--------|

### Medium
| CVE | Package | Affected | Fixed In | Status |
|-----|---------|----------|----------|--------|

### Low
| CVE | Package | Affected | Fixed In | Status |
|-----|---------|----------|----------|--------|

## Updates Applied

### Security Updates
| Package | From | To | CVE Fixed | Tests | Code Changes |
|---------|------|----|-----------|-------|-------------|

### Patch Updates
| Package | From | To | Tests | Code Changes |
|---------|------|----|-------|-------------|

### Minor Updates
| Package | From | To | Tests | Code Changes |
|---------|------|----|-------|-------------|

### Major Updates
| Package | From | To | Tests | Code Changes | Breaking Changes |
|---------|------|----|-------|-------------|-----------------|

### Reverted Updates
| Package | Attempted | Reason for Revert |
|---------|-----------|-------------------|

## Unfixed Vulnerabilities
| CVE | Package | Severity | Reason | Mitigation |
|-----|---------|----------|--------|-----------|
[For each vulnerability that could not be fixed, explain why and what mitigation is in place]

## License Report
### Summary
| License | Count |
|---------|-------|
| MIT | [N] |
| Apache-2.0 | [N] |
| ... | ... |

### Flagged
| Package | License | Concern | Resolution |
|---------|---------|---------|-----------|

## Test Verification
- Unit tests: PASS ([count])
- Integration tests: PASS ([count])
- E2E tests: PASS ([count])
- Linter: PASS
- Build: PASS

## Recommendations
1. [Actionable recommendation based on findings]
2. [e.g., "Schedule major update of [package] to v[X] -- requires migration"]
3. [e.g., "Monitor CVE-XXXX -- no fix available yet, mitigation in place"]

## Next Audit
Recommended: [date -- typically 1-4 weeks depending on findings]
```

---

## Step 8: Update PROJECT_STATE.md

Append to `.agents/handoff/PROJECT_STATE.md`:

```markdown
## Dependency Audit -- [ISO timestamp]
- Vulnerabilities found: [count] | Fixed: [count] | Remaining: [count]
- Dependencies updated: [count]
- License issues: [count]
- Full report: DEPENDENCY_AUDIT.md
```

If critical or high vulnerabilities remain unfixed, add them to the Known Issues section of PROJECT_STATE.md with clear documentation of why they are unfixed and what mitigation is in place.
