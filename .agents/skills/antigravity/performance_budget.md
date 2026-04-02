---
description: Define performance targets across frontend, backend, and database layers. Set Core Web Vitals targets, API latency SLAs, throughput goals, caching strategy, and CDN configuration grounded in industry benchmark research.
invoked_from:
  - workflows/antigravity/07_architecture_spec.md
  - workflows/antigravity/08_testing_strategy.md
  - workflows/antigravity/10_spec_review.md
produces:
  - Frontend performance budget (bundle sizes, LCP/FID/CLS, TTI)
  - Backend API latency SLAs (p50/p95/p99)
  - Throughput targets per endpoint class
  - Database query time limits
  - Caching strategy (layers, TTLs, invalidation)
  - CDN configuration
  - Performance monitoring and alerting thresholds
browser_usage: Moderate (research industry benchmarks, Core Web Vitals thresholds, CDN provider docs)
---

# Skill: Performance Budget

Define measurable performance targets for every layer of the system. A performance budget is a contract — if a metric exceeds the budget, the feature that caused the regression does not ship until it is fixed.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `TECH_STACK.md` — for framework, bundler, hosting, and CDN choices
- `PRD.md` — for NFRs (non-functional requirements), user expectations, and scale targets
- `ARCHITECTURE.md` (partial) — for understanding the system topology

---

## Step 1: Research Industry Benchmarks

**Browser: 4-6 searches**

1. Search for "Core Web Vitals thresholds [current year]" — get current Google thresholds for good/needs improvement/poor
2. Search for "[product domain] website performance benchmarks" (e.g., "e-commerce page load benchmarks", "SaaS dashboard performance")
3. Search for "[frontend framework from TECH_STACK.md] bundle size optimization"
4. Search for "API latency SLA benchmarks [product type]"
5. Search for "[database from TECH_STACK.md] query performance benchmarks"
6. Search for "[CDN/hosting from TECH_STACK.md] performance optimization guide"

Record specific numbers from benchmarks to justify targets in subsequent steps.

---

## Step 2: Frontend Performance Budget

### Core Web Vitals Targets

| Metric | Good Threshold | Target | Measurement Point | Tool |
|--------|---------------|--------|-------------------|------|
| **LCP** (Largest Contentful Paint) | <= 2.5s | [Target] | 75th percentile | Lighthouse, CrUX |
| **FID** (First Input Delay) | <= 100ms | [Target] | 75th percentile | Lighthouse, CrUX |
| **INP** (Interaction to Next Paint) | <= 200ms | [Target] | 75th percentile | Lighthouse, CrUX |
| **CLS** (Cumulative Layout Shift) | <= 0.1 | [Target] | 75th percentile | Lighthouse, CrUX |
| **TTFB** (Time to First Byte) | <= 800ms | [Target] | 75th percentile | WebPageTest |

Targets should be set tighter than "Good" thresholds to provide margin.

### Bundle Size Budget

| Bundle | Max Size (gzipped) | Max Size (uncompressed) | Contents |
|--------|-------------------|------------------------|----------|
| Initial JS | [target] KB | [target] KB | Framework runtime, router, critical app code |
| Initial CSS | [target] KB | [target] KB | Critical styles, design tokens |
| Vendor chunk | [target] KB | [target] KB | Third-party libraries |
| Per-route chunk (avg) | [target] KB | [target] KB | Route-specific code |
| Total (all routes loaded) | [target] KB | [target] KB | Complete application |

Reference guidelines:
- Total initial JS should be under 150KB gzipped for fast TTI on 3G
- Each additional route chunk should be under 50KB gzipped
- CSS should be under 50KB gzipped total

### Loading Performance Targets

| Metric | Target | Condition |
|--------|--------|-----------|
| Time to Interactive (TTI) | [target]s | Fast 3G, mid-tier mobile device |
| First Contentful Paint (FCP) | [target]s | Fast 3G |
| Speed Index | [target]s | Fast 3G |
| Total Blocking Time (TBT) | [target]ms | Fast 3G |
| Lighthouse Performance Score | >= [target] | Mobile, simulated throttling |

### Image and Asset Budget

| Asset Type | Max Size | Format | Lazy Loading |
|-----------|---------|--------|-------------|
| Hero images | [target] KB | WebP/AVIF with JPEG fallback | No (above fold) |
| Content images | [target] KB | WebP/AVIF with JPEG fallback | Yes |
| Icons | SVG or icon font | N/A | No |
| Fonts | [target] KB per family | WOFF2 | Font-display: swap |

---

## Step 3: Backend API Performance SLAs

### Latency Targets by Endpoint Class

| Endpoint Class | p50 | p95 | p99 | Timeout | Example |
|---------------|-----|-----|-----|---------|---------|
| Health check | < 5ms | < 10ms | < 50ms | 1s | GET /health |
| Simple read (by ID) | < 20ms | < 50ms | < 100ms | 5s | GET /users/:id |
| List with pagination | < 50ms | < 150ms | < 300ms | 10s | GET /users?page=1 |
| List with filtering/search | < 100ms | < 300ms | < 500ms | 15s | GET /users?search=term |
| Simple write (create/update) | < 50ms | < 150ms | < 300ms | 10s | POST /users |
| Complex write (multi-step) | < 200ms | < 500ms | < 1000ms | 30s | POST /orders (with payment) |
| File upload | < 500ms | < 2000ms | < 5000ms | 60s | POST /files |
| Report/export generation | < 1000ms | < 5000ms | < 10000ms | 120s | GET /reports/:id/export |

### Throughput Targets

| Metric | Target | Measurement |
|--------|--------|-------------|
| Requests per second (sustained) | [target] RPS | Across all endpoints |
| Concurrent connections | [target] | Simultaneous open connections |
| Peak throughput | [target] RPS | During expected peak (define when) |

### Availability SLA

| Metric | Target |
|--------|--------|
| Uptime | 99.9% (8.76 hours downtime/year) |
| Planned maintenance window | [define] |
| Recovery Time Objective (RTO) | [target] |
| Recovery Point Objective (RPO) | [target] |

---

## Step 4: Database Performance Budget

### Query Time Limits

| Query Type | Max Execution Time | Alert Threshold | Kill Threshold |
|-----------|-------------------|-----------------|----------------|
| Simple SELECT by PK/index | < 5ms | > 10ms | > 100ms |
| JOIN (2-3 tables) | < 20ms | > 50ms | > 500ms |
| Aggregation query | < 50ms | > 200ms | > 2000ms |
| Full-text search | < 100ms | > 300ms | > 3000ms |
| Batch insert | < 100ms per 100 rows | > 500ms | > 5000ms |
| Migration (DDL) | N/A | N/A | Statement timeout exempt |

### Connection Pool Configuration

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Min connections | [value] | Keep-alive for baseline traffic |
| Max connections | [value] | Prevent database overload |
| Connection timeout | [value]ms | Fail fast if pool exhausted |
| Idle timeout | [value]ms | Reclaim unused connections |
| Max lifetime | [value]ms | Prevent stale connection issues |

### Database-Specific Targets

| Metric | Target |
|--------|--------|
| Connection count (max) | [value] (reserve headroom for admin) |
| Cache hit ratio | > 99% for frequently accessed data |
| Index hit ratio | > 95% |
| Dead tuple ratio (PostgreSQL) | < 10% per table |
| Replication lag (if applicable) | < 100ms |

---

## Step 5: Caching Strategy

### Cache Layers

```
Client (Browser Cache / Service Worker)
    |
CDN (Edge Cache)
    |
Application Cache (Redis / In-Memory)
    |
Database Query Cache
    |
Database (Buffer Pool / Shared Buffers)
```

### Cache Definitions

For each cacheable resource, define:

| Resource | Cache Layer | TTL | Invalidation Strategy | Cache Key Pattern | Stale-While-Revalidate |
|----------|-----------|-----|----------------------|-------------------|----------------------|
| Static assets (JS/CSS) | CDN + Browser | 1 year | Content-hash in filename | URL with hash | No |
| API: User profile | Application | 5 min | Write-through on update | `user:{id}` | Yes (30s) |
| API: Public listings | CDN + Application | 1 min | Time-based + event-based | `listings:{params_hash}` | Yes (10s) |
| API: Auth tokens | Application | Token TTL | Explicit revocation | `token:{jti}` | No |
| Database: Hot queries | Database layer | Query-dependent | ORM cache or manual | Query hash | N/A |

### Cache Invalidation Rules

| Trigger | Action | Scope |
|---------|--------|-------|
| Entity created/updated/deleted | Invalidate entity cache + related list caches | Specific entity + affected lists |
| Bulk import | Clear all caches for affected entity type | Entity type |
| Deploy | Invalidate CDN (static assets auto-bust via hashing) | All static assets |
| Config change | Invalidate config cache | Application-wide |

### Cache Failure Mode

| Scenario | Behavior |
|----------|----------|
| Cache unavailable (Redis down) | Bypass cache, serve from database (degraded performance, not outage) |
| Cache cold start | Lazy population on first request (accept initial latency hit) |
| Cache stampede | Use cache locks or probabilistic early recomputation |
| Stale cache after failed invalidation | TTL provides upper bound on staleness |

---

## Step 6: CDN Configuration

### CDN Setup

| Property | Value | Rationale |
|----------|-------|-----------|
| Provider | [from TECH_STACK.md] | |
| Edge locations | [auto / specific regions] | Based on user geography |
| SSL termination | CDN edge | Reduce latency |
| HTTP/2 or HTTP/3 | Enabled | Multiplexing, header compression |
| Compression | Brotli (fallback: gzip) | Smaller payloads |

### Cache Rules

| Path Pattern | Cache Behavior | TTL | Headers |
|-------------|---------------|-----|---------|
| `/_next/static/*` or `/assets/*` | Cache, immutable | 1 year | `Cache-Control: public, max-age=31536000, immutable` |
| `/api/*` | Do not cache at CDN (or short cache) | 0 or per-endpoint | `Cache-Control: private, no-cache` or per-endpoint |
| `/*.html` | Cache with revalidation | 5 min | `Cache-Control: public, max-age=0, must-revalidate` |
| `/favicon.ico`, `/robots.txt` | Cache | 1 day | `Cache-Control: public, max-age=86400` |

---

## Step 7: Performance Monitoring and Alerting

### Metrics to Collect

| Metric | Source | Collection Method | Granularity |
|--------|--------|------------------|-------------|
| Core Web Vitals (LCP, FID, CLS, INP) | Real User Monitoring (RUM) | Browser API + analytics | Per page, per device type |
| API latency (p50, p95, p99) | Application | Middleware timing | Per endpoint |
| Error rate (4xx, 5xx) | Application / Load Balancer | Log aggregation | Per endpoint |
| Database query time | Database | Slow query log + APM | Per query |
| Cache hit/miss ratio | Cache layer | Redis INFO / custom metrics | Per cache key pattern |
| CDN cache hit ratio | CDN provider | Provider dashboard/API | Per path pattern |
| Memory / CPU utilization | Infrastructure | System metrics | Per service instance |

### Alert Thresholds

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| API p95 latency | > 2x target | > 5x target | Page on-call |
| Error rate (5xx) | > 1% | > 5% | Page on-call |
| Database query time | > 500ms avg | > 2000ms avg | Investigate slow queries |
| Cache hit ratio | < 90% | < 70% | Investigate cache invalidation |
| CDN error rate | > 0.5% | > 2% | Check origin health |
| CPU utilization | > 70% sustained | > 90% sustained | Scale out |
| Memory utilization | > 80% | > 90% | Investigate leaks, scale |

---

## Step 8: Performance Testing Plan

### Test Types

| Test Type | Tool | When to Run | Pass Criteria |
|-----------|------|-------------|---------------|
| Lighthouse audit | Lighthouse CI | Every PR | Score >= [target] |
| Bundle size check | `bundlesize` or equivalent | Every PR | Under budget |
| Load test (sustained) | k6 / Artillery / Locust | Pre-release | Meet latency SLAs at target RPS |
| Stress test (peak) | k6 / Artillery / Locust | Pre-release | Graceful degradation at 2x target |
| Soak test (endurance) | k6 / Artillery / Locust | Monthly | No memory leaks, stable latency over 4+ hours |
| Database benchmark | pgbench / custom | Schema changes | Meet query time limits |

### CI/CD Integration

- Bundle size check runs on every PR — blocks merge if budget exceeded
- Lighthouse audit runs on every PR — warns if score drops, blocks if below minimum
- Load tests run in staging before every production release
- Performance regression tests compare against baseline metrics

---

## Step 9: Cross-Reference Validation

Before finalizing, verify:

- [ ] Frontend targets are achievable with the chosen framework and bundle approach
- [ ] Backend latency SLAs are realistic given the database query limits
- [ ] Caching strategy covers the highest-traffic endpoints
- [ ] CDN configuration aligns with the hosting setup in TECH_STACK.md
- [ ] Alert thresholds match the SLA targets (alerts fire before SLA breach)
- [ ] Performance testing plan covers all defined budgets
- [ ] Targets are documented as testable assertions (not vague goals)

---

## Output

The final output feeds into the Performance section of `ARCHITECTURE.md`. The performance budget serves as the reference for CI/CD quality gates, load testing configurations, and production monitoring dashboards.
