# Escalation Log

Issues flagged by Claude Code during execution that required user/Antigravity
resolution before continuing. Newest first. Resolved entries are kept for
provenance — do not delete.

---

## ESC-002 — `RequirePlanTier(pro)` 402-walls every self-minted token (post-Brain stale gate)

- **Status:** OPEN (raised 2026-06-08) — does NOT block Phase 3a; needs an owner decision
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
