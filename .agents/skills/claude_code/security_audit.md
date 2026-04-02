---
description: Security-focused analysis covering dependency vulnerability scanning, static analysis for injection vectors, secrets detection, CSP header verification, auth flow verification, CORS configuration checks, and rate limiting verification. Reports findings against the project's threat model.
invoked_from:
  - workflows/claude_code/04_quality_gates.md
  - workflows/claude_code/06_deploy_verify.md
  - After adding new dependencies
  - After modifying authentication or authorization logic
  - Before any production release
produces:
  - Security audit report with severity-ranked findings
  - Dependency vulnerability list with CVEs and remediation steps
  - Secrets detection results (code, logs, env files, git history)
  - CSP and security header compliance report
  - Auth flow verification against threat model
  - CORS and rate limiting configuration audit
---

# Skill: Security Audit

Run a comprehensive security audit covering the most common vulnerability categories. Security findings are classified by severity: Critical (blocks release, must fix immediately), High (blocks release, must fix before deploy), Medium (should fix, tracked as issue), Low (fix when convenient).

---

## Prerequisites

Before running this audit, ensure the following exist:

- `TECH_STACK.md` — for language, framework, and dependency management tooling
- `ARCHITECTURE.md` — for understanding auth flows, data flows, and trust boundaries
- Security threat model (if produced by the `security_threat_model` skill) — for targeted verification
- Access to the codebase and dependency manifests
- Access to a running instance (for header and configuration checks)

---

## Step 1: Dependency Vulnerability Scan

Scan all project dependencies for known vulnerabilities.

### Actions

1. **Run the language-appropriate vulnerability scanner:**

   ```bash
   # Node.js / npm
   npm audit --json > npm-audit-results.json
   npm audit

   # Node.js / yarn
   yarn audit --json > yarn-audit-results.json

   # Node.js / pnpm
   pnpm audit --json > pnpm-audit-results.json

   # Python
   pip-audit --format json --output pip-audit-results.json
   # Or: safety check --json > safety-results.json

   # Go
   govulncheck ./...

   # Rust
   cargo audit

   # Ruby
   bundle-audit check --update

   # General (if Snyk is configured)
   snyk test --json > snyk-results.json
   ```

2. **Parse and categorize results:**

   | Severity | Package | Installed Version | Vulnerable Range | CVE | Fix Available | Fixed Version |
   |----------|---------|------------------|-----------------|-----|---------------|---------------|
   | Critical | [pkg] | [version] | [range] | [CVE-XXXX-XXXXX] | Yes/No | [version] |
   | High | [pkg] | [version] | [range] | [CVE-XXXX-XXXXX] | Yes/No | [version] |
   | Medium | [pkg] | [version] | [range] | [CVE-XXXX-XXXXX] | Yes/No | [version] |
   | Low | [pkg] | [version] | [range] | [CVE-XXXX-XXXXX] | Yes/No | [version] |

3. **For each critical/high vulnerability:**
   - Check if the vulnerable code path is actually used in the project.
   - Check if a fix version exists and whether upgrading introduces breaking changes.
   - If no fix exists, document the mitigation strategy (e.g., WAF rule, input validation).

4. **Check for outdated dependencies with known issues:**
   ```bash
   # Node.js
   npx npm-check-updates

   # Python
   pip list --outdated

   # Go
   go list -m -u all
   ```

---

## Step 2: Static Analysis for Injection Vectors

Scan the codebase for common injection vulnerabilities.

### Actions

1. **SQL Injection:**
   ```bash
   # Search for string concatenation in SQL queries
   grep -rn "SELECT.*+.*FROM\|INSERT.*+.*INTO\|UPDATE.*+.*SET\|DELETE.*+.*FROM" \
     --include='*.ts' --include='*.js' --include='*.py' --include='*.go' src/

   # Search for template literals in SQL
   grep -rn '`SELECT\|`INSERT\|`UPDATE\|`DELETE' --include='*.ts' --include='*.js' src/

   # Search for f-strings or format strings in SQL (Python)
   grep -rn 'f"SELECT\|f"INSERT\|\.format.*SELECT\|%.*SELECT' --include='*.py' src/
   ```
   **Pass criteria:** All SQL queries use parameterized queries or prepared statements. Zero string concatenation or interpolation in SQL.

2. **Cross-Site Scripting (XSS):**
   ```bash
   # Search for dangerouslySetInnerHTML (React)
   grep -rn 'dangerouslySetInnerHTML' --include='*.tsx' --include='*.jsx' src/

   # Search for innerHTML assignments
   grep -rn 'innerHTML\s*=' --include='*.ts' --include='*.js' src/

   # Search for v-html (Vue)
   grep -rn 'v-html' --include='*.vue' src/

   # Search for unescaped template rendering
   grep -rn '{{{.*}}}' --include='*.hbs' --include='*.mustache' src/  # Handlebars/Mustache
   grep -rn '|safe\||mark_safe\||raw' --include='*.py' --include='*.html' src/  # Django/Jinja
   ```
   **Pass criteria:** Every instance of raw HTML insertion has a documented justification and is sanitized with DOMPurify or equivalent.

3. **Command Injection:**
   ```bash
   # Search for shell execution
   grep -rn 'exec(\|execSync(\|spawn(\|child_process\|os\.system\|subprocess\.call\|subprocess\.run\|subprocess\.Popen' \
     --include='*.ts' --include='*.js' --include='*.py' src/

   # Search for eval
   grep -rn 'eval(\|new Function(' --include='*.ts' --include='*.js' src/
   ```
   **Pass criteria:** Zero instances of shell execution with user-supplied input. Zero instances of `eval()` or `new Function()` with dynamic input.

4. **Path Traversal:**
   ```bash
   # Search for file system operations with dynamic paths
   grep -rn 'readFile\|writeFile\|readFileSync\|createReadStream\|fs\.\|os\.path\.join\|open(' \
     --include='*.ts' --include='*.js' --include='*.py' src/ | grep -v 'node_modules\|test\|spec'
   ```
   **Pass criteria:** All file paths are validated against a whitelist or canonicalized and verified to stay within the expected directory.

5. **NoSQL Injection (if using MongoDB or similar):**
   ```bash
   grep -rn '\.find(\|\.findOne(\|\.aggregate(\|\$where\|\$gt\|\$ne' \
     --include='*.ts' --include='*.js' src/ | grep -v 'node_modules\|test'
   ```
   **Pass criteria:** All NoSQL queries sanitize user input and do not allow operator injection via `$gt`, `$ne`, `$where`, etc.

---

## Step 3: Secrets Detection

Verify that no credentials, API keys, tokens, or secrets exist in the codebase, logs, or committed environment files.

### Actions

1. **Scan the codebase for secrets:**
   ```bash
   # Using gitleaks (if available)
   gitleaks detect --source=. --no-git --report-format=json --report-path=gitleaks-report.json

   # Using trufflehog (if available)
   trufflehog filesystem --directory=. --json > trufflehog-report.json

   # Manual pattern search
   grep -rn 'AKIA\|sk_live\|sk_test\|ghp_\|gho_\|github_pat_\|xox[bpsa]-\|hooks\.slack\.com' \
     --include='*.ts' --include='*.js' --include='*.py' --include='*.go' --include='*.env*' \
     --include='*.json' --include='*.yml' --include='*.yaml' .

   # Search for generic secret patterns
   grep -rn 'password\s*=\s*["\x27][^"\x27]\+["\x27]\|secret\s*=\s*["\x27][^"\x27]\+["\x27]\|api_key\s*=\s*["\x27][^"\x27]\+["\x27]' \
     --include='*.ts' --include='*.js' --include='*.py' --include='*.go' --include='*.env*' .
   ```

2. **Check for committed .env files:**
   ```bash
   # .env files should NEVER be committed
   find . -name '.env' -o -name '.env.local' -o -name '.env.production' -o -name '.env.staging' | grep -v node_modules
   # Verify these are in .gitignore
   cat .gitignore | grep -i '\.env'
   ```

3. **Check git history for leaked secrets (if this is a git repository):**
   ```bash
   # Using gitleaks on git history
   gitleaks detect --source=. --report-format=json --report-path=gitleaks-history-report.json

   # Manual check: search for secrets in git log
   git log --all --oneline -p | grep -i 'password\|secret\|api_key\|token' | head -20
   ```

4. **Verify environment variable usage:**
   ```bash
   # Ensure secrets are loaded from env vars, not hardcoded
   grep -rn 'process\.env\.\|os\.environ\|os\.Getenv\|env::var' \
     --include='*.ts' --include='*.js' --include='*.py' --include='*.go' src/
   ```

5. **Check log output for secret leakage:**
   ```bash
   # Ensure logging does not dump sensitive fields
   grep -rn 'console\.log\|logger\.\|log\.' --include='*.ts' --include='*.js' src/ | \
     grep -i 'password\|token\|secret\|key\|credential\|authorization'
   ```

**Pass criteria:** Zero secrets in code, zero .env files tracked by git, zero secrets in git history, zero secrets in log output.

---

## Step 4: CSP Header Verification

Verify Content Security Policy and other security headers are properly configured.

### Actions

1. **Check security headers on the running application:**
   ```bash
   curl -sI http://localhost:3000 | grep -i \
     'content-security-policy\|x-frame-options\|x-content-type-options\|strict-transport-security\|referrer-policy\|permissions-policy\|x-xss-protection'
   ```

2. **Verify each header:**

   | Header | Expected Value | Actual Value | Status |
   |--------|---------------|-------------|--------|
   | Content-Security-Policy | Restrictive policy (no `unsafe-inline`, no `unsafe-eval` without justification) | [value] | PASS/FAIL |
   | X-Frame-Options | `DENY` or `SAMEORIGIN` | [value] | PASS/FAIL |
   | X-Content-Type-Options | `nosniff` | [value] | PASS/FAIL |
   | Strict-Transport-Security | `max-age=31536000; includeSubDomains` (minimum) | [value] | PASS/FAIL |
   | Referrer-Policy | `strict-origin-when-cross-origin` or stricter | [value] | PASS/FAIL |
   | Permissions-Policy | Restrict unnecessary browser features | [value] | PASS/FAIL |

3. **CSP policy analysis (if CSP is present):**
   - Verify `default-src` is set (ideally to `'self'`).
   - Verify `script-src` does not include `'unsafe-inline'` or `'unsafe-eval'` unless justified.
   - Verify `style-src` does not include `'unsafe-inline'` unless justified (CSS-in-JS frameworks may require it).
   - Verify `img-src` is restricted to known domains.
   - Verify `connect-src` is restricted to known API domains.
   - Verify `frame-ancestors` is set to prevent clickjacking.

4. **Check for CSP report-uri or report-to:**
   - Is there a reporting endpoint configured to catch CSP violations in production?

---

## Step 5: Auth Flow Verification

Verify authentication and authorization flows against the threat model.

### Actions

1. **Authentication checks:**

   | Check | Expected | Actual | Status |
   |-------|----------|--------|--------|
   | Password hashing | bcrypt/scrypt/argon2 with appropriate cost | [implementation] | PASS/FAIL |
   | Session token generation | Cryptographically random, >= 128 bits | [implementation] | PASS/FAIL |
   | Session expiration | Configurable, reasonable default (e.g., 24h) | [value] | PASS/FAIL |
   | Refresh token rotation | New refresh token on each use, old revoked | [implementation] | PASS/FAIL |
   | Login rate limiting | Max N attempts per IP/account per time window | [implementation] | PASS/FAIL |
   | Account lockout | Temporary lockout after N failed attempts | [implementation] | PASS/FAIL |
   | Password requirements | Minimum length >= 8, complexity or blocklist | [implementation] | PASS/FAIL |

2. **Authorization checks:**

   | Check | Expected | Actual | Status |
   |-------|----------|--------|--------|
   | Route protection | All protected routes check authentication | [implementation] | PASS/FAIL |
   | Role-based access | Authorization checks on every protected endpoint | [implementation] | PASS/FAIL |
   | IDOR prevention | Resource access verified against requesting user | [implementation] | PASS/FAIL |
   | Privilege escalation | Cannot modify own role/permissions | [implementation] | PASS/FAIL |
   | API key scoping | API keys have minimum necessary permissions | [implementation] | PASS/FAIL |

3. **Test for common auth vulnerabilities:**
   ```bash
   # Check for unprotected endpoints
   grep -rn 'router\.\(get\|post\|put\|patch\|delete\)' --include='*.ts' --include='*.js' src/ | \
     grep -v 'auth\|middleware\|protect\|guard\|verify'

   # Check for JWT issues (if using JWT)
   grep -rn 'algorithm.*none\|verify.*false\|ignoreExpiration' --include='*.ts' --include='*.js' src/
   ```

---

## Step 6: CORS Configuration Check

Verify that CORS is configured restrictively.

### Actions

1. **Find the CORS configuration:**
   ```bash
   grep -rn 'cors\|Access-Control-Allow-Origin\|Access-Control-Allow-Methods\|Access-Control-Allow-Headers' \
     --include='*.ts' --include='*.js' --include='*.py' --include='*.go' src/
   ```

2. **Verify CORS settings:**

   | Setting | Dangerous Value | Expected | Actual | Status |
   |---------|----------------|----------|--------|--------|
   | Allow-Origin | `*` (in production) | Specific domain(s) | [value] | PASS/FAIL |
   | Allow-Credentials | `true` with `Origin: *` | `true` only with specific origins | [value] | PASS/FAIL |
   | Allow-Methods | All methods | Only needed methods | [value] | PASS/FAIL |
   | Allow-Headers | `*` | Only needed headers | [value] | PASS/FAIL |
   | Expose-Headers | Sensitive headers | Minimum necessary | [value] | PASS/FAIL |
   | Max-Age | Very long | Reasonable (e.g., 86400) | [value] | PASS/FAIL |

3. **Test CORS with curl:**
   ```bash
   # Preflight request from unauthorized origin
   curl -sI -X OPTIONS \
     -H "Origin: https://malicious-site.com" \
     -H "Access-Control-Request-Method: POST" \
     http://localhost:3000/api/resource

   # Should NOT return Access-Control-Allow-Origin: https://malicious-site.com
   ```

---

## Step 7: Rate Limiting Verification

Verify that rate limiting is in place on sensitive endpoints.

### Actions

1. **Identify endpoints that require rate limiting:**
   - Login / authentication endpoints.
   - Registration / account creation.
   - Password reset.
   - API endpoints accessible with API keys.
   - Any endpoint that sends emails or notifications.
   - Any endpoint that performs expensive operations.

2. **Check rate limiting configuration:**
   ```bash
   grep -rn 'rateLimit\|rate-limit\|throttle\|RateLimiter\|limiter' \
     --include='*.ts' --include='*.js' --include='*.py' --include='*.go' src/
   ```

3. **Test rate limiting:**
   ```bash
   # Send rapid requests to a rate-limited endpoint
   for i in $(seq 1 50); do
     STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/api/auth/login \
       -X POST -H "Content-Type: application/json" \
       -d '{"email":"test@test.com","password":"wrong"}')
     echo "Request $i: $STATUS"
   done
   # Should see 429 (Too Many Requests) after the limit is reached
   ```

4. **Verify rate limit headers in responses:**
   ```bash
   curl -sI http://localhost:3000/api/auth/login \
     -X POST -H "Content-Type: application/json" \
     -d '{"email":"test@test.com","password":"wrong"}' | \
     grep -i 'x-ratelimit\|retry-after'
   ```

   | Header | Expected | Actual | Status |
   |--------|----------|--------|--------|
   | X-RateLimit-Limit | Present | [value] | PASS/FAIL |
   | X-RateLimit-Remaining | Present | [value] | PASS/FAIL |
   | X-RateLimit-Reset | Present | [value] | PASS/FAIL |
   | Retry-After (on 429) | Present | [value] | PASS/FAIL |

---

## Output Format

```markdown
## Security Audit Report

**Date:** [timestamp]
**Scope:** [full audit / targeted audit — specify areas]
**Tools used:** [npm audit, gitleaks, manual review, etc.]

### Summary

| Category | Critical | High | Medium | Low | Status |
|----------|---------|------|--------|-----|--------|
| Dependency Vulnerabilities | [n] | [n] | [n] | [n] | PASS/FAIL |
| Injection Vectors | [n] | [n] | [n] | [n] | PASS/FAIL |
| Secrets Detection | [n] | [n] | [n] | [n] | PASS/FAIL |
| Security Headers | [n] | [n] | [n] | [n] | PASS/FAIL |
| Auth Flow | [n] | [n] | [n] | [n] | PASS/FAIL |
| CORS Configuration | [n] | [n] | [n] | [n] | PASS/FAIL |
| Rate Limiting | [n] | [n] | [n] | [n] | PASS/FAIL |

**Overall: PASS / FAIL**

### Critical Findings (Block Release)

| # | Category | Finding | CVE/CWE | Remediation |
|---|----------|---------|---------|-------------|
| 1 | [category] | [description] | [reference] | [fix] |

### High Findings (Block Release)

| # | Category | Finding | CVE/CWE | Remediation |
|---|----------|---------|---------|-------------|
| 1 | [category] | [description] | [reference] | [fix] |

### Medium Findings (Track as Issue)

| # | Category | Finding | CVE/CWE | Remediation |
|---|----------|---------|---------|-------------|
| 1 | [category] | [description] | [reference] | [fix] |

### Low Findings (Fix When Convenient)

| # | Category | Finding | CVE/CWE | Remediation |
|---|----------|---------|---------|-------------|
| 1 | [category] | [description] | [reference] | [fix] |
```

Every finding must include a specific remediation step. Do not report issues without solutions.
