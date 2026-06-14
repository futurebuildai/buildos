# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**BuildOS** — a Go backend (the "system of execution") for residential construction project management. **Single-tenant per customer fork** (see ADR-002), REST API + River job queue. It is a **self-contained standalone deployment**: auth, AI, and 3rd-party credentials are all native and admin-configurable inside BuildOS (no external "Brain" service, no A2A webhooks, no billing engine). The core domain object is a project schedule computed by a deterministic **Critical Path Method (CPM) physics engine**.

Companion frontends now live in this monorepo: the operator web console in [web/](web/) (Vite + Lit + TypeScript, Vanilla CSS, dark-only) and the Flutter field app in `mobile/`. Their binding design specs are in [.agents/handoff/frontend/](.agents/handoff/frontend/) (FRONTEND_ARCHITECTURE, DESIGN_SYSTEM_COMPONENTS, UX_AUTH_ONBOARDING, UX_CORE_SCREENS). The web console is built separately and served same-origin behind the Go server in production; see [web/README.md](web/README.md).

**North star:** [VISION.md](VISION.md) at repo root is the canonical product-direction doc — BuildOS as an *agentic OS* (deterministic CPM core wrapped by an isolated, configurable AI harness). For product direction it **supersedes** the pre-pivot `.agents/` specs (PRD, ARCHITECTURE, EXECUTION_PLAN); those stay authoritative only for their narrow mechanical contracts (`.agents/handoff/API_CONTRACT.md`, `.agents/TECH_STACK.md`). Read VISION.md before any harness/agentic work.

**Deployment model (per [ADR-002](.agents/handoff/ADR-002-single-tenant-fork-model.md)):** every customer gets their own forked BuildOS repo and their own deployment instance — they own the core code and data. **Tenant isolation = deployment isolation.** No Postgres RLS, no per-tenant rate limiting, no multi-region routing logic in the application. A possible future co-op variant runs multi-tenant within one deployment; ships only if/when the product roadmap calls for it.

Status: Sprints 1-5 done + Phase F core (production Dockerfile, D8 build-tag hardening, cmd/buildos-fork-init keypair/vault-key generator) + observability (Prometheus /metrics, OpenTelemetry tracing, Sentry with PII masking) + secret-source abstraction + the embedded onboarding wizard (migration 010, SetupGate, bootstrap tokens — see "Onboarding wizard" below) + the standalone pivot (native email/password auth, native Anthropic AI, encrypted BYOK vault; Brain/A2A/billing removed, migration 013).

**Since the pivot (current as of 2026-06-14):**
- **Agentic-OS roadmap Phases 1–4 are COMPLETE.** The isolated `internal/agentic` harness + a real `delay_cascade`; the four harness roles (ingestion = AI invoice extract → `invoices`; foresight = `foresight_sweep` risk cards; experience = `POST /api/v1/agents/chat` bounded tool-use loop; orchestration); DB-backed agent + MCP-connector config (migrations 016–018) with an admin web UI (`internal/connectors`, `internal/authz`); and production-readiness hardening (security/load review — `docs/security-posture.md`, worker observability, Prometheus alerting). See VISION.md + NEXT_STEPS for the per-phase record.
- **Deployment infra is LIVE on Railway.** `staging.futurebuild.ai` auto-deploys `main` (GitHub Actions `deploy-staging.yml` → build → migrate → roll → version-asserted smoke); `app.futurebuild.ai` promotes pinned, staging-verified digests (`promote-production.yml`). This repo is **"fork zero"** — Kelbrook Construction's own instance runs from it; true per-customer forks start with builder #2.
- **Operational-coordination layer — first slice SHIPPED:** Daily Reports → Client Updates (BuildOS's first object storage + first PUBLIC unauth surface; migrations 022–025 — see "Operational-coordination domain" below).

See [HANDOFF.md](HANDOFF.md) for current per-session state and [.agents/handoff/NEXT_STEPS.md](.agents/handoff/NEXT_STEPS.md) for the prioritized backlog.

## Common commands

```bash
make build              # builds bin/server and bin/worker
make build-migrate      # builds bin/migrate (separate target)
make build-prod         # CGO_ENABLED=0 + tags=prod + trimpath + stripped (matches Dockerfile)
make build-fork-init    # offline keypair generator for new customer fork onboarding
make fork-init OUT=...  # invoke the generator (see docs/fork-onboarding.md)
make test               # unit tests only (no Docker)
make test-prod          # prod-tagged subset (D8 hardening — auth_prod.go stub, etc.)
make test-integration   # integration tests via Testcontainers (Docker required)
make lint               # golangci-lint run ./...
make lint-migrations    # bash scripts/lint-migrations.sh — 5 rules, see below
make lint-migrations-test # regression suite for the migration linter itself
make db-up              # docker compose up -d db (Postgres 16, port 5433)
make db-down
make migrate            # runs migrations up against $DATABASE_URL
make migrate-dry-run    # lists pending migrations without applying
make migrate-down       # rolls back
make bench-physics      # CPM benchmarks through tools/bench-gate (CI hard gate)
make docker-build       # multi-stage distroless image; entrypoint dispatches by BUILDOS_ROLE
make audit              # full pre-merge gate (lint-migrations + lint-migrations-test
                        # + test + test-prod + bench-physics)
```

**⚠️ `make audit` is NOT full CI parity.** CI additionally runs (a) a repo-wide `gofmt -l .` check (fails on ANY unformatted file) and (b) the **full, unfiltered** integration suite (`go test -tags=integration ./internal/...`). Before pushing to `main` (which auto-deploys staging), also run `gofmt -l . | grep -v '^vendor/'` (must be empty) and the unfiltered integration suite — a `-run`-filtered local pass hides failures, and a red CI gate **silently skips the staging auto-deploy**. (Both gaps have bitten a merge; closing them in `make audit` is a tracked follow-up.)

Run a single Go test: `go test ./internal/physics/... -run TestCPMDeterminism -count=1`
Run a single benchmark: `go test -bench=BenchmarkCPM80Tasks -benchtime=10x ./internal/physics/...`
Run a single integration test: `go test -tags=integration -count=1 -run TestFinancialsStore_CreateInvoice_RoundTrip ./internal/store/...`

Local Postgres listens on **port 5433** (not 5432) to avoid conflicts. Default DSN is hardcoded into the `Makefile`.

Integration tests live behind the `//go:build integration` build tag. They spawn ephemeral Postgres 16 containers via Testcontainers, apply all `migrations/*.up.sql`, and tear down at test exit. The shared fixtures live in [internal/testdb](internal/testdb/) — call `testdb.NewPool(t)` from any test file with the integration tag and you get a freshly migrated pool with auto-cleanup.

## Architecture

Three binaries under `cmd/`:
- `cmd/server` — Chi HTTP API on `$PORT` (default 8080). Loads config, opens pgxpool, builds the `auth.Verifier` from the RSA public key for JWT validation, mounts routes, graceful shutdown.
- `cmd/worker` — River job daemon. Same DB pool. Registers job kinds defined in [internal/worker/jobs.go](internal/worker/jobs.go): `daily_briefing`, `procurement_check`, `hydrate_project`, `corporate_rollup`, `certification_alerts`, `maintenance_reminders`, `field_notification_retry`, `delay_cascade`, `pipeline_analytics`, `permit_issued_transition`, `foresight_sweep`.
- `cmd/migrate` — runs River's internal migrations first, then `migrations/NNN_*.up.sql` / `.down.sql` against `schema_migrations`.

Internal package layout:
- [internal/api](internal/api/) — Chi handlers, thin. [router.go](internal/api/router.go) is the single source of truth for routes and RBAC. Auth and RBAC middleware live in [internal/api/middleware](internal/api/middleware/).
- [internal/physics](internal/physics/) — the CPM engine. **Determinism is non-negotiable**: durations and times are integer nanoseconds, never floats. [cpm.go](internal/physics/cpm.go) does the forward/backward pass over a gonum DAG; [dhsm.go](internal/physics/dhsm.go) scales durations by GSF (Gross Square Footage); [swim.go](internal/physics/swim.go) applies weather adjustments. The golden-master test in `cpm_determinism_test.go` will catch any drift.
- [internal/service](internal/service/) — business logic. [schedule.go](internal/service/schedule.go) is the canonical pattern: open a pgx tx, load via store, run physics, persist via store, enqueue follow-up River jobs, commit. Don't enqueue outside the tx (no phantom jobs).
- [internal/store](internal/store/) — raw `pgx/v5` SQL. **No ORM**. Tenant isolation is per-query; every query filters by `org_id`.
- [internal/worker](internal/worker/) — River client setup and job arg types.
- [internal/models](internal/models/) — domain structs.
- [internal/config](internal/config/) — env loading.
- [internal/auth](internal/auth/) — native auth primitives: argon2id password hashing ([password.go](internal/auth/password.go)) and the RS256 JWT `TokenIssuer` / `Verifier` ([token.go](internal/auth/token.go)). BuildOS mints AND validates its own access tokens against the per-fork RSA keypair.
- [internal/ai](internal/ai/) — native Anthropic client. Calls the Anthropic Messages API directly with the BYOK key resolved from the encrypted vault. [image.go](internal/ai/image.go) handles image input (InvoiceExtract); [resilience.go](internal/ai/resilience.go) adds retry/backoff.
- [internal/cryptobox](internal/cryptobox/) — AES-256-GCM seal/open for the encrypted credential vault, keyed by `VAULT_MASTER_KEY`. Holds the Anthropic key, the Resend key, the `object_store` (R2) credentials, and 3rd-party vendor credentials — never leaves the deployment.
- [internal/mailer](internal/mailer/) — transactional email via Resend ([resend.go](internal/mailer/resend.go)). Used for password-reset emails; the Resend API key is set in-app via the vault.
- [internal/currency](internal/currency/) — safe integer-cents arithmetic for the Composite Currency Pattern (USD/CAD only). `ErrCrossCurrency` is the sentinel for forbidden cross-currency math.
- [internal/storage](internal/storage/) — **object storage** (S3-compatible / Cloudflare R2). **Leaf package** (imports no other `internal/*`; isolation Check 4). Holds a hand-rolled AWS **SigV4 presigner** (no `aws-sdk` dep), the `ObjectStore` port, and the `R2Store` adapter; presigned PUT/GET direct-to-client. Credentials come from the vault `object_store` provider. See "Operational-coordination domain" below.
- [internal/authz](internal/authz/) — the single role-ladder primitive (`RoleAtLeast`) shared by the auth middleware and the services/agentic tools (so RBAC is one source of truth). Leaf.
- [internal/setup](internal/setup/)-adjacent code — the embedded onboarding wizard (see "Onboarding wizard" below). Spans `service/setup.go`, `store/setup.go`, `api/setup.go`, `models/setup.go`, and `api/middleware/setup_gate.go`.
- `internal/agentic` — the isolated **agentic harness** (the orchestration/agent runtime; see "Agentic harness" below). **Leaf package.** It depends on the rest of BuildOS only through **ports** (interfaces it declares); the deterministic core depends on it *never*.
- `internal/connectors` — the **integration / MCP connector** layer (Phase 3b). Imports **only** `internal/agentic` (isolation Check 3; core ⊬ connectors via Check 3b). The built-in `reference` connector + a hand-rolled (no-dep) MCP Streamable-HTTP client with an SSRF private-IP egress denylist + per-(org,endpoint) circuit breaker. Connector tools merge into the assistant registry namespaced + admin-floored, fail-closed.

### Agentic harness (`internal/agentic`)

Per [VISION.md](VISION.md), BuildOS is an agentic OS: a deterministic CPM core wrapped by an isolated, configurable AI harness. `internal/agentic` is that harness's runtime — orchestrator + ports + an in-code agent/tool registry (the DB-backed registry is a later phase). Intended state:

- **Leaf / isolation gate.** `internal/agentic` imports **only** stdlib + `github.com/google/uuid` + `log/slog`. It imports **nothing** from `internal/service`, `internal/store`, `internal/ai`, `internal/worker`, `internal/physics`, or `internal/currency`. It declares its own **port interfaces** (the ERP capabilities it needs — load context, create feed card, audit, AI reasoning); **adapters in `internal/service`** implement those ports and inject them. The deterministic core (`internal/physics`, `internal/currency`) and the domain services must **never** import `internal/agentic`. This is enforced by a CI gate: `make lint-isolation` (`scripts/check-isolation.sh`), which walks the import graph and fails the build on any forbidden edge.
- **Exact/fuzzy boundary.** The harness owns judgment only — it decides *what to feed the engine* and *what to do with the output*. It **never** does schedule or money math; the deterministic engine (`internal/physics`) and integer-cents arithmetic (`internal/currency`) stay authoritative.
- **Orchestrator pattern.** The canonical flow is: *triggering event → load cross-module context → AI reasons → apply recommended actions across modules in one tx → audit per-module deltas → surface via feed cards.* The reference implementation of the load→AI→apply-in-one-tx→audit shape lives in `service.AgentsService.RecommendScheduleAdjustments` (`internal/service/agents.go`); the first orchestration to use the harness is the real `delay_cascade` worker.

Request flow for a schedule recalc: HTTP → JWT middleware → RBAC middleware → handler → `ScheduleService.RecalculateSchedule` (begins tx) → `ScheduleStore` loads tasks/deps → `physics.ForwardPass` + `BackwardPass` → `ScheduleStore.UpdateSchedule` writes `early_start`, `late_finish`, `total_float`, `is_critical` → if critical path changed, enqueue `DelayCascadeArgs` River job → commit.

## Operational-coordination domain (Daily Reports → Client Updates)

The first slice of the operational-coordination / system-of-record layer (VISION layer 4). Field photos → derived daily reports → an AI office digest + a client-SAFE homeowner update → emailed + a public shareable link. It introduced BuildOS's **first object storage** and its **first PUBLIC unauthenticated surface**. Migrations 022 (assets), 023 (project client contact), 024 (client_updates), 025 (share_links). Spans `internal/storage`, `internal/{models,store,service,api}/{assets,reports,client_update,share_link}.go`, `api/public_share.go`, and `api/field.go`. Contract: API_CONTRACT §13e–13i; design + as-built notes: [.agents/handoff/DAILY_REPORTS_CLIENT_UPDATES.md](.agents/handoff/DAILY_REPORTS_CLIENT_UPDATES.md).

Two hard rules this domain establishes (both are CI-tested + were the focus of an adversarial security review):

- **Deterministic client-content redaction.** The homeowner-facing AI draft is built behind an **allowlist** at the service boundary (`service.buildClientRequest`) — safety incidents, crew identities, GPS, and `*_cents` amounts can NEVER reach the client prompt or the public projection. Homeowner contact (`client_name`/`client_email`/`client_phone`) is `json:"-"` on `models.Project` (read server-side only; never serialized to the role-ungated project endpoints or AI tool context).
- **Public surface = projection + curated-asset proxy, never raw ERP.** `GET /p/{token}` (and `/p/{token}/photos/{assetID}`) mount as a **sibling of `MountAuthRoutes`, OUTSIDE the auth `r.Group`** — they bypass Auth + SetupGate **without weakening them** (a dedicated rate limiter + RealIP + security headers still apply). The response is a server-rendered `models.PublicUpdate` projection that *physically cannot carry* ERP fields, plus a same-origin photo proxy. **`AssetService.ServeAsset` (the proxy's only caller) refuses any image type it cannot EXIF-strip** (webp/heic → 404) so GPS EXIF never reaches an unauthenticated viewer; the operator surface uses signed R2 redirects instead. Tokens are 32-byte CSPRNG, sha256-at-rest, expirable + revocable, with a uniform 404 on any bad/expired/revoked value.

When adding any new public/unauth surface or any client-facing AI content, preserve both rules: gate at the service boundary (not the model), project rather than serialize the ERP struct, and add a no-auth reachability + ERP-absence test.

## Composite Currency Pattern (hard CI gate)

**All monetary values are stored as `amount_cents BIGINT` paired with `currency_code VARCHAR(3) DEFAULT 'USD'`. No floats, ever.** Supported currencies: USD, CAD. Cross-currency arithmetic is forbidden — aggregations must group by `currency_code`.

[scripts/lint-migrations.sh](scripts/lint-migrations.sh) is a hard CI fail with no exemptions. It runs **5 rules** (3-5 added 2026-05-01 per the migration-safety hardening):
1. Forbidden types (`DECIMAL`, `NUMERIC`, `REAL`, `DOUBLE PRECISION`, `MONEY`, `FLOAT`) on columns matching `cost|price|amount|total|budget|cents|fee|payment|invoice|balance|revenue|expense`. The only allowed `DOUBLE PRECISION` columns are GPS coords (`gps_lat`, `gps_lng`, `lat`, `lng`).
2. Any `_cents` column without a `currency_code` column in the same `CREATE TABLE`.
3. **Paired up/down**: every `migrations/NNN_name.up.sql` must have a matching `.down.sql`. For irreversible migrations the down can be a single comment line documenting why.
4. **Destructive ops require opt-in**: `DROP TABLE` / `DROP COLUMN` / `TRUNCATE` / `ALTER TYPE ... DROP VALUE` require a `-- buildos:destructive: <reason>` header anywhere in the file. Forces operator consent and surfaces the reason in PR review.
5. **CREATE INDEX must use CONCURRENTLY** (or per-line opt-out). Plain `CREATE INDEX` takes ACCESS EXCLUSIVE for the duration of the build. Opt-out: append `-- buildos:lock-ok: <reason>` on the same line for genuinely-small / fresh-table cases (the early migrations are all annotated this way since their indexes land on tables freshly created in the same migration).

Go convention: monetary fields end in `Cents` (e.g. `TotalActualCostCents`) with a sibling `CurrencyCode` field. Don't introduce `float64` for money.

The linter has its own regression suite at `scripts/lint-migrations.test.sh` with four fixtures (pass + three fail-modes). `make lint-migrations-test` runs it; both lint targets are part of `make audit`.

## Native auth, AI, and credentials

BuildOS is self-contained. The three surfaces that were formerly delegated to an external "Brain" service are now native:

1. **Identity** — native email/password. Passwords are argon2id-hashed ([internal/auth/password.go](internal/auth/password.go)). BuildOS mints its own RS256 access tokens via `auth.TokenIssuer` and validates them with `auth.Verifier` against the per-fork RSA keypair (`JWT_PRIVATE_KEY_PEM` / `JWT_PUBLIC_KEY_PEM`). The unauthenticated auth surface mounts under `/api/v1/auth`: `claim`, `login`, `refresh`, `logout`, `password-reset/request`, `password-reset/confirm` (see [internal/api/auth.go](internal/api/auth.go) `MountAuthRoutes`). RBAC roles are `owner` > `admin` > `superintendent` > `field_worker`. Validation middleware: [internal/api/middleware/auth.go](internal/api/middleware/auth.go).
2. **AI** — BuildOS calls the Anthropic Messages API directly ([internal/ai](internal/ai/)) with the BYOK key from the encrypted vault. Image input (InvoiceExtract) is supported. A missing key soft-fails: the server boots, and AI-dependent endpoints return 503 until an admin configures the key.
3. **Credentials** — the encrypted vault ([internal/cryptobox](internal/cryptobox/), AES-256-GCM under `VAULT_MASTER_KEY`) stores the Anthropic key, the Resend key, the `object_store` (Cloudflare R2) credentials, and 3rd-party vendor credentials (Gable, LocalBlue). Credentials are sealed at rest and never leave the deployment. Vendor seams call upstream APIs directly with the unsealed key. The provider set is enumerated by `VaultService.Capabilities` (each surfaces a `*Configured` flag + non-secret fingerprint).

Password reset emails go out via Resend ([internal/mailer](internal/mailer/)); the Resend API key is set in-app via the vault.

JWT wire-protocol contract: `iss="buildos"`, `aud="buildos"` (defaults; overridable via `JWT_ISSUER` / `JWT_AUDIENCE`). Because BuildOS is both the issuer and the verifier, these can be changed freely per deployment — no external coordination required.

### Alternative auth for non-production

**`DEV_AUTH_MODE=header`** (dev / CI) — middleware reads claims directly from an `X-Dev-Auth: <sub>,<org_id>,<role>[,<plan_tier>]` request header instead of validating a JWT. Any role/org/persona can be exercised per request without infra. Implementation: [auth_dev.go](internal/api/middleware/auth_dev.go) `claimsFromDevHeader`. Leave the env unset (or `""`) in production. The header path is **build-tag gated** (D8 hardening): the `prod` build ships an `auth_prod.go` stub that no-ops the header bypass, and `cmd/server` refuses to start if a prod binary sees `DEV_AUTH_MODE` set.

## Performance gates

`make bench-physics` enforces:
- `BenchmarkCPM80Tasks` ≤ **200ms**
- `BenchmarkCPM200Tasks` ≤ **500ms**

Failures block CI via [tools/bench-gate](tools/bench-gate/). When changing anything in `internal/physics/`, run `make bench-physics` locally before pushing.

## Dual-agent workflow

This repo is co-developed by two agents under `.agents/`:
- **Antigravity** designs specs (PRD, ARCHITECTURE, API_CONTRACT, DESIGN_SYSTEM, EXECUTION_PLAN) into `.agents/handoff/`.
- **Claude Code** (you) executes them as a zero-trust auditor. Full protocol in [.agents/claude_instructions.md](.agents/claude_instructions.md).

Practical implications when working here:
- The single source of truth for tool/library choices is [.agents/TECH_STACK.md](.agents/TECH_STACK.md). Do not introduce dependencies not listed there without flagging.
- API routes, schemas, and status codes must match `.agents/handoff/API_CONTRACT.md` exactly.
- If a spec is ambiguous, contradictory, missing, or impractical, **do not improvise**: write to `.agents/handoff/ESCALATION_LOG.md` and pause for the user.
- Anthropic Claude is the default and only AI provider. Open-source models are permitted only for on-device Flutter inference or domain-specific embeddings.

## Onboarding wizard (setup subsystem)

Every fork ships with an embedded onboarding wizard that must complete before the deployment serves operational traffic. The subsystem spans five files: [internal/models/setup.go](internal/models/setup.go), [internal/store/setup.go](internal/store/setup.go), [internal/service/setup.go](internal/service/setup.go), [internal/api/setup.go](internal/api/setup.go), and [internal/api/middleware/setup_gate.go](internal/api/middleware/setup_gate.go). Schema lands in `migrations/010_setup_infrastructure.*`.

- **`SetupGate` middleware** ([setup_gate.go](internal/api/middleware/setup_gate.go)) 403s (`SETUP_INCOMPLETE`) every authenticated request whose org has `onboarding_complete=false`. It runs **after** auth (needs JWT claims) and exempts `DefaultSetupGateExemptPrefixes`: `/api/v1/setup`, `/health`, `/ready`, `/metrics`. Wired in [router.go](internal/api/router.go) only when `cfg.SetupService` is non-nil; when nil, both the `/setup/*` routes and the gate are skipped.
- **`SetupService`** ([service/setup.go](internal/service/setup.go)) follows the canonical one-tx-per-mutation + audit pattern. Wizard steps: company info → trades → cost codes → working calendar (+ holidays) → permit jurisdictions → complete. `Complete` enforces minimum prereqs (legal name, ≥1 trade, ≥1 cost code, a default calendar) and is idempotent. Every step writes a `setup.*` audit action so `/audit?action_prefix=setup.` reconstructs the run.
- **Bootstrap tokens** gate the first-owner claim. 32-byte CSPRNG cleartext (43-char base64url), only the sha256 hash is stored in `setup_bootstrap_tokens`. Shown once by `cmd/buildos-fork-init` / operator scripts. `cmd/server` can seed one at boot from `BUILDOS_BOOTSTRAP_TOKEN` via `SeedBootstrapTokenIfNeeded` (idempotent on the UNIQUE hash). Redemption (`RedeemBootstrapTokenForSubject`) returns a **uniform** `ErrInvalidBootstrapToken` on any failure to avoid leaking probe info, and refuses cross-org redemption without consuming the token. Default TTL 7 days.

## Build-tag posture (D8 hardening)

Two build modes:

- **Default (`go build ./...`)** — dev/CI. `internal/api/middleware/auth_dev.go` provides a `DEV_AUTH_MODE=header` bypass for local rigs.
- **Production (`go build -tags=prod ./...`)** — what the Dockerfile builds.
  - `auth_prod.go` stub replaces `claimsFromDevHeader` — `DEV_AUTH_MODE=header` is a no-op even with the env set.
  - `cmd/server` fails fast at startup if a prod binary sees `DEV_AUTH_MODE` (refusing to start beats serving uniform 401s).

Adding new dev-only auth paths? Tag them `//go:build !prod` and ship a `prod` stub that no-ops or errors loudly. Test both modes (`make test` + `make test-prod`).

## Observability

Three independent layers, all turn-on-when-configured (empty config = no-op, no error):

- **Sentry** — panics + tagged exception capture. `SENTRY_DSN` enables. PII is scrubbed via `BeforeSend` using the `internal/pii` classification catalog (see "PII handling" below).
- **Prometheus `/metrics`** — HTTP request count + duration (chi route pattern, not raw URL — bounded cardinality), HTTP error responses by error code + status class, native AI request count + duration by task kind + model + outcome, River job runs by kind + outcome, DB pool connection gauges (acquired/idle/total/max), and River queue depth + oldest-available-job age. Mount unauth (Prometheus convention); restrict via network policy.
- **OpenTelemetry traces** — `OTEL_EXPORTER_OTLP_ENDPOINT` enables. Router stack mounts `otelhttp.NewHandler` (every inbound request is a span). Default sample rate 0.1.

`internal/obs.CorrelatingHandler` wraps the JSON slog handler so every log record carries the standard correlation trio: `request_id` (from chi), `trace_id` + `span_id` (from active OTel span). Egress wrappers should always log via `*Context` variants (`InfoContext`, `WarnContext`, etc.) so the trio gets stamped.

## PII handling

`internal/pii` package owns the data taxonomy + masking. Four classifications (per SOC 2 / ISO 27001 / GDPR convention):

- **Public** — org names, UUIDs, build versions
- **Internal** — request_id, trace_id, event_type, action, resource_id, org_id (kept clear so triage works)
- **Confidential** — vendor/invoice/project names, *_cents amounts (length-preserved mask: "Acme Corp" → "A********")
- **Restricted** — emails, phones, names, GPS coords, OIDC subject, IP addresses (full redaction: "[REDACTED]"; length intentionally NOT preserved to defend against length-based fingerprinting)

`pii.FieldClass` is the central catalog of field-name → class mappings (incl. the homeowner-contact fields `client_email`/`client_phone`/`client_name`/`recipient_email` → Restricted). `pii.ScrubMap` / `pii.ScrubJSON` walk arbitrary nested data. All three consumers are wired: Sentry `BeforeSend` (Restricted threshold), audit-log JSONB scrubbing (`store.scrubAuditPayloads` in `InsertAudit`), and structured-log scrubbing (`obs.scrubAttr` in `CorrelatingHandler`).

## Secret management

`internal/config.SecretSource` interface decouples secret sources from the rest of BuildOS. `CONFIG_SOURCE` env var selects the implementation:

- `""` / `"env"` — `EnvSecretSource` (default; same behavior as direct `os.Getenv`)
- `"file:/path"` — `FileSecretSource` reads `<path>/<KEY>`. Matches kubernetes secret-mount convention.
- `"chain:a,b,..."` — `ChainSecretSource` priority-ordered fallback. Transport errors short-circuit (Vault down ≠ silent downgrade to env).

Vault / AWS Secrets Manager / GCP Secret Manager backends follow the same prefix scheme; additive in follow-up PRs.

Sensitive fields routed through the source: `DATABASE_URL`, `JWT_PRIVATE_KEY_PEM`, `JWT_PUBLIC_KEY_PEM`, `VAULT_MASTER_KEY`, `BUILDOS_BOOTSTRAP_TOKEN`, `SENTRY_DSN`, `OTEL_EXPORTER_OTLP_ENDPOINT`. Non-sensitive scalars (`PORT`, `DB_POOL_MAX`, etc.) keep direct env reads.

## Per-customer fork lifecycle

Each customer fork gets its own RSA-2048 JWT signing keypair, AES-256 vault master key, and first-owner bootstrap token. Generation is one command:

```
make fork-init OUT=./forks/acme/secrets KID=acme-2026-q2 ORG_ID=<uuid>
```

Outputs: `private.pem` (NEVER commit; the JWT signing key → `JWT_PRIVATE_KEY_PEM`), `public.pem` (committable; the JWT verification key → `JWT_PUBLIC_KEY_PEM`), `vault_master_key.txt` (NEVER commit; AES-256 vault key → `VAULT_MASTER_KEY`), `bootstrap_token.txt` (NEVER commit; one-shot first-owner claim → `BUILDOS_BOOTSTRAP_TOKEN`, redeemed at `POST /api/v1/auth/claim`), and `fork.yaml` (operator-readable: kid + fingerprint + env-var reference).

Full provisioning runbook: [docs/fork-onboarding.md](docs/fork-onboarding.md).

## Per-session handoff state

[HANDOFF.md](HANDOFF.md) at repo root is the living per-session state document. Update it at the end of every working session: what shipped, what's in flight, what's blocked, what's next. The next Claude Code session (different workstation, different time) should be able to land cold and know what to do in 60 seconds.

[.agents/handoff/NEXT_STEPS.md](.agents/handoff/NEXT_STEPS.md) is the prioritized backlog with concrete file-path entry points. Pick from the top.
