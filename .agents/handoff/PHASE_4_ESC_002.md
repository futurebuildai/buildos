# Phase 4 · chunk 1 — ESC-002: drop the vestigial `RequirePlanTier(pro)` gate

**Status:** BUILT on `feat/phase-4-esc-002-drop-pro-gate` (committed, not pushed/merged). All local Go gates green. Awaiting owner review → merge.
**Resolves:** [ESC-002](./ESCALATION_LOG.md#esc-002) (owner chose **Option 2** — drop the gate). Phase 4 plan: [PHASES_2-4_ULTRALOOP_PLAN.md](./PHASES_2-4_ULTRALOOP_PLAN.md) §"Phase 4".

## Why (verified against code)
Post-pivot, BuildOS mints **every** access token with an empty `plan_tier` claim (`internal/service/auth.go` — login/refresh both `Mint(..., "")`). The `RequirePlanTier` middleware ranked an empty tier as *free* (rank 1) and returned **402 `UPGRADE_REQUIRED`** for any route gated at `pro` (rank 3). Net: the entire `/api/v1/agents/*` AI surface — the flagship product story — was **unreachable for every real caller**. It was invisible in CI because the `DEV_AUTH_MODE=header` bypass defaults `plan_tier` to `enterprise`. Billing was already removed in the standalone pivot ([ESC-001](./ESCALATION_LOG.md)), so the tier gate had no backing system — it was vestigial.

## What changed (Go only; no migration, no new dependency)
- **`internal/api/router.go`** — removed all three `RequirePlanTier(mw.PlanTierPro)` usages, **keeping the role gates**:
  - `/api/v1/agents` group (`POST /daily-briefing`) — now any authenticated role (a caller's own briefing).
  - `POST /api/v1/agents/chat` — keeps `RequireMinRole(superintendent)`.
  - `POST /api/v1/projects/{id}/schedule/recommend-adjustments` — keeps `RequireMinRole(superintendent)`.
- **Deleted** `internal/api/middleware/plan.go` + `plan_test.go` (dead after the removals).
- **Kept** the `plan_tier` claim plumbing — `Claims.PlanTier`, the `Mint(..., planTier)` param, the dev-header 4th field, the `organizations.plan_tier` column — so a tiering model can return from git history with no schema change.
- Updated stale comments in `assistant.go` / `service/agents.go`; refreshed `API_CONTRACT.md` §12 + the RBAC matrix; recorded the resolution in `ESCALATION_LOG.md`.

## Verification
- **New regression guard:** `TestNewRouter_AgentsSurface_RealTokenNotPlanWalled` (`internal/api/router_test.go`) mints a **real RS256 token with `plan_tier=""`** (the production shape, not the dev header), runs it through real JWT verification, and asserts `POST /api/v1/agents/daily-briefing` is reachable (200, **not 402**). Pre-change this route 402'd; the test fails if the gate is ever re-added without populating `plan_tier`.
- **Gates green:** `make audit` ALL PASSED (full unit suite incl. the new test + test-prod + bench 148µs/346µs + migration linter + backup suite); `make lint-isolation` PASSED; `go build`/`go vet` clean default + `-tags=prod` + `-tags=integration`; `make test-integration` (Testcontainers). govulncheck is CI-covered (this chunk removes code / adds no deps).

## Definition of done
- [x] Spec (this file) · [x] feature branch, local gates green · [ ] `/code-review` (owner) · [x] capability demonstrable (the regression test proves the AI surface is reachable for a real token) · [ ] HANDOFF/NEXT_STEPS updated + next chunk seeded.
