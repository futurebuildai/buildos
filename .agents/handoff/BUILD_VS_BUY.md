# Build vs Buy Analysis

> **⚠️ HISTORICAL (pre-pivot, 2026-04-02).** Evaluated build-vs-buy for OIDC, AI,
> credential vault, A2A, and billing under the assumption that "The Brain" was a
> buy/reuse option. The **2026-06-01 standalone pivot** ([ESCALATION_LOG.md](./ESCALATION_LOG.md)
> ESC-001) decided to **BUILD all natively** (native auth, native Anthropic client,
> encrypted BYOK vault). Kept as the record of the analysis that informed that call.
> Current model: [../../CLAUDE.md](../../CLAUDE.md).

**Date:** 2026-04-02
**Pipeline Stage:** 03 - Solution Design
**Status:** COMPLETE

---

## Decision Framework

| Capability | Decision | Rationale |
|-----------|----------|-----------|
| **CPM Scheduling Engine** | BUILD (preserve + extend) | Crown jewel. No open-source Go CPM exists. gonum DependencyGraph is battle-tested. Resource leveling is the evolution, not replacement. |
| **SWIM Weather Model** | BUILD + BUY API | Build the prediction logic in Go. Buy weather data from Tomorrow.io API ($0 free tier, enterprise for production). Preserve fallback to legacy static multipliers. |
| **Weather Data Source** | BUY (Tomorrow.io) | 500+ parameters, hourly forecasts, 14-day horizon. Free tier sufficient for MVP. No value in building a weather prediction model. |
| **Dashboard UI** | BUILD (Lit Web Components) | GableLBM design system is proprietary. Lit/Vite stack is committed. No suitable dark Industrial dashboard library exists for Lit. |
| **Design Token System** | BUILD | GableLBM tokens are unique to FutureBuild. CSS custom properties with Lit adoption layer. |
| **Background Job Queue** | BUY (River, open source) | Migrate from Asynq (Redis) to River (PostgreSQL). Eliminates Redis dependency. Transactional guarantees align with "database is source of truth." |
| **Autonomous Agents** | BUILD | Core differentiator. DailyFocus/Procurement/SubLiaison patterns are proven. A2A protocol compatibility is additive. |
| **Tribunal AI Engine** | BUILD | Multi-model consensus is unique. ConsensusEngine with Claude + Gemini jury is proprietary architecture. |
| **AIA Billing (G702/G703)** | BUILD | PDF generation from existing budget data. Template-based with Go html/template + headless Chrome. Small scope, high value. |
| **Payroll Processing** | BUY (integrate) | Regulatory complexity (Davis-Bacon, certified payroll, prevailing wage, multi-state tax). Integrate with Hammr, eBacon, or similar via API. Never build payroll. |
| **Time Tracking** | BUILD | Geolocation-based, feeds into job costing. Flutter mobile integration required. Simple enough to own. |
| **Certification Tracking** | BUILD | Already have EmployeeServicer.GetExpiringCertifications and TypeCertificationAlerts cron. UI layer needed. |
| **Fleet GPS/Telematics** | BUY (integrate) | Hardware integration with GPS devices is not core. Integrate with Samsara API for location/usage data. Build allocation and scheduling in-house. |
| **Fleet Management Logic** | BUILD | Equipment allocation, availability checking, maintenance scheduling. FleetServicer interface exists. Links to CPM equipment constraints. |
| **Document Search (RAG)** | BUILD (preserve) | pgvector with document_chunks table is already working. Expand embedding coverage. |
| **Invoice Extraction** | BUILD (preserve) | InvoiceServicer with Gemini Flash extraction is production-deployed. Improve accuracy iteratively. |
| **Mobile Field App** | BUILD (Flutter) | Offline-first requirement eliminates web-only solutions. Flutter provides cross-platform with native performance. |
| **Authentication** | BUY (delegate to The Brain) | The Brain is the OIDC provider per TECH_STACK.md. BuildOS validates JWTs only. |
| **Email/SMS** | BUY (SendGrid + Twilio) | Already integrated via NotificationServicer. No reason to change. |
| **Object Storage** | BUY (DigitalOcean Spaces / MinIO) | S3-compatible, already configured. No change needed. |
| **A2A Protocol** | ADOPT (open standard) | Google A2A protocol (Linux Foundation). Implement client/server in Go. Do not build proprietary agent communication. |
| **GL Sync (QuickBooks)** | BUILD integration | QuickBooks API (OAuth2) for GL export. Build the sync logic, buy nothing. GLSyncLog model exists. |

---

## Cost Implications

### BUILD Costs (Development)
| Component | Estimated Effort | Team |
|-----------|-----------------|------|
| CPM Resource Leveling | 6-8 weeks | 1 senior Go developer |
| SWIM v2 + Tomorrow.io | 8-12 weeks | 1 Go developer + API integration |
| Industrial Dark Dashboard | 12-16 weeks | 1-2 frontend developers (Lit/TS) |
| A2A Agent Protocol | 6-8 weeks | 1 Go developer |
| Corporate Financials Module | 8-10 weeks | 1 Go + 1 frontend developer |
| HR Module (excl. payroll) | 6-8 weeks | 1 Go + 1 frontend developer |
| Fleet Module | 6-8 weeks | 1 Go + 1 frontend developer |
| Tribunal Procurement | 8-12 weeks | 1 senior Go developer |
| Flutter Field App | 16-20 weeks | 1 Flutter developer |
| AIA Billing | 4-6 weeks | 1 Go developer |

### BUY/INTEGRATE Costs (Annual)
| Service | Estimated Annual Cost | Notes |
|---------|---------------------|-------|
| Tomorrow.io API | $0-$5,000 | Free tier for MVP, Enterprise for production |
| Samsara Telematics API | $3,000-$10,000 | Per-vehicle pricing, API access tier |
| Hammr Payroll Integration | $0 (API) | Customer pays Hammr directly |
| SendGrid | $500-$2,000 | Current, no change |
| Twilio SMS | $1,000-$5,000 | Current, no change |
| DigitalOcean/Railway | $5,000-$15,000 | Current hosting, may scale |

---

## Risk Assessment by Decision

| Decision | Risk if Wrong | Mitigation |
|----------|--------------|-----------|
| BUILD CPM engine | None -- already production-proven | Golden master tests prevent regression |
| BUY Tomorrow.io | API deprecation or pricing change | Maintain legacy SWIM multipliers as fallback |
| BUILD dashboard | Longer time-to-market vs buying template | GableLBM tokens reduce design decisions; Lit ecosystem has proven scale |
| BUY River queue | Library abandonment | River has strong community (GitHub stars, active maintainers); PostgreSQL-native reduces risk |
| BUY payroll integration | Vendor lock-in | Abstract behind EmployeeServicer interface; swap vendors without code changes |
| ADOPT A2A protocol | Protocol instability (v0.3) | Maintain parallel cron triggers; A2A is additive, not replacement |
