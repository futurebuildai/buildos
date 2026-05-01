# ADR-002: BuildOS deployment is single-tenant per customer fork

**Status:** ACCEPTED — owner-confirmed 2026-05-01.
**Date:** 2026-05-01
**Author:** Claude (codifying owner direction)
**Supersedes:** Implicit assumption in earlier production planning that BuildOS would need RLS for multi-tenant isolation.

---

## Decision

**BuildOS is deployed as a single-tenant, multi-user system. Each builder customer gets their own forked repo and their own deployment instance.** The Brain is the multi-tenant central service that all forks connect back to for OIDC, Maestro AI, billing, and 3rd-party API proxying.

A future "co-op variant" — multi-tenant within one BuildOS deployment for member organizations — is documented as a possibility but is **not** part of the canonical deployment model and will not drive any architectural decisions in the current phase.

## What this means concretely

| Concern | How it's solved |
|---|---|
| **Tenant isolation** | Deployment isolation. Each customer's data sits in its own database, in its own deployment, with its own credentials. There is one `org_id` per deployment. |
| **Application-layer access control** | Per-user RBAC via JWT role claims (`owner` / `admin` / `superintendent` / `field_worker`). Already implemented. |
| **Defense in depth at the DB layer** | NOT pursued via RLS. The threat model that RLS addresses (one tenant exfiltrating another) does not exist when each tenant is its own deployment. SQL injection in a fork can only leak that fork's data, which the customer already owns. |
| **Customer data sovereignty** | Customer owns their fork repo + their database. The Brain holds operational metadata (audit logs of cross-product orchestrations, AI usage metering) but never the customer's project / financial / personnel data. |
| **Per-customer customization** | Customer can fork and add their own branches; "core BuildOS" releases land via upstream merges. The customization boundary is in the fork's own commit history, not in a runtime plugin system. |

## What we are NOT doing (and why)

### NOT enabling Postgres RLS on tenant tables
Multi-tenant SaaS uses RLS as a hard guarantee that a service-layer bug can't return cross-tenant rows. We are not multi-tenant SaaS. RLS would add a ~3–10% query overhead (policy evaluation per row, blocked index pushdowns in some cases) for zero security benefit in a single-tenant deployment. If the co-op variant ever lands, RLS is the right tool for that variant — not retroactively for the fork model.

### NOT building per-tenant rate limiting
Per-IP rate limiting (D5, shipped) handles DDoS / runaway scripts. There is no second tenant whose budget needs protecting from the first.

### NOT building multi-region data residency in the application
Each fork picks its deployment region at provisioning time. The application is region-agnostic; the deployment is region-specific. No application-level routing logic.

### NOT cross-tenant query patterns
Anywhere we have `org_id` on a table, it's because The Brain side of A2A events identifies which org the event is for (relevant for the future co-op variant + for clean data export). Service-layer queries can rely on `DefaultOrgID` from config; no need to plumb tenant context through every call chain. (We do plumb it today for code that's intended to also work in the co-op variant later, but that's belt-and-suspenders, not a hard requirement.)

## What this DOES change in the production plan

### Drops from the critical path:
- **Phase E (RLS)** — removed. Replaced by ADR + brief docs note.

### Promoted to first-class:
- **Phase F (deployment pipeline)** — every customer fork needs a production-grade build + deploy story. This is the unique-to-BuildOS shape that justifies investment beyond a typical SaaS Dockerfile.
- **Fork lifecycle / release engineering** — new phase. Covers: how a new customer gets a fork provisioned (template repo + Brain registration + JWKS keypair generation), how upstream `buildos` releases propagate to existing forks (automated merges with conflict detection), how to keep core code mergeable without forcing customers to rewrite every release.
- **Per-deployment secret management** — Vault / AWS Secrets Manager / GCP Secret Manager integration, scoped to one deployment's secrets. Includes A2A signing key generation per fork.
- **Per-deployment backup + DR** — each customer's fork is independently backed up, with documented RPO/RTO. No shared restore drill across forks.
- **Per-deployment compliance posture** — SOC 2 / GDPR / DPA artifacts are scoped per fork. The Brain has its own compliance posture for the metering/orchestration data it holds.

### Stays the same:
- **Phase B (Brain integration)** — every fork talks to The Brain the same way.
- **Phase C (frontends)** — Lit web + Flutter mobile, with theming hooks so each fork can brand its UI.
- **Phase D (production hardening)** — applies per-deployment.
- **Phase G (compliance)** — applies per-deployment.
- **Phase H (pre-launch)** — applies per-deployment.

## Reversibility

Single-tenant → multi-tenant is a one-way migration of significant cost (RLS retrofit, ID rewrites, audit log re-classification, compliance scope expansion). Multi-tenant → single-tenant is a refactor that throws away work but doesn't break correctness.

The fork model is the more conservative starting position: it keeps the architecture simpler and forecloses fewer options. If the co-op variant ever gains traction we add RLS + per-tenant routing on the co-op deployment specifically; existing customer forks stay single-tenant.

## Implements

1. **PRODUCTION_READINESS_PLAN.md update** — drop Phase E from the critical path; promote Phase F + a new fork-lifecycle phase.
2. **CLAUDE.md** — already documents the model; add a pointer to this ADR for the architectural rationale.
3. **No code change** in this PR — this is the decision document. Implementation follows in Phase F.
