---
description: Bootstrap a new project from Antigravity's architecture specs. Creates directory structure, installs dependencies, configures tooling, and produces a verified development environment.
inputs: ARCHITECTURE.md, TECH_STACK.md, EXECUTION_PLAN.md (from .agents/handoff/)
outputs: SCAFFOLD_REPORT.md (in .agents/handoff/), working project skeleton, configured tooling
---

# Workflow: Scaffold Project

Set up the entire project foundation so that `make dev` (or equivalent) works, the linter passes on empty source files, tests can run, and the CI pipeline has something to execute. This is always the first workflow run on a fresh build.

---

## Step 1: Read and Internalize Specifications

Read the following artifacts completely before writing any code:

1. **`.agents/TECH_STACK.md`** -- determines every tooling decision: language, framework, package manager, linter, formatter, test runner, CI platform.
2. **`.agents/handoff/ARCHITECTURE.md`** -- defines directory structure, module boundaries, package layout, and dependency graph.
3. **`.agents/handoff/EXECUTION_PLAN.md`** -- confirms the scaffolding tasks and their expected outputs (typically Milestone 0).
4. **`.agents/handoff/DESIGN_SYSTEM.md`** (if frontend exists) -- identifies any CSS/component framework to install.
5. **`.agents/handoff/API_CONTRACT.md`** (if applicable) -- identifies API framework dependencies.

Summarize your understanding of the project structure in 5-10 bullet points. **Pause for user approval before proceeding.**

---

## Step 2: Create Directory Structure

Build the directory layout specified in ARCHITECTURE.md. Typical structure (adapt to the actual spec):

```
project-root/
  cmd/ or src/             # Application entry points
  internal/ or lib/        # Core business logic (respect module boundaries)
  pkg/ or shared/          # Shared/public packages
  api/                     # API definitions, OpenAPI specs, proto files
  migrations/              # Database migration files
  scripts/                 # Build, seed, and utility scripts
  test/ or __tests__/      # Test fixtures, helpers, e2e tests
  docs/                    # Developer documentation
  .github/ or .gitlab-ci/  # CI/CD configuration
  .agents/                 # Preserve the agents directory (do not modify)
```

Rules:
- Create **only** the directories specified in ARCHITECTURE.md. Do not invent additional structure.
- Add a `.gitkeep` in empty directories that must exist for the build to work.
- Verify the structure matches ARCHITECTURE.md Section 2.1 (or equivalent) exactly.

---

## Step 3: Initialize Build Tools and Package Manager

Based on TECH_STACK.md:

### Language-Specific Initialization
| Stack | Action |
|-------|--------|
| Go | `go mod init [module-path]` per ARCHITECTURE.md |
| Node/TS | `npm init -y` or `pnpm init`, then configure `package.json` fields (name, version, scripts, engines) |
| Python | Create `pyproject.toml` with build system, or `setup.py` + `requirements.txt` |
| Rust | `cargo init` with workspace if multi-crate |
| Other | Follow TECH_STACK.md instructions exactly |

### TypeScript Configuration (if applicable)
- Create `tsconfig.json` with strict mode enabled.
- Configure path aliases matching ARCHITECTURE.md module boundaries.
- Set `compilerOptions` per TECH_STACK.md (target, module, moduleResolution).

### Build Targets
If the project uses a compiled language or bundler, configure the build pipeline:
- Development build (fast, with source maps / debug symbols).
- Production build (optimized, minified where applicable).
- Watch mode for development iteration.

---

## Step 4: Install Dependencies

Install **only** the dependencies listed in ARCHITECTURE.md and TECH_STACK.md. Categorize them:

### Production Dependencies
Install the exact packages and versions specified. If versions are not pinned in the spec, use the latest stable version and pin it in the lock file.

### Development Dependencies
Install tooling dependencies:
- Linter (from TECH_STACK.md)
- Formatter (from TECH_STACK.md)
- Test framework and runner (from TECH_STACK.md)
- Type checking tools (if applicable)
- Build tools (if applicable)

### Dependency Verification
After installation:
- [ ] Run the package manager's integrity check (`npm ls`, `go mod verify`, `pip check`).
- [ ] Confirm no peer dependency warnings or conflicts.
- [ ] Confirm no deprecated packages in direct dependencies.
- [ ] Run `npm audit` / `go vuln check` / equivalent -- note any findings but do not block scaffolding.

**Do NOT install packages not listed in the specs.** If a needed package is missing from the spec, log it in ESCALATION_LOG.md and ask the user.

---

## Step 5: Configure Linter and Formatter

### Linter Setup
Create the linter configuration file (`.eslintrc.json`, `.golangci.yml`, `ruff.toml`, etc.) with:
- Rules specified in TECH_STACK.md.
- If no specific rules are given, use the recommended/strict preset for the chosen linter.
- Configure import ordering rules matching ARCHITECTURE.md module boundaries.
- Enable all security-related lint rules available.

### Formatter Setup
Create the formatter configuration (`.prettierrc`, `gofmt` config, `black.toml`, etc.) with:
- Settings from TECH_STACK.md.
- If no specific settings, use the tool's defaults (consistency over preference).

### Editor Configuration
Create `.editorconfig`:
```ini
root = true

[*]
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
charset = utf-8

[*.{js,ts,jsx,tsx,json,css,md,yaml,yml}]
indent_style = space
indent_size = 2

[*.{go,py,rs}]
indent_style = [per language convention]
indent_size = [per language convention]

[Makefile]
indent_style = tab
```

### Validation
- Run the linter on the (empty) project -- it must exit cleanly.
- Run the formatter -- it must exit cleanly.

---

## Step 6: Configure Test Framework

Based on TECH_STACK.md and TESTING_STRATEGY.md:

- Create test configuration file (`jest.config.ts`, `vitest.config.ts`, `pytest.ini`, etc.).
- Configure coverage reporting with thresholds from TESTING_STRATEGY.md.
- Set up test path patterns matching the directory structure.
- Configure test environment (jsdom, node, etc.) per TECH_STACK.md.
- Create a single placeholder test that asserts `true === true` to verify the test runner works.

### Validation
- Run the test suite -- the placeholder test must pass.
- Run coverage reporting -- it must generate a report (even if coverage is 0%).

---

## Step 7: Create Makefile or Taskfile

Create a task runner with **at minimum** these targets:

```makefile
# Development
dev          # Start the application in development mode (hot-reload if applicable)
dev-deps     # Start infrastructure dependencies (database, cache, etc.)

# Quality
lint         # Run linter
format       # Run formatter
format-check # Check formatting without modifying files (for CI)
typecheck    # Run type checker (if applicable)

# Testing
test         # Run unit tests
test-int     # Run integration tests
test-e2e     # Run end-to-end tests
test-all     # Run all tests
coverage     # Run tests with coverage reporting

# Build
build        # Production build
clean        # Remove build artifacts

# Database (if applicable)
db-migrate   # Run pending migrations
db-rollback  # Rollback last migration
db-seed      # Seed development data
db-reset     # Drop, create, migrate, seed

# Utilities
setup        # First-time project setup (install deps, create .env, migrate, seed)
check        # Run all quality checks (format-check + lint + typecheck + test)
```

Each target must have a brief comment explaining what it does. Targets must work -- do not create targets that reference tools or scripts that do not exist yet.

---

## Step 8: Create Environment Configuration

### `.env.example`
Create with every environment variable the application needs, using placeholder values:
```
# Application
APP_ENV=development
APP_PORT=8080
APP_LOG_LEVEL=debug

# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=appname_dev
DB_USER=postgres
DB_PASSWORD=CHANGE_ME

# External Services (reference only -- actual secrets managed outside repo)
# STRIPE_API_KEY=sk_test_...
# SENDGRID_API_KEY=SG...
```

Rules:
- Include **every** variable referenced in ARCHITECTURE.md.
- Use safe development defaults.
- Comment out any variables for external services (secrets must never have real values in this file).
- Add a header comment: `# Copy this to .env and fill in real values. Never commit .env.`

### `.gitignore`
Create a comprehensive `.gitignore`:
- Language-specific build artifacts.
- IDE/editor files (`.idea/`, `.vscode/settings.json`, `*.swp`).
- OS files (`.DS_Store`, `Thumbs.db`).
- Environment files (`.env`, `.env.local`, `.env.*.local`).
- Dependency directories (`node_modules/`, `vendor/` if not committed).
- Coverage and test output (`coverage/`, `*.lcov`, `.nyc_output/`).
- Build output (`dist/`, `build/`, `out/`, `bin/`).
- Log files (`*.log`).

---

## Step 9: Configure Git Hooks

Set up pre-commit hooks using the tool specified in TECH_STACK.md (husky, lefthook, pre-commit, etc.):

### Pre-Commit Hook
1. Run formatter on staged files.
2. Run linter on staged files.
3. Run type checker (if applicable).
4. Scan for secrets (using gitleaks, detect-secrets, or equivalent).

### Pre-Push Hook (optional, if specified in TECH_STACK.md)
1. Run full test suite.
2. Verify build succeeds.

### Commit Message Hook (optional)
If TECH_STACK.md specifies conventional commits, configure commitlint or equivalent.

### Validation
- Stage a test file and run the pre-commit hook manually -- it must execute all checks.

---

## Step 10: Verify the Scaffold

Run the full verification sequence:

```
1. make setup        # (or equivalent) -- must complete without errors
2. make lint         # -- must pass
3. make format-check # -- must pass
4. make test         # -- placeholder test must pass
5. make build        # -- must produce output (even if minimal)
6. make dev          # -- must start (verify it binds to a port or outputs a ready message, then Ctrl+C)
```

If any step fails, fix it before proceeding. Do not skip verification.

---

## Step 11: Produce SCAFFOLD_REPORT.md

Write `.agents/handoff/SCAFFOLD_REPORT.md`:

```markdown
# Scaffold Report

## Date: [ISO timestamp]

## Project Structure
[Tree output of the created directory structure]

## Dependencies Installed
### Production
| Package | Version | Purpose |
|---------|---------|---------|

### Development
| Package | Version | Purpose |
|---------|---------|---------|

## Tooling Configured
| Tool | Config File | Status |
|------|------------|--------|
| Linter | [file] | Passing |
| Formatter | [file] | Passing |
| Test Runner | [file] | Passing |
| Git Hooks | [file] | Passing |
| Build | [file] | Passing |

## Make Targets Available
[List all targets with descriptions]

## Environment Variables
[Count] variables defined in .env.example

## Verification Results
| Check | Result |
|-------|--------|
| make lint | PASS |
| make format-check | PASS |
| make test | PASS |
| make build | PASS |
| make dev | PASS |

## Audit Findings
### Dependency Vulnerabilities
[Any findings from npm audit / go vuln / etc.]

### Notes
[Any deviations from spec, decisions made, or items for user review]
```

---

## Step 12: Update PROJECT_STATE.md

Update `.agents/handoff/PROJECT_STATE.md`:

```markdown
## Completed Steps
| Step | Description | Commit | Audit Status |
|------|-------------|--------|--------------|
| 0.1 | Project scaffolding | [hash] | PASS |

## Phase: scaffolding -> development
```

Proceed to the next step in EXECUTION_PLAN.md.
