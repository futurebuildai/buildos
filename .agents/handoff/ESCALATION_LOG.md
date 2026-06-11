# Escalation Log

Issues flagged by Claude Code during execution that required user/Antigravity
resolution before continuing. Newest first. Resolved entries are kept for
provenance — do not delete.

---

## DEC-001 — Object-storage substrate (Chunk A): two TECH_STACK decisions (decisions-logged, not blockers)

**Date:** 2026-06-11. **Context:** Building Chunk A of
`DAILY_REPORTS_CLIENT_UPDATES.md` (the object-storage substrate for jobsite
photos) on branch `feature/object-storage`. Two choices touch `TECH_STACK.md`
posture. Both match existing repo precedent (no new module dependency), so they
are recorded here as **decisions**, not blocking escalations — proceeding per
the spec's §A.1 escalation note and D-9 default.

1. **Hand-rolled AWS SigV4 presigner instead of `aws-sdk-go-v2`.**
   `internal/storage` implements the S3/R2 presigning (PUT/GET) + header-signed
   GET/DELETE with stdlib `crypto/hmac` + `crypto/sha256` only — **zero new
   `go.mod` dependency**. This mirrors the owner-approved hand-rolled MCP
   Streamable-HTTP client precedent (`TECH_STACK.md`: *"the MCP client is
   hand-rolled … NO new dependency … Owner-approved 2026-06-09"*). The signer is
   validated against the canonical AWS SigV4 presigned-GET test vector
   (`internal/storage/sigv4_test.go`, expected signature
   `aeeed9bb…d404`). Rejected: `aws-sdk-go-v2` (a large transitive tree for two
   signed-URL shapes; would need an owner dep-approval). Spec D-9 / §9-3 default.

2. **`internal/storage` added to the leaf-isolation allowlist.**
   `internal/storage` is a LEAF (stdlib + `crypto/*` only) declaring the
   `ObjectStore` port; the per-fork credential adapter
   (`service.NewVaultObjectStoreResolver`) lives in `internal/service`, so the
   vault and `internal/storage` never meet inside the leaf. `scripts/
   check-isolation.sh` gained **Check 4 / 4b** (storage imports no other
   internal/* package; core has no dependency on storage), mirroring the agentic
   leaf checks. `make lint-isolation` is green.

**Per-fork credentials (ADR-002):** endpoint + bucket + region resolve from
config (`OBJECT_STORE_ENDPOINT/_BUCKET/_REGION`, falling back to the deploy
workflows' `R2_ENDPOINT`/`R2_BUCKET`) through `config.SecretSource`; the access
key + secret are sealed in the existing encrypted vault under a new
`object_store` provider (`VaultService.ObjectStoreCreds`), resolved per-org at
call time exactly like the Anthropic/Resend keys. Unconfigured ⇒ soft-fail
(uploads 503 `STORAGE_UNAVAILABLE`), same posture as AI/mailer.

**Open §9 items NOT decided here (deferred to later chunks / owner):** §9-4 EXIF
(this chunk ships strip-on-serve for jpeg/png via stdlib decode→encode; WebP/HEIC
pass through pending a decoder — flagged), and all Chunk C/D/E §9 items.

---

## ESC-003 — Field equipment screen (Phase 4a-ii) is unspecified by any binding doc

### What was found (verified against the code + specs)
Phase 4a-ii ("equipment field") asked for an equipment view in the Flutter field
app, but **no binding spec defines a field-visible equipment screen.** A
4-grounder ultraplan confirmed:
- Every binding doc treats fleet as **operator-only**: API_CONTRACT §11 / the
  RBAC matrix, INFORMATION_ARCHITECTURE §3.3 (exactly five field screens),
  UX_CORE_SCREENS, and DESIGN_SYSTEM all exclude `field_worker` from fleet.
- `field_worker` (role rank 1) currently **403s on the entire `/fleet` group**
  (`RequireMinRole(superintendent)`, router.go:369). No field-facing fleet
  endpoint exists.
- The only pro-field signal is **VISION.md**'s forward roadmap, which lists
  "equipment screens" as a mobile gap to close.
- `fleet_assets` + `equipment_allocations` carry **zero monetary columns**
  (migration 003), so there is no financial data to leak — but the field
  projection still must not re-serve the operator model (`org_id`).

### Why it surfaced during Phase 4a-ii
The chunk is explicitly gated in NEXT_STEPS/HANDOFF as "CROSS-STACK + a product
decision: is fleet field-visible?". Per CLAUDE.md, a missing spec must be logged
here and paused for the owner rather than improvised.

### Options (for owner decision)
1. **Defer** — keep fleet operator-only; mark 4a-ii deferred. Spec-compliant.
2. **Read-only "equipment on my projects"** — add an equipment array to the
   existing `GET /api/v1/field/sync` envelope (which `field_worker` already
   reaches), scoped to the caller's assigned sites, via a field-safe DTO; a
   read-only More-tab screen. No new route/RBAC/migration.
3. **Interactive** (check-out/in, condition/defect, hour-meter) — net-new write
   tables + endpoints; contradicts every spec; rejected.

### Resolution (2026-06-09, owner chose Option 2)
Owner selected the read-only view (Option 2). Built on
`feat/phase-4a-ii-field-equipment` with the ultraplan (PHASE_4A_II_FIELD_EQUIPMENT.md)
as the **working spec**, with these deviations from the original draft applied
after an adversarial critique:
- **Full-set, server-wins** equipment (ignores `?since`): neither fleet table
  has `updated_at`, and relevance pivots on the allocation window + status, so a
  `created_at` delta would make the list go permanently stale. The mobile cache
  REPLACES (delete-then-fill), not upserts.
- **Scoping**: equipment on a project where the caller has a NON-completed
  assigned task (mirrors the task-pull visibility); allocation active "today"
  (`[start, end)`). Org isolation via the `projects` join (defense in depth).
- **Field-safe DTO** (`models.FieldEquipment`), never `models.FleetAsset`.

### Spec backfill owed to Antigravity (tracked, non-blocking for this build)
The owner authorized building ahead of the formal spec; Antigravity should
backfill so the docs match shipped reality: **API_CONTRACT §11** (field/sync
`+equipment` array + the field-safe DTO), **INFORMATION_ARCHITECTURE §3.3**
(fifth → sixth field screen), **UX_CORE_SCREENS** (equipment card),
**DESIGN_SYSTEM** (reconcile the nav RBAC row — field_worker now reads fleet).
Also: the field crew-checkin shape shipped as `{name, role}` (free text) vs the
API_CONTRACT's illustrative `{worker_id, …}` — same doc-drift to reconcile.

---

## ESC-002 — `RequirePlanTier(pro)` 402-walls every self-minted token (post-Brain stale gate)

- **Status:** RESOLVED 2026-06-09 (Phase 4 chunk 1) — owner chose **Option 2** (drop the vestigial pro gate). See "Resolution" below.
- **Raised by:** Claude Code (surfaced by the Phase 3a ultraplan design-critique)
- **Severity:** Functional (the AI `/api/v1/agents/*` surface is unreachable for all real callers)

### What was found (verified against the code)
Post-Brain, BuildOS mints **all** access tokens with an empty `plan_tier`
claim: `internal/service/auth.go:345` (login) and `:521` (refresh) both call
`s.issuer.Mint(..., "")`. The `RequirePlanTier` middleware
(`internal/api/middleware/plan.go:60-71`) treats an empty/unrecognized
`plan_tier` as **free** (rank 1) and returns **402 `UPGRADE_REQUIRED`** for any
route gated at `pro` (rank 3).

Net effect: every `pro`-gated route is hard-402-walled for **every** real
caller today. That includes the Phase 2c Experience endpoint
`POST /api/v1/agents/chat` (gated `RequireMinRole(superintendent)` +
`RequirePlanTier(pro)`) and the whole `/api/v1/agents/*` group
(`internal/api/router.go`). The `plan.go` doc comments still reference "Brain
populating this claim at token-issue time" — stale since the standalone pivot
(ESC-001); nothing populates it now.

### Why it surfaced during Phase 3a
Phase 3a (the agent config registry) adds an admin surface to enable/disable +
tune the harness capabilities, including `experience`. The config registry is
correct and independently valuable, but the Experience *endpoint* it governs is
currently unreachable end-to-end because of this inherited gate. 3a should not
silently ship a polished config surface for an endpoint nobody can invoke.

### Scope / non-blocking rationale
Phase 3a is **not blocked**: the config registry works regardless, and 2 of the
3 governed capabilities are reachable without any plan gate —
`delay_cascade` and `foresight` are **River worker flows** (no HTTP plan gate),
and the new admin surface `/api/v1/admin/agents` is **admin-gated, not
plan-gated**. Only the Experience HTTP path is affected, and only for its
reachability — its enable/disable config is still valid.

### Options (for owner decision)
1. **Populate `plan_tier` at mint from the org record.** Add a `plan_tier`
   column to the org (or a single-tenant fork default of `pro`/`enterprise`),
   and thread it into both `Mint(...)` calls. Restores the gate's intent.
2. **Treat the `pro` gate as vestigial post-pivot and remove it.** Single-tenant
   fork = the owner already paid for the deployment; per-request billing tiers
   were a Brain-era concept (ESC-001 dropped billing entirely). Drop
   `RequirePlanTier(pro)` from `/api/v1/agents/*` and keep role gates only.
3. **Default-mint `pro`.** Minimal change: `Mint(..., PlanTierPro)` in both
   sites until/unless a real tiering model returns.

Recommendation: **Option 2** (the pivot dropped billing; the tier gate has no
backing system), or **Option 3** as a one-line stopgap. Either is a small,
separate change from Phase 3a.

### What is blocked
Nothing in Phase 3a. This entry exists so the owner decides the gate's fate
rather than the harness silently depending on an unreachable endpoint.

### Resolution (2026-06-09, Phase 4 chunk 1 — owner chose Option 2)
Dropped the vestigial `RequirePlanTier(pro)` gate. Removed all three usages in
`internal/api/router.go` (the `/api/v1/agents` group, `/api/v1/agents/chat`, and
`POST .../schedule/recommend-adjustments`) — **role gates (`RequireMinRole`) are
retained**, only the billing-tier gate is gone. Deleted the now-dead
`internal/api/middleware/plan.go` + `plan_test.go`. The `plan_tier` claim plumbing
(`Claims.PlanTier`, the `Mint(..., planTier)` param, the dev-header 4th field,
`organizations.plan_tier`) is **kept** so a tiering model can return from git
history without a schema change. Regression guard:
`TestNewRouter_AgentsSurface_RealTokenNotPlanWalled` mints a REAL RS256 token with
`plan_tier=""` (the production shape) and asserts the agents surface is reachable
(not 402) — the prior router tests used the dev-header bypass (defaults
`plan_tier="enterprise"`), which masked the wall.

---

## ESC-001 — Brain-dependent specs contradict the standalone deployment goal

- **Status:** RESOLVED (2026-06-01)
- **Raised by:** Claude Code
- **Severity:** Architectural (blocks WS1–WS4)

### What was being implemented
The handoff specs (`API_CONTRACT.md`, `ARCHITECTURE.md`, `TECH_STACK.md`) and
`CLAUDE.md` described a hub-and-spoke model in which every BuildOS fork is a
relying party of a separate proprietary service, **The Brain**, owning five
load-bearing surfaces:

1. OIDC identity provider (login, MFA, password reset, JWKS).
2. AI gateway (Maestro) — Anthropic key, token metering, markup.
3. Hub credential vault — per-tenant 3rd-party API keys.
4. 3rd-party API proxy.
5. Billing engine.

Plus an outbound agent-to-agent (A2A) webhook emitter with a per-fork RSA-2048
JWS signer.

### What was unclear / conflicting
The product directive is to ship **"a polished standalone spoke to market, and
pause the hub-and-spoke approach for the future."** A standalone fork with no
Brain connection — per the specs' own admission — has *no auth, no AI, no 3rd-
party integrations, and no cross-product workflows*. The specs and the shipping
goal were therefore mutually exclusive: you cannot market a self-contained
deployment whose identity, AI, integrations, and billing all live in an
external service that the customer does not run.

### Options considered
1. **Ship as specced (Brain-dependent).** Rejected — contradicts the
   standalone goal; every fork would hard-depend on FutureBuild-operated infra.
2. **Stub the Brain locally.** Rejected — a mock IdP/gateway is not a product;
   it defers the same contradiction to launch day.
3. **Make every Brain surface native and admin-configurable inside BuildOS,
   with BYOK.** Selected.

### Resolution (user-approved)
Adopt option 3. BuildOS owns all five surfaces natively:

- **Auth** — native email/password (argon2id), BuildOS mints/validates its own
  RS256 JWTs (`iss=buildos`, `aud=buildos`), server-revocable opaque refresh
  tokens, bootstrap-token first-owner claim. No external OIDC/JWKS.
- **AI** — `internal/ai` calls the Anthropic Messages API directly using the
  org's BYO key from the encrypted vault. Missing/invalid key soft-fails
  (`503 SERVICE_UNAVAILABLE`); the server boots without keys.
- **3rd-party credentials** — native `internal/cryptobox` AES-256-GCM vault
  (`VAULT_MASTER_KEY`), admin-managed via `PUT/DELETE /api/v1/integrations/{provider}`.
  Raw secret bytes never leave the fork and are never returned by the API.
- **Billing** — dropped entirely. No metering, markup, or brokerage fees.
- **A2A** — removed entirely (`internal/a2a`, `internal/a2asigner`, inbound
  receiver, signer key, `A2A_*` env). `RequestVendorReview` now writes a local
  `vendor_review_requested` feed card (`201 {feed_card_id}`) instead of emitting
  a webhook.
- **Email** — native Resend via `internal/mailer` for password-reset delivery;
  Resend key set in-app via the vault; absent key soft-fails.

`cmd/buildos-fork-init` was repurposed to generate the JWT keypair + AES-256
vault master key + bootstrap token (no Brain registration step). The `go.mod`
`replace` for the sibling Brain repo was removed.

### What was blocked until resolution
WS1 (native auth), WS2 (native AI), WS3 (vault + mailer), WS4 (Brain/A2A/billing
removal), and the docs pass (API_CONTRACT, TECH_STACK, fork-onboarding, HANDOFF).
All now unblocked and executed under this resolution.

### Follow-up notes
- The future hub-and-spoke ("co-op") variant is paused, not cancelled. Removed
  code is preserved in git history (PRs #13–#26 reference the Brain-era
  implementation); see the HANDOFF.md pivot banner.
- `CLAUDE.md` still documents the Brain-era architecture in places and trails
  the pivot; treat this log + the updated handoff specs as authoritative where
  they disagree.
