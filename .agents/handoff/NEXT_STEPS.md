# NEXT_STEPS.md — prioritized backlog with entry points

Working list of what an L8 PE would queue next. Pick from the top;
each item names the concrete files an entry point would touch.
Update [HANDOFF.md](../../HANDOFF.md) when you ship one.

Priorities reflect the "best-in-class enterprise" framing the owner
chose over "shortest path to a customer" — see ADR-002 + the
[PRODUCTION_READINESS_PLAN.md](./PRODUCTION_READINESS_PLAN.md) updates
of 2026-05-01.

---

## Tier 1 — ✅ ALL SHIPPED (verified 2026-06-02)

All three original Tier-1 items are already implemented in the tree. Entries
kept (collapsed) for provenance; verify before re-opening.

### 1. Audit-log JSONB scrub — ✅ SHIPPED
`internal/store/audit.go` `scrubAuditPayloads` wraps Before/After/Metadata
with `pii.ScrubJSON(..., pii.Restricted)` inside `InsertAudit`; idempotent,
parse-failure passes through unchanged. (Covered by the store integration
suite.)

### 2. Structured-log PII scrubbing — ✅ SHIPPED
`internal/obs/logger.go` `scrubAttr` masks Restricted-class attr values
(string → `pii.MaskString`; non-string → `[REDACTED]` sentinel; group attrs
recurse) in both `CorrelatingHandler.Handle` (per-call attrs) and `WithAttrs`
(logger-baked attrs). Confidential and below pass through for triage.

### 3. D7 wave 3 — Schedule + Pipeline audit recording — ✅ SHIPPED
Both services take an `AuditRecorder` (nil-safe → no-op) and call
`s.audit.Record(ctx, tx, AuditEntry{...})` inside the mutation tx:
`internal/service/schedule.go` (RecalculateSchedule) and
`internal/service/pipeline.go` (prospect create/advance/lose, estimate +
permit transitions).

---

## Tier 1b — current coverage frontier (active)

Test-coverage hardening toward production readiness. Recent (2026-06-02):
`internal/api` handler + router-RBAC suites (→ 84.6%, no 0%-funcs left),
`internal/service` pure-helper units (→ 16.5%), and `internal/store`
native-auth + vault integration tests (→ 62.8%). **Next:** DB-backed service
mutations (behind the `integration` tag — clone the store-test `testdb.NewPool`
pattern up into `service/*_integration_test.go`) and the partial `internal/api`
branches (`readinessHandler` DB-down leg, `CreateInvoice`/`DailyBriefing`
error paths).

---

## Tier 2 — meaningful enterprise items (when a specific need shows up)

### 4. Vault backend for SecretSource
**Why:** Customer forks deploying to enterprises with HashiCorp
Vault need a first-class integration. Today they can use the
`file:` source if Vault Agent writes secrets to a tmpfs.
**Files:**
- `internal/config/secrets_vault.go` (new) — implements
  `SecretSource` against Vault's KV-v2 API. Auth via Kubernetes
  service-account token by default; AppRole as fallback.
- Extend `LoadSecretSource` factory to accept `vault://path`
- `internal/config/secrets_vault_test.go` — table-driven against
  Vault's testing module (`github.com/hashicorp/vault-testing-stepwise`)
  or vault-in-docker via testcontainers.
**Scope:** ~300 LOC + tests. 1 session.
**Trigger:** open this when the first customer fork commits to Vault.

### 5. Backup automation + DR runbook — ✅ SHIPPED (2026-06-02)
Shipped `scripts/backup-db.sh` (pg_dump -Fc + sha256 sidecar +
storage-agnostic `BACKUP_UPLOAD_CMD` hook + filename-timestamp retention
with a most-recent floor), `scripts/restore-db.sh` (integrity verify +
`--confirm` destructive guard + `pg_restore --clean`), a DB-free
`scripts/backup-db.test.sh` regression suite (wired into `make audit`),
`docs/dr-runbook.md` (RPO/RTO, scheduling, restore drill, failure
playbook), and `make backup-db`/`restore-db`/`backup-db-test` targets.
GFS tiering deferred to object-store lifecycle rules by design.

<details><summary>original entry</summary>

**Why:** Per-fork model means each customer's database is
independently backed up. We have no automation script today.
**Files:**
- `scripts/backup-db.sh` — `pg_dump` → S3-compatible blob storage,
  with retention policy (daily 30d, weekly 12w, monthly 12m).
- `scripts/restore-db.sh` — reverse of above; documented restore
  drill checklist.
- `docs/dr-runbook.md` (new) — RPO/RTO documented; restore drill
  procedure; what to do when the primary database fails.
- Optional: River cron job that exercises a restore drill against
  a fresh empty instance weekly + alerts on failure.
**Scope:** ~200 LOC scripts + 4-5KB docs. 1 session.
</details>

### 6. AWS-SM / GCP-SM backends for SecretSource
**Why:** Same as Vault but for cloud-managed alternatives.
**Files:** mirror `secrets_vault.go` shape per backend.
**Scope:** ~200 LOC each. ½ session each.
**Trigger:** open when a customer fork commits to AWS / GCP.

### 7. mTLS option for Brain calls
**Why:** Some enterprise customers' security review process flags
"Bearer JWT only" as insufficient for service-to-service auth.
mTLS is the conventional answer.
**Files:**
- `internal/brain/client.go` — `Config.ClientCertPath` /
  `Config.ClientKeyPath`; load + wrap the http.Client transport
  with a `tls.Config` that presents the cert.
- Brain side needs matching `caCertPath` to verify.
- `docs/brain-mtls.md` — operator setup guide.
**Scope:** ~100 LOC + Brain coordination. ½ session BuildOS, ½
session Brain.

---

## Tier 3 — bigger initiatives (multi-session, owner direction needed)

### 8. Frontends — ✅ built in-monorepo (2026-06-01)
The companion frontends now live in this repo (decision reversed from
"separate repos"): the operator web console in `web/` (Vite + Lit +
TS-strict, Vanilla CSS, dark-only) and the Flutter field app in
`mobile/`. Built against the native backend + the binding specs in
`.agents/handoff/frontend/`.
- **web/** Phases A–F done: scaffold/tokens/typed API client
  (single-flight 401→refresh), `fb-*` component library, auth/onboarding
  wizard/BYOK, portfolio + command-center workspaces, a11y hardening.
- **mobile/** Phase G done: go_router + Riverpod, Drift offline outbox
  (FIFO exponential-backoff drain, server-wins), dio 401-refresh, field
  screens, FCM wake-hint, EN/ES i18n.

**Remaining (carryover):** backend-dependent E2E harness — web journeys
(login→setup→portfolio, recalc→cascade, BYOK→AI-on) + Flutter
`integration_test` (airplane-mode → queue → reconnect → drain) + golden
tests. Deferred from the backend-free CI sweeps.

### 9. OpenAPI spec generation + drift detection
**Why:** Contract today lives in `.agents/handoff/API_CONTRACT.md`
(human-readable). Frontends + customer integrators want a
machine-readable spec. CI should fail when code diverges from spec.
**Files:**
- Tag handler functions with comments; use
  [swag](https://github.com/swaggo/swag) or
  [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
  to generate.
- `Makefile` target: `make openapi`
- CI step: regenerate + diff; fail if dirty.
**Scope:** 1 session.

### 10. A5 — SubLiaisonAgent (real Twilio sender)
**Why deferred:** needs Twilio account, real phone numbers, TCPA
opt-in flow design. Owner decision.
**What's ready:** `NotificationDelivery` interface + DLQ + River
retry already in place from A1; just need a `TwilioSender`
implementation that satisfies `NotificationSender`.

### 11. Phase G — compliance + commercial
- Stripe billing integration (Brain side)
- AI pricing tier definition
- GDPR right-to-be-forgotten endpoints
- DPA templates
**Owner direction needed** before any code lands.

### 12. Phase H — pre-launch
- Load test (k6)
- Chaos test (kill DB primary, blackhole Brain, full disk)
- Runbook automation
- Alerting wiring (Sentry alerts, Prometheus → Grafana dashboards)
- On-call playbook
**Trigger:** after Phase F is fully shipped + customer fork is
running in staging.

---

## Tier 0 — carryovers

### `.github/workflows/{ci,release}.yml` push
The YAML is correct (built + tested in an earlier session). The
Claude Code OAuth token doesn't have GitHub's `workflow` scope.
Push from a user clone or via the GitHub UI. See the appendix in
[HANDOFF.md](../../HANDOFF.md) for the recipe.
