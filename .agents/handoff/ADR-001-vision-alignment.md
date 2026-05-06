# ADR-001: Vision Alignment — Decisions on the 16 Open Questions

**Status:** PARTIALLY ACCEPTED — see per-decision status below; remaining items still PROPOSED.
**Date:** 2026-04-29
**Author:** Claude (executing the L8 review the owner requested)
**Inputs:** [VISION_ALIGNMENT.md](./VISION_ALIGNMENT.md), [TECH_STACK.md](../TECH_STACK.md), [ARCHITECTURE.md](./ARCHITECTURE.md), [API_CONTRACT.md](./API_CONTRACT.md), futurebuild.ai, futurebuildai/futurebuild-ecosystem

**Ratification log:**
- D5 (Maestro client contract): ACCEPTED at Gate 1 — owner-confirmed 2026-05-05.
- D6 (Hub credential vault interface): ACCEPTED at Gate 1 — owner-confirmed 2026-05-05.
- D7 (A2A receiver scope for Sprint 5): ACCEPTED with payload-alignment caveat at Gate 2 — owner-confirmed 2026-05-06; see [ADR-003](./ADR-003-gate-2-ratification.md).
- D14 (LocalBlue lead → BuildOS Prospect): ACCEPTED at Gate 2 — owner-confirmed 2026-05-06; see [ADR-003](./ADR-003-gate-2-ratification.md).
- D1, D2, D3, D4, D8, D9, D10, D11, D12, D13, D15, D16: still PROPOSED.

---

## How to read this document

Each decision (D1–D16) follows the same shape:

- **Question** — recap from VISION_ALIGNMENT §7
- **Decision** — what to do, concretely
- **Why** — 3–5 reasons
- **Alternatives considered** — what got rejected and why
- **Risk / reversibility** — blast radius, undo cost
- **Implements** — concrete next action

L8 bias enforced throughout: prefer reversible decisions, lean on industry conventions, anchor to constraints rather than preferences, write things you can hand to a junior to execute. Each decision is one short paragraph of reasoning, not an essay.

---

# Group A: Naming and repository identity

## D1 — Go module path

**Question:** What is the canonical Go module path?

**Decision:** `github.com/futurebuildai/buildos`. Rename the GitHub repo from `BuildOS-main` to `buildos` to match. Drop the `-main` suffix.

**Why:**
- Go module paths are conventionally lowercase; mixed-case modules are a known footgun (case-insensitive filesystems, IDE autocomplete misses, module proxy cache collisions).
- The `-main` suffix is redundant — `main` is the default branch convention, not part of the project's identity. Customer forks will be named `buildos-acme`, etc.; "main" is not a peer.
- The product brand "BuildOS" is for display; the technical identifier is `buildos`. Same pattern as "GitHub" → `github.com`, "VS Code" → `vscode`.
- Public Go modules at FAANG companies (`googleapis/googleapis`, `grpc/grpc-go`, `kubernetes/kubernetes`) are uniformly lowercase even when the marketing brand isn't.
- Open-source forks discoverable via `go get github.com/futurebuildai/buildos` are easier to remember and type than the mixed-case alternative.

**Alternatives considered:**
- *Match the existing `BuildOS-main` slug exactly.* Rejected — locks in convention drift forever, and the cost of fixing it grows monotonically.
- *Lowercase but keep the `-main` suffix (`buildos-main`).* Acceptable but the `-main` carries no information; remove it.
- *Keep current `github.com/futurebuild/futurebuild-os` path forever.* Rejected — module-path branding would no longer match the product. Internal-only argument doesn't hold once we publish customer fork docs.

**Risk / reversibility:** Renaming a Go module path is an O(N) sed pass across every `.go` file plus `go.mod`. ~50–100 file edits, mechanical. GitHub repo rename redirects old URLs for the foreseeable future. Easy to revert; harder to leave half-renamed. **Do it once, atomically.**

**Implements:** Rename GH repo `BuildOS-main` → `buildos`. Sed-rewrite imports `github.com/futurebuild/futurebuild-os` → `github.com/futurebuildai/buildos` across all `.go` files. Update `go.mod` module declaration. Update [Makefile](../../Makefile) `DATABASE_URL` references if any embed the old path (none currently). Commit as one logical change with a clear migration note.

---

## D2 — GableLBM repo model

**Question:** Is `GableLBM-main` analogous to BuildOS (per-yard fork) or a single instance?

**Decision:** Same model as BuildOS — per-customer (per-yard) forks of an open-source `GableLBM-main` template. Existing demo repos (`shoemaker-demo`, `hioutlet-demo`, `lenoble-demo`, `bernards-demo`, `millcreek-demo`) confirm the pattern is already in use.

**Why:**
- Five yard-specific demo forks already exist in the org. The fork-per-customer model is established practice, not a new experiment.
- Same data-sovereignty reasoning as BuildOS (yards own their inventory data).
- One fork pattern across the open-source surfaces is operationally simpler than mixed.

**Alternatives considered:**
- *Single shared GableLBM with multi-yard tenancy.* Rejected — GableLBM exists primarily as branded customer-facing portals; the fork-with-branding model fits.

**Risk / reversibility:** Observation, not a code change. **No risk.**

**Implements:** Document this in CLAUDE.md as part of the BuildOS dependency picture. No GableLBM repo changes from this repo's PR.

---

## D3 — The Brain GitHub repo name

**Question:** Rename `futurebuild-brain` to align with the public "The Brain" brand?

**Decision:** Keep the GitHub slug `futurebuild-brain`. Public brand stays "The Brain". The two are decoupled.

**Why:**
- The Brain repo is private and is referenced by customer forks **only via env vars** (`BRAIN_JWKS_URL`, etc.), never by GitHub path. The slug is engineering-internal.
- Renaming churns commit history references, deploy configs, secrets bound to the old name, and CI integrations — all for zero customer-visible benefit.
- Stable identifiers for proprietary services should outlast brand revisions. (Twitter's repo was once `birdsong`. Stripe's data warehouse runs on something named `mosaic`. Internal names diverge from external brands routinely.)

**Alternatives considered:**
- *Rename to `the-brain`.* Rejected — pure aesthetic gain, real operational cost.
- *Rename to `brain`.* Same as above, plus the slug becomes unprefixed and easier to confuse with non-FutureBuild internal projects later.

**Risk / reversibility:** N/A — no change.

**Implements:** Document in CLAUDE.md and ADR. No code change.

---

# Group B: Architecture & wire contracts

## D4 — Wire-protocol values: `iss="fb-brain"` and `aud="fb-os"`

**Question:** What's the cutover plan?

**Decision:** Migrate `aud` from `"fb-os"` → `"buildos.api"`. Migrate `iss` to URL form per OIDC convention (it should already be URL form via `BRAIN_ISSUER_URL`; the legacy `"fb-brain"` value in spec docs is an example, not a wire value). Stage the cutover with a dual-audience window: BuildOS accepts both old and new for two sprints, Brain emits new only after both repos deploy.

**Why:**
- OIDC standard says `iss` is the issuer URL; using a literal string like `"fb-brain"` deviates from spec and breaks OIDC discovery.
- `aud` should be a URI or namespace-style identifier (`urn:buildos:api` is also acceptable). `"fb-os"` carries the old brand; `"buildos.api"` is brand-aligned and stable.
- Dual-aud cutover is the standard low-risk pattern: deploy validator first (accept both), then deploy emitter (emit new), then drop old support after grace period.
- The cost of touching this once is small; the cost of leaving legacy strings in the wire forever is permanent confusion for every new engineer.

**Alternatives considered:**
- *Leave wire values as legacy `fb-brain`/`fb-os` forever.* Defensible but gives up the cleanup window. Rejected because the cost is small and we're already touching auth code.
- *Big-bang flag-day rename.* Rejected — coordinating two repo deploys with zero overlap is fragile.
- *Use UUIDs for opaque identity.* Rejected — URLs/URIs are more debuggable.

**Risk / reversibility:** Medium — requires coordination with the Brain team. Botched cutover = invalid tokens for live users. Mitigated by the dual-aud window. Reversible by extending dual-support indefinitely.

**Implements:**
1. Brain team adds dual-`aud` emission (`["fb-os", "buildos.api"]`).
2. BuildOS auth.go updates `AnyAudience: jwt.Audience{"fb-os", "buildos.api"}`.
3. Both deploy.
4. After 2 sprints, Brain drops `"fb-os"` and BuildOS drops it from accepted set.

---

## D5 — Maestro client contract from BuildOS's side

**Question:** What endpoints does BuildOS call? What request/response shapes?

**Decision:** A single semantic interface in The Brain at `POST /v1/ai/tasks`, taking a typed task envelope and returning a typed result. Async variant at `POST /v1/ai/tasks/async` for long-running work, with completion delivered via A2A webhook back to BuildOS. Define in a new spec doc `BRAIN_CLIENT_CONTRACT.md` co-owned by both repos. BuildOS imports it via a thin client at `internal/brain/maestro.go`.

**Sketch:**

```jsonc
// POST /v1/ai/tasks  (sync)
{
  "task_type": "daily_briefing",
  "context": { "org_id": "...", "as_of": "2026-04-29" },
  "params":  { "project_ids": ["..."] }
}
// → 200
{
  "run_id": "...",
  "result": { /* task-specific shape */ },
  "tokens_used": 4321,
  "cost_cents": 12,
  "currency_code": "USD"
}
```

**Why:**
- One semantic endpoint lets Brain swap models, add caching, change orchestrators, route to regex/Claude/human tiers — all without breaking BuildOS clients.
- Typed task envelopes (one per use case: `daily_briefing`, `intent_classify`, `invoice_extract`, `procurement_recommend`, `tribunal_review`) keep request/response shapes auditable while sharing transport.
- Centralizing cost telemetry in the response makes billing transparent and lets BuildOS surface usage to end users without round-tripping to Brain.
- Async-via-A2A reuses existing webhook infrastructure (Sprint 5 receiver work) — no new transport.

**Alternatives considered:**
- *N specialized endpoints (one per task type) on Brain side.* Rejected — duplicates routing/auth/billing logic, increases Brain API surface, slows iteration.
- *Direct Anthropic SDK calls in BuildOS with Brain as a billing observer.* Rejected — violates "open source tools connected by proprietary AI" thesis. The Brain owning the AI key is the moat.
- *gRPC instead of REST.* Rejected — adds tooling burden for negligible benefit at this scale; REST + JSON is industry default.

**Risk / reversibility:** New endpoint, low risk; breaking changes via versioned `task_type` schema. Reversible by adding new task types instead of mutating old ones.

**Implements:** Brain team specifies and ships `/v1/ai/tasks`. BuildOS adds `internal/brain/maestro.go` with one Go method per task_type, type-safe wrappers calling the generic endpoint. First task type to wire: `daily_briefing` in Sprint 4 (per existing plan).

---

## D6 — Hub credential vault interface: proxy vs scoped token

**Question:** Does Brain (a) proxy upstream calls, or (b) hand BuildOS a scoped token to call directly?

**Decision:** **Proxy by default.** Brain exposes one client endpoint per integration: `POST /v1/clients/quickbooks/...`, `/v1/clients/gable/...`, `/v1/clients/localblue/...`, etc. BuildOS calls Brain; Brain resolves the tenant's encrypted credentials via the Hub, calls the upstream API, normalizes the response, returns it. Token-handout pattern is reserved for streaming/webhook scenarios that don't fit request/response.

**Why:**
- Centralized rate limiting, retry, and quota management. Each upstream's quirks (QuickBooks's CloudEvents migration, Gmail's OAuth scope politics) are encapsulated once in Brain.
- Single audit point for all 3p calls — required for compliance posture and for billing the markup/brokerage fees.
- Customer fork code stays simple: it doesn't accumulate N upstream auth implementations over time.
- Brain can normalize disparate upstream APIs into consistent BuildOS-shaped responses (e.g., line items always have `amount_cents` + `currency_code`).
- Matches the System-of-Connection role of Brain in the architecture.

**Alternatives considered:**
- *Scoped token handout for everything.* Rejected — leaks complexity and creates N auth implementations in BuildOS, partially defeats the centralization purpose of Hub.
- *Hybrid by default.* Adds decision overhead; only do hybrid when proxying genuinely doesn't fit.

**Risk / reversibility:** Adds Brain-hop latency on 3p calls (typically 50–150ms). Mitigated by Brain-side caching on idempotent reads. Easily reversible per-integration if perf becomes a problem.

**Implements:** Brain team defines `/v1/clients/<service>/...` shapes per upstream. BuildOS `internal/brain/clients.go` exposes Go methods like `BrainClient.QuickBooks.CreateInvoice(ctx, req)`. First wire-up: Sprint 3 procurement (Gable, 1Build).

---

## D7 — A2A receiver scope for Sprint 5

**Status:** ACCEPTED with payload-alignment caveat — owner-confirmed 2026-05-06 (Gate 2). Six receivers shipped (the original five plus `localblue.lead_captured` per D14). Per-event payload schemas locked for `update_schedule` and `delivery_confirmation`; the other four payloads await Brain-side alignment. See [ADR-003](./ADR-003-gate-2-ratification.md).

**Question:** Five event types are defined; what handlers actually exist in BuildOS?

**Decision:** Sprint 5 implements all 5 receivers as proper service-layer handlers. No stubs remain. Each handler is idempotent (keyed on event ID), creates a feed card or updates a domain entity, and writes an audit log entry. Failure to process raises `5xx` so Brain retries via exponential backoff (already specified in `ARCHITECTURE.md` §11).

**Why:**
- Stubbed receivers are silent failure modes. If LocalBlue starts emitting leads via A2A and BuildOS swallows them, the contractor never sees their leads — that's the worst possible UX for a top-of-funnel feature.
- The 5 events were chosen as the MVP set; doing fewer means doing none of them well.
- Idempotency is non-optional for at-least-once delivery (Brain retries on 5xx).
- Feed cards + audit logs are the unified observability surface — every async-arriving event produces a user-visible artifact.

**Alternatives considered:**
- *Implement 2 of 5 in Sprint 5, defer the rest.* Rejected — picks winners arbitrarily and leaves Brain's emitter side wondering which of its events will actually be processed.

**Risk / reversibility:** Standard handler work. Per-event reversible.

**Implements:** Sprint 5 ticket per event type, each with: idempotency key check, domain action, feed card emit, audit log entry, integration test against a Brain-style fixture.

---

# Group C: Customer-fork lifecycle

## D8 — Fork creation workflow

**Question:** How does a customer fork get created?

**Decision:** Mark `BuildOS-main` (post-D1 rename: `buildos`) as a GitHub template repo. New customer fork via `gh repo create futurebuildai/buildos-acme --template futurebuildai/buildos --private`. A `make brand` target rewrites display strings from a `branding/customer.yaml` file. Customer-specific branding lives in a `branding/` directory; everything else stays in lockstep with upstream.

**Sketch — `branding/customer.yaml`:**

```yaml
customer_id: acme
display_name: "Acme Construction"
support_email: support@acme.example
brand_colors:
  primary: "#FFAA00"
  accent: "#1B1F2A"
oidc:
  client_id: "acme-buildos"  # registered in The Brain
locale_default: en-US
currency_default: USD
```

`make brand` runs a small Go tool that:
- Generates a `branding/applied.go` build-info file
- Updates `package.json` `name`
- Replaces logo/favicon assets from `branding/assets/`
- Rewrites `<title>` and CSS custom-property defaults

**Why:**
- GitHub template repos are the standard pattern for "fork this and start" — well-known, no custom tooling.
- Centralizing customer-specific overrides in one directory minimizes merge conflicts when pulling upstream.
- A `make` target is discoverable; a custom CLI tool is yet another thing to install and maintain.

**Alternatives considered:**
- *Custom Go CLI (`fb create-customer-fork acme`).* Rejected — premature; one more tool to maintain for marginal ergonomic gain.
- *Customer fork + manual brand editing.* Rejected — invites drift, hard to QA per customer.

**Risk / reversibility:** Low — adds ~200 LOC of tooling. Reversible per customer.

**Implements:** Sprint 7 (frontend) parallel work: ship the template, ship `make brand`, document the workflow in a new `docs/customer-forks.md`.

---

## D9 — Upstream upgrade story

**Question:** Who merges upstream changes into customer forks?

**Decision:** Customers merge upstream. FutureBuild AI publishes a versioned `UPGRADE_NOTES.md` with each release: changelog, breaking changes, migration steps. A `make upgrade-check` Make target in customer forks compares HEAD to upstream's latest release tag and prints the diff summary plus any blocking conflicts in non-`branding/` paths. Major releases (breaking) come quarterly at most; minor releases (non-breaking) ship continuously.

**Why:**
- Forcing FutureBuild AI to push merges into N customer forks is O(N) operational overhead per release. Doesn't scale past ~5 customers.
- Customers retain control over their own deploy timeline — critical for businesses with active project schedules they don't want disrupted.
- Industry pattern: GitLab CE, Mastodon instances, WordPress core. All push-fewer / pull-more.
- Isolating customizations to `branding/` per D8 minimizes conflict surface.

**Alternatives considered:**
- *FutureBuild AI runs all merges centrally.* Rejected — ops overhead, plus customers may want to skip a release if it conflicts with a critical schedule freeze.
- *Force-rebase customers' branches off upstream.* Rejected — destroys customer customizations; not a viable model.

**Risk / reversibility:** Some customers will delay merging and accumulate drift. Mitigation: clear release cadence, opinionated `UPGRADE_NOTES.md`, SLA on critical security patches.

**Implements:** Sprint 8 (or earlier as a meta-task): release-tagging discipline, `UPGRADE_NOTES.md` template, `make upgrade-check` script.

---

## D10 — Co-op multi-tenant variant

**Question:** What does "multi-tenant for a co-op" look like operationally?

**Decision:** **One BuildOS instance, N member orgs.** The codebase already filters every query by `org_id`; the same fork-and-deploy pattern works, the operator just registers N orgs in the database instead of one. No code branch is required. As a hardening step before the first co-op customer, add Postgres Row-Level Security (RLS) policies that enforce `org_id` isolation at the database layer (defense-in-depth on top of application-level filters).

**Why:**
- The data model is already multi-tenant by design (`org_id` on every domain table). Single-tenant forks are just multi-tenant with N=1.
- One instance = one upgrade, one Brain registration, one infra footprint. Operationally drastically simpler than N forks for a co-op.
- RLS at the Postgres layer is industry best practice for multi-tenant SaaS — Heroku, Supabase, AWS RDS docs all recommend it. Catches the case where a developer forgets a `WHERE org_id = $1`.
- Co-op operator gets full control over their members' onboarding/offboarding without involving FutureBuild AI per change.

**Alternatives considered:**
- *Co-op operator runs N forks (one per member).* Rejected — duplicates infra, complicates upgrade story by N.
- *Add a tenant-isolation mode flag.* Rejected — code path branching is a maintenance tax for no functional benefit; the same code already works.

**Risk / reversibility:** RLS adds a small migration and ~one-day eng task to implement and test. Reversible by dropping the policies if they conflict with future query patterns.

**Implements:** New migration `005_row_level_security.up.sql` adds RLS policies to all org-scoped tables, with a per-request `SET LOCAL app.org_id = $1` set in middleware. Defer until first co-op signs; at that point it's a 1-sprint task.

---

## D11 — Branding kit

**Question:** Env-driven, build-time, or config file?

**Decision:** **Layered.** YAML config (`branding/customer.yaml`) is the source of truth — committed to the customer fork. `make brand` consumes it at build time to generate Go build-info + asset overrides. A small subset of values is also exposed at runtime via env vars (`BUILDOS_BRAND_NAME`, `BUILDOS_SUPPORT_EMAIL`) so ops can hot-fix without rebuilding.

**Why:**
- YAML is the canonical, reviewable, version-controlled description of a customer's brand. One file to look at.
- Build-time generation gives compile-time enforcement (typos in YAML caught at `make brand` runtime).
- Runtime env override is the "break-glass" mechanism: support engineer can fix a typo'd support email in production without a deploy.
- All three layers are normal patterns; combining them in this order is standard SaaS practice (e.g., Sentry, Discourse).

**Alternatives considered:**
- *Pure runtime env vars.* Rejected — hard to audit a customer's full branding config from a Helm chart. YAML wins for review.
- *Pure build-time.* Rejected — deploys for typos.

**Risk / reversibility:** Adds branding YAML schema; needs versioning if it grows. Reversible by simplifying YAML.

**Implements:** Same Sprint 7 work as D8.

---

# Group D: Cross-product user journeys

## D12 — Contractor (BuildOS) ordering from a yard (GableLBM)

**Question:** Brain proxy or direct B2B?

**Decision:** **Brain proxy via the Hub MCP `GableERP` server.** BuildOS issues an RFQ → Brain identifies the relevant yards from the Hub's per-tenant integration map → Brain queries each yard's GableLBM via MCP → results aggregated back to BuildOS as quotes. Brain captures the brokerage fee (`BROKERAGE_FEE_BPS=150` already configured) on each routed PO.

**Why:**
- Consistent with the website thesis: the network effect lives in The Brain; both BuildOS and GableLBM are open-source endpoints connected via proprietary glue.
- Brokerage fee monetization happens automatically because Brain sees every routed RFQ.
- One fork-customer doesn't need to discover/auth/integrate with N yards — the Brain knows the network.
- Single audit trail of cross-product transactions for both compliance and business intelligence.

**Alternatives considered:**
- *Direct B2B integration with a Brain-issued service token.* Rejected — undermines the Brain-as-network-effect thesis, complicates fee capture, scatters integration code.
- *BuildOS pulls a yard catalog manually.* Rejected — doesn't scale and doesn't capture cross-product flow.

**Risk / reversibility:** Brain-mediated routing adds latency on RFQs (likely 200–500ms p95). Acceptable for human-driven flows; not acceptable for high-frequency automation. Reversible to direct B2B if a specific contractor-yard relationship demands it (rare exception, not default).

**Implements:** Sprint 3 procurement work depends on this contract. Brain team defines `POST /v1/clients/gable/rfq` with multi-yard fan-out; BuildOS calls it from the procurement service.

---

## D13 — GableLBM ↔ BuildOS data overlap

**Question:** Does the contractor's procurement view show the yard's live inventory? Where does inventory truth live?

**Decision:** **Yard owns inventory truth in GableLBM.** BuildOS sees yard inventory only as the result of an RFQ via Brain (D12); no direct inventory mirroring. BuildOS caches RFQ responses in its own `procurement_quotes` table for project history — the cache is a quote snapshot, not a live inventory replica.

**Why:**
- One source of truth principle. Inventory is volatile; replication leads to bad data.
- RFQ-driven model is how contractors actually buy — they don't browse yard inventory like a retail catalog, they request quotes for specific spec'd items.
- Yard owns its own data per the data-sovereignty thesis.
- Cached quotes are fine for project records; staleness is bounded by quote expiry which yards already manage.

**Alternatives considered:**
- *BuildOS subscribes to yard inventory events and maintains a mirror.* Rejected — high replication cost, stale data risk, unclear value over RFQ flow.
- *BuildOS can query yard inventory directly without an RFQ.* Rejected — yards don't want all-comers browsing their stock; quote-gated access is the business norm.

**Risk / reversibility:** None — this is the do-nothing-unusual answer. Reversible by adding inventory mirroring later if a use case emerges.

**Implements:** Define the RFQ response schema in Sprint 3.

---

## D14 — LocalBlue lead → BuildOS Prospect

**Status:** ACCEPTED — owner-confirmed 2026-05-06 (Gate 2). Auto-flow shipped in PRs #22 + #23 (2026-05-05): `localblue.lead_captured` A2A handler creates a `pre_construction_prospects` row at `stage='LEAD'` atomically (prospect + feed-card + audit, single tx, six atomicity tests). See [ADR-003](./ADR-003-gate-2-ratification.md).

**Question:** Auto-flow into pipeline, or manual import?

**Decision:** **Auto-flow via A2A webhook.** LocalBlue → Brain → BuildOS at the A2A endpoint. Lead enters BuildOS as a `pre_construction_prospects` row with `stage='lead'` and `source='localblue'`. The contractor (via Feed Card) sees a "New lead from LocalBlue" notification immediately. Manual qualification (advancing to `qualified`, `estimate_pending`, etc.) stays in the contractor's hands inside BuildOS.

**Why:**
- This is the **moment-of-truth flow** for the entire ecosystem: a lead generated by LocalBlue becoming revenue in BuildOS is the demonstrable network effect that justifies the proprietary glue.
- Auto-import removes friction at the highest-value point in the funnel.
- Manual qualification preserves the contractor's authority over their pipeline.
- Webhook delivery is already a planned A2A event; this just makes LocalBlue a producer.

**Alternatives considered:**
- *Leads stay in LocalBlue with a "promote to BuildOS" button.* Rejected — extra step at the most valuable funnel moment. Leads will leak.
- *Polling sync.* Rejected — stale data, wastes API calls.

**Risk / reversibility:** Mis-formed payloads land in pipeline as garbage rows. Mitigated by the standard A2A receiver pattern (idempotency, validation, dead-letter on failure). Reversible by disabling the LocalBlue → Brain emit on Brain side.

**Implements:** Sprint 5 A2A work adds a `localblue.lead_captured` event handler. New event type, but reuses the receiver scaffolding.

---

## D15 — AI pricing model (commercial; recommendation only)

**Question:** What's the customer-visible AI pricing?

**Decision (recommended):** **Per-seat with bundled token allowance + transparent overage.** Sample tier:

- $99/seat/month: 100K Anthropic tokens included
- Overage: $0.012/1K tokens (incorporates Brain's 100% markup over Anthropic list)
- Optional unlimited tier: $299/seat/month (fair-use ceiling, unmetered AI calls)

Brain's billing engine handles the metering and markup transparently. BuildOS surfaces a small per-org dashboard ("AI usage this month: 42K of 100K tokens") so contractors aren't surprised. AI costs above the bundle are itemized on the invoice, not hidden.

**Why:**
- Per-seat anchors price to contractor headcount, which is how construction businesses budget software.
- Bundled allowance reduces friction (most users never see overages).
- Transparent overage matches AWS / Twilio / Stripe — predictable, defensible, builds trust.
- The Brain's 100% markup is a real cost-of-platform; making it visible (via overage billing) lets customers self-regulate AI use.
- Unlimited tier is the "I don't want to think about it" plan for high-AI-use contractors and de-risks the freemium pull from cost-conscious shoppers.

**Alternatives considered:**
- *Bundled with no overage; cap and degrade at limit.* Rejected — degrading the product mid-month is a NPS killer.
- *Pure per-token pricing.* Rejected — unpredictable bills are anathema to construction GCs who budget jobs at fixed price.
- *AI hidden as part of the platform fee.* Rejected — Brain's 100% markup means hidden AI cost gets reverse-engineered by a customer's bookkeeper, eroding trust.

**Risk / reversibility:** This is a commercial decision; the owner can override. Reversible by changing pricing pages and Stripe SKUs.

**Implements:** Owner decision, then Brain team implements metering + Stripe billing. Out-of-scope for BuildOS code changes.

---

# Group E: Sprint plan

## D16 — Continue 8+1 plan or restructure?

**Question:** Original plan still valid?

**Decision:** **Keep the original plan with two adjustments:**

1. **Insert Sprint 1.5 — Brain client foundation** (~3 days). Adds `internal/brain/` package: HTTP client, retry/timeout policy, types for `maestro.go` and `clients.go` per D5/D6. Does not implement business logic; just the transport layer. Unblocks every subsequent sprint that needs a Brain call.

2. **Pull Sprint 5 (A2A receiver) earlier — make it Sprint 4.** Reasoning: D14 establishes that LocalBlue lead auto-flow is a top-of-funnel feature. The pre-construction pipeline (M11) shipped in Sprint 4 can't accept inbound leads if the A2A receiver is Sprint 5. Sequence them together.

**Why:**
- The CPM engine + scaffold work (Sprints 0–1) is intact and didn't churn.
- Rebrand and alt-auth were infrastructure detours that didn't consume sprint budget — treated as engineering operations, not feature work.
- The insert + reorder is a one-time perturbation; everything past Sprint 5 stays as planned.
- Surfacing the brain-client transport layer early means Sprints 2–4 can stub their integrations against a real client, rather than re-stub the same thing four times.

**Alternatives considered:**
- *Total replan.* Rejected — the existing plan is sound; the rebrand was naming, not architecture.
- *Defer A2A entirely to Sprint 6+.* Rejected — kills the LocalBlue→BuildOS ecosystem story for two months. Bad product timing.

**Risk / reversibility:** Sprint 1.5 is a small add. Reordering Sprint 5→4 is a label change.

**Implements:** Update `.agents/handoff/SPRINT_PLAN.md` to reflect the insert and reorder. (Separate edit; not in this ADR's commit.)

---

# What this unblocks

After owner sign-off, the following can move:

| Item | Action | Owner |
|---|---|---|
| D1 | Execute GH rename + Go module path rename in one PR | Claude |
| D4 | Coordinate dual-aud cutover with Brain team | Brain team + Claude |
| D5 | Brain team writes `BRAIN_CLIENT_CONTRACT.md`; Claude scaffolds `internal/brain/maestro.go` | Brain team + Claude |
| D6 | Same — Brain spec, Claude `internal/brain/clients.go` | Brain team + Claude |
| D7 | Sprint 5→4 reorder; implement 5 receivers in that sprint | Claude |
| D8/D9/D11 | Template repo + `make brand` + `UPGRADE_NOTES.md` template | Claude (Sprint 7) |
| D10 | RLS migration deferred to first co-op signing | Claude (on demand) |
| D12/D13 | Brain spec defines `/v1/clients/gable/rfq`; Sprint 3 wires BuildOS side | Brain team + Claude (Sprint 3) |
| D14 | Brain emits `localblue.lead_captured`; BuildOS receiver in Sprint 4 | Brain team + Claude (Sprint 4) |
| D15 | Owner pricing decision; Brain team implements billing | Owner + Brain team |
| D16 | Update `SPRINT_PLAN.md` | Claude |

---

# Disagreement protocol

Any of these decisions can be revised in two ways:

1. **Owner overrides:** edit this ADR with strikethroughs and an addendum noting the override and why. The git diff is the audit trail.
2. **New evidence:** if implementation reveals a decision is wrong, write a follow-up ADR (`ADR-002-...`) that supersedes the relevant section. Do not edit history.

---

**End of ADR-001.**
