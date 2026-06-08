# Phases 2–4 — Execution via the Ultra-Loop

> Companion to [VISION.md](../../VISION.md). Phase 1 (the `internal/agentic` substrate + real
> `delay_cascade`) is the first loop iteration and establishes the patterns every chunk below reuses.
> **Last updated:** 2026-06-08.

## How each chunk runs (the loop)

Every **work-chunk** below is one full iteration of:

1. **ultraplan** *(Claude drives)* — a design pass (plan mode + fanned-out Explore/Plan agents, or a
   design Workflow) that produces a file-level spec at `.agents/handoff/PHASE_<n>_<chunk>.md` with
   entry points, the isolation boundary, a task breakdown, and verification criteria — checked against
   VISION.md. Advance only when unambiguous; else write to `ESCALATION_LOG.md` and pause.
2. **ultracode** *(Claude drives)* — a Workflow that executes the spec. Sequential pipeline for
   interdependent code (each stage compiles before handoff); parallel fan-out for independent work;
   adversarial verification baked in. Lands on a feature branch.
3. **Local hard gates** *(Claude runs)* — `make audit` (incl. `lint-isolation`, `lint-migrations`,
   `test`, `test-prod`, `bench-physics`) + the determinism golden-master + Composite Currency linter.
   No green gates → no review.
4. **ultrareview** *(OWNER triggers; Claude cannot)* — `/code-review ultra` on the branch/PR. The human
   checkpoint each cycle. Claude triages findings and loops back to ultracode until clean.
5. **Merge + handoff** *(Claude drives)* — update `HANDOFF.md` + `NEXT_STEPS.md` + the active spec so a
   cold session can resume.

**Chunking principle:** each chunk is a PR-sized vertical slice that compiles, tests, and delivers a
visible capability. Keep them small enough to plan+build+review in a bounded number of sessions.

---

## Phase 2 — Fill out the four harness roles on the Phase-1 substrate

All three chunks reuse `internal/agentic` (ports + orchestrator + registry) and `ai.Client.callTool`.

| Chunk | Goal | ultracode scope & entry points | ultrareview focus |
|---|---|---|---|
| **2a · Ingestion** | Turn the orphaned `invoice_extract` AI task into a real pipeline: invoice image/PDF → structured `invoices` row + review feed card | New ingestion port + adapter in `internal/agentic`/`internal/service`; persist via existing financials store (`invoices` table, no schema change if columns suffice; else additive migration); idempotency key to dedupe; API/worker entry to submit a doc. Reuse `ai/tasks.go` `InvoiceExtract`, `feed_cards`, `audit`. | Money stays integer-cents + currency_code; dedupe correctness; tx-atomicity (row + card + audit) |
| **2b · Foresight** | Cross-module risk/recommendation agents (procurement criticality, schedule-slip risk, budget burn) surfaced as feed cards | New orchestrations registered in the agentic `Registry`, riding the Phase-1 `RunDelayCascade` pattern; triggered by River jobs (`procurement_check`, `pipeline_analytics`) or a scheduled sweep | No AI math on the deterministic core; soft-fail when AI unconfigured; feed-card targeting correctness |
| **2c · Experience** | Conversational assistant over the ERP that queries + acts through the tool layer | Generalize `ai.Client.callTool` into an agentic **tool registry** (ERP read/act capabilities as typed tools); a chat endpoint that plans tool calls; RBAC-scoped tool exposure | Tool authorization per role; no tool bypasses RBAC/tenant scoping; bounded/audited actions |

**Sequence:** 2a → 2b (both extend the orchestrator) → 2c (heaviest; benefits from the tool registry maturing).

## Phase 3 — Configurability + integration/MCP layer

| Chunk | Goal | ultracode scope & entry points | ultrareview focus |
|---|---|---|---|
| **3a · Config registry** | Replace Phase-1's in-code `Registry` with a DB-backed one: enable/tune agents post-deploy | Additive migration (`agents_config` + audit); store + admin API under `/api/v1/...`; `SetupGate`/RBAC-aware; the agentic registry reads config at runtime | Migration linter (paired up/down, CONCURRENTLY); no secrets in plaintext; admin-only |
| **3b · Integration/MCP seam** | Vault-backed 3p connectors exposed to agents as tools; an MCP connector seam | Generalize the tool registry to mount external connectors; credentials via `internal/cryptobox` vault; per-connector enable/config | Credentials never leave the deployment; connector failures soft-fail; tool isolation |
| **3c · Admin config UI** | Operator web console screens to manage agents + integrations | `web/` Lit components + API client wiring; capability-gated | A11y + design-system conformance; no key leakage to client |

**Sequence:** 3a (foundational) → 3b → 3c.

## Phase 4 — Production-readiness for handoff to a real builder

| Chunk | Goal | ultracode scope & entry points | ultrareview focus |
|---|---|---|---|
| **4a · Flutter field gaps** | Ship the missing field screens: check-in, schedule, equipment | `mobile/lib/screens/*` per `UX_CORE_SCREENS` §9; offline-first via existing outbox/sync; widget + golden tests | Offline correctness; i18n (EN/ES); parity with web RBAC |
| **4b · End-to-end hardening** | Operator + field + harness workflows production-quality; onboarding/deploy polish; observability/alerting | Wire alerting on the Prometheus metrics; harden onboarding wizard + fork-init; error-path UX | No regressions to determinism/currency gates; graceful degradation |
| **4c · Security + load** | Security review + load/smoke before handoff | `security-review` skill on the surface; k6 load on CPM + API; chaos/smoke | AuthZ gaps; rate/abuse; data-exposure |

**Sequence:** 4a ∥ 4b (parallel across sessions) → 4c (gate to handoff).

---

## Cadence & continuity

- **One chunk = one loop = (typically) one PR.** The owner's recurring touchpoint is **ultrareview** each cycle.
- **Cross-session memory:** `HANDOFF.md` (living state) + `NEXT_STEPS.md` (backlog) + the active
  `.agents/handoff/PHASE_<n>_<chunk>.md` spec. A cold session reads these and picks up the top chunk.
- **Autonomy default:** Claude runs ultraplan → ultracode → local gates autonomously, **pauses at
  ultrareview** (owner triggers `/code-review ultra`), and escalates genuine ambiguity rather than improvising.
- **Invariants that gate every chunk:** isolation (`make lint-isolation`), determinism golden-master,
  bench-physics, Composite Currency linter — all stay green, every loop.

## Per-chunk definition of done (template)

- [ ] Spec at `.agents/handoff/PHASE_<n>_<chunk>.md` (ultraplan)
- [ ] Feature branch; `make audit` green incl. isolation + determinism + currency gates (ultracode + local)
- [ ] `/code-review ultra` clean (owner-triggered ultrareview)
- [ ] Capability demonstrable end-to-end (test or manual flow per VISION.md verification)
- [ ] `HANDOFF.md` + `NEXT_STEPS.md` updated; next chunk seeded
