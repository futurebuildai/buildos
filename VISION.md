# BuildOS — Vision, Architecture & Roadmap

> **Status:** Canonical north star · **Last updated:** 2026-06-14 · **Owner:** colton@futurebuild.ai
>
> This is **the** canonical north-star doc for BuildOS's direction post the ESC-001 standalone pivot.
> For *product direction / vision*, it **supersedes** the largely pre-pivot material under `.agents/`
> (PRD, ARCHITECTURE, EXECUTION_PLAN, etc.). Those remain authoritative only for their narrow
> mechanical contracts (e.g. `.agents/handoff/API_CONTRACT.md` for exact routes/status codes,
> `.agents/TECH_STACK.md` for the dependency allowlist). When this doc and a pre-pivot spec
> disagree about *where the product is going*, this doc wins.

---

## Progress update (2026-06-14) — supersedes the stale verdicts below

The "gap audit" table and Phase-1..4 roadmap further down were written **2026-06-08** as a pre-execution snapshot ("agentic harness = Grade D, biggest gap"). That snapshot is now **out of date** — read it as history. Current reality:

- **The agentic harness is BUILT (Phases 1–4 complete).** `internal/agentic` is the isolated leaf substrate; `delay_cascade` is a real AI-reasoned cross-module cascade; all four roles ship (ingestion = AI invoice extract; foresight = `foresight_sweep` risk cards; experience = `POST /api/v1/agents/chat` bounded tool-use loop; orchestration). DB-backed agent + MCP-connector config (`internal/connectors`) with an **admin web UI** is live. Production-readiness (security/load review, worker observability, alerting) is done. The harness layer that "barely existed" is now the working core of the product.
- **It's deployed.** Railway: `staging.futurebuild.ai` auto-deploys `main`; this repo is "fork zero" (Kelbrook's instance). The operator web console + Flutter field app run against it.
- **Layer 4 (system of record) has begun.** The agentic-UX polish loop made the harness visible/actionable in the console, then the **first operational-coordination slice shipped**: Daily Reports → Client Updates (field photos → daily reports → AI office digest + client-safe homeowner update → email + public link). This is what reframes layer 4 from "accounting, deprioritized" into the broader **operational-coordination / system-of-record layer** described below.

The vision, principles, and beta bar below remain correct; only the "today" status moved.

## Context

The repo owner asked to (1) understand the current state of BuildOS and (2) align on the
long-term vision and how to get there. This document is the output: a refined vision, an
honest audit of how well today's software reflects it, and a priority-ordered path forward.

**Why now:** the backend is essentially beta-complete (CPM engine passing determinism + bench
gates, ~98% API coverage, no blockers), and recent sessions have been test-coverage polishing —
the signature of a codebase whose *capability* has outrun its *direction*. The defining recent
event was the **ESC-001 standalone pivot (2026-06-01)**: the external "Brain" dependency was
removed and auth/AI/credentials/email/integrations all became native. The owner wants those
native services kept **isolated** within the repo and the product reframed around an **agentic
OS** rather than a traditional ERP build-out.

## The vision

**BuildOS is an agentic operating system for residential builders** — *Claude for Small Business
embedded inside an ERP whose source of truth is a deterministic CPM scheduling/PM core.* The
agents are construction-aware and grounded in an engine whose numbers are exact.

Four layers, in **priority order**:

1. **Deterministic core** — CPM scheduling + project management. The exact, trustworthy spine.
   *Strong today; maintain.*
2. **Agentic harness** (isolated, configurable) — the four roles: **experience** (conversational
   surface over the ERP), **orchestration** (an event in one module reasons + acts across others),
   **foresight** (cross-module risk/recommendations), **ingestion** (unstructured reality →
   structured ERP records). **Top priority. Barely exists today.**
3. **3p integration + MCP layer** (isolated, configurable, vault-backed) — connectors a builder
   enables and configures post-deploy. *Built (Phase 3); configurable post-deploy.*
4. **Operational coordination + system of record** — the day-to-day workflows a GC actually runs the
   business on, each a vertical slice (field capture → office → external comms), with the agentic harness
   layered over them (digest/draft/foresight). **Intended sequence**, roughly demand-ordered:
   **daily reports → client updates** (✅ first slice shipped — object storage + the first public surface),
   then **subcontractor coordination** (assign/track subs on the schedule), **bid management / RFQs**
   (solicit + compare sub bids), **invoicing / AP** (sub & vendor invoices against budget; builds on the
   `invoices` table the ingestion role already populates), and finally deep **accounting / GL / AP-AR /
   progress billing / retainage / change orders** — the deepest end, still **deprioritized past beta**.
   Each slice preserves the two hard rules the first slice established (deterministic client-content
   redaction at the service boundary; public/client surfaces project rather than serialize ERP).

**The wedge** is the *integration* of these layers (owner-confirmed), so no layer can be skipped —
but the agentic harness is what makes BuildOS more than "another scheduler with a chatbot."

## Architectural principles

- **Exact/fuzzy separation.** The deterministic core (`internal/physics`, `internal/currency`)
  owns all arithmetic — schedule and money are bit-exact, no floats. The harness owns judgment,
  connection, and ingestion. AI chooses *what to feed the engine* and *what to do with the output*;
  it never does the math. (Already latent: CPM determinism + Composite Currency on one side, agents
  on the other.)
- **Isolation.** The agentic + integration + MCP layer is a cleanly-bounded, swappable module set.
  It depends on domain services through interfaces (ports); the deterministic core depends on it
  *never*. This keeps the separation benefit of the old "Brain" without the deployment dependency.
  Concretely: `internal/agentic` is a **leaf** package — it imports only stdlib +
  `github.com/google/uuid` + `log/slog`, declares its own port interfaces, and imports nothing from
  `internal/service`, `internal/store`, `internal/ai`, `internal/worker`, `internal/physics`, or
  `internal/currency`. Adapters in `internal/service` implement those ports. This boundary is a
  CI gate (`make lint-isolation` / `scripts/check-isolation.sh`).
- **Configurability.** One-command single-builder deploy (`make fork-init` exists), then heavily
  configurable post-deploy — which agents run, which integrations/MCP connectors are on, how they
  are tuned — DB-backed config, admin-editable, no redeploy. (The "Claude for Small Business" feel.)
  The DB-backed registry is a **Phase 3** deliverable; Phase 1 ships an **in-code registry only**.
- **Determinism & Composite Currency remain hard CI gates** (unchanged).

## Current state vs vision — the gap audit (grounded)

| Layer | Vision target | Today | Verdict |
|---|---|---|---|
| Deterministic core | Exact CPM spine everything hangs off | `internal/physics` cpm/dhsm/swim, determinism golden-master, bench gates ≤200/500ms, ~98% API cov | **Strong — reflects the vision** |
| Agentic harness | Nervous system: experience + orchestration + foresight + ingestion | ~6 typed AI task bindings exist (`internal/ai/tasks.go`: `daily_briefing`, `intent_classify`, `invoice_extract`, `procurement_recommend`, `tribunal_review`, `update_schedule`) but each is invoked as an **isolated RPC** with **nothing composing them** — no orchestrator, no event bus, no agent/tool framework, no config. `delay_cascade` worker is a **stub** (`internal/worker/jobs.go:253`, logs "not yet implemented"); `InvoiceExtract` ingestion is a built **orphan** (no persistence). | **Grade D — biggest gap** |
| Integration / MCP | Isolated, post-deploy-configurable connectors | AES-256 vault (`internal/cryptobox`) + `SecretSource` exist; no MCP/connector framework, no post-deploy config surface | **Partial — substrate only** |
| Accounting | Full system of record (GL, AP/AR, billing, retainage, COs) | `project_budgets` estimated/committed/actual by WBS; AR is a read-only snapshot; no GL/AP/AR ledger, no progress billing/retainage/change orders | **Thin slice — acceptable (deprioritized)** |

**Reusable primitives the harness should be built on (already present):** River job queue
(transactional enqueue), feed cards (`internal/models/feed.go` + `internal/store/feed_cards.go`,
the surfacing mechanism), the immutable audit log (`internal/service/audit.go`, a latent event
source), and the resilient native AI client (`internal/ai`, per-org BYOK, circuit breaker,
JSON-schema task bindings). The existing tool-layer substrate is `ai.Client.callTool`
(`internal/ai/client.go`) — the forced typed tool-call + JSON-schema plumbing that the ~6 typed
tasks ride on. The foundation is sound (audit graded it "B+ primitives / D integration"); the
orchestration layer on top is what's missing.

## Proposed agentic-harness architecture

A new isolated package (`internal/agentic`) layered on the existing primitives:

- **Tool/MCP layer** — exposes ERP capabilities (recalc schedule, read budget, flag procurement,
  create feed card, etc.) as typed tools the agents call. Internal capabilities and external 3p
  integrations present through the same tool interface. This is the "harness." Built on the existing
  `ai.Client.callTool` substrate (`internal/ai/client.go`), which already handles forced typed
  tool calls + JSON-schema validation for the ~6 task bindings.
- **Orchestrator** — the generalized pattern the `delay_cascade` stub should become: *triggering
  event → load cross-module context → AI reasons → apply recommended actions across modules in one
  tx → audit per-module deltas → surface via feed cards.* Single-module decisions stay hardcoded;
  AI is invoked only when cross-module judgment is needed. The canonical reference for this
  load→AI→apply-in-one-tx→audit shape is `service.AgentsService.RecommendScheduleAdjustments`
  (`internal/service/agents.go`).
- **Agent registry + config** — construction-aware agents (DailyFocus, Procurement, a cross-module
  "site reasoner," etc.) declared in a registry. **Phase 1 ships an in-code registry only**; the
  **DB-backed, admin-editable, post-deploy-tunable registry is explicitly a Phase 3 deliverable.**
- **Ingestion pipelines** — wire `InvoiceExtract` (and future doc/text/photo intake) to persist
  into ERP tables and emit a review feed card. First pipeline: invoice image → `invoices` row.

The core never imports this package; the package reaches the core only through service interfaces
(ports it declares; adapters in `internal/service` implement them). `internal/physics` and
`internal/currency` must **never** import `internal/agentic`.

### Phase 1's first concrete fix — the `DelayCascadeArgs` OrgID gap

`DelayCascadeArgs` (`internal/worker/jobs.go`) currently carries only `ProjectID`. A cross-module
cascade is tenant-scoped, so the orchestrator needs the org. **First Phase 1 code change:** add an
`OrgID` field to `DelayCascadeArgs` and populate it at the enqueue site
(`internal/service/schedule.go`, where `callerOrgID` is already in scope). No schema migration is
needed in Phase 1 — the cascade reuses the existing `feed_cards` and `audit_log` tables. Note also
that `cmd/worker/main.go` does not currently wire the vault/AI client; Phase 1 adds that, modeled
on `cmd/server/main.go`.

## Beta definition — the bar for handing off to a real home builder

**Beta = a production-ready agentic OS on the deterministic project core, minus deep accounting/GL.**
The agentic layer *is the product* (owner-confirmed: "an agentic layer with a deterministic core at
the project level"), so all four harness roles are in beta scope. "Production-ready for a real builder"
means a real GC's office **and field crew** run their projects on it daily — which pulls the frontends
to a production bar too.

In scope for beta:
- Deterministic CPM scheduling + PM core (already strong).
- Full agentic harness: experience + orchestration + foresight + ingestion, on the isolated/configurable substrate.
- Operator web console + Flutter field app finished to production quality (close the mobile gaps:
  check-in, schedule, equipment screens).
- Smooth single-builder deploy + post-deploy configurability; observability, backup/DR (exist); security review.

Out of scope for beta (deferred, still in long-term vision): GL / AP-AR ledger / progress billing /
retainage / change-order accounting.

> This is a multi-month program, not a single change. The phases below are dependency-ordered (all
> are beta scope); each warrants its own detailed design pass when we execute it.

## Roadmap — how to get there

- **Phase 1 — Harness substrate + first orchestration slice.** Stand up the isolated `internal/agentic`
  package: tool layer (ERP capabilities as typed tools), agent runtime, orchestrator pattern, and an
  **in-code** config registry skeleton (the DB-backed registry is Phase 3). Prove it by making
  `delay_cascade` a real AI-reasoned cross-module cascade (schedule slip → procurement/crew/budget →
  actionable feed cards), applied + audited in one tx. The first concrete fix is the `DelayCascadeArgs`
  OrgID gap (above). This carries the framework *and* ships a real feature, establishing the isolation
  boundary + test pattern everything else plugs into.
- **Phase 2 — Fill out the four harness roles on the substrate.** Ingestion (`InvoiceExtract` → `invoices`
  row + review card, then field/photo/text intake); Experience (conversational assistant over the ERP via
  the tool layer); Foresight (cross-module risk/recommendation agents — procurement criticality, schedule
  risk, budget burn — surfaced as feed cards).
- **Phase 3 — Configurability + integration/MCP layer.** DB-backed agent/connector registry, admin config
  surface (enable + tune agents and integrations post-deploy, no redeploy), MCP connector seam, vault-backed
  credential UI. This is the "Claude for Small Business inside the ERP" configurable experience.
- **Phase 4 — Production-readiness for handoff.** Close Flutter field-app gaps (check-in/schedule/equipment);
  harden the operator + field + harness workflows end-to-end; onboarding/deploy polish; security review;
  load/smoke. (Runs partly in parallel with 1–3; it is the definition-of-done gate, not a strict tail.)
- **Phase 5 — Operational coordination / system of record (layer 4).** The day-to-day workflows a GC runs
  on, each a vertical slice (field capture → office → external comms) with the harness layered over it.
  Demand-ordered: **daily reports → client updates** (✅ shipped — introduced object storage + the first
  public surface), then **subcontractor coordination**, **bid management / RFQs**, **invoicing / AP**
  (against budget, on the `invoices` table ingestion already feeds). Each new public/client surface
  preserves the redaction + projection rules the first slice set (see CLAUDE.md "Operational-coordination
  domain"). Driven by real-builder (Kelbrook) feedback once the first slice's R2 + Resend credentials are in.
- **Post-beta — Accounting depth.** GL / AP-AR / progress billing / retainage / change orders as
  real-builder feedback demands it.

## Working process — ultraplan → ultracode → ultrareview over long sessions

The beta is a multi-month, multi-session program. We run it as a repeating loop per **work-chunk**
(a vertical slice ≈ one PR-sized coherent feature — e.g., "the `internal/agentic` tool layer + a real
`delay_cascade` orchestration"). Each chunk runs the three ultra-tiers, with the repo's hard gates and
handoff docs as the connective spine. This is the multi-agent, supercharged form of the repo's existing
dual-agent protocol (spec → execute → zero-trust audit): **ultraplan writes the spec, ultracode executes
it, ultrareview audits it.**

**The loop (per chunk):**

1. **ultraplan** — *Claude drives.* A convention (not a built-in command): a deep planning pass via plan
   mode + fanned-out Explore/Plan agents, and — when ultracode is on — a planning Workflow that generates
   and adversarially critiques 2–3 design approaches. Output: a file-level spec written to `.agents/handoff/`
   with entry points, the isolation boundary, task breakdown, and explicit verification criteria, checked
   against this north-star doc. Advance only when unambiguous (else escalate).
2. **ultracode** — *Claude drives.* Standing opt-in for this program. Claude authors a **Workflow** that
   fans the spec's tasks across subagents (parallel where independent, pipelined where staged) with
   **adversarial verification baked in** (each change checked by an independent agent; tests written).
   Lands on a feature branch. Isolation enforced: the deterministic core never imports `internal/agentic`.
3. **Local hard gates** — *Claude runs; non-negotiable.* `make audit` (lint-migrations + lint-migrations-test
   + test + test-prod + bench-physics) + the determinism golden-master + Composite Currency linter + the
   isolation gate (`make lint-isolation`). The agentic layer is fuzzy; these keep the deterministic core
   exact. No green gates → no review.
4. **ultrareview** — *Owner triggers; Claude cannot.* `/code-review ultra` on the branch (or `<PR#>`).
   Billed, multi-agent cloud review — the natural human checkpoint each cycle. Claude hands off explicitly
   here, then triages findings and loops back to ultracode for fixes until clean.
5. **Merge + handoff** — *Claude drives.* Update `HANDOFF.md` (living state) + `.agents/handoff/NEXT_STEPS.md`
   (backlog) + `ESCALATION_LOG.md` (blockers). This is the cross-session memory: a fresh session lands cold,
   reads these + the active spec, and picks up the next chunk.

**Continuity & autonomy:** north star = this vision doc. Per-loop state = HANDOFF + NEXT_STEPS. Default
autonomy: Claude runs ultraplan → ultracode → local gates autonomously within a session, **pauses at
ultrareview** (owner triggers), and escalates on genuine ambiguity rather than improvising.
**First chunk = Phase 1** (the `internal/agentic` substrate + a real `delay_cascade` orchestration slice).

## Verification — how we'll know the software reflects the vision

- A schedule slip produces an **AI-reasoned, cross-module** cascade (procurement/crew/budget) surfaced
  as actionable feed cards — `delay_cascade` is no longer a stub. (Integration test against ephemeral PG.)
- An invoice image **ingests into a structured `invoices` row** + review card (was an orphan).
- The agentic layer is **isolated**: `go list`/import graph shows `internal/physics` & domain cores
  do not import `internal/agentic`; the harness reaches the core only via interfaces. Enforced by
  `make lint-isolation` / `scripts/check-isolation.sh`.
- Agents/integrations are **enabled and tuned post-deploy** via admin config — no redeploy. (Phase 3.)
- The **Flutter field app is production-complete** (check-in/schedule/equipment screens shipped) and a
  field crew can run a day's work on it; the operator web console is production-complete.
- Determinism golden-master + bench gates (`make bench-physics`) + Composite Currency linter stay green.
- A real residential builder's office + field crew are running their projects on it and generating feedback.

## Decisions (confirmed with owner)

1. **Product definition** — BuildOS *is* the agentic layer on a deterministic project-level core; the
   full harness (experience + orchestration + foresight + ingestion) is the product, not a fast-follow.
2. **Deployment target** — must be **production-ready to hand off to a real home builder** for real use
   (office + field), not a throwaway pilot.
3. **Accounting** — in long-term scope but **deferred past beta**; no GL/AP-AR/billing depth required to ship.
4. **Isolation + configurability** — the AI/integration/MCP layer stays cleanly isolated in-repo and is
   configurable post-deploy ("Claude for Small Business inside our ERP").
