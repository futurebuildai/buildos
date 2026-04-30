# Metrics Framework

**Date:** 2026-04-02
**Pipeline Stage:** 04 - Scope & Prioritization
**Status:** PAUSED AT APPROVAL GATE 1

---

## Guiding Principles

1. **Measure what the legacy system already tracks.** CorporateFinancialsServicer, ProcurementAgent, and DailyFocusAgent already produce measurable outputs (CorporateBudget records, procurement status transitions, feed cards). Instrument these before inventing new telemetry.
2. **Builder trust is earned, not assumed.** Autonomy metrics (Tribunal approval rates, auto-order success) gate progression from Level 1 to Level 2+. No metric bypass.
3. **Determinism is binary.** The CPM physics engine either produces identical output across architectures or it does not. There is no "percentage deterministic."

---

## North Star Metrics

| Metric | Definition | Target | Timeline | Source |
|--------|-----------|--------|----------|--------|
| Admin Hours Saved | Weekly hours Sarah spends on tasks now automated (procurement tracking, AIA prep, cert monitoring, financial rollup) | 60% reduction from baseline | 12 months | Time-tracking survey pre/post deployment |
| Procurement Automation Rate | Percentage of routine material orders placed without human intervention (Tribunal Level 2+) | 80% | 18 months (Level 2 at month 10, Level 3 at month 18) | ProcurementAgent status transitions: CRITICAL -> AUTO_ORDERED / MANUAL_ORDERED |
| Schedule Prediction Accuracy | Percentage of tasks whose actual completion date falls within the CPM-predicted Early Finish +/- float window | >85% | 12 months | Compare actual_completion_date against early_finish + total_float from BackwardPass |
| Field Adoption Rate | Percentage of assigned tasks receiving a progress update via Flutter app within 24 hours of completion | 90% | 15 months | task_progress records with reported_via = 'mobile' / total completed tasks |
| Financial Visibility Latency | Time between Tom opening the dashboard and seeing current-day financial summary | <5 seconds page load; data <24 hours stale | 9 months | TypeCorporateRollup last_run timestamp vs. current time; frontend performance monitoring |

---

## Walking Skeleton Metrics (Week 1-4)

These metrics validate that the architecture works end-to-end. They are pass/fail gates, not continuous KPIs.

| Metric | Pass Criteria | Measurement |
|--------|--------------|-------------|
| JWT Auth Flow | User authenticates via The Brain JWT, receives valid session in BuildOS | Manual test: login flow completes without Clerk dependency |
| River Queue Health | daily_briefing task enqueues, executes, and completes without error | River job dashboard: 0 failed jobs over 24-hour window |
| Dashboard Render | fb-budget-summary card renders with mock CorporateBudget data in GableLBM Industrial Dark theme | Visual inspection: Deep Space background, Gable Green accents, glassmorphism card, JetBrains Mono numbers |
| Flutter Offline Read | Task list displays cached data when device is in airplane mode | Manual test: load tasks with connectivity, disconnect, verify list still displays |
| CI/CD Pipeline | GitHub Actions builds Go backend, Lit frontend, and Flutter app without failure | Green pipeline on main branch for 3 consecutive commits |

---

## MVP Feature Metrics (Week 5-20)

### M1: Corporate Financials Dashboard

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Dashboard Load Time (p95) | <3 seconds for 10-project portfolio | Frontend performance API: navigation to fb-financials-view contentful paint |
| Data Freshness | CorporateBudget data <24 hours old | last_rollup_at timestamp on /api/v1/org/{orgID}/financials/summary response |
| Budget Variance Accuracy | Committed vs Actual within 1% of QuickBooks GL | Monthly reconciliation: CorporateBudget.TotalActualCostCents vs. QuickBooks trial balance |
| AR Aging Accuracy | Aging buckets match manual invoice review | Quarterly audit: ARAgingSnapshot buckets vs. manual categorization of INVOICES table |
| Tom's Review Time | <10 minutes for full portfolio review | User observation session (Journey 3 from USER_JOURNEYS.md) |

### M2: CPM-res1.0 Preservation + API

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Determinism Guarantee | 100% bit-identical output across amd64/arm64 | cpm_determinism_test.go golden master suite: zero failures across CI matrix |
| API Response Time (p95) | <2 seconds for 100-task project recalculation | /api/v1/projects/{id}/schedule/recalculate latency histogram |
| Critical Path Accuracy | Matches manual CPM calculation for 5 reference projects | Validation against hand-computed critical paths using known WBS templates |
| Golden Master Regression | Zero test modifications allowed | CI gate: cpm_determinism_test.go must pass unmodified from reference-vault |

#### Physics Engine Performance Budgets (L8 Standard)

These are **computation-only** targets for the deterministic physics engine, measured via Go benchmark tests integrated into `make audit`. These budgets prevent performance regressions during the River migration and ensure the dashboard remains reactive during schedule recalculations.

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| CPM ForwardPass + BackwardPass (80-task residential graph) | **<200ms** | `BenchmarkCPM80Tasks` in `cpm_test.go` — hard gate in `make audit` |
| CPM ForwardPass + BackwardPass (200-task multi-phase graph) | <500ms | `BenchmarkCPM200Tasks` in `cpm_test.go` — hard gate in `make audit` |
| DHSM Duration Calculation (per task) | <1ms | `BenchmarkDHSMPerTask` in `dhsm_test.go` |
| SWIM Weather Adjustment (per task, incl. cache lookup) | <5ms | `BenchmarkSWIMPerTask` in `swim_test.go` |
| Full Schedule Recalculation (80-task, end-to-end API) | <800ms | Prometheus histogram on `/api/v1/projects/{id}/schedule/recalculate` |
| Scoping + DependencyGraph Construction (80-task) | <50ms | `BenchmarkGraphConstruction` in `cpm_test.go` |

**CI Integration:** `make audit` must run `go test -bench=BenchmarkCPM -benchtime=10x ./internal/physics/...` and fail if any benchmark exceeds its budget. This is a hard gate — no exemptions during River migration.

### M3: SWIM v2 (Tomorrow.io)

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Forecast Cache Hit Rate | >80% of weather lookups served from cache | Cache hit/miss counter on tomorrow_client.go |
| API Rate Limit Compliance | <500 calls/day (free tier) | Daily call counter with circuit breaker at 450 |
| Weather Prediction Accuracy | >75% of delay predictions confirmed within 5-day window | Compare predicted weather risk score against actual delays reported on weather-sensitive tasks (WBS < 10.0 or 13.x) |
| Fallback Activation Rate | <5% of requests fall back to legacy static multipliers | Counter on ApplyWeatherAdjustment fallback path |
| Briefing Weather Relevance | Superintendents rate weather section of briefing as "useful" >80% of time | In-app feedback button on DailyFocusAgent briefing card |

### M4: Agent Morning Briefing (Flutter)

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Push Notification Delivery | >95% delivery rate via FCM | Firebase Cloud Messaging delivery receipts |
| Briefing Consumption Time | <5 minutes average | App session duration: notification_opened to last_card_dismissed |
| Briefing Consumption Rate | >85% of delivered briefings opened before 7 AM local | notification_opened timestamp vs. delivery timestamp |
| Offline Availability | 100% of last-cached briefing accessible without connectivity | Automated test: load briefing, kill network, verify display |
| Card Action Rate | >60% of cards receive an action (not just dismissed) | card_action_taken / total_cards_delivered per briefing |

### M5: Procurement Agent + Tribunal Level 1

| Metric | Target | Measurement Method |
|--------|--------|--------------------|
| Critical Item Detection Rate | 100% of items with MustOrderDate < today flagged as CRITICAL | Audit: procurement_items WHERE must_order_date < NOW() AND status != 'CRITICAL' should return 0 |
| Tribunal Recommendation Accuracy | >90% of Tribunal recommendations match what human would have decided | Monthly review: sample 20 Tribunal decisions, have Sarah/Tom evaluate correctness |
| False Positive Rate | <10% of CRITICAL alerts are spurious | Track alerts dismissed without action / total CRITICAL alerts |
| Notification Dampening | Zero duplicate notifications within 72-hour window | hasRecentCommunication() audit: no duplicate feed cards for same item within window |
| Recommendation-to-Action Time | <4 hours from CRITICAL detection to human action | Timestamp delta: procurement_item.status_changed_at (CRITICAL) to card_action_taken_at |

---

## Post-MVP Phase Metrics

### Phase 2: Operations (Week 21-32)

| Feature | Key Metric | Target |
|---------|-----------|--------|
| M6: Fleet Management | Equipment double-booking rate | 0% (CheckEquipmentAvailability prevents conflicts) |
| M6: Fleet Management | Equipment utilization rate | >70% across fleet |
| M7: HR Certification | Work stoppages due to expired certs | 0 per quarter |
| M7: HR Certification | Alert-to-renewal time | <7 days from 30-day alert to renewal action |
| M9: Resource Leveling | Schedule compression from leveling | 5-15% reduction in total project duration |
| M9: Resource Leveling | Leveling computation time | <5 seconds for 100-task single-project graph |

### Phase 3: Billing & Compliance (Week 33-40)

| Feature | Key Metric | Target |
|---------|-----------|--------|
| M8: AIA Billing | G702/G703 preparation time | <30 minutes per project (from current ~4 hours) |
| M8: AIA Billing | PDF accuracy vs. manual preparation | >98% field-match rate |
| GL Sync | Sync lag to QuickBooks | <24 hours |
| GL Sync | Reconciliation discrepancy rate | <0.5% of total transactions |

### Phase 4: Advanced AI (Week 41-52)

| Feature | Key Metric | Target |
|---------|-----------|--------|
| M10: A2A Agent Cards | The Brain successful skill invocations | >95% success rate |
| Tribunal Level 2 | Auto-order approval rate (owner confirms within 24h) | >90% |
| Tribunal Level 2 | Auto-order error rate (orders that needed reversal) | <2% |
| SWIM v2 Calibration | Prediction accuracy improvement with historical data | >85% (up from 75% target) |

---

## Technical Health Metrics

These are operational metrics tracked continuously, not tied to specific features.

| Category | Metric | Target | Alert Threshold |
|----------|--------|--------|----------------|
| **Availability** | API uptime (5xx rate) | >99.5% | >0.5% 5xx in 5-minute window |
| **Latency** | API p95 response time | <500ms (read), <2s (write/compute) | p95 > 1s (read), >5s (write) |
| **Queue Health** | River job failure rate | <1% | >5% failure rate in 1-hour window |
| **Queue Health** | River job queue depth | <100 pending | >500 pending jobs |
| **Database** | PostgreSQL connection pool utilization | <70% | >85% pool utilization |
| **Database** | Slow query rate (>1s) | <5 per hour | >20 slow queries in 1-hour window |
| **Frontend** | Largest Contentful Paint (LCP) | <2.5 seconds | >4 seconds |
| **Frontend** | Cumulative Layout Shift (CLS) | <0.1 | >0.25 |
| **Mobile** | Flutter app crash rate | <0.5% of sessions | >2% crash rate |
| **Mobile** | Offline sync success rate | >99% | <95% sync success |
| **Security** | JWT validation failure rate | <0.1% (legitimate traffic) | >1% failure rate (possible attack) |
| **Agents** | DailyFocusAgent completion time | <5 minutes for 50 projects | >15 minutes |
| **Agents** | ProcurementAgent scan cycle time | <3 minutes | >10 minutes |

---

## Measurement Infrastructure

### Data Collection

| Source | Collection Method | Storage |
|--------|------------------|---------|
| API latency | OpenTelemetry spans on chi middleware | Prometheus + Grafana |
| River job metrics | River built-in observability (job states, durations) | PostgreSQL river_job table + Grafana |
| Frontend performance | Web Vitals API (LCP, CLS, FID) via Lit component lifecycle | Sentry Performance |
| Flutter app metrics | Firebase Analytics + Crashlytics | Firebase Console |
| Business metrics (procurement, financial) | Custom PostgreSQL queries on existing tables | Grafana dashboards with pg_stat queries |
| User satisfaction | In-app feedback mechanism (thumbs up/down on feed cards) | feedback table in PostgreSQL |

### Dashboards

| Dashboard | Audience | Refresh | Key Widgets |
|-----------|----------|---------|-------------|
| System Health | Engineering | Real-time | API latency heatmap, River queue depth, error rate sparklines |
| Agent Performance | Engineering + Product | Hourly | Briefing generation time, procurement scan results, sub confirmation rates |
| Business KPIs | Product + Leadership | Daily | Admin hours saved trend, procurement automation rate, field adoption funnel |
| Financial Accuracy | Finance + Product | Weekly | Budget variance by project, AR aging accuracy, GL sync reconciliation |

### Baseline Measurement

Before any feature deployment, establish baselines for:

1. **Sarah's weekly hours** on: procurement tracking, AIA billing, cert monitoring, financial consolidation (time diary for 4 weeks)
2. **Tom's financial review time** per Monday morning session (observation + stopwatch for 4 weeks)
3. **Mike's daily phone calls** to subs for confirmation (call log review for 4 weeks)
4. **Material delay incidents** per quarter (retrospective analysis of last 12 months)
5. **Schedule prediction accuracy** of legacy system (compare CPM output vs. actual for 10 completed projects)

---

## Graduation Criteria

Metrics that gate progression between system autonomy levels (per Concept 6 from SOLUTION_CANDIDATES.md):

| Transition | Required Metrics | Minimum Duration |
|-----------|-----------------|-----------------|
| Level 0 -> Level 1 (Recommend) | System deployed, Tribunal operational, >50 decisions logged | Walking skeleton complete |
| Level 1 -> Level 2 (Auto with approval) | Tribunal accuracy >90% over 100+ decisions, zero false positives on orders >$1000, legal review complete | 30 days at Level 1 |
| Level 2 -> Level 3 (Fully autonomous) | Auto-order error rate <2% over 200+ orders, owner opt-in consent, insurance carrier approval | 90 days at Level 2 |

---

## Assumptions Validation via Metrics

Cross-reference with ASSUMPTIONS_REGISTER.md:

| Assumption | Validating Metric | Pass/Fail Threshold |
|-----------|-------------------|-------------------|
| A3: SWIM multipliers accurate | SWIM v2 weather prediction accuracy | >75% delay prediction accuracy |
| A6: PostgreSQL handles scale | Database connection pool utilization, slow query rate | Pool <70%, slow queries <5/hour at 50 projects |
| A7: Asynq/River viable | River job failure rate, queue depth | <1% failure, <100 pending |
| A8: Lit handles data-dense tables | Dashboard LCP, CLS with 1000+ row table | LCP <2.5s, CLS <0.1 |
| A9: Flutter offline-first reliable | Offline sync success rate | >99% sync success |
| A10: Market adopts AI scheduling | Field adoption rate, briefing consumption rate | >85% briefing consumption, >90% daily log completion |
| A13: Autonomous procurement viable | Tribunal accuracy, auto-order error rate | >90% accuracy, <2% error rate |
