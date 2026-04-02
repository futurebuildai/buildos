---
description: Code style enforcement skill. Runs project linters and formatters per TECH_STACK.md configuration. Auto-fixes all fixable violations, reports unfixable violations requiring manual attention, verifies CI will pass, and includes type checking where applicable.
invoked_from:
  - workflows/claude_code/03_implement.md
  - workflows/claude_code/04_quality_gates.md
  - Before every commit
  - When code style issues are reported
produces:
  - Auto-fixed code files (formatting, import ordering, simple lint fixes)
  - Report of unfixable violations requiring manual intervention
  - Type checking results (if applicable)
  - CI pass/fail prediction
---

# Skill: Lint, Format, and Fix

Run all project linters, formatters, and type checkers. Fix everything that can be auto-fixed. Report everything that cannot. The goal is to ensure CI will pass before pushing code.

---

## Prerequisites

Before running this skill, ensure the following exist:

- `TECH_STACK.md` — for the project's linting and formatting tooling
- Project configuration files (`.eslintrc`, `.prettierrc`, `tsconfig.json`, `pyproject.toml`, `.golangci.yml`, etc.)
- All project dependencies installed (`npm install`, `pip install`, `go mod download`, etc.)

---

## Step 1: Discover Project Tooling

Before running anything, identify what tools the project uses.

### Actions

1. **Check for configuration files:**
   ```bash
   # JavaScript / TypeScript ecosystem
   ls -la .eslintrc* .prettierrc* .stylelintrc* tsconfig*.json biome.json* 2>/dev/null

   # Python ecosystem
   ls -la pyproject.toml setup.cfg .flake8 .pylintrc .mypy.ini .ruff.toml ruff.toml 2>/dev/null

   # Go ecosystem
   ls -la .golangci.yml .golangci.yaml 2>/dev/null

   # Rust ecosystem
   ls -la rustfmt.toml .rustfmt.toml clippy.toml 2>/dev/null

   # General
   ls -la .editorconfig 2>/dev/null
   ```

2. **Check package.json scripts (for JS/TS projects):**
   ```bash
   cat package.json | grep -A 20 '"scripts"'
   ```
   Look for: `lint`, `format`, `typecheck`, `check`, `style`, `fix`.

3. **Check for pre-commit hooks:**
   ```bash
   ls -la .husky/ .git/hooks/ 2>/dev/null
   cat .lintstagedrc* lint-staged.config.* 2>/dev/null
   cat package.json | grep -A 10 'lint-staged'
   ```

4. **Build the tool inventory:**

   | Tool | Purpose | Config File | Fix Command | Check Command |
   |------|---------|-------------|-------------|---------------|
   | [tool] | [linting/formatting/types] | [file] | [command] | [command] |

---

## Step 2: Run Formatters (Auto-Fix)

Run formatters first because they make the most changes and lint rules sometimes depend on formatting.

### Actions

1. **Run the project's formatter:**

   **Prettier (JS/TS/CSS/HTML/JSON/YAML/Markdown):**
   ```bash
   # Fix all files
   npx prettier --write .

   # Or fix only changed files
   npx prettier --write $(git diff --name-only --diff-filter=ACMR HEAD)

   # Check without writing (dry run)
   npx prettier --check .
   ```

   **Biome (JS/TS — Prettier + ESLint alternative):**
   ```bash
   npx biome format --write .
   npx biome check --write .
   ```

   **Black (Python):**
   ```bash
   black .
   # Check without writing
   black --check .
   ```

   **Ruff (Python — fast formatter):**
   ```bash
   ruff format .
   # Check without writing
   ruff format --check .
   ```

   **gofmt / goimports (Go):**
   ```bash
   gofmt -w .
   goimports -w .
   ```

   **rustfmt (Rust):**
   ```bash
   cargo fmt
   # Check without writing
   cargo fmt --check
   ```

2. **Run import sorting:**

   **TypeScript/JavaScript:**
   ```bash
   # If using ESLint with import sorting rules, this is handled in Step 3
   # If using a dedicated import sorter:
   npx import-sort --write 'src/**/*.{ts,tsx}'
   ```

   **Python (isort):**
   ```bash
   isort .
   # Check without writing
   isort --check-only .
   ```

3. **Verify formatting changes:**
   ```bash
   git diff --stat
   ```
   Review the diff to ensure formatting did not break anything.

---

## Step 3: Run Linters (Auto-Fix Where Possible)

Run linters with auto-fix enabled, then report remaining issues.

### Actions

1. **Run the project's linter with auto-fix:**

   **ESLint (JS/TS):**
   ```bash
   # Fix all auto-fixable issues
   npx eslint --fix .

   # Fix only changed files
   npx eslint --fix $(git diff --name-only --diff-filter=ACMR HEAD -- '*.ts' '*.tsx' '*.js' '*.jsx')

   # Report remaining issues (no fix)
   npx eslint .
   ```

   **Biome (JS/TS):**
   ```bash
   npx biome lint --write .
   # Report remaining issues
   npx biome lint .
   ```

   **Ruff (Python — fast linter):**
   ```bash
   ruff check --fix .
   # Report remaining issues
   ruff check .
   ```

   **Flake8 (Python — no auto-fix):**
   ```bash
   flake8 .
   ```

   **Pylint (Python — no auto-fix):**
   ```bash
   pylint src/
   ```

   **golangci-lint (Go):**
   ```bash
   golangci-lint run --fix ./...
   # Report remaining issues
   golangci-lint run ./...
   ```

   **Clippy (Rust):**
   ```bash
   cargo clippy --fix --allow-dirty --allow-staged
   # Report remaining issues
   cargo clippy -- -D warnings
   ```

2. **Run CSS/SCSS linter (if applicable):**
   ```bash
   # Stylelint
   npx stylelint --fix "**/*.{css,scss}"
   npx stylelint "**/*.{css,scss}"
   ```

3. **Capture remaining unfixable violations:**
   ```bash
   # Run lint check mode (no fix) and capture output
   npx eslint . 2>&1 | tee lint-results.txt
   ```

4. **Categorize results:**

   | Severity | Rule | Count | Auto-Fixed | Manual Fix Needed | Files |
   |----------|------|-------|------------|-------------------|-------|
   | Error | [rule-id] | [n] | [n] | [n] | [file list] |
   | Warning | [rule-id] | [n] | [n] | [n] | [file list] |

---

## Step 4: Run Type Checker

Run the project's type checker to catch type errors.

### Actions

1. **TypeScript:**
   ```bash
   npx tsc --noEmit

   # If the project has multiple tsconfig files
   npx tsc --noEmit --project tsconfig.json
   npx tsc --noEmit --project tsconfig.test.json  # if exists
   ```

2. **Python (mypy):**
   ```bash
   mypy src/
   # Or with the project's mypy config
   mypy .
   ```

   **Python (pyright):**
   ```bash
   npx pyright
   # Or
   pyright
   ```

   **Python (pytype):**
   ```bash
   pytype
   ```

3. **Go (type checking is part of build):**
   ```bash
   go build ./...
   go vet ./...
   ```

4. **Rust (type checking is part of build):**
   ```bash
   cargo check
   ```

5. **Capture type errors:**

   | File | Line | Error | Description |
   |------|------|-------|-------------|
   | [path] | [line] | [code] | [message] |

6. **Common type errors to watch for:**
   - `any` type that should be narrowed.
   - Missing null/undefined checks.
   - Incorrect generic type parameters.
   - Type assertions (`as`) that may be unsafe.
   - Missing return type annotations.

---

## Step 5: Run Additional Checks

Run any additional code quality checks the project configures.

### Actions

1. **Spell checking (if configured):**
   ```bash
   # cspell
   npx cspell "**/*.{ts,tsx,js,jsx,py,go,rs,md}" --no-progress

   # codespell
   codespell src/
   ```

2. **Markdown linting (if applicable):**
   ```bash
   npx markdownlint-cli2 "**/*.md"
   ```

3. **YAML/JSON validation:**
   ```bash
   # Check YAML files
   npx yaml-lint .github/**/*.yml

   # Check JSON files
   find . -name '*.json' -not -path '*/node_modules/*' -exec python3 -m json.tool {} \; > /dev/null
   ```

4. **Dockerfile linting (if applicable):**
   ```bash
   hadolint Dockerfile
   ```

5. **Shell script linting (if applicable):**
   ```bash
   shellcheck scripts/*.sh
   ```

---

## Step 6: Verify CI Will Pass

Simulate the CI pipeline locally to predict whether the push will succeed.

### Actions

1. **Run the same commands CI runs:**
   ```bash
   # Check for CI configuration
   cat .github/workflows/*.yml 2>/dev/null | grep -A 5 'run:'
   cat .gitlab-ci.yml 2>/dev/null | grep -A 5 'script:'
   ```

2. **Run the full check sequence (no auto-fix):**
   ```bash
   # Example for a typical JS/TS project:
   npx prettier --check . && \
   npx eslint . && \
   npx tsc --noEmit && \
   npm test

   # Example for a typical Python project:
   ruff format --check . && \
   ruff check . && \
   mypy src/ && \
   pytest

   # Example for a typical Go project:
   gofmt -l . && \
   golangci-lint run ./... && \
   go build ./... && \
   go test ./...
   ```

3. **Report CI prediction:**

   | Check | Command | Result | Blocking |
   |-------|---------|--------|----------|
   | Formatting | [command] | PASS/FAIL | Yes |
   | Linting | [command] | PASS/FAIL | Yes |
   | Type checking | [command] | PASS/FAIL | Yes |
   | Tests | [command] | PASS/FAIL | Yes |

---

## Output Format

```markdown
## Lint, Format, and Fix Report

**Date:** [timestamp]
**Tools used:** [list of tools and versions]

### Auto-Fixed Issues

| Category | Files Modified | Issues Fixed |
|----------|---------------|-------------|
| Formatting | [n] | [n] |
| Import sorting | [n] | [n] |
| Lint auto-fix | [n] | [n] |
| **Total** | **[n]** | **[n]** |

### Remaining Issues Requiring Manual Fix

| # | File | Line | Rule/Error | Description | Severity |
|---|------|------|-----------|-------------|----------|
| 1 | [path] | [line] | [rule] | [message] | Error/Warning |

### Type Checking Results

| Result | Count |
|--------|-------|
| Errors | [n] |
| Warnings | [n] |

### CI Prediction

| Check | Status |
|-------|--------|
| Format check | PASS/FAIL |
| Lint check | PASS/FAIL |
| Type check | PASS/FAIL |
| Tests | PASS/FAIL |

**Overall: CI WILL PASS / CI WILL FAIL**

### Manual Fix Instructions

For each unfixable issue, provide the specific fix:

1. **[File:line] [Rule]:** [Explanation of what is wrong and how to fix it]
```

If CI will fail, fix all auto-fixable issues and provide explicit instructions for every remaining issue. Do not leave the codebase in a state where CI will fail without clearly communicating what needs to be done.
