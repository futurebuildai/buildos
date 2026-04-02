---
description: Git commit hygiene practices. Ensures commits are atomic (one logical change per commit), follow conventional commit message format, contain no large binary files, have no secrets in history, and are covered by a complete .gitignore for all generated and sensitive files.
invoked_from:
  - workflows/claude_code/03_implement.md
  - workflows/claude_code/04_quality_gates.md
  - Before every commit
  - Before opening a pull request
produces:
  - Git hygiene report with pass/fail per check
  - Suggested commit message following conventional commit format
  - List of files that should be in .gitignore but are not
  - Large file warnings
  - Secrets-in-history scan results
---

# Skill: Git Hygiene

Enforce clean git practices on every commit. Git history is a permanent record — mistakes in commits are expensive to fix later. This skill runs before committing to ensure the commit is atomic, well-messaged, and free of files that should not be in version control.

---

## Prerequisites

Before running this skill, ensure the following:

- The working directory is a git repository.
- `TECH_STACK.md` — for understanding which files are generated and should be ignored.
- Staged changes exist (or changes to be staged).

---

## Step 1: Verify Atomic Commits

Ensure each commit contains exactly one logical change.

### Actions

1. **Review the staged changes:**
   ```bash
   git diff --cached --stat
   git diff --cached --name-only
   ```

2. **Check for atomicity violations:**

   A commit is NOT atomic if it contains:
   - Changes to unrelated features (e.g., a bug fix and a new feature in the same commit).
   - Formatting changes mixed with logic changes (format in a separate commit).
   - Dependency updates mixed with code changes (update deps in a separate commit).
   - Test changes for a different feature than the code changes.
   - Multiple independent refactors in different modules.

3. **Heuristic checks:**
   ```bash
   # Count the number of files changed
   FILE_COUNT=$(git diff --cached --name-only | wc -l)

   # Count the number of directories touched
   DIR_COUNT=$(git diff --cached --name-only | xargs -I {} dirname {} | sort -u | wc -l)

   # Count lines changed
   LINES_CHANGED=$(git diff --cached --stat | tail -1)
   ```

   **Warning thresholds:**
   - More than 10 files changed — review if all changes are related.
   - More than 5 directories touched — likely multiple concerns.
   - More than 500 lines changed — consider splitting.

   These are soft warnings, not hard failures. A large refactor touching many files may be legitimately atomic.

4. **If the commit is not atomic, recommend splitting:**
   ```bash
   # Suggest which files belong to which commit
   # Group by: feature area, type of change (test/implementation/config), module
   ```

   Example recommendation:
   ```
   This commit appears to contain multiple logical changes:

   Commit 1 (feat: add user profile endpoint):
     - src/api/users/profile.ts
     - src/services/profile.ts
     - tests/api/users/profile.test.ts

   Commit 2 (fix: correct email validation regex):
     - src/utils/validation.ts
     - tests/utils/validation.test.ts

   Commit 3 (chore: update prettier config):
     - .prettierrc
   ```

---

## Step 2: Conventional Commit Message

Write or verify a conventional commit message.

### Actions

1. **Determine the commit type from the staged changes:**

   | Type | When to Use | Example |
   |------|-------------|---------|
   | `feat` | A new feature for the user | `feat: add user profile page` |
   | `fix` | A bug fix | `fix: correct email validation for plus addresses` |
   | `refactor` | Code change that neither fixes a bug nor adds a feature | `refactor: extract validation into shared module` |
   | `test` | Adding or updating tests only | `test: add integration tests for payment flow` |
   | `docs` | Documentation only changes | `docs: update API endpoint documentation` |
   | `style` | Formatting, missing semicolons, etc. (no logic change) | `style: apply prettier formatting` |
   | `chore` | Build process, dependency updates, tooling | `chore: upgrade TypeScript to 5.3` |
   | `perf` | Performance improvement | `perf: add database index for user lookup query` |
   | `ci` | CI/CD configuration changes | `ci: add coverage check to PR pipeline` |
   | `build` | Build system or external dependency changes | `build: configure webpack code splitting` |
   | `revert` | Reverting a previous commit | `revert: revert "feat: add user profile page"` |

2. **Construct the commit message:**

   Format:
   ```
   <type>(<optional scope>): <description>

   [optional body]

   [optional footer(s)]
   ```

   Rules:
   - **Subject line:** Imperative mood ("add feature", not "added feature" or "adds feature").
   - **Subject line:** Max 72 characters. No period at the end.
   - **Subject line:** Lowercase first letter after the colon.
   - **Scope:** Module or feature area in parentheses (e.g., `feat(auth):`, `fix(api):`).
   - **Body:** Wrap at 72 characters. Explain **what** and **why**, not **how** (the diff shows how).
   - **Footer:** Reference issues (`Closes #123`, `Fixes #456`). Note breaking changes (`BREAKING CHANGE: description`).

3. **Verify against existing commit history:**
   ```bash
   # Check recent commit messages for style consistency
   git log --oneline -20
   ```
   Match the project's existing convention. If the project does not use conventional commits, match whatever format is already in use.

4. **Examples of good commit messages:**

   ```
   feat(auth): add OAuth2 login with Google provider

   Implement OAuth2 authorization code flow for Google login.
   Users can now sign in with their Google account in addition
   to email/password authentication.

   - Add Google OAuth2 strategy
   - Add callback handler for token exchange
   - Store OAuth tokens encrypted in user profile
   - Add login button to authentication page

   Closes #142
   ```

   ```
   fix(api): prevent N+1 query in user list endpoint

   The GET /api/users endpoint was issuing one database query per
   user to fetch their profile data. This caused response times
   to scale linearly with the number of users.

   Replace individual queries with a single JOIN query, reducing
   the endpoint response time from ~2s to ~50ms for 100 users.

   Fixes #289
   ```

5. **Examples of bad commit messages (do not write these):**
   - `fix stuff`
   - `WIP`
   - `updates`
   - `addressed review comments`
   - `fix: fix the bug` (redundant)
   - `feat: implemented the new feature for the user profile page that shows all the user data` (too long)

---

## Step 3: No Large Binary Files

Verify that no large binary files are being committed.

### Actions

1. **Check staged files for large binaries:**
   ```bash
   # List staged files larger than 1MB
   git diff --cached --name-only | while read file; do
     if [ -f "$file" ]; then
       SIZE=$(wc -c < "$file")
       if [ "$SIZE" -gt 1048576 ]; then
         echo "WARNING: $file is $(( SIZE / 1024 ))KB"
       fi
     fi
   done
   ```

2. **Check for binary file types that should not be committed:**
   ```bash
   # Common binary files that should not be in git
   git diff --cached --name-only | grep -iE '\.(zip|tar|gz|rar|7z|exe|dll|so|dylib|bin|dat|db|sqlite|pdf|doc|docx|xls|xlsx|ppt|pptx|mp3|mp4|avi|mov|wav|flac|jpg|jpeg|png|gif|bmp|tiff|ico|psd|ai|sketch|fig|woff|woff2|ttf|eot|otf)$'
   ```

3. **Exceptions (files that ARE acceptable):**
   - Small images used in the application (icons, logos) under 100KB.
   - Font files if self-hosted (should still be in .gitignore if pulled via CDN).
   - Test fixtures that are intentionally binary.

4. **If large files are found, recommend alternatives:**
   - Use Git LFS for large files that must be tracked.
   - Store large assets in a CDN or object storage (S3, GCS, etc.).
   - Add the file to `.gitignore` if it is generated.

5. **Check total commit size:**
   ```bash
   # Estimate the size of the staged changes
   git diff --cached --stat | tail -1
   ```
   If the commit adds more than 5MB of new content, flag for review.

---

## Step 4: No Secrets in Git History

Verify that no secrets are being committed and that none exist in history.

### Actions

1. **Scan staged changes for secrets:**
   ```bash
   # Check staged diff for secret patterns
   git diff --cached | grep -iE \
     'password\s*[:=]\s*["\x27]|api[_-]?key\s*[:=]\s*["\x27]|secret\s*[:=]\s*["\x27]|token\s*[:=]\s*["\x27]|AKIA[0-9A-Z]{16}|sk_live_|sk_test_|ghp_[a-zA-Z0-9]{36}|gho_[a-zA-Z0-9]{36}|xox[bpsa]-[a-zA-Z0-9-]+'
   ```

2. **Check for .env files being staged:**
   ```bash
   git diff --cached --name-only | grep -iE '^\.env|\.env\.'
   ```
   **Rule: .env files must NEVER be committed.** If a `.env.example` file is being committed (with placeholder values only), that is acceptable.

3. **Scan git history for previously committed secrets (periodic check):**
   ```bash
   # Using gitleaks (if available)
   gitleaks detect --source=. --report-format=json --report-path=gitleaks-report.json 2>/dev/null

   # Using trufflehog (if available)
   trufflehog git file://. --json 2>/dev/null | head -20

   # Manual spot check
   git log --all --oneline -p -- '*.env' '*.env.*' | head -50
   ```

4. **If secrets are found in history:**
   - Do NOT attempt to rewrite git history automatically (this is destructive and requires coordination).
   - Report the finding to the user with:
     - The commit hash where the secret was introduced.
     - The file and line where the secret appears.
     - Instructions for remediation:
       1. Rotate the compromised credential immediately.
       2. Use `git filter-branch` or `BFG Repo-Cleaner` to remove the secret from history.
       3. Force push (requires team coordination).
       4. Notify anyone who may have cloned the repo.

---

## Step 5: Verify .gitignore Coverage

Ensure that `.gitignore` covers all generated, sensitive, and temporary files.

### Actions

1. **Check for a .gitignore file:**
   ```bash
   cat .gitignore
   ```

2. **Verify essential entries per tech stack:**

   **All projects:**
   | Entry | Purpose | Present |
   |-------|---------|---------|
   | `.env` | Environment variables with secrets | REQUIRED |
   | `.env.*` (except `.env.example`) | Environment variants | REQUIRED |
   | `*.log` | Log files | REQUIRED |
   | `.DS_Store` | macOS metadata | REQUIRED |
   | `Thumbs.db` | Windows metadata | RECOMMENDED |

   **Node.js / TypeScript:**
   | Entry | Purpose | Present |
   |-------|---------|---------|
   | `node_modules/` | Installed dependencies | REQUIRED |
   | `dist/` or `build/` | Build output | REQUIRED |
   | `.next/` | Next.js build output | If using Next.js |
   | `coverage/` | Test coverage output | REQUIRED |
   | `*.tsbuildinfo` | TypeScript incremental build | RECOMMENDED |
   | `.turbo/` | Turborepo cache | If using Turborepo |

   **Python:**
   | Entry | Purpose | Present |
   |-------|---------|---------|
   | `__pycache__/` | Python bytecode cache | REQUIRED |
   | `*.pyc` | Compiled Python files | REQUIRED |
   | `.venv/` or `venv/` | Virtual environment | REQUIRED |
   | `*.egg-info/` | Package metadata | REQUIRED |
   | `.mypy_cache/` | mypy cache | RECOMMENDED |
   | `.pytest_cache/` | pytest cache | RECOMMENDED |
   | `htmlcov/` | Coverage HTML output | RECOMMENDED |

   **Go:**
   | Entry | Purpose | Present |
   |-------|---------|---------|
   | Binary output (project name) | Compiled binary | REQUIRED |
   | `vendor/` | Vendored dependencies (if not committed) | DEPENDS |
   | `coverage.out` | Coverage output | RECOMMENDED |

   **General / IDE:**
   | Entry | Purpose | Present |
   |-------|---------|---------|
   | `.idea/` | JetBrains IDE | RECOMMENDED |
   | `.vscode/` (or specific files) | VS Code settings | RECOMMENDED |
   | `*.swp`, `*.swo` | Vim swap files | RECOMMENDED |

3. **Check for tracked files that should be ignored:**
   ```bash
   # Find files that match .gitignore patterns but are already tracked
   git ls-files --cached --ignored --exclude-standard 2>/dev/null
   ```

4. **Check for untracked files that should be ignored:**
   ```bash
   # List untracked files that might need to be added to .gitignore
   git status --porcelain | grep '??' | awk '{print $2}' | head -20
   ```

5. **If missing entries are found, add them:**
   - Append missing entries to `.gitignore`.
   - If a file is already tracked but should be ignored:
     ```bash
     # Remove from tracking without deleting the file
     git rm --cached <file>
     # Then add to .gitignore
     ```

---

## Step 6: Pre-Commit Validation Summary

Run a final validation before the commit proceeds.

### Actions

1. **Compile the full pre-commit checklist:**

   | Check | Status | Blocking |
   |-------|--------|----------|
   | Changes are atomic (single logical change) | PASS/FAIL/WARN | WARN |
   | Commit message follows conventional format | PASS/FAIL | YES |
   | No large binary files (> 1MB) staged | PASS/FAIL | YES |
   | No secrets in staged changes | PASS/FAIL | YES |
   | .gitignore covers all generated/sensitive files | PASS/FAIL | YES |
   | No .env files staged | PASS/FAIL | YES |
   | No TODO/FIXME added without ticket reference | PASS/WARN | WARN |

2. **If the project has pre-commit hooks, verify they will pass:**
   ```bash
   # Check for husky hooks
   ls .husky/pre-commit 2>/dev/null && cat .husky/pre-commit

   # Check for pre-commit framework
   cat .pre-commit-config.yaml 2>/dev/null

   # Run pre-commit manually
   npx lint-staged --dry-run 2>/dev/null
   ```

3. **Generate the recommended commit command:**
   ```bash
   git add <specific files>
   git commit -m "<type>(<scope>): <description>

   <body>

   <footer>"
   ```

---

## Output Format

```markdown
## Git Hygiene Report

**Date:** [timestamp]
**Branch:** [branch name]
**Files staged:** [count]

### Pre-Commit Checklist

| # | Check | Status | Details |
|---|-------|--------|---------|
| 1 | Atomic commit | PASS/WARN | [details] |
| 2 | Conventional message | PASS/FAIL | [details] |
| 3 | No large binaries | PASS/FAIL | [details] |
| 4 | No secrets staged | PASS/FAIL | [details] |
| 5 | .gitignore complete | PASS/FAIL | [details] |
| 6 | No .env files | PASS/FAIL | [details] |

### Recommended Commit Message

```
<type>(<scope>): <description>

<body>
```

### .gitignore Updates Needed

| Entry | Reason |
|-------|--------|
| [pattern] | [why it should be ignored] |

### Warnings

- [Any non-blocking warnings about commit quality]

### Blocking Issues

- [Any issues that must be resolved before committing]

**Ready to commit: YES / NO**
```

If the report says NO, do not commit. Fix the blocking issues first.
