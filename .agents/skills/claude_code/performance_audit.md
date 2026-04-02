---
description: Run and interpret performance measurements against the project's performance budget. Covers frontend bundle analysis, Lighthouse audits, load test interpretation, database query plan analysis, and API response time measurement. Flags any metric exceeding the defined budget.
invoked_from:
  - workflows/claude_code/04_quality_gates.md
  - workflows/claude_code/06_deploy_verify.md
  - When performance regression is suspected
  - Before any production release
produces:
  - Performance audit report with pass/fail per budget category
  - Bundle size breakdown with comparison to budget
  - Lighthouse scores with delta from previous run
  - Database query plan analysis with flagged slow queries
  - API response time measurements against SLA targets
---

# Skill: Performance Audit

Run measurable performance checks against the budgets defined in the project's performance specification. Every metric either passes or fails — there are no soft warnings. If a metric exceeds the budget, the audit fails and the issue must be resolved before proceeding.

---

## Prerequisites

Before running this audit, ensure the following exist:

- `ARCHITECTURE.md` or a performance budget document — for target thresholds
- `TECH_STACK.md` — for tooling, framework, and infrastructure choices
- A buildable, runnable application (or the specific component under test)
- Access to the database (for query plan analysis)

If no performance budget document exists, use the industry-standard defaults specified in each section below and flag the absence of a project-specific budget in the report.

---

## Step 1: Frontend Bundle Analysis

Measure the JavaScript and CSS bundle sizes and compare against the budget.

### Actions

1. **Build the production bundle:**
   ```bash
   # Adapt to the project's build system from TECH_STACK.md
   npm run build        # Node.js / Next.js / Vite
   yarn build           # Yarn-based projects
   pnpm build           # pnpm-based projects
   ```

2. **Analyze bundle sizes:**
   - **Webpack:** `npx webpack-bundle-analyzer stats.json` (generate stats with `--json > stats.json`)
   - **Vite/Rollup:** `npx rollup-plugin-visualizer` or check build output
   - **Next.js:** Check `.next/` build output; Next.js prints bundle sizes after build
   - **Generic:** Use `du -sh` on the output directory and `gzip -k` to measure compressed sizes

3. **Measure specific bundles:**
   ```bash
   # List all JS files in the build output with sizes
   find dist/ -name '*.js' -exec ls -lh {} \;
   # Measure gzipped sizes
   find dist/ -name '*.js' -exec sh -c 'echo "$(gzip -c "$1" | wc -c) $1"' _ {} \;
   ```

4. **Compare against budget:**

   | Bundle | Budget (gzipped) | Actual (gzipped) | Status |
   |--------|-----------------|------------------|--------|
   | Initial JS | Per budget doc or <= 150 KB | [measured] | PASS/FAIL |
   | Initial CSS | Per budget doc or <= 50 KB | [measured] | PASS/FAIL |
   | Vendor chunk | Per budget doc or <= 100 KB | [measured] | PASS/FAIL |
   | Largest route chunk | Per budget doc or <= 50 KB | [measured] | PASS/FAIL |
   | Total application | Per budget doc or <= 400 KB | [measured] | PASS/FAIL |

5. **Identify the largest dependencies:**
   - List the top 10 largest packages in the bundle.
   - Flag any single dependency exceeding 50 KB gzipped.
   - Suggest lighter alternatives if available.

### Tree-Shaking Verification

1. Check that unused exports from large libraries are not included:
   ```bash
   # Look for barrel imports that defeat tree-shaking
   grep -rn "import .* from '[^']*'" --include='*.ts' --include='*.tsx' src/ | grep -v "import {" | head -20
   ```
2. Verify that dynamic imports are used for route-level code splitting.

---

## Step 2: Lighthouse Audit

Run a Lighthouse audit and compare scores against the budget.

### Actions

1. **Run Lighthouse from CLI:**
   ```bash
   # Ensure the application is running locally or use a deployed URL
   npx lighthouse http://localhost:3000 \
     --output=json \
     --output-path=./lighthouse-report.json \
     --chrome-flags="--headless --no-sandbox" \
     --only-categories=performance
   ```

2. **Extract key metrics from the report:**
   ```bash
   # Parse the JSON report
   cat lighthouse-report.json | python3 -c "
   import json, sys
   data = json.load(sys.stdin)
   audits = data['audits']
   print(f'Performance Score: {data[\"categories\"][\"performance\"][\"score\"] * 100}')
   print(f'LCP: {audits[\"largest-contentful-paint\"][\"numericValue\"]:.0f}ms')
   print(f'FID/TBT: {audits[\"total-blocking-time\"][\"numericValue\"]:.0f}ms')
   print(f'CLS: {audits[\"cumulative-layout-shift\"][\"numericValue\"]:.4f}')
   print(f'FCP: {audits[\"first-contentful-paint\"][\"numericValue\"]:.0f}ms')
   print(f'Speed Index: {audits[\"speed-index\"][\"numericValue\"]:.0f}ms')
   print(f'TTI: {audits[\"interactive\"][\"numericValue\"]:.0f}ms')
   "
   ```

3. **Compare against budget:**

   | Metric | Budget | Actual | Status |
   |--------|--------|--------|--------|
   | Performance Score | >= 90 (or per budget) | [measured] | PASS/FAIL |
   | LCP | <= 2500ms (or per budget) | [measured] | PASS/FAIL |
   | TBT | <= 200ms (or per budget) | [measured] | PASS/FAIL |
   | CLS | <= 0.1 (or per budget) | [measured] | PASS/FAIL |
   | FCP | <= 1800ms (or per budget) | [measured] | PASS/FAIL |
   | Speed Index | <= 3400ms (or per budget) | [measured] | PASS/FAIL |
   | TTI | <= 3800ms (or per budget) | [measured] | PASS/FAIL |

4. **Review Lighthouse opportunities and diagnostics:**
   - List every item in the "Opportunities" section with estimated savings.
   - List every item in the "Diagnostics" section.
   - Prioritize by impact (largest savings first).

---

## Step 3: Load Test Interpretation

If load test results exist, interpret them against the performance budget.

### Actions

1. **Check for existing load test results:**
   ```bash
   # Look for k6, Artillery, or Locust output
   find . -name '*.json' -path '*/load-test*' -o -name '*.json' -path '*/k6*' -o -name '*artillery*report*' 2>/dev/null
   ```

2. **If load test tooling exists, run it:**
   ```bash
   # k6
   k6 run load-test.js --out json=load-test-results.json

   # Artillery
   npx artillery run load-test.yml --output load-test-results.json

   # Locust (start and stop manually or via script)
   ```

3. **Extract and compare key metrics:**

   | Metric | Budget | Actual | Status |
   |--------|--------|--------|--------|
   | p50 latency | Per endpoint class in budget | [measured] | PASS/FAIL |
   | p95 latency | Per endpoint class in budget | [measured] | PASS/FAIL |
   | p99 latency | Per endpoint class in budget | [measured] | PASS/FAIL |
   | Error rate | < 1% | [measured] | PASS/FAIL |
   | Throughput (RPS) | Per budget | [measured] | PASS/FAIL |

4. **Flag degradation patterns:**
   - Does latency increase linearly with load or exponentially? Exponential indicates a bottleneck.
   - Are there specific endpoints with disproportionately high latency?
   - Does error rate spike at a specific load level?

---

## Step 4: Database Query Plan Analysis

Analyze slow or critical database queries using EXPLAIN ANALYZE.

### Actions

1. **Identify critical queries:**
   - Authentication and authorization queries.
   - Queries on high-traffic endpoints.
   - Queries involving JOINs across 3+ tables.
   - Queries with ORDER BY, GROUP BY, or subselects.
   - Any query mentioned in the performance budget as needing optimization.

2. **Run EXPLAIN ANALYZE on each critical query:**
   ```sql
   EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
   SELECT ... FROM ... WHERE ...;
   ```

3. **For each query, check:**

   | Check | Pass Criteria | Failure Indicator |
   |-------|--------------|-------------------|
   | Seq Scan on large tables | Not present (use index) | `Seq Scan` on table with > 10K rows |
   | Index usage | Index Scan or Index Only Scan | `Seq Scan` where index exists |
   | Execution time | Within budget per query class | Exceeds budget threshold |
   | Rows estimate vs actual | Within 10x of each other | `rows=1` but `actual rows=100000` (stale stats) |
   | Nested loops on large sets | Avoided or Hash/Merge Join used | `Nested Loop` with large inner relation |
   | Buffer hits vs reads | Hit ratio > 95% | Excessive `read` vs `hit` in Buffers |

4. **Check for missing indexes:**
   ```sql
   -- PostgreSQL: Find sequential scans that should use indexes
   SELECT relname, seq_scan, seq_tup_read, idx_scan, idx_tup_fetch
   FROM pg_stat_user_tables
   WHERE seq_scan > 100 AND idx_scan = 0
   ORDER BY seq_tup_read DESC;
   ```

5. **Check for N+1 query patterns:**
   - Review ORM-generated queries for loops that issue one query per iteration.
   - Use query logging to count queries per request on critical endpoints.

---

## Step 5: API Response Time Measurement

Measure actual API response times and compare against SLA targets.

### Actions

1. **Measure response times for each endpoint class:**
   ```bash
   # Simple read endpoint
   curl -o /dev/null -s -w "time_total: %{time_total}s\ntime_starttransfer: %{time_starttransfer}s\n" \
     http://localhost:3000/api/resource/1

   # List endpoint with pagination
   curl -o /dev/null -s -w "time_total: %{time_total}s\n" \
     http://localhost:3000/api/resources?page=1&limit=20

   # Write endpoint
   curl -o /dev/null -s -w "time_total: %{time_total}s\n" \
     -X POST -H "Content-Type: application/json" \
     -d '{"name": "test"}' \
     http://localhost:3000/api/resources
   ```

2. **Run multiple iterations for statistical significance:**
   ```bash
   # Run 100 requests and extract timing data
   for i in $(seq 1 100); do
     curl -o /dev/null -s -w "%{time_total}\n" http://localhost:3000/api/resource/1
   done | sort -n | awk '
     { a[NR] = $1; sum += $1 }
     END {
       print "p50:", a[int(NR*0.50)]
       print "p95:", a[int(NR*0.95)]
       print "p99:", a[int(NR*0.99)]
       print "avg:", sum/NR
     }
   '
   ```

3. **Compare against budget:**

   | Endpoint Class | p50 Budget | p50 Actual | p95 Budget | p95 Actual | Status |
   |---------------|-----------|-----------|-----------|-----------|--------|
   | Health check | < 5ms | [measured] | < 10ms | [measured] | PASS/FAIL |
   | Simple read | < 20ms | [measured] | < 50ms | [measured] | PASS/FAIL |
   | List with pagination | < 50ms | [measured] | < 150ms | [measured] | PASS/FAIL |
   | Simple write | < 50ms | [measured] | < 150ms | [measured] | PASS/FAIL |
   | Complex write | < 200ms | [measured] | < 500ms | [measured] | PASS/FAIL |

4. **Check for response size issues:**
   - Are responses over-fetching data? Compare response payload size against what the client actually needs.
   - Is pagination working correctly? Are unbounded list queries possible?
   - Are responses compressed (gzip/brotli)?

---

## Output Format

```markdown
## Performance Audit Report

**Date:** [timestamp]
**Environment:** [local / staging / production]
**Application version:** [commit hash or version]

### Summary

| Category | Status | Critical Issues |
|----------|--------|----------------|
| Bundle Size | PASS/FAIL | [count] |
| Lighthouse | PASS/FAIL | [count] |
| Load Test | PASS/FAIL/SKIPPED | [count] |
| Database Queries | PASS/FAIL | [count] |
| API Response Time | PASS/FAIL | [count] |

**Overall: PASS / FAIL**

### Detailed Results

[Include the tables from each step above]

### Issues Requiring Immediate Attention

1. [CRITICAL] [Category] [Description] — exceeds budget by [X]%
2. [WARNING] [Category] [Description] — approaching budget threshold

### Recommendations

1. [Action item with estimated impact]
2. [Action item with estimated impact]
```

Every metric that exceeds the budget must have a corresponding action item. Do not report failures without recommending a fix.
