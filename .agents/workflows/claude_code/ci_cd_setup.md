---
description: Configure CI/CD pipeline from TECH_STACK.md specifications. Generates pipeline configuration files, sets up staged quality gates, configures environment targets, and produces a verification report.
inputs: TECH_STACK.md (CI/CD platform and tooling), ARCHITECTURE.md (build targets), TESTING_STRATEGY.md (test stages)
outputs: Pipeline configuration files, CI_CD_REPORT.md (in .agents/handoff/)
---

# Workflow: CI/CD Setup

Configure the continuous integration and continuous deployment pipeline. The pipeline must catch every defect before it reaches production: formatting, linting, type errors, test failures, security vulnerabilities, and build failures. It should be fast, reliable, and require zero manual intervention for the happy path.

---

## Step 1: Read Platform and Tooling Specifications

Read the following to determine the CI/CD configuration:

1. **`.agents/TECH_STACK.md`** -- identifies:
   - CI/CD platform (GitHub Actions, GitLab CI, CircleCI, Jenkins, etc.).
   - Container registry (if applicable).
   - Deployment targets (cloud provider, platform, service).
   - Infrastructure-as-code tool (Terraform, Pulumi, CDK, etc.), if any.
   - Secret management approach.

2. **`.agents/handoff/ARCHITECTURE.md`** -- identifies:
   - Build outputs (binary, container image, static assets, etc.).
   - Service architecture (monolith, microservices, monorepo).
   - Environment requirements (Node version, Go version, system dependencies).

3. **`.agents/handoff/TESTING_STRATEGY.md`** -- identifies:
   - Test stages and their commands.
   - Coverage thresholds.
   - Test infrastructure needs (database for integration tests, browser for e2e).

Summarize the CI/CD architecture in a few bullet points. **Pause for user review before generating pipeline files.**

---

## Step 2: Generate Pipeline Configuration

Create the pipeline configuration file(s) for the platform specified in TECH_STACK.md.

### GitHub Actions Example Structure
```
.github/
  workflows/
    ci.yml          # Main CI pipeline (runs on every push/PR)
    deploy.yml      # Deployment pipeline (runs on merge to main or tag)
    dependency.yml  # Scheduled dependency audit (weekly)
```

### GitLab CI Example Structure
```
.gitlab-ci.yml      # Complete pipeline definition
```

Adapt to the platform specified in TECH_STACK.md.

---

## Step 3: Configure CI Pipeline Stages

The CI pipeline runs on every push and pull request. Stages execute in strict order -- a stage failure stops the pipeline.

### Stage 1: Setup
```yaml
# Install dependencies and cache them for subsequent stages
- Checkout code
- Set up language runtime (exact version from TECH_STACK.md)
- Cache dependencies (node_modules, go mod cache, pip cache, etc.)
- Install dependencies
```

### Stage 2: Lint and Format Check
```yaml
# Fast checks that catch style issues without running code
- Run formatter check (format-check, not format -- CI should not modify code)
- Run linter
- Run type checker (if applicable)
- Check for secrets in code (gitleaks, detect-secrets, etc.)
```

**Rationale:** These run first because they are the fastest checks. Failing fast on style issues saves CI minutes.

### Stage 3: Unit Tests
```yaml
# Run unit tests with coverage reporting
- Run unit tests
- Generate coverage report
- Upload coverage to reporting tool (Codecov, Coveralls, etc.) if configured
- Fail if coverage is below threshold (from TESTING_STRATEGY.md)
```

### Stage 4: Build
```yaml
# Verify the project builds successfully
- Run production build
- Verify build output exists and is the expected size/format
- Cache build artifacts for later stages
```

### Stage 5: Integration Tests
```yaml
# Run tests that require infrastructure
- Start service dependencies (database, cache, message queue) using Docker Compose or service containers
- Wait for services to be healthy
- Run database migrations
- Seed test data
- Run integration tests
- Tear down services
```

### Stage 6: E2E Tests (if applicable)
```yaml
# Run end-to-end tests that require the full application running
- Start the application
- Wait for the application to be healthy
- Run E2E test suite (Playwright, Cypress, etc.)
- Capture screenshots/videos on failure (as artifacts)
- Tear down
```

### Stage 7: Security Audit
```yaml
# Check for known vulnerabilities
- Run dependency vulnerability audit (npm audit, go vuln, safety, etc.)
- Run SAST tool if configured in TECH_STACK.md
- Fail on critical/high severity vulnerabilities (warn on medium/low)
```

---

## Step 4: Configure Deployment Pipeline

The deployment pipeline runs only on specific triggers (merge to main, tag push, manual dispatch). It uses the artifacts produced by CI.

### Environment Configuration
Define deployment targets from TECH_STACK.md:

```yaml
environments:
  staging:
    trigger: merge to main
    approval: automatic
    url: [staging URL placeholder]

  production:
    trigger: git tag (v*)
    approval: manual (require explicit approval)
    url: [production URL placeholder]
```

### Deployment Stages

#### Stage 1: Build Release Artifact
```yaml
# Build the deployable artifact
- Run production build (with production environment variables)
- Build container image (if containerized) with appropriate tags:
  - Git SHA (e.g., app:abc1234)
  - Semver tag (e.g., app:v1.2.3) if this is a tagged release
  - latest (for the most recent main build)
- Push artifact to registry/storage
```

#### Stage 2: Deploy to Staging
```yaml
# Deploy to staging environment
- Deploy artifact to staging
- Wait for deployment to be healthy (health check endpoint)
- Run smoke tests against staging
- Notify on success/failure
```

#### Stage 3: Deploy to Production
```yaml
# Deploy to production (manual approval required)
- Require manual approval (GitHub environment protection, GitLab manual job, etc.)
- Deploy artifact to production
- Wait for deployment to be healthy
- Run smoke tests against production
- Notify on success/failure
```

### Rollback Configuration
```yaml
# If deployment health check fails
- Automatically roll back to previous version
- Notify team of rollback
- Mark deployment as failed
```

---

## Step 5: Configure Secrets References

**CRITICAL: Never put actual secrets in pipeline files.** Configure references to secrets that must be set in the CI/CD platform's secret management.

### Required Secrets Documentation
Create a section in the pipeline configuration (as comments) documenting every secret needed:

```yaml
# REQUIRED SECRETS (configure in CI/CD platform settings):
#
# Repository/Environment Secrets:
#   DEPLOY_KEY          - SSH key or token for deployment access
#   CONTAINER_REGISTRY  - Registry URL (e.g., ghcr.io/org/app)
#   REGISTRY_USERNAME   - Container registry username
#   REGISTRY_PASSWORD   - Container registry password/token
#
# Staging Environment:
#   STAGING_DB_URL      - Database connection string for staging
#   STAGING_API_KEYS    - External service API keys for staging
#
# Production Environment:
#   PROD_DB_URL         - Database connection string for production
#   PROD_API_KEYS       - External service API keys for production
#
# Optional:
#   CODECOV_TOKEN       - For coverage reporting
#   SLACK_WEBHOOK_URL   - For deployment notifications
```

### Environment Variable Configuration
For each environment, document the non-secret configuration:

```yaml
env:
  # Non-secret configuration (safe to commit)
  APP_ENV: staging  # or production
  APP_LOG_LEVEL: info
  APP_PORT: 8080

  # Secret references (values stored in CI/CD platform)
  DB_URL: ${{ secrets.STAGING_DB_URL }}
  API_KEY: ${{ secrets.STAGING_API_KEY }}
```

---

## Step 6: Configure Pipeline Optimizations

### Caching
Configure dependency caching to speed up pipelines:
- Cache key should include the lock file hash (so cache invalidates when dependencies change).
- Restore from a fallback key if exact match is not found.

### Concurrency
- Cancel in-progress pipelines when a new push to the same branch occurs.
- Limit concurrent deployments to one per environment.

### Path Filtering (for monorepos)
If the project is a monorepo, configure pipelines to only run when relevant paths change:
```yaml
# Only run backend pipeline if backend code changed
paths:
  - 'backend/**'
  - 'shared/**'
  - 'package.json'
```

### Timeout Configuration
Set reasonable timeouts to prevent hung pipelines:
| Stage | Timeout |
|-------|---------|
| Lint/Format | 5 minutes |
| Unit Tests | 10 minutes |
| Build | 10 minutes |
| Integration Tests | 15 minutes |
| E2E Tests | 20 minutes |
| Deployment | 10 minutes |

---

## Step 7: Create Scheduled Pipelines

### Dependency Audit (Weekly)
```yaml
# Run weekly to catch newly disclosed vulnerabilities
schedule: "0 9 * * 1"  # Monday at 9 AM UTC
steps:
  - Run dependency vulnerability audit
  - Create issue/notification if new vulnerabilities found
```

### Stale Dependency Check (Monthly)
```yaml
# Run monthly to identify outdated dependencies
schedule: "0 9 1 * *"  # First of month at 9 AM UTC
steps:
  - Check for outdated dependencies
  - Generate report of available updates
  - Create issue with update recommendations
```

---

## Step 8: Validate Pipeline Configuration

### Syntax Validation
- For GitHub Actions: use `actionlint` or the GitHub Actions VS Code extension.
- For GitLab CI: use `gitlab-ci-lint` or the GitLab CI linter API.
- For other platforms: use the platform's validation tool.

### Dry Run
If the platform supports it, run the pipeline in dry-run mode:
```bash
act  # for GitHub Actions local testing
gitlab-runner exec  # for GitLab CI local testing
```

### Manual Verification Checklist
- [ ] Pipeline triggers are correct (push, PR, tag, schedule).
- [ ] Stages execute in the correct order.
- [ ] Stage dependencies are correctly defined (later stages depend on earlier ones).
- [ ] Failure in any stage stops the pipeline.
- [ ] Secrets are referenced, never hardcoded.
- [ ] Caching is configured and uses appropriate keys.
- [ ] Timeouts are set for all stages.
- [ ] Artifacts are configured for test results and build outputs.
- [ ] Notifications are configured for failures.
- [ ] Environment protection rules are set for production deployments.
- [ ] Concurrency limits are configured.

---

## Step 9: Produce CI_CD_REPORT.md

Write `.agents/handoff/CI_CD_REPORT.md`:

```markdown
# CI/CD Report

## Date: [ISO timestamp]

## Platform
- CI/CD: [platform name and version]
- Container Registry: [if applicable]
- Deployment Target: [platform/service]

## Pipeline Files Created
| File | Purpose |
|------|---------|
| [path] | [description] |

## CI Pipeline Stages
| Stage | Trigger | Duration (est.) | Failure Action |
|-------|---------|-----------------|----------------|
| Lint & Format | Push/PR | ~2 min | Block merge |
| Unit Tests | Push/PR | ~5 min | Block merge |
| Build | Push/PR | ~3 min | Block merge |
| Integration Tests | Push/PR | ~8 min | Block merge |
| E2E Tests | Push/PR | ~12 min | Block merge |
| Security Audit | Push/PR | ~3 min | Block on critical/high |

## Deployment Pipeline
| Environment | Trigger | Approval | Rollback |
|-------------|---------|----------|----------|
| Staging | Merge to main | Automatic | Automatic on health check failure |
| Production | Tag (v*) | Manual required | Automatic on health check failure |

## Scheduled Pipelines
| Pipeline | Schedule | Purpose |
|----------|----------|---------|
| Dependency Audit | Weekly (Monday) | Vulnerability detection |
| Stale Dependencies | Monthly (1st) | Update recommendations |

## Required Secrets
| Secret Name | Environment | Purpose | Configured |
|-------------|-------------|---------|------------|
| [name] | [env] | [purpose] | [ ] |

## Configuration Validation
| Check | Result |
|-------|--------|
| Syntax valid | PASS |
| Triggers correct | PASS |
| Stage ordering correct | PASS |
| Secrets referenced (not hardcoded) | PASS |
| Caching configured | PASS |
| Timeouts set | PASS |

## Setup Instructions
To complete CI/CD setup, the following manual steps are required:
1. [ ] Configure secrets in [platform] settings: [list]
2. [ ] Enable environment protection for production
3. [ ] Configure notification channels (Slack, email, etc.)
4. [ ] Verify container registry access
5. [ ] Run the pipeline once to verify caching works

## Notes
[Any deviations from spec, decisions made, or items for user review]
```

---

## Step 10: Update PROJECT_STATE.md

Update `.agents/handoff/PROJECT_STATE.md`:

```markdown
## Completed Steps
| Step | Description | Commit | Audit Status |
|------|-------------|--------|--------------|
| 0.3 | CI/CD pipeline setup | [hash] | PASS |
```

Inform the user of the manual setup steps required (configuring secrets, enabling environment protection, etc.) before the pipeline will be fully functional.
