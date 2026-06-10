# NEXT_STEPS.md — prioritized backlog with entry points

Working list of what an L8 PE would queue next. Pick from the top;
each item names the concrete files an entry point would touch.
Update [HANDOFF.md](../../HANDOFF.md) when you ship one.

Priorities reflect the "best-in-class enterprise" framing the owner
chose over "shortest path to a customer" — see ADR-002 + the
[PRODUCTION_READINESS_PLAN.md](./PRODUCTION_READINESS_PLAN.md) updates
of 2026-05-01.

---

## Tier 0 — ▶ ACTIVE: agentic-OS roadmap (north star = [/VISION.md](../../VISION.md))

The product direction is now the **agentic OS**: a deterministic CPM core wrapped by an isolated,
configurable AI harness (`internal/agentic`). [/VISION.md](../../VISION.md) is canonical and supersedes
the pre-pivot `.agents/` specs for direction. The dependency-ordered phases below sit **above** the old
Tier-1..3 enterprise backlog; pick from here.

### P1 — Phase 1: `internal/agentic` substrate + a real `delay_cascade` (✅ DONE — landed `8e11f15` on local `main`, not pushed; ultrareview 7/7 fixed)
**Goal:** stand up the isolated harness (orchestrator + ports + in-code registry — DB-backed registry is
Phase 3) and turn the dead `delay_cascade` worker into a real AI-reasoned cross-module cascade:
schedule slip → load procurement/crew/budget context → AI reasons → apply (feed cards + audit) in ONE
tx → surface.
**Hard isolation:** `internal/agentic` is a leaf — imports only stdlib + `github.com/google/uuid` +
`log/slog`; declares ports; adapters in `internal/service` implement them; `internal/physics` /
`internal/currency` never import it. New gate `make lint-isolation` / `scripts/check-isolation.sh`.
**Entry points:**
- `internal/agentic/` (new leaf package: ports + orchestrator + in-code registry).
- `internal/worker/jobs.go` — `DelayCascadeWorker.Work` (`:253`, the stub) and `DelayCascadeArgs`
  (`:79`, add `OrgID` json `org_id`). Registered at `internal/worker/registry.go:47`.
- `internal/service/schedule.go` — enqueue site (`~:159`, `callerOrgID` in scope) populates `OrgID`;
  add the `internal/service` adapter(s) implementing the agentic ports.
- `cmd/worker/main.go` — wire vault/AI (modeled on `cmd/server/main.go ~:145-156`).
- Templates: `ai.Client.callTool` (`internal/ai/client.go ~:400`) + the `procurement_recommend` task
  (`internal/ai/tasks.go`); canonical load→AI→apply-in-one-tx→audit is
  `service.AgentsService.RecommendScheduleAdjustments` (`internal/service/agents.go ~:305`); feed cards
  `FeedCardsStore.CreateFeedCard` (`internal/store/feed_cards.go ~:39`); audit `AuditRecorder`
  (`internal/service/audit.go ~:44`).
- `scripts/check-isolation.sh` + `Makefile` (`lint-isolation` target).
**No schema migration in Phase 1** — reuse `feed_cards` + `audit_log`. **No new Go deps, no new store SQL.**
**Verify:** integration test (ephemeral PG) proving a slip → AI-reasoned cross-module cascade as
actionable feed cards; `make audit` + bench gates + the new isolation gate green.

### P2 — Phase 2: the four harness roles on the substrate (✅ COMPLETE — 2a + 2b + 2c all done & pushed)
Dependency-ordered, each its own PR-sized chunk:
1. **Ingestion** — ✅ DONE (`f83d135`, local `main`, not pushed). `POST /api/v1/projects/{id}/invoices/ingest`
   → `ai.InvoiceExtract` → validated `invoices` row (`source=ai_ingest`) + review feed card, idempotent via
   the `invoice_ingestions` outbox. Spec: [PHASE_2A_INGESTION.md](./PHASE_2A_INGESTION.md). Deferred to a
   later chunk: raw upload / PDF rasterization / async River trigger (the trigger that promotes the agentic
   port, §11) / relational line-item persistence.
2. **Foresight** — ✅ DONE (`66c773d`, local `main`, not pushed). Periodic `foresight_sweep` → deterministic
   metrics (procurement/schedule/budget) → AI materiality judgment → DEDUPED risk feed cards (migration 015
   `subject_code` skip-if-active dedup). Spec: [PHASE_2B_FORESIGHT.md](./PHASE_2B_FORESIGHT.md). Deferred to
   Phase 3: auto-expire reaper for resolved risks; "foresight active" health surface.
3. **Experience** — ✅ DONE (`60a99f0` + `e1f37db`, pushed to `origin/main`). `POST /api/v1/agents/chat`
   runs a bounded Claude tool-use loop over 8 READ-ONLY, caller-scoped ERP tools (new `internal/authz`
   ladder; agentic `Tool`/`Assistant` ports; `ai.RunToolLoop`). 5-layer structural RBAC. Spec:
   [PHASE_2C_EXPERIENCE.md](./PHASE_2C_EXPERIENCE.md).
   **2c fast-follows (deferred from the backstop review — none are hazards; all gates were green):**
   - **(A)** `ai.RunToolLoop` `finalize()` rebuilds the synthesis turn from the original request history,
     dropping the loop's tool context when the model emits zero text across all iterations (rare; degraded
     synthesis only). Fix: pass the accumulated `msgs` — mind the Anthropic API nuance (consecutive-role
     turns; sending historical `tool_use` without `tools[]`; or use `tool_choice:{type:"none"}`).
   - **(B)** `MaxResultBytes` is checked PRE-execution (`chatloop.go` ~line 225) so the first tool result
     can overshoot the cumulative "hard ceiling" by its own size (bounded in practice by
     `assistantToolMaxPerPage=50`). Fix: account result bytes post-execution.
   - **(D)** `buildRegistry` writes each tool's `MinRole` twice (the `Tool.MinRole` field + the executor's
     `minRole` arg — identical literals, adjacent lines). Single-source via a helper to kill future drift
     (the role×tool matrix integration test covers the behavior today).
   - **(E)** `get_org_financials` fail-fast drops a successful `GetProjectFinancials` if `GetARAging`
     errors. Consider returning the partial result instead of only the error.
   - Larger deferrals (per spec): mutating/act tools (behind human-confirmation), server-persisted
     conversations (migration 016/017), and SSE streaming.

### P3 — Phase 3: configurability + integration/MCP layer
DB-backed agent/connector registry, admin config surface (enable + tune agents/integrations post-deploy,
no redeploy), MCP connector seam, vault-backed credential UI. In dependency order 3a → 3b → 3c:

- **3a · Config registry (✅ DONE — merged + pushed to `origin/main`, HEAD `27e2986`; max-effort review
  clean, 3 fixes applied).** `agents_config` table (migration 016) + leaf `agentic.ConfigResolver` port +
  `service.AgentConfigService` (admin CRUD + resolver) + `/api/v1/admin/agents` admin API. Per-org
  enable/disable for all three capabilities + foresight threshold tuning, post-deploy. Spec:
  [PHASE_3A_CONFIG_REGISTRY.md](./PHASE_3A_CONFIG_REGISTRY.md). **Entry points for the resolver/gate seam:**
  `internal/agentic/config.go` (port), `internal/service/agent_config.go` (adapter, two faces),
  `internal/store/agent_config.go`, `internal/api/agent_config.go`, `cmd/{server,worker}/main.go` (wiring).
- **3b · Integration/MCP seam — SPLIT (owner-approved after a design critique found one chunk under-secured):**
  - **3b-i · connector framework (✅ DONE on branch `feat/phase-3b-connector-framework`, reviewed clean, gates
    green — awaiting owner merge).** `internal/connectors` (Connector interface + built-in `reference`
    connector, no network) + `connectors_config` (migration 017, default-OFF) + `ConnectorService` +
    `/api/v1/admin/connectors` + fail-closed namespaced admin-floored `buildRegistry` merge + isolation
    Check 3. Spec: [PHASE_3B_CONNECTOR_FRAMEWORK.md](./PHASE_3B_CONNECTOR_FRAMEWORK.md). Entry points for
    3b-ii: `internal/connectors` (add the MCP connector type), `internal/service/connector_config.go`
    (resolver + cached tools), `internal/cryptobox`/`integration_credentials` vault (the `connector:<id>` key).
  - **3b-ii · MCP connector + egress security (✅ DONE — merged + pushed to `origin/main`, HEAD `9a84235`;
    61-agent security review clean, no SSRF bypass).** Hand-rolled (no-dep) full MCP Streamable HTTP client
    (`internal/connectors/{egress,mcpclient,breaker,mcp}.go`) + SSRF private-IP denylist guard +
    per-(org,endpoint) breaker + `connector_tools` cache (migration 018) + the `mcp` connector type
    (`ConnectorService` Set/RefreshTools/ToolsFor) + admin `POST .../refresh`. Spec:
    [PHASE_3B_II_MCP_CONNECTOR.md](./PHASE_3B_II_MCP_CONNECTOR.md).
- **3c · Admin config UI — DONE (merged → local `main`, HEAD `11f108d`; NOT pushed).** `web/` Lit screens
  `/settings/agents` (`fb-agents-page`) + `/settings/connectors` (`fb-connectors-page`) wire
  `/api/v1/admin/agents` + `/api/v1/admin/connectors` (+ the `connector:<name>` vault credential via
  `/api/v1/integrations`, managed in-place). admin+, NOT plan-gated; create/enable/configure/refresh/delete
  MCP instances; foresight threshold tuning; a11y + design-system conformant; new `fb-connector-card`
  molecule + `api/endpoints/admin.ts`. Built via 9-agent design critique → 64-agent max review (6 findings
  fixed); cloud ultrareview timed out (infra). Spec: [PHASE_3C_ADMIN_UI.md](./PHASE_3C_ADMIN_UI.md).
  **This completes Phase 3 (3a + 3b + 3c). Next: Phase 4 (P4 below).**

**[ESC-002](./ESCALATION_LOG.md#esc-002) — RESOLVED + MERGED + PUSHED (Phase 4 chunk 1; owner chose Option 2,
drop the gate; `origin/main` HEAD `8a1fcc3`).** Removed `RequirePlanTier(pro)` from `/api/v1/agents/*` (kept
role gates) on the backend AND dropped the symmetric web-console `requiresPro` wall — the AI surface is
reachable for real tokens again end-to-end. Spec: [PHASE_4_ESC_002.md](./PHASE_4_ESC_002.md).

### P4 — Phase 4: production-readiness for handoff
A Phase-4 ultraplan (6-agent assessment, 2026-06-09) decomposed the phase + ordered the chunks (grounded in
the actual code, which made the original "3 missing field screens" framing partly stale):
1. **ESC-002 · drop the pro gate — DONE: merged + PUSHED to `origin/main` (HEAD `8a1fcc3`).** Unblocked the
   `/api/v1/agents/*` AI surface end-to-end (backend + web console); prerequisite for 4c load/security
   testing it. Spec: [PHASE_4_ESC_002.md](./PHASE_4_ESC_002.md).
2. **4a · Flutter field.**
   - **4a-i · standalone check-in + offline affordance — DONE: merged + PUSHED (`origin/main` `4f7698e`).** New `CheckInScreen` (crew roster name+role, GPS, notes, offline-queued) + reusable
     `FbDashedBorder` (amber dashed affordance), wired into the More tab; the crew-less stub extracted out of
     `daily_log`. 9 widget tests + a golden; mobile gates green. Spec:
     [PHASE_4A_I_FIELD_CHECKIN.md](./PHASE_4A_I_FIELD_CHECKIN.md).
   - **4a-ii · read-only equipment — DONE: merged + PUSHED (`origin/main` `f0c2df0`). Phase 4a COMPLETE.**
     Owner chose read-only (ESC-003). An `equipment` array on `GET /api/v1/field/sync` (full-set, server-wins,
     scoped to the caller's active sites; field-safe DTO) → a `CachedEquipment` Drift cache (first v1→v2
     migration, delete-then-fill) → a read-only `EquipmentScreen` (More tab). No new endpoint/RBAC/migration.
     Spec: [PHASE_4A_II_FIELD_EQUIPMENT.md](./PHASE_4A_II_FIELD_EQUIPMENT.md).
3. **4b · operator hardening.**
   - **4b-i · alerting rules + runbooks — DONE: merged + PUSHED (`origin/main` `ff22d74`).**
     `deploy/prometheus/buildos.rules.yml` (8 alerts) + `deploy/prometheus/README.md` +
     `docs/observability-runbook.md` + `docs/deploy-runbook.md`. Config+docs only. **Grounding correction to
     the original plan:** the suggested setup-gate / DB-pool / River-job alerts have NO backing metric — only
     4 `buildos_*` metrics are emitted (server-only); `buildos_river_job_runs_total` is never incremented and
     the worker serves no `/metrics`. Shipped as documented gaps. Spec:
     [PHASE_4B_I_ALERTING.md](./PHASE_4B_I_ALERTING.md).
   - **4b-ii · worker observability — DONE: merged + PUSHED (`origin/main` `19fc7d3`).**
     `cmd/worker` serves `/metrics`+`/health`+`/ready` and records River job outcomes into
     `buildos_river_job_runs_total` (event subscription) + the worker AI client feeds `buildos_ai_*`; re-added
     the `buildos-jobs` alert group + `BuildOSWorkerDown` + the worker scrape job. Spec:
     [PHASE_4B_II_WORKER_OBS.md](./PHASE_4B_II_WORKER_OBS.md).
   - **4b-iii · error-path UX + metric follow-ups** — Retry-After on 5xx/429, AI circuit-breaker surfacing,
     `cmd/migrate --dry-run`; plus `pgxpool.Stat()` → `buildos_db_pool_*`, a per-error-code/SetupGate counter,
     and a worker queue-depth / oldest-available-job gauge (the "worker alive but stuck" gap).
4. **4c · security + load (FINAL gate) — BUILT on `feat/phase-4c-security-load`, awaiting review.** An
   8-surface multi-agent security audit (no critical; 2 HIGH fixed — SSRF in invoice `document_url`, the
   XFF-spoof rate-limit bypass) + `scripts/k6/` load harness. Posture report: `docs/security-posture.md`;
   spec `.agents/handoff/PHASE_4C_SECURITY_LOAD.md`. Closes Phase 4. Tracked follow-ups in the posture doc
   (per-account lockout, per-(org,user) AI throttle, deployment TLS/HSTS + TRUSTED_PROXY_CIDRS).

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

Test-coverage hardening toward production readiness. **Status (2026-06-04): the
deterministic-guard / reachable-error frontiers are now CLOSED across the board**
— every leg a test can hit without fault injection or a heavy rig has been
covered. Map:
- **Go backend — at deterministic ceilings.** `internal/api` 98.4% (handler +
  router-RBAC guard frontier closed), `internal/service` guard frontier closed,
  `internal/store` 86.0% (list/reader sweep at ceiling — see below), `internal/ai`
  96.3%, `internal/mailer` 95.8%, `internal/cryptobox` 88.9%. Remaining zero-blocks
  are unreachable defensive guards (`json.Marshal` of plain structs, valid-URL
  `NewRequest`, length-subsumed crypto errors, non-injectable `rand.Reader`) or
  non-deterministic `fmt.Errorf("query/scan …: %w")` wraps — both are the deferred
  **POST-BETA fault-injection** territory documented below.
- **Flutter `mobile/` core — services all 100%.** `sync_service.dart`,
  `api_client.dart`, `token_store.dart`, `auth_service.dart`, and the
  `models/user.dart` + `models/field_sync.dart` wire models are fully covered.
  `app_database.dart` is at its practical ceiling (~56% — the rest are Drift column
  declarations shadowed by generated overrides at runtime).
- **Next (lower-value, needs heavier rigs):** `mobile/` widget/screen coverage via
  `testWidgets` + Riverpod `ProviderScope` overrides; the one cheap unit win left is
  `models/feed_card.dart` `fromJson`. See HANDOFF.md ▶ NEXT for the full candidate
  list and the live-E2E / OpenAPI alternatives.

#### Store-layer status (2026-06-03): list/reader frontier at deterministic ceiling

The `internal/store` query-round-trip sweep (Tier-1b clusters 29–33) closed
every deterministic leg on the readers and list functions: not-found
short-circuits, `RowsAffected()==0` mutators, input-guard clamps, empty-vs-
populated scan bodies, and guard/default early-returns. The package sits at
**86.0%**. Every remaining uncovered leg on the list/reader functions is a
**non-deterministic `fmt.Errorf("query …"/"scan …": %w)` error wrap** — verified
across all `List*` functions (financials, hr, setup, pipeline, procurement,
field, fleet, integration_credentials). There is **no deterministic test-only
gain left** on this frontier; it is at its ceiling.

#### POST-BETA: fault-injection pass to lift the deterministic ceiling

**Deferred until after beta production go-live** (owner directive 2026-06-03).
Once the product is live and the integration-suite runtime budget can absorb it,
add a fault-injection pass to drive the query/scan/exec error-wrap legs that the
deterministic sweep deliberately left at the ceiling:
- **Technique:** call each store reader/list with a **pre-cancelled `context`**
  (`ctx, cancel := context.WithCancel(parent); cancel()`) so `tx.Query` /
  `tx.Exec` return `context.Canceled` → the `fmt.Errorf("query …: %w")` wrap
  fires deterministically. Lifts each `List*` ~81.8%→~90.9% (the inner
  per-row `scan …` wrap still needs a typed-mismatch or partial-read harness —
  a `pgxmock`/fake `pgx.Rows`, evaluate if worth the dep).
- **Scope:** one consolidated `*_faultinjection_integration_test.go` per store
  file (or a shared table-driven helper) exercising the query-error leg of every
  reader/list; the scan-error leg only where a fake-rows harness is justified.
- **Why deferred:** these wraps are trivial passthroughs with no business logic;
  covering them pre-beta is lower-value than feature/flow coverage. Capturing the
  plan now so the ceiling is a deliberate, documented decision — not an oversight.
- **Entry points:** `internal/store/*.go` `List*`/`Get*` readers; mirror the
  `testdb.NewPool` harness; gate behind `//go:build integration`.

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

**Status (2026-06-04): mostly shipped.** The backend-dependent E2E harness is DONE
(`scripts/e2e-backend.sh` + the `e2e` and `e2e-mobile` CI lanes): web drives
claim→wizard→portfolio + recalc→cascade + BYOK→AI-on; the Flutter `e2e-mobile` lane
drives airplane-mode → queue → reconnect → drain + 409 idempotency replay. The
offline/sync **golden tests are shipped** (`mobile/test/fb_sync_chip_golden_test.dart`,
`sync_status_screen_golden_test.dart`, opt-in `--tags golden`). **Remaining:**
widening the live journeys (Flutter-side widget/screen unit coverage; deeper E2E
flows) — tracked in HANDOFF.md ▶ NEXT.

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

## Carryovers (infra)

### `.github/workflows/{ci,release}.yml` push
The YAML is correct (built + tested in an earlier session). The
Claude Code OAuth token doesn't have GitHub's `workflow` scope.
Push from a user clone or via the GitHub UI. See the appendix in
[HANDOFF.md](../../HANDOFF.md) for the recipe.
