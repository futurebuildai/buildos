# Vision Alignment — BuildOS in the FutureBuild Ecosystem

**Status:** DRAFT — captures my current understanding for owner sign-off before further build-out.
**Last updated:** 2026-04-29
**Supersedes for-naming:** `VISION_BRIEF.md` (kept; legacy product names still appear there).
**Companion:** `WORKSPACE_STATE.md` in the `futurebuild-ecosystem` repo is the canonical pipeline ledger; this doc is the *post-rebrand product framing* the engineering team works from.

---

## 1. The ecosystem in one picture

```
                       ┌───────────────────────────────────────┐
                       │              The Brain                │
                       │   (proprietary; FutureBuild AI)       │
                       │  ─────────────────────────────────    │
                       │  • OIDC IdP (RS256, JWKS, MFA)        │
                       │  • Maestro AI gateway (Anthropic)     │
                       │  • Hub credential vault (per-tenant)  │
                       │  • MCP registry + 3p API proxy        │
                       │  • Billing engine (markup + fees)     │
                       │  • A2A webhook bus (JWS-signed)       │
                       └───┬───────────┬───────────┬───────────┘
                           │           │           │
                  ┌────────▼─┐   ┌─────▼──────┐   ┌▼────────────┐
                  │ BuildOS  │   │  GableLBM  │   │  LocalBlue  │
                  │ (open    │   │ (open ERP  │   │ (SaaS lead  │
                  │  source) │   │  for LBM)  │   │  gen + site)│
                  │ per-     │   │ per-yard   │   │ per-trade   │
                  │ customer │   │ fork or    │   │ contractor  │
                  │ fork     │   │ multi-     │   │ multi-tenant│
                  │          │   │ tenant     │   │ SaaS        │
                  └──────────┘   └────────────┘   └─────────────┘
```

**One-line positioning** (futurebuild.ai): *"Open source tools connected by proprietary AI. Built for the builders."*

The open-source surfaces (BuildOS, GableLBM) are the customer's data and execution plane. The Brain is the network effect: identity, AI, integrations, money flow. A customer who unplugs from The Brain keeps their code and data but loses auth, AI, 3rd-party integrations, billing, and cross-product workflows.

---

## 2. BuildOS product scope

**Audience:** family-owned & SMB residential general contractors managing 5–50 active projects in the $500K–$5M range (1500–6000 GSF). USD and CAD.

**Three workspaces** (per `INFORMATION_ARCHITECTURE.md`):

| Workspace | Surface | Primary users |
|---|---|---|
| Portfolio Dashboard | Web (Lit + Industrial Dark theme) | Owner, Admin |
| Agent Command Center | Web + Tablet | Superintendent, Admin, AI agents |
| Field Portal | Flutter, offline-first (Drift) | Field worker, subcontractors |

**Lifecycle covered** (post-2026-04-02 scope expansion):

```
Lead → Prospect → Estimate → Permit → [Atomic transition] → CPM Project → Field Execution → Closeout
        ╰────── pre-construction pipeline (Kanban) ──────╯ ╰── deterministic CPM-res1.0 ──╯
```

The atomic Kanban→CPM transition at `PERMIT_ISSUED` is a load-bearing architectural decision: it preserves CPM determinism while still letting the system manage probabilistic pre-permit work.

**P0 features (M1–M5, then M11):**
- M1: Deterministic CPM scheduling (gonum DAG, integer ns, FS/SS/FF/SF deps)
- M2: Composite Currency Pattern financials (USD + CAD)
- M3: Procurement automation w/ autonomous agent recommendations
- M4: Daily Briefing agent + Tribunal-governed approvals
- M5: Field Portal sync (Flutter, offline-first, FIFO outbox)
- M11: Pre-Construction Pipeline (added post-Stage 06)

---

## 3. Per-customer deployment model

**Confirmed by owner (this conversation):**

> Each customer has their own repo/instance and owns their core code and data. We might offer a multi-tenant version if hired by a co-op to do so for their membership.

This is **fork-per-customer** as the default, with an optional multi-tenant variant for co-ops. Implications:

| Concern | Single-customer fork (default) | Co-op multi-tenant |
|---|---|---|
| `org_id` filter | One stable org id; could even hardcode | Real multi-tenant per-row filter — `org_id` becomes load-bearing |
| Branding | Customer-specific theming/strings allowed | Co-op brand or sub-brand per member org |
| Brain client registration | One OIDC client per fork | One OIDC client representing the co-op; tenants identified by `org_id` claim |
| Currency | Usually one (USD or CAD) | Mixed; SQL linter already requires currency_code on every row, so safe |
| Upstream upgrades | Customer fork merges from `main` | Co-op operator merges from `main` |

The codebase **already** assumes `org_id` filtering on every query — this is a feature, not a bug. Single-customer forks pay essentially zero overhead for the multi-tenant capability.

**What is *not* yet defined and needs decision:**
- The fork creation workflow (template repo? `gh repo create --template`? a brand-kit overlay?)
- The upstream-upgrade story (does FutureBuild AI run merges, or does the customer?)
- The deployment model (self-hosted, FutureBuild-managed, Railway-as-a-service?)

---

## 4. The Brain dependency — the contract in detail

| Surface | Wire | Today | Risks |
|---|---|---|---|
| Identity | OIDC RS256 JWT, JWKS at `BRAIN_JWKS_URL`, 5-min cache | `iss="fb-brain"`, `aud="fb-os"` literals — **legacy wire values** | Renaming requires coordinated change across both repos; not a unilateral decision |
| AI gateway (Maestro) | REST through The Brain; OS never holds `ANTHROPIC_API_KEY` | **Not yet wired in BuildOS** — Sprint 1 stubs do not call AI | Maestro contract not yet defined in `API_CONTRACT.md` from BuildOS side |
| Hub credential vault | The Brain holds AES-256-GCM-encrypted per-tenant 3rd-party keys | **Not yet wired in BuildOS** | BuildOS has no calls to Hub yet; procurement integrations stubbed |
| 3rd-party API proxy | OS calls Brain endpoints; Brain resolves credentials & calls upstream | **Not yet wired** | The 7 MCP servers (GableERP, LocalBlue, XUI, QuickBooks, 1Build, Gmail, Outlook) will be reached this way |
| Billing | Brain meters AI tokens, applies markup, charges via Stripe (assumed) | `AI_MARKUP_BPS=10000` (100%), `TRANSACTION_FEE_BPS=10` (0.1%), `BROKERAGE_FEE_BPS=150` (1.5%) | Revenue model lives in The Brain; BuildOS has no billing surface |
| A2A webhooks | Brain → OS at `/api/v1/a2a/webhook`, JWS-signed | Receiver **stubbed** in [a2a.go](../../internal/api/a2a.go), not yet processing | 5 event types defined in `API_CONTRACT.md` |

**Standalone-without-Brain compatibility plane** (what we just built):
- `DEV_AUTH_MODE=header` for dev/CI — no JWT, no JWKS
- `cmd/dev-idp` for staging and sales demos — full RS256 token issuance with personas
- These cover the **identity** surface only. AI / Hub / 3p / billing / A2A all still require The Brain in any non-trivial scenario.

---

## 5. Cross-product flows (open questions)

The website lists three open-source surfaces (BuildOS, GableLBM, LocalBlue is SaaS) connected by The Brain. The handoff docs are silent on the cross-product user journeys. These are the high-value flows worth defining:

### 5a. Contractor (BuildOS) ordering from a lumber yard (GableLBM)

A BuildOS user creates a procurement RFQ. The yard runs GableLBM. Two possible architectures:

**Option A — Brain proxy:** BuildOS asks Brain → Brain routes to the yard's GableLBM via the MCP `GableERP` server → yard responds → Brain bills both sides.

**Option B — Direct B2B:** BuildOS has a direct integration to GableLBM via a Brain-issued service account.

Today's `MCP registry` model implies Option A. Need to confirm.

### 5b. Lead (LocalBlue) becoming a Prospect (BuildOS)

LocalBlue chatbot captures a lead at a contractor's site. Does it:
- POST directly to the contractor's BuildOS pipeline endpoint (using a Brain-issued service token)?
- Send via A2A webhook (LocalBlue → Brain → BuildOS)?
- Sit in LocalBlue and require manual import?

This is the **revenue-generating top-of-funnel** for BuildOS contractors. Worth defining early.

### 5c. AI-driven cross-product workflow (Maestro orchestrating both)

`MaterialsFlow` (Brain orchestrator) coordinates: BuildOS RFQ → GableERP catalog query → LocalBlue lead enrichment → quote return. Currently spec'd in The Brain's `.agents/handoff/`. BuildOS needs the matching A2A receiver work to complete the loop.

---

## 6. Where we are vs the plan

| System | Plan | Reality |
|---|---|---|
| BuildOS | 8 backend sprints + 1 frontend (S0–S8), 18 weeks | **S0–S1 landed:** scaffold (Sprint 0) + CPM physics engine + benchmark gates + transactional schedule service (Sprint 1). Most domain handlers are stubs. |
| The Brain | 6 sprints (S0–S5), 12 weeks | Per ecosystem state, Stages 00-08 planning complete; implementation status unknown from this repo |
| Cross-system | Brain S0 blocks OS S0; Brain S3 blocks OS S5 | OS S0 unblocked because OS validates JWTs against any compliant JWKS — `cmd/dev-idp` removes the cross-team blocker for early sprints |

Recent rebrand work in this repo (commits `47bce30`, `d46ac52`):
- "FutureBuild OS" → "BuildOS"; "FB-Brain" → "The Brain"; "FB-OS" → "BuildOS" across 23+ files
- Per-customer fork model documented in [CLAUDE.md](../../CLAUDE.md)
- 5-surface Brain dependency documented
- Alt-auth implemented (`DEV_AUTH_MODE=header` + `cmd/dev-idp`)

Wire-protocol values (`iss="fb-brain"`, `aud="fb-os"`) **deliberately unchanged** — they are wire contracts with The Brain.

---

## 7. Open questions — alignment gate

> **Status update (2026-04-29):** All 16 questions are answered in [ADR-001-vision-alignment.md](./ADR-001-vision-alignment.md) (PROPOSED, awaiting owner sign-off). The questions remain below for context; cross-reference each to the corresponding decision (D1–D16) in the ADR.

These are questions whose answers shape what we build next. Some are small, some are strategic. Roughly ordered by urgency.

### Naming / repo

1. **Go module path target** — current: `github.com/futurebuild/futurebuild-os` (note: org part lacks the `ai`). Candidates:
   - `github.com/futurebuildai/buildos` (lowercase, conventional, clean)
   - `github.com/futurebuildai/BuildOS-main` (matches GH repo slug exactly)
   - Keep current path forever and just rename in docs (path is internal, no import-statement leakage to customers)
2. **GableLBM repo** — `futurebuildai/GableLBM-main` exists; is it analogous to `BuildOS-main` (per-customer fork model) or is it a single-instance ERP?
3. **The Brain GH repo** — currently `futurebuild-brain`; rename to e.g. `the-brain` for consistency, or leave as-is? (Public-facing brand stays "The Brain" regardless.)

### Architecture / contracts

4. **Brain ↔ BuildOS wire contract rename**. Need decision on a coordinated cutover: `iss="fb-brain"` → `iss="the-brain"`, `aud="fb-os"` → `aud="buildos"`. Until this is decided, **legacy values stay on the wire**.
5. **Maestro API contract from BuildOS's side.** What endpoints does BuildOS call? What request/response shapes? Where does this go in `API_CONTRACT.md` — extension to existing OS contract, or a separate `BRAIN_CLIENT_CONTRACT.md`?
6. **Hub credential vault interface.** When BuildOS wants to call the customer's QuickBooks, does it (a) call a Brain endpoint that proxies, (b) request a short-lived scoped token from Brain and call upstream itself, or (c) something else?
7. **A2A receiver scope for Sprint 5.** Five event types are defined; what handlers actually exist in BuildOS? `internal/api/a2a.go` is currently a stub.

### Customer-fork lifecycle

8. **Fork creation workflow.** Template repo? Branded overlay? Manual?
9. **Upstream-upgrade story.** Does FutureBuild AI cherry-pick fixes into customer forks, or do customers merge from `main`? Conflict-resolution playbook?
10. **Co-op multi-tenant variant.** What does "multi-tenant for a co-op" look like operationally — one BuildOS instance with N member orgs, or N forks managed by the co-op? `org_id` already supports the former.
11. **Branding kit.** Per-customer theming (colors, logos, app name overrides) — env-driven? Build-time? Config file?

### Product / ecosystem

12. **Cross-product user journeys** — see §5 above. Need owner-level decisions on Options A/B for each.
13. **GableLBM ↔ BuildOS data overlap.** A contractor's procurement view: does it show the yard's live inventory, or only quotes? Where does inventory truth live?
14. **LocalBlue lead pipeline.** Auto-flow into BuildOS Prospects, or stay in LocalBlue with manual promotion?
15. **AI-First constraint and cost.** TECH_STACK.md says Anthropic-only with 100% AI markup billed by The Brain. What's the customer-visible AI pricing model? Per-seat? Metered? Bundled?

### Sprint plan

16. **Continue the original 8+1 sprint plan as written, or restructure** given (a) the rebrand, (b) the per-customer fork model, (c) the alt-auth work that wasn't in the original plan?

---

## 8. Recommended next steps (after this doc is approved)

1. **Owner sign-off on §7 questions 1–3** (naming) so we can finalize the Go module rename and any matching repo renames in one batch.
2. **Owner sign-off on §7 questions 4–7** (Brain contract) so BuildOS can start wiring real Brain calls in Sprint 2 onward instead of stubs.
3. **Cross-product flow decisions (§7 12–14)** — these unlock the procurement, pipeline, and ecosystem milestones.
4. **Resume sprint execution with Sprint 2** (financials) once the above are settled. The CPM engine and physics gates are intact and won't be re-litigated.

---

## 9. What changes if this doc is wrong

If owner says "your understanding is off in §X," edit that section, recommit, and pause Sprint 2 work until corrected. The cost of getting alignment wrong here compounds: every line of code written against a misunderstood Brain contract or fork model is rework.
