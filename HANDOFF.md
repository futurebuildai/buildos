# HANDOFF — current state of play

This file is the per-session living state of the BuildOS branch. Update
it at the end of every working session so the next session (different
workstation, different Claude instance, you on Monday morning) lands
with full context in 60 seconds.

**Update protocol:** at the end of a session, edit the four sections
below: "Last shipped", "In flight", "Blocked", "Next up". Keep prose
short — bullet lists, not essays. Anchor to commit SHAs and PR numbers.

Companion docs:
- [CLAUDE.md](./CLAUDE.md) — architecture + conventions (slow-changing)
- [.agents/handoff/ADR-001](./.agents/handoff/ADR-001-vision-alignment.md), [ADR-002](./.agents/handoff/ADR-002-single-tenant-fork-model.md) — strategic decisions
- [.agents/handoff/PRODUCTION_READINESS_PLAN.md](./.agents/handoff/PRODUCTION_READINESS_PLAN.md) — phase plan
- [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md) — prioritized backlog with concrete entry points
- [docs/fork-onboarding.md](./docs/fork-onboarding.md) — per-customer fork provisioning runbook

---

## Last shipped (most recent → older)

- **2026-05-05** PR #20 [`715af97`] S3 7.2 caller wiring: `ProcurementService.RequestVendorReview` is the first real consumer of the `a2a.Emitter` shipped in PR #19. Opens one tx, verifies item ownership (cross-org isolation via existing `GetProcurementItem`), calls `EmitReviewMaterialQuote` (`InsertTx` on the same tx so enqueue commits/rolls back atomically with audit), records one audit row keyed by `procurement_item_id` (`procurement.vendor_review.requested`), returns the emitter's idempotency key for caller correlation. New `VendorReviewEmitter` consumer interface (defined in procurement package — service doesn't pin the full `*a2a.Emitter` surface). Constructor signature grew to 5 args: pool, items, maestro, emitter, audit. `cmd/server` constructs `*a2a.Emitter` once over the shared River insert client; `cmd/worker` passes nil (daily `RecomputeStatuses` sweep doesn't trigger operator-driven review flows). New `ErrA2AEmitterUnavailable` sentinel mirrors `ErrMaestroUnavailable`. Validation split: service validates org/item/emitter-presence; wire-shape validation (vendor non-empty, total_cents non-negative, currency_code USD/CAD) delegated to emitter, its `ErrInvalidArgs` wrapped as `ErrInvalidInput` so handlers see the uniform service-layer sentinel. Audit metadata captures idempotency_key + structured fields (vendor, total_cents, currency_code, rfq_id, has_reasoning bool — Reasoning text excluded as Confidential narrative). 2 new tests (validation matrix; nil-emitter sentinel); happy-path with real tx deferred to integration suite. **No labor-bid wiring** — BuildOS doesn't have a labor RFQ surface yet (S6 SubLiaisonAgent territory); `EmitReviewLaborBid` stays unused but emitter test coverage from PR #19 keeps it live. `make audit` ALL PASSED. CPM80=182µs, CPM200=423µs.
- **2026-05-05** PR #19 [`9dc9eae`] S3 Session 7.2: outbound A2A emitter package. New `internal/a2a` package (depends on `worker` + `river` only — pointedly NOT `internal/service`, to avoid the import cycle that would form when domain services and `service/a2a.go` both want to call into the emitter). `Emitter.EmitReviewMaterialQuote(ctx, tx, args) (uuid.UUID, error)` and `Emitter.EmitReviewLaborBid(ctx, tx, args) (uuid.UUID, error)` validate inputs, marshal a typed JSON payload, and `InsertTx` a `worker.A2AWebhookDispatchArgs` on the supplied `pgx.Tx` so enqueue commits / rolls back atomically with the surrounding domain mutation. Returns the freshly-minted idempotency key for caller correlation. The actual JWS signing + HTTPS POST happens later in the River worker via the already-shipped `service.A2AOutboundService.DeliverA2AWebhook`. `Enqueuer` interface decouples tests (`fakeEnqueuer` captures the dispatch args) from a real River client; compile-time assertion `var _ Enqueuer = (*river.Client[pgx.Tx])(nil)` keeps the prod-side wiring honest. Wire-shape detail: `rfq_id` is `*uuid.UUID` (not `uuid.UUID`) in the JSON payload so `omitempty` actually drops the field when `uuid.Nil` is passed — Brain's ETL/dedup needs to distinguish "absent" from "nil-uuid". Event-type constants (`review_material_quote`, `review_labor_bid`) duplicated from `service/a2a.go` by design (zero coupling). Validation up-front and pure: nil `org_id`, empty `vendor`/`bidder`, negative amounts, unsupported `currency_code` all return `ErrInvalidArgs` with no tx interaction. 9 test functions: nil-`Enqueuer` panic, validation matrix (table tests, both events), envelope shape, omitempty contract for rfq_id/reasoning/timeline, enqueue-error propagation. `make audit` ALL PASSED. CPM80=181µs, CPM200=440µs. Caller wiring (procurement service → emitter; pipeline service → emitter) lands in a follow-up.
- **2026-05-05** PR #18 [`c6c86c1`] S3 Session 7.1: Procurement Maestro `RecommendVendors` + migration 009. New `MaestroProcurementRecommender` interface (typed `procurement_recommend` task surface) wired into `ProcurementService` via a nil-tolerant constructor — `cmd/server` passes `brainClient.Maestro`; `cmd/worker` passes `nil` (daily `RecomputeStatuses` tick has no recommendation surface, so `RecommendVendors` short-circuits with `ErrMaestroUnavailable`). `RecommendVendors` opens one tx, verifies item ownership via new `GetProcurementItem` store method, calls Maestro with the item's `estimated_cost_cents` as budget context, persists each ranked vendor via new `CreateProcurementRecommendation` store method (all share `resp.RunID`), and writes one batch audit row keyed by `procurement_item_id` with metadata `{run_id, recommendation_count, tokens_used, cost_cents, currency_code}`. New migration `009_procurement_recommendations` follows Composite Currency Pattern (`predicted_spend_cents` + `predicted_spend_currency_code`); `confidence_pct SMALLINT` (0..100) instead of float per no-floats culture — Maestro's float64 `Confidence` is rounded * 100 with NaN/negative/>1 clamping (defends against a buggy Brain response violating CHECK + rolling back the tx). `vendor_id` is nullable, no FK (vendors table not yet modeled). 3 indexes (item+created_at DESC, run_id, org+created_at DESC) all single-line + `-- buildos:lock-ok:` (fresh table). New audit constant `AuditResourceProcurementRecommendation`, model struct `models.ProcurementRecommendation`, sentinel `ErrMaestroUnavailable`. `fakeProcurementRecommender` test double + 3 new tests (validation gate, nil-Maestro path, `confidenceToPct` table test covering edges). `make audit` ALL PASSED. CPM80=183µs, CPM200=431µs.
- **2026-05-05** PR #17 [`62bf293`] S2 Session 5.1: outbound `buildos.prospect_promoted` A2A event on PROSPECT → PROJECT atomic promotion. Extends `transitionToPermitIssued` to enqueue an `A2AWebhookDispatch` River job inside the same tx as the project insert + prospect update + HydrateProject job — Brain only ever receives the announcement for a project that committed. Payload is feed-card-shaped per ADR-001 D14 (`card_type=pipeline.prospect_promoted`, title, body, priority, empty actions, metadata{prospect_id, project_id, org_id, gsf, permit_issued_date RFC3339, optional address}). `address` is omitted when nil so Brain's dedup/ETL distinguishes absent from empty. `EventTypeProspectPromoted` is exported as the cross-repo event_type identifier. New helper + struct are package-private. Tests assert explicit envelope shape (renamed JSON key fails loud) plus the nil-address omission contract. Full transition path stays deferred to integration suite. `make audit` ALL PASSED. CPM80=187µs, CPM200=425µs.
- **2026-05-05** PR #16 [`6ee6752`] S1.5 part 4 / Session 3.4 (closes S1.5; **triggered Gate 1; ratified by owner 2026-05-05**): wire typed `Maestro.DailyBriefing` into `agents.GenerateDailyBriefing` + per-call billing-audit row. `MaestroChatter` (free-form `Chat`) replaced with `MaestroDailyBriefer` (typed `DailyBriefing(ctx, brain.DailyBriefingRequest) (*brain.DailyBriefingResponse, error)`). The local `buildDailyBriefingPrompt` helper is deleted — Brain's `daily_briefing` task owns the prompt template per ADR-001 D5; BuildOS only assembles the structured context (`Tasks`, `Alerts`, `UserRole`). After a successful Maestro call, a short standalone tx writes one audit row with `action="ai.daily_briefing.invoked"`, `resource_type="ai_run"` (new `AuditResourceAIRun` constant), `resource_id=resp.RunID`, and metadata JSON `{run_id, session_id, tokens_used, cost_cents, currency_code, task_count, alert_count, task}`. This is the per-call meter that rolls into the org's monthly AI usage line. Audit is best-effort — `AuditService.Record` log-and-swallow path keeps the briefing from being held back if the audit insert fails. `NewAgentsService` constructor takes `AuditRecorder` (nil → `NoopAuditRecorder`); `cmd/server/main.go` wired. Tests: typed `fakeBriefer` for the new interface, nil-audit-fallback regression, validation-only paths (the three pre-existing `buildDailyBriefingPrompt` tests dropped — function gone). `make audit` ALL PASSED. CPM80=184µs, CPM200=438µs.
- **2026-05-05** PR #15 [`543eac6`] S1.5 part 3 / Session 3.3: Brain Hub credential proxy stubs (ADR-001 D6). New `internal/brain/hub.go` adds `HubClient.GetCredential(ctx, GetCredentialRequest) (*Credential, error)` and `HubClient.RefreshIfExpired(ctx, credentialID) error`. Mode is fork-static via `Config.HubDirectMode` (driven by `BRAIN_HUB_DIRECT` env): proxy mode (default) returns `Credential.ProxyHandle` only — raw secret never enters the fork; direct mode appends `?mode=direct` and Brain returns `Credential.Secret` if its policy gate authorizes the fork. Wired into `Client.Hub` alongside `Maestro` / `Billing` so it inherits S1.5 part 1 retry + breaker + per-method timeouts. `Credential.Secret` is Restricted-class — slog (PR #11) + audit (PR #10) scrubbers already mask it on egress. Validation rejects empty provider / `uuid.Nil` credentialID client-side. Empty Scope auto-defaults to `"default"`. 11 new tests cover both modes, empty-scope default, custom-scope path-escape, 404 → ErrNotFound, 502 → ErrTransient, fork-static mode independence. Sets up Session 3.4 (DailyBriefing wiring + Gate 1 trigger).
- **2026-05-04** PR #14 [`55325d0`] S1.5 part 2 / Session 3.2: typed Maestro task envelopes (ADR-001 D5). New `internal/brain/maestro_tasks.go` adds the typed `/v1/ai/tasks` surface alongside the existing session-based `Chat`. Five typed methods (`DailyBriefing`, `IntentClassify`, `InvoiceExtract`, `ProcurementRecommend`, `TribunalReview`) each route through one shared `runTask()` helper that handles the discriminated `{task, input}` envelope. Every response carries an embedded `CostMetadata{run_id, tokens_used, cost_cents, currency_code}` per ADR-001 D5 cost-tracking shape so callers can write a per-call billing-audit row. Composite Currency Pattern preserved everywhere monetary fields appear (invoice total, line items, vendor predicted-spend, AI run cost). Client-side validation matches existing pattern (utterance/dispute_id/material_request_id required; either document_url or text). 11 new tests cover round-trip envelope encoding, cost metadata propagation, validation rejection without server hit, and `*HTTPError` chain through the typed wrapper. Cross-repo blocking edge: Brain-side OpenAPI fixture for `/v1/ai/tasks` not yet published — contract tests deferred. Sets up Sessions 3.3 (Hub stubs / D6) and 3.4 (DailyBriefing wiring + Gate 1).
- **2026-05-04** PR #13 [`89b671e`] S1.5 part 1 / Session 3.1: brain-client resilience layer. (a) In-house circuit breaker (`internal/brain/resilience.go`, ~80 LOC) with closed→open→half-open state machine, generation-counter for stale-outcome safety, defaults 5 failures / 60s window, 30s open. No external `gobreaker` dep per `.agents/TECH_STACK.md` policy. (b) Per-method timeouts via `Config.Timeouts` (Maestro 30s, Billing 5s); caller's tighter ctx deadline wins. (c) Retry defaults retuned to plan spec: 3 attempts, 200ms base, 4× multiplier (200→800→3.2s spacing). 4xx (client error) does NOT count toward breaker threshold. New `ErrCircuitOpen` sentinel for service-layer 503 mapping. 8 new resilience tests; race-clean. Sets up Sessions 3.2 (Maestro typed envelopes / D5), 3.3 (Hub proxy stubs / D6), 3.4 (Daily Briefing wiring + Gate 1).
- **2026-05-04** PR #12 [`376eefa`] Tier 1 #3 / D7 wave 3: schedule + pipeline transition audit recording. `ScheduleService.RecalculateSchedule` writes `schedule.recalculated` audit row inside the recalc tx with `{task_count, critical_path_size, compute_ms, project_end}`. `PipelineService.AdvanceProspect` / `LoseProspect` / `transitionToPermitIssued` write `pipeline.stage_transitioned` inside the existing tx with before/after stage + probability_pct (project_id metadata for permit-issued, reason for lose). Both constructors take `AuditRecorder` (nil → noop), matching the Fleet/Procurement/Field/Budget pattern. API handlers extract `claims.Sub` via `mw.MustClaimsFromContext` and pass through. New `AuditResourceSchedule` constant. Mock services + happy-path tests assert `user_sub` propagates. Closes the audit-trail gap for the two highest-mutation surfaces (CPM recalcs + pipeline stage transitions).
- **2026-05-04** PR #11 [`e9edb96`] Tier 1 #2: slog Restricted-class PII attribute scrub via `obs.CorrelatingHandler`. `Handle` rebuilds the `slog.Record` to mask attrs whose key matches `pii.FieldClass` at Restricted (email, phone, gps_*, oidc_subject, ip_address, etc.). Group attrs recurse. `WithAttrs`-baked attrs are also scrubbed so long-lived loggers can't smuggle PII. Confidential and below pass through unchanged (vendor names, *_cents, request_id/trace_id/span_id retained for triage). 6 new test functions (~14 catalog cases). Completes the 3-leg PII-egress trio: Sentry (PR #6) + audit JSONB (PR #10) + slog (PR #11).
- **2026-05-04** PR #10 [`92231c0`] Tier 1 #1: audit-log JSONB Restricted-class PII scrub. `AuditStore.InsertAudit` now wraps Before/After/Metadata in `pii.ScrubJSON(blob, pii.Restricted)` before INSERT. Confidential-class fields (vendor, *_cents, project) intentionally preserved for investigative value. 5 unit tests + 1 integration round-trip test.
- **2026-05-04** PR #9 [`6119ab3`] CI + release workflows activated at `.github/workflows/` (5 commits: relocation, gofmt sweep across repo, Trivy action `v0.36.0` real-tag pin, Dockerfile alpine bump 3.20→3.22). All 6 CI jobs green on first activation. Plus L8 SRE audit gate added to PR description protocol.
- **2026-05-04** PR #8 [`chore/workstation-switch`] CI workflow YAMLs recovered to `docs/ci-templates/` + workstation-switch checklist added to this file.
- **2026-05-01** PR #7 [`8ea5a3e`] HANDOFF.md + NEXT_STEPS.md + CLAUDE.md refresh — cross-session continuity docs.
- **2026-05-01** PR #6 [`d71a98f`] PII classification + Sentry BeforeSend masking. `internal/pii/` package + `obs.scrubSentryEvent`. Closes the GDPR/CCPA gap on Sentry egress.
- **2026-05-01** PR #5 [`ca36fa3`] OpenTelemetry tracing — `internal/obs/tracer.go`, brain-client `otelhttp` wrap, router middleware, log-correlation (trace_id+span_id alongside request_id).
- **2026-05-01** PR #4 [`7facfce`] `cmd/buildos-fork-init` keypair generator + `docs/fork-onboarding.md`.
- **2026-05-01** PR #3 [`29ea6fe`] SecretSource abstraction (env/file/chain) + `LoadWithSource()`.
- **2026-05-01** PR #2 [`699a64d`] Sprints 1-5 + Phase F core (44 commits — domain endpoints, Brain integration, production hardening, Dockerfile, D8 build-tag hardening).

19 PRs merged under the L8 self-audit gate (PR #9 added the L8 SRE
audit gate as a second checklist applied per PR). Every landed
commit had `make audit` green + integration suite green +
govulncheck clean. PRs #9 onward also have CI green at merge time
(no longer just local audit).

## In flight

Nothing on a branch waiting for review right now.

**Gate 1 ratified by owner (2026-05-05).** S1.5 Brain Client
Foundation is locked in: D5 typed Maestro envelopes
(`internal/brain/maestro_tasks.go`) and D6 Hub proxy/direct mode
(`internal/brain/hub.go`) are now stable cross-repo contracts.
Maestro-touching sprints (S2-S6) cleared to proceed. **S2 Session
5.1 shipped (PR #17)** — PROSPECT → PROJECT atomic promotion now
emits `buildos.prospect_promoted` A2A event for Brain to disseminate.
**S3 Session 7.1 shipped (PR #18)** — `ProcurementService.RecommendVendors`
calls Brain Maestro `procurement_recommend` and persists each
ranked vendor + a batch audit row inside one tx; migration 009
lands the `procurement_recommendations` table per Composite Currency
Pattern. **S3 Session 7.2 shipped (PR #19)** — new `internal/a2a`
package with typed `EmitReviewMaterialQuote` / `EmitReviewLaborBid`
methods for domain services to enqueue outbound A2A events on a
`pgx.Tx`; signing + delivery happens later in the River worker
via the already-shipped `service.A2AOutboundService`. **S3 7.2
caller wiring shipped (PR #20)** — `ProcurementService.RequestVendorReview`
is the first real consumer; opens one tx, ownership-checks, emits,
audits, returns idempotency key. `cmd/server` constructs the
`*a2a.Emitter` over the shared River insert client and threads it
through. Labor-bid wiring deferred to S6 (no labor RFQ surface
exists yet). S2 Session 5.2 was already materially shipped in
production code (handlers/RBAC/tests); next up: Vault SecretSource
backend (Tier 2 #4) or S3 Session 8.1 (A2A receiver idempotency
replay tests).

## Blocked

Nothing blocked right now. (PR #9 cleared the workflow-activation
blocker; CI is now live on `.github/workflows/{ci,release}.yml`.)

Known follow-up surfaced by PR #9 (not blocking, queued):

- `make audit` is weaker than CI: doesn't run `gofmt -l` strictly
  and doesn't smoke-build the Dockerfile. Add both to the local
  audit so future sessions catch issues before CI does.

## Next up (prioritized — pick from the top)

See [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md)
for the full prioritized backlog with entry-point file paths.

Top three an L8 PE would queue (S1.5 + S2 5.1 + S3 7.1 + S3 7.2 +
S3 7.2 caller wiring shipped; S2 5.2 materially complete in prod
code; clear to compound on the Maestro/A2A track):

1. **Vault SecretSource backend** (Tier 2 #4) — `internal/config/vault.go`
   implementing the `SecretSource` interface for HashiCorp Vault.
   Required before first-fork cutover (Phase H). Independent of
   Maestro/A2A track — can run in parallel. Use `hashicorp/vault/api`;
   path scheme `vault://kv/data/buildos/<fork>/<key>`. Wire into
   `LoadWithSource` chain. Tests: spin up Vault dev mode in a
   testcontainer; round-trip a secret.
2. **S3 Session 8.1 — A2A receiver idempotency replay tests.**
   `internal/api/a2a_test.go`. Same envelope twice with same
   `X-Idempotency-Key` → second returns `already_processed: true`
   with same `feed_card_id`. Unknown `event_type` → `400 UNKNOWN_EVENT`.
   Body > 1 MiB → `413 PAYLOAD_TOO_LARGE`. Sets up Session 8.2
   (LocalBlue lead-captured wiring; Gate 2 trigger).
3. **HTTP handler / RBAC for `RequestVendorReview`** — small
   follow-up to PR #20. Add a `POST /api/v1/projects/{projectID}/procurement/{itemID}/request-review`
   handler that calls `ProcurementService.RequestVendorReview`,
   gated to `superintendent` or higher. Returns `{idempotency_key}`
   on 202. Integration test asserts the river_job row hits with
   the correct event_type and payload. Closes the loop so an
   operator can actually trigger a review request from the API.

## Working agreement (L8 self-audit gate)

The owner delegated the merge gate to Claude when they said "merge
any commits should they pass your L8 quality and scope alignment
gate." This means a session can:

1. Open a branch
2. Ship work to L8 quality
3. Self-audit (build + vet + lint + test + integration + govulncheck
   + composite-currency lint + bench-physics; per-commit messages
   PR-quality; backward compat preserved or breaking change documented)
4. Open a PR with a real description
5. Merge it via `gh pr merge N --merge` if the gate passes

Don't merge if any of: tests failing, vet failing, govulncheck
flagging, integration regression, composite-currency violation, or
the change has a meaningful security implication you haven't
explicitly thought through.

When in doubt, open the PR but don't merge — let the human review.

## Things to NOT do

- **Don't add Postgres RLS** (per ADR-002). Single-tenant fork model =
  tenant isolation through deployment isolation. RLS would add 3-10%
  query overhead for zero security benefit.
- **Don't enable per-tenant rate limiting**. Per-IP is enough; one
  tenant per deployment.
- **Don't use `git push origin --force` or rewrite published history**
  on main. Force-push to feature branches only when strictly necessary.
- **Don't commit secrets**. The fork-init tool writes `private.pem`
  with mode 0600; document that it goes into a secret store, not the
  repo.
- **Don't try to push `.github/workflows/*`** with the current Claude
  Code OAuth token — it doesn't have `workflow` scope. Push from a
  user clone or via the UI.
- **Don't break composite-currency invariants**. Every monetary column
  pairs `*_cents BIGINT` with `*_currency_code VARCHAR(3)`. The
  migration linter is hard CI.
- **Don't bypass D8** by adding new dev-only auth paths without the
  `//go:build !prod` tag.

## Cross-repo coordination (BuildOS ↔ The Brain)

These items live across both repos; tracked here because they're easy
to forget when working on one side only. The Brain repo lives at
`../futurebuild-brain` (per `replace` directive in `go.mod`).

| Item | Side | Status | Notes |
|---|---|---|---|
| OIDC issuer + JWKS | Brain | live | stable wire protocol; `iss="fb-brain"` `aud="fb-os"` legacy values, do not rename without coordination |
| A2A inbound webhook | BuildOS | live | `/api/v1/a2a/webhook`; JWS-verified |
| A2A outbound webhook | BuildOS | live (signing key per fork) | each fork signs with its own RSA-2048; public key registers in Brain's JWKS |
| `WebhookEvent.OrgID` field | Brain | optional today, should become required | when Brain enforces this, BuildOS continues to send it as it does now |
| Maestro Chat | Brain | live | called from `internal/service/agents.go` DailyBriefing |
| Billing usage | Brain | live | proxied at `/api/v1/billing/usage` |
| LocalBlue → Brain → BuildOS | Brain | partial | BuildOS handler shipped (`internal/service/a2a.go` `handleLocalblueLeadCaptured`); Brain-side type definitions deleted 2026-05-04 (orphan branch never merged). When Brain emitter wiring resumes, re-derive `LocalblueLeadCapturedPayload` from BuildOS's `localblueLeadCapturedPayload` struct as the canonical reference. |
| Stripe billing engine | Brain | not yet | gating G1 |
| Vault / SecretsManager backends for SecretSource | BuildOS | env+file shipped; vault next when first customer fork needs it | `internal/config/secrets.go` interface ready |

---

## Workstation switch checklist

Use when picking BuildOS up on a fresh workstation (e.g. switching
from a travel laptop to a primary dev box). The repos themselves are
already clean — this list is for the new environment.

### One-time setup on the new workstation

```bash
# 1. Clone both repos as siblings (the go.mod replace directive expects
#    futurebuild-brain at ../futurebuild-brain relative to buildos):
mkdir ~/repos && cd ~/repos
git clone https://github.com/futurebuildai/buildos.git
git clone https://github.com/futurebuildai/futurebuild-brain.git

# 2. Toolchain:
#    - Go 1.26+        (for build + tests)
#    - Docker           (for testcontainers-backed integration tests)
#    - golangci-lint    (for `make lint`)
#    - govulncheck      `go install golang.org/x/vuln/cmd/govulncheck@latest`
#    - gh CLI           (for PR ops)
#    - make

# 3. gh auth with workflow scope from the start. This unblocks the
#    `.github/workflows/` push that the Mac session couldn't do:
gh auth login                # follow prompts; choose HTTPS or SSH
gh auth refresh -h github.com -s workflow

# 4. git identity:
git config --global user.name "Your Name"
git config --global user.email "you@example.com"

# 5. Verify everything builds + tests:
cd ~/repos/buildos
make audit
make test-integration       # spawns Postgres via testcontainers — needs Docker

# 6. Activate the CI workflows that have been waiting in docs/ci-templates/
git checkout -b ci/activate-workflows
git mv docs/ci-templates/ci.yml      .github/workflows/ci.yml
git mv docs/ci-templates/release.yml .github/workflows/release.yml
git rm docs/ci-templates/README.md
git commit -m "ci: activate CI + release workflows"
git push -u origin ci/activate-workflows
gh pr create --base main --title "ci: activate CI + release workflows"
```

### What to read first

1. [HANDOFF.md](./HANDOFF.md) (this file) — top-of-file sections give
   you 60 seconds of "what just happened, what's next."
2. [CLAUDE.md](./CLAUDE.md) — architecture, conventions, hard CI gates.
3. [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md) —
   pick from Tier 1 to start work.
4. [.agents/handoff/ADR-002-single-tenant-fork-model.md](./.agents/handoff/ADR-002-single-tenant-fork-model.md)
   — the most recent strategic decision. Keep this top-of-mind: BuildOS
   is single-tenant per customer fork, not multi-tenant SaaS.

### Things you don't need to bring with you

- **The worktree** — every commit is on the remote.
- **Local secrets** — `.env` files are gitignored. The
  `internal/config.SecretSource` abstraction means a fresh workstation
  with its own `.env` (or no `.env`, with `CONFIG_SOURCE=file:/path`)
  works identically.
- **Local generated artifacts** — `bin/`, `bin/prod/`, `*.test`. All
  rebuildable.

### Things to NOT carry over

- Any `private.pem` from `make fork-init` runs. Those go straight into
  a customer's secret store, never into a worktree on a personal
  laptop.
- Any tokens or credentials baked into local shell history. Audit your
  `~/.bash_history` / `~/.zsh_history` before disposal if you're
  recycling the old machine.

---

## Appendix: workflow YAML (if not in worktree)

The CI + release YAMLs are tracked at
[`docs/ci-templates/`](./docs/ci-templates/) until a session with
`workflow` OAuth scope relocates them to `.github/workflows/`. See
that directory's README for activation steps. Summary of what they do:

**`ci.yml`** mounts six jobs on every PR + push to main:
- lint-migrations (5 rules) + linter regression suite
- gofmt + go vet (both default and `-tags=prod`)
- govulncheck CI-blocking (both build modes)
- unit tests + prod-mode test
- integration tests (Testcontainers + Postgres)
- bench-physics (CPM gate)
- multi-arch docker build (linux/amd64 + linux/arm64) + Trivy scan

**`.github/workflows/release.yml`** fires on tag push (`v*.*.*`):
- multi-arch image to GHCR
- cosign keyless OIDC signing (no signing key to manage)
- syft SBOM in CycloneDX + SPDX, attached via `cosign attest`
- GitHub Release with auto-changelog from `git log`
