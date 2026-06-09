# Phase 3a — Configurability: DB-backed Agent Config Registry (file-level implementation spec)

> **Loop:** ultraplan (this doc) → ultracode (Workflow) → local hard gates → ultrareview (owner) → merge + handoff.
> **North star:** [VISION.md](../../VISION.md) — Phase 3 ("Configurability + integration/MCP layer"): *"Agents/integrations are enabled and tuned post-deploy via admin config — no redeploy."* This chunk delivers the **agent** half (3a). Connectors/MCP (3b) and the admin UI (3c) follow.
> **Plan row:** [PHASES_2-4_ULTRALOOP_PLAN.md](./PHASES_2-4_ULTRALOOP_PLAN.md) §"Phase 3 · 3a · Config registry".
> **Last updated:** 2026-06-08.

---

## 0. Decision summary (read this first)

Replace Phase 1/2's purely **in-code** capability gate with a **DB-backed, per-org, admin-editable** one — so an operator enables/disables and tunes each harness capability **post-deploy, no redeploy**. The deterministic core and the isolation contract are untouched.

**The load-bearing idea: split the static *catalog* from the dynamic *config*.**

- **Catalog** = what the *binary* can run. Compile-time fact. Stays in-code in `agentic.Registry` (today: `delay_cascade`, `foresight`, `experience`). Unchanged role; gains per-capability **defaults**.
- **Config** = whether a capability is *enabled* and how it's *tuned*, **per org**. Runtime fact. Lives in a new `agents_config` table, read through a new **leaf-declared port** `agentic.ConfigResolver`, implemented by a `service` adapter.

**Why this approach wins.** It keeps `internal/agentic` a pure leaf (the resolver is a port; `CapabilityConfig{Enabled bool; Config json.RawMessage}` and `uuid.UUID` use only already-imported stdlib + `uuid`), it co-locates the new "enabled?" check next to the existing "known?" check (`registry.Lookup`) inside each orchestrator's gate, and it reuses the established admin-CRUD template (integration credentials) verbatim. The runner-up — folding config into the `Registry` type — was rejected: `Registry` is documented "populate once, then read-only / not concurrency-safe," and giving it a `Resolve(ctx, orgID)` method drags per-request/per-org state and `context.Context` into a shared immutable type, conflating the very split we're making.

**Default semantics:** no DB row ⇒ **enabled with the catalog default config**. Preserves today's behavior (all three capabilities run unconditionally now), needs **no migration seeding**, and makes the admin surface purely **opt-out / override**. An admin disables by writing `enabled=false`; resets by `DELETE` (remove the override row).

**The one real "tune" proof (not just on/off): foresight thresholds.** `foresight` already has a documented "phase-3 seam" — `ForesightThresholds{ScheduleFloatDays, BudgetBurnPercent}` baked into the (shared) workspace. 3a makes those **per-org configurable** end-to-end: set `{"budget_burn_percent":50}` via the API → that org's next sweep surfaces budget risks at 50% burn. `delay_cascade` and `experience` get **enable/disable only** in 3a; the generic `config` JSONB + resolver make their tuning a trivial follow-up.

**Capability-by-capability gate placement** (principled — the gate lives at each capability's real *trigger boundary*):

| Capability | Trigger shape | Where the enabled/config gate lives | Resolver injected into |
|---|---|---|---|
| `delay_cascade` | 1 River job per (org, project) | inside `Orchestrator.RunDelayCascade` (resolve, then gate) | `agentic.Orchestrator` |
| `experience` | 1 HTTP request per (org, caller) | inside `Assistant.Converse` (resolve, then gate) | `agentic.Assistant` |
| `foresight` | 1 periodic **cross-org sweep** fanning out to N projects | inside `service.ForesightSweepService` at the **per-org boundary** (memoized) — *not* per-project | `service.ForesightSweepService` |

Foresight diverges deliberately: the sweep is a cross-org keyset fan-out (`foresight_sweep.go` — projects interleave across orgs, not grouped). Resolving inside the per-project orchestrator would do **O(projects)** identical reads and — worse — a resolver DB error would fall into the sweep's per-project *log-and-continue* bucket and silently kill the whole fleet's foresight with zero failed jobs. So the sweep memoizes **one resolve per org per sweep**, short-circuits a disabled org before its project loop, and a resolve error **fails the sweep retryably** (River retries).

**This chunk is NOT blocked by — but explicitly surfaces — [ESC-002](./ESCALATION_LOG.md#esc-002):** every self-minted token carries `plan_tier=""`, so `RequirePlanTier(pro)` 402-walls the Experience HTTP endpoint for all real callers today (inherited, post-Brain-stale). 3a's config registry is correct and reachable regardless (`delay_cascade`/`foresight` are unguarded worker flows; the admin surface is admin-gated, not plan-gated). The owner decides the gate's fate separately.

---

## 1. Package / file layout

### New files

| File | Purpose | Isolation |
|---|---|---|
| `internal/agentic/config.go` | **Leaf.** `ConfigResolver` port, `CapabilityConfig`, `ForesightTuning` + `withDefaults`, `ErrCapabilityDisabled` sentinel, foresight default consts. | stdlib + `uuid` only |
| `internal/agentic/config_test.go` | **Leaf.** `ForesightTuning.withDefaults` table test; a local leaf-pure fake `ConfigResolver` used by orchestrator tests. | no `internal/*` import |
| `internal/models/agent_config.go` | `AgentConfig` domain struct (org-scoped row shape). | — |
| `internal/store/agent_config.go` | `AgentConfigStore` (stateless; `pgx.Tx`; org-scoped SQL). `Upsert` / `GetByCapability` / `ListByOrg` / `DeleteByCapability`. | — |
| `internal/store/agent_config_integration_test.go` | `//go:build integration` round-trips against ephemeral PG (`testdb.NewPool`). | — |
| `internal/service/agent_config.go` | `AgentConfigService` — **two faces**: (1) admin CRUD (`Get`/`List`/`Set`/`Reset`), (2) `agentic.ConfigResolver` (`Resolve`). One-tx-per-mutation + audit. | imports `agentic` (adapter side — allowed) |
| `internal/service/agent_config_integration_test.go` | `//go:build integration` — service CRUD + resolver default/override/disabled paths. | — |
| `internal/api/agent_config.go` | `AgentConfigHandler` (`List`/`Set`/`Reset`) + `writeAgentConfigError` + `MountAgentConfigRoutes`. | — |
| `internal/api/agent_config_test.go` | Handler unit tests (RBAC, 404-unknown-capability, 400-malformed, 204-idempotent-reset). | — |
| `migrations/016_agents_config.up.sql` / `.down.sql` | Additive `agents_config` table + index. | — |

### Edited files

| File | Edit |
|---|---|
| `internal/agentic/registry.go` | `Descriptor` gains `DefaultEnabled bool` + `DefaultConfig json.RawMessage`; `NewRegistry` seeds all three (foresight's `DefaultConfig` = `{"schedule_float_days":2,"budget_burn_percent":80}`, `DefaultEnabled:true` for all). |
| `internal/agentic/orchestrator.go` | `NewOrchestrator` gains trailing `resolver ConfigResolver`; `RunDelayCascade` resolves + gates after `Lookup`. |
| `internal/agentic/assistant.go` | `NewAssistant` gains trailing `resolver ConfigResolver`; `Converse` gains `orgID uuid.UUID` param; resolves + gates after `Lookup`, returns `ErrCapabilityDisabled` when disabled. |
| `internal/agentic/foresight.go` | `ForesightWorkspace.LoadForesightContext` port **signature change**: gains `tuning ForesightTuning`. `RunForesight` gains a `tuning ForesightTuning` param, forwards it to `LoadForesightContext`. **No** resolver on the foresight orchestrator (the sweep owns its gate). |
| `internal/service/agentic.go` | `CascadeWorkspace` adapter: no change to its methods. (Cascade resolver lives in the orchestrator, injected at the factory.) |
| `internal/service/foresight.go` | `ForesightWorkspace` adapter: `LoadForesightContext` consumes `tuning` (replaces the `w.cfg` reads at the two threshold sites). Delete the `cfg ForesightThresholds` field + the `defaultForesight*` consts + the `ForesightThresholds` type; consolidate defaults to the leaf. `NewForesightWorkspace` drops its `cfg` param. |
| `internal/service/foresight_sweep.go` | `ForesightSweepService` gains `config agentic.ConfigResolver`; the fan-out loop memoizes per-org `{enabled, tuning}`, resolve-error → retryable sweep error, disabled org → skip + log-once, passes `tuning` into `RunForesight`. `newOrch` signature unchanged. |
| `internal/service/assistant.go` | `AssistantService` gains `config agentic.ConfigResolver`; `NewAssistantService` gains the param; `Converse` passes `callerOrgID` + resolver into `agentic.NewAssistant(...)` and threads orgID to `Converse`. Propagate `ErrCapabilityDisabled` verbatim (like `ErrAssistantUnavailable`). |
| `internal/api/assistant.go` (or `agents.go`) | `writeAIServiceError` gains a branch: `agentic.ErrCapabilityDisabled` → **403 `CAPABILITY_DISABLED`** (distinct code; **not** 503). |
| `internal/api/router.go` | New `RouterConfig.AgentConfigService api.AgentConfigServicer`; conditionally `MountAgentConfigRoutes` under Auth + SetupGate + `RequireMinRole(admin)`, **off** the `/api/v1/agents` (pro-gated) tree. |
| `cmd/server/main.go` | Build `store.NewAgentConfigStore()` + `service.NewAgentConfigService(...)`; pass it to `RouterConfig.AgentConfigService` **and** into `NewAssistantService(..., agentConfigService)`. |
| `cmd/worker/main.go` | Build the same `*service.AgentConfigService` (resolver face); inject into `cascadeOrchestratorFactory` (→ `NewOrchestrator(..., resolver)`) and into `NewForesightSweepService(..., resolver, ...)`. |
| `internal/agentic/orchestrator_test.go`, `assistant_test.go`, `foresight*_test.go` and the `service`/`api` test call sites | Mechanical call-site updates for the constructor/port signature changes (see §11, "blast radius"). |
| `.agents/handoff/API_CONTRACT.md` | Add the 3 endpoints + status codes, an RBAC matrix row, a SetupGate note (non-optional per CLAUDE.md). |

### Explicitly NOT touched (isolation proof)
`internal/physics`, `internal/currency` — no edits, no new imports. `internal/agentic` gains **zero** new third-party/`internal/*` imports (`encoding/json`, `context`, `uuid`, `errors`, `fmt`, `log/slog` are all already present). `make lint-isolation` stays green (Check 1: core ⊬ agentic; Check 2: agentic imports no `internal/*`, walking `.Imports` **and** `.TestImports` — so every test-side `ConfigResolver` fake must be a local leaf-pure struct).

---

## 2. Key Go types / interfaces / signatures

### 2a. `internal/agentic/config.go` (NEW leaf)

```go
package agentic

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// ErrCapabilityDisabled signals an org has explicitly disabled a capability via
// admin config. Synchronous flows (experience) propagate it so the handler can
// map a deliberate admin state to a clean 403 (distinct from the 503 a missing
// AI key produces). Worker flows do NOT use it — they clean no-op instead.
var ErrCapabilityDisabled = errors.New("agentic: capability disabled by config")

// CapabilityConfig is the resolved, per-org runtime config for one capability.
// Enabled gates the flow; Config is the capability-specific tuning blob (a JSON
// object; "{}" when untuned). The leaf never interprets Config's shape except
// via the typed helpers below — interpretation is the caller's job.
type CapabilityConfig struct {
	Enabled bool
	Config  json.RawMessage
}

// ConfigResolver resolves per-org capability config at runtime. The adapter in
// internal/service reads the agents_config table and falls back to the in-code
// catalog default when no row exists. A read error is an infrastructure failure
// the caller must treat as hard (retry / 5xx) — NOT a soft-fail.
//
// orgID is the TENANT key only. It must never influence RBAC/tool scoping
// (that stays structural elsewhere).
type ConfigResolver interface {
	Resolve(ctx context.Context, orgID uuid.UUID, c Capability) (CapabilityConfig, error)
}

// --- foresight tuning (the one capability whose Config drives behavior in 3a) ---

const (
	defaultForesightScheduleFloatDays = 2  // schedule risk: critical OR total_float <= this
	defaultForesightBudgetBurnPercent = 80 // budget risk: actual*100/estimated >= this
)

// ForesightTuning is the typed per-org foresight knobs parsed from
// CapabilityConfig.Config. The workspace receives this (typed ints), never raw
// JSON — the leaf does selection/plumbing, never schedule/money math.
type ForesightTuning struct {
	ScheduleFloatDays int `json:"schedule_float_days"`
	BudgetBurnPercent int `json:"budget_burn_percent"`
}

// DefaultForesightTuning is the single in-code source of truth for the default
// thresholds. NewRegistry marshals it into the foresight Descriptor.DefaultConfig.
func DefaultForesightTuning() ForesightTuning {
	return ForesightTuning{
		ScheduleFloatDays: defaultForesightScheduleFloatDays,
		BudgetBurnPercent: defaultForesightBudgetBurnPercent,
	}
}

// withDefaults replaces any non-positive field with its default, so a partial or
// garbage config still yields fully-valid thresholds (defensive; never errors).
func (t ForesightTuning) withDefaults() ForesightTuning {
	if t.ScheduleFloatDays <= 0 {
		t.ScheduleFloatDays = defaultForesightScheduleFloatDays
	}
	if t.BudgetBurnPercent <= 0 {
		t.BudgetBurnPercent = defaultForesightBudgetBurnPercent
	}
	return t
}

// ParseForesightTuning totally-parses a config blob into thresholds: a malformed
// or partial blob yields defaults, NEVER an error (a config that passed the
// write-time object check must not be able to fail a worker job at read time).
func ParseForesightTuning(cfg json.RawMessage) ForesightTuning {
	var t ForesightTuning
	if len(cfg) > 0 {
		_ = json.Unmarshal(cfg, &t) // ignore error: defensive, defaults below
	}
	return t.withDefaults()
}
```

### 2b. `internal/agentic/registry.go` (edit)

```go
type Descriptor struct {
	Capability     Capability      `json:"capability"`
	Description    string          `json:"description"`
	DefaultEnabled bool            `json:"default_enabled"` // no-row fallback (true today; lets a future expensive agent ship OFF)
	DefaultConfig  json.RawMessage `json:"default_config"`  // no-row tuning fallback ("{}" for untuned capabilities)
}
```

`NewRegistry()` seeds all three with `DefaultEnabled:true`; `delay_cascade`/`experience` `DefaultConfig: json.RawMessage("{}")`; `foresight` `DefaultConfig:` the marshaled `DefaultForesightTuning()` (= `{"schedule_float_days":2,"budget_burn_percent":80}`). Keep `Register`'s empty-key panic.

### 2c. `internal/agentic/orchestrator.go` (edit)

```go
func NewOrchestrator(reasoner Reasoner, workspace CascadeWorkspace, resolver ConfigResolver, logger *slog.Logger) *Orchestrator

// RunDelayCascade gate (after the existing registry.Lookup(DelayCascade) check):
cfg, err := o.resolveEnabled(ctx, in.OrgID, DelayCascade) // helper; nil resolver => {Enabled:true, default}
if err != nil {
	return CascadeResult{}, fmt.Errorf("agentic: resolve config: %w", err) // hard → River retries
}
if !cfg.Enabled {
	log.InfoContext(ctx, "delay cascade disabled by config", slog.String("reason", "capability_disabled"))
	return CascadeResult{}, nil // clean no-op
}
// ... unchanged: LoadCascadeContext → reasoner → ApplyCascade ...
```

`resolveEnabled` is a tiny private helper on the orchestrator: `if o.resolver == nil { d,_ := o.registry.Lookup(c); return CapabilityConfig{Enabled: d.DefaultEnabled, Config: d.DefaultConfig}, nil }; return o.resolver.Resolve(ctx, orgID, c)`. **nil resolver ⇒ enabled-with-default** — preserves current behavior and the (now-updated) tests that pass `nil`.

### 2d. `internal/agentic/assistant.go` (edit)

```go
func NewAssistant(p ChatPlanner, resolver ConfigResolver, b LoopBounds, logger *slog.Logger) *Assistant

// orgID added; doc note: used ONLY to key resolver.Resolve — never tool scoping.
func (a *Assistant) Converse(ctx context.Context, orgID uuid.UUID, sys string, in ChatInput, reg *AssistantRegistry) (ChatResult, error) {
	if _, ok := a.registry.Lookup(Experience); !ok { /* unchanged not-registered error */ }
	cfg, err := a.resolveEnabled(ctx, orgID, Experience)
	if err != nil {
		return ChatResult{}, fmt.Errorf("agentic: resolve config: %w", err) // hard → 500
	}
	if !cfg.Enabled {
		a.logger.InfoContext(ctx, "experience disabled by config", slog.String("reason", "capability_disabled"))
		return ChatResult{}, ErrCapabilityDisabled // → handler 403 CAPABILITY_DISABLED
	}
	// ... unchanged: a.planner.Plan(...) ...
}
```

### 2e. `internal/agentic/foresight.go` (edit) — the port-contract change

```go
type ForesightWorkspace interface {
	// tuning carries the per-org thresholds (already defaulted). The workspace
	// reads tuning.ScheduleFloatDays / .BudgetBurnPercent where it previously
	// read its baked-in cfg fields.
	LoadForesightContext(ctx context.Context, in ForesightInput, tuning ForesightTuning) (ForesightContext, error)
	ApplyForesight(ctx context.Context, in ForesightInput, c ForesightContext, plan ForesightPlan) (ForesightResult, error)
}

// RunForesight gains tuning; NO resolver/enabled check here (the SWEEP gates it).
func (o *ForesightOrchestrator) RunForesight(ctx context.Context, in ForesightInput, tuning ForesightTuning) (ForesightResult, error) {
	if _, ok := o.registry.Lookup(Foresight); !ok { /* unchanged */ }
	fc, err := o.workspace.LoadForesightContext(ctx, in, tuning) // <- tuning threaded
	// ... rest unchanged (HasMaterialSignal gate, JudgeRisks soft-fail, ApplyForesight) ...
}
```

### 2f. `internal/models/agent_config.go` (NEW)

```go
type AgentConfig struct {
	ID         uuid.UUID       `json:"id"`
	OrgID      uuid.UUID       `json:"org_id"`
	Capability string          `json:"capability"`
	Enabled    bool            `json:"enabled"`
	Config     json.RawMessage `json:"config"`
	UpdatedBy  string          `json:"updated_by"` // OIDC subject of last writer
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
```

### 2g. `internal/store/agent_config.go` (NEW — mirrors `integration_credentials.go`)

```go
type AgentConfigStore struct{}
func NewAgentConfigStore() *AgentConfigStore { return &AgentConfigStore{} }

type UpsertAgentConfigParams struct {
	OrgID      uuid.UUID
	Capability string
	Enabled    bool
	Config     []byte // raw JSONB; nil => "{}"
	UpdatedBy  string
}

func (s *AgentConfigStore) Upsert(ctx context.Context, tx pgx.Tx, p UpsertAgentConfigParams) (models.AgentConfig, error)
func (s *AgentConfigStore) GetByCapability(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, capability string) (models.AgentConfig, error) // pgx.ErrNoRows → store.ErrNotFound
func (s *AgentConfigStore) ListByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]models.AgentConfig, error)                        // ORDER BY capability
func (s *AgentConfigStore) DeleteByCapability(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, capability string) (int64, error)           // returns rows affected
```

`Upsert` SQL: `INSERT ... ON CONFLICT (org_id, capability) DO UPDATE SET enabled=EXCLUDED.enabled, config=EXCLUDED.config, updated_by=EXCLUDED.updated_by, updated_at=now() RETURNING <all cols>`. Every query filters `WHERE org_id=$1` first.

### 2h. `internal/service/agent_config.go` (NEW — two faces)

```go
type AgentConfigService struct {
	pool    *pgxpool.Pool
	store   *store.AgentConfigStore
	catalog *agentic.Registry // catalog (existence authority + defaults)
	audit   AuditRecorder
	logger  *slog.Logger
}

func NewAgentConfigService(pool *pgxpool.Pool, st *store.AgentConfigStore, audit AuditRecorder, logger *slog.Logger) *AgentConfigService

// --- Face 1: admin CRUD ---
type EffectiveAgentConfig struct {       // one row of the GET list
	Capability  string
	Description string
	Enabled     bool
	Config      json.RawMessage
	Source      string                    // "default" (no row) | "override" (row present)
	UpdatedBy   string                    // "" when default
	UpdatedAt   *time.Time                // nil when default
}
func (s *AgentConfigService) ListEffective(ctx context.Context, orgID uuid.UUID) ([]EffectiveAgentConfig, error) // catalog ⟕ rows
func (s *AgentConfigService) Set(ctx context.Context, in SetAgentConfigInput) (models.AgentConfig, error)        // validate capability∈catalog (ErrNotFound), validate config (ErrInvalidInput), upsert+audit in one tx
func (s *AgentConfigService) Reset(ctx context.Context, orgID uuid.UUID, capability, userSub string) error       // validate capability∈catalog; delete; audit ONLY if a row was removed; idempotent (no row => nil)

// --- Face 2: agentic.ConfigResolver ---
func (s *AgentConfigService) Resolve(ctx context.Context, orgID uuid.UUID, c agentic.Capability) (agentic.CapabilityConfig, error)
```

`Resolve` (read-only tx): `GetByCapability`; on `store.ErrNotFound` return the catalog default `{Enabled: d.DefaultEnabled, Config: d.DefaultConfig}`; on a row return `{row.Enabled, row.Config}`; any other error → wrap and return (hard).

`Set` validation: capability must be in `catalog.Lookup` (else `ErrNotFound` → 404); `config` must be a JSON **object** (`json.Valid` + first non-space byte `{`); for `foresight`, additionally reject non-integer/negative `schedule_float_days`/`budget_burn_percent` (`ErrInvalidInput` → 400). `org_id` is from claims (handler), never the body. Audit `agent.config.updated` inside the upsert tx.

### 2i. `internal/api/agent_config.go` (NEW — mirrors `integrations.go`)

```go
type AgentConfigServicer interface {            // router depends on the interface (nil-safe mount)
	ListEffective(ctx context.Context, orgID uuid.UUID) ([]service.EffectiveAgentConfig, error)
	Set(ctx context.Context, in service.SetAgentConfigInput) (models.AgentConfig, error)
	Reset(ctx context.Context, orgID uuid.UUID, capability, userSub string) error
}

func (h *AgentConfigHandler) List(w, r)  // GET    /api/v1/admin/agents
func (h *AgentConfigHandler) Set(w, r)   // PUT    /api/v1/admin/agents/{capability}
func (h *AgentConfigHandler) Reset(w, r) // DELETE /api/v1/admin/agents/{capability}

func MountAgentConfigRoutes(r chi.Router, h *AgentConfigHandler) {
	r.Route("/api/v1/admin/agents", func(r chi.Router) {
		r.Use(mw.RequireMinRole(mw.RoleAdmin))
		r.Get("/", h.List)
		r.Put("/{capability}", h.Set)
		r.Delete("/{capability}", h.Reset)
	})
}
```

`writeAgentConfigError`: `service.ErrNotFound` → 404 `NOT_FOUND`; `service.ErrInvalidInput` → 400 `VALIDATION_ERROR`; default → 500 `INTERNAL_ERROR`. Handlers use `callerOrgIDFromClaims(w, r)` + `mw.MustClaimsFromContext` + `chi.URLParam(r,"capability")` + `writeJSON`.

---

## 3. End-to-end flow

**Admin disables foresight for an org (no redeploy):**
`PUT /api/v1/admin/agents/foresight {"enabled":false,"config":{}}` → Auth → SetupGate → `RequireMinRole(admin)` → `AgentConfigHandler.Set` → `AgentConfigService.Set` (capability∈catalog ✓, config valid ✓) → one tx: `store.Upsert` + `audit.Record(agent.config.updated)` → `200 {data:{agent}}`. **Next** foresight sweep: at this org's first project the sweep `Resolve(org, foresight)` → row `enabled=false` → memoize → **skip all this org's projects**, log once `reason=capability_disabled`. No worker, no AI call, no cards. Re-enable (`PUT enabled:true`) or `DELETE` (reset) → next sweep runs again.

**Admin tunes the budget-burn threshold:**
`PUT /api/v1/admin/agents/foresight {"enabled":true,"config":{"budget_burn_percent":50}}` → upsert. Next sweep: `Resolve` → `ParseForesightTuning` → `{ScheduleFloatDays:2 (default), BudgetBurnPercent:50}` → `RunForesight(ctx, in, tuning)` → `LoadForesightContext` flags budget lines breached at ≥50% (was ≥80%). More risks surface — **behavior changed with no redeploy**.

**Operator disables the chat assistant:**
`PUT /api/v1/admin/agents/experience {"enabled":false}` → next `POST /api/v1/agents/chat` → `Assistant.Converse` resolves disabled → `ErrCapabilityDisabled` → `AssistantService.Converse` propagates → `writeAIServiceError` → **403 `CAPABILITY_DISABLED`**.

---

## 4. Migration — `migrations/016_agents_config.{up,down}.sql`

`up.sql` (passes all 5 linter rules — no money columns so rules 1-2 are N/A; index uses `lock-ok` because the migrate runner wraps every file in a tx, `cmd/migrate/main.go:119`, so `CONCURRENTLY` would 25001):

```sql
CREATE TABLE agents_config (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    capability  TEXT NOT NULL,                       -- matches an in-code agentic.Capability key
    enabled     BOOLEAN NOT NULL DEFAULT true,
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,  -- capability-specific tuning; NEVER secrets (use the vault)
    updated_by  TEXT NOT NULL DEFAULT '',            -- OIDC subject of last writer
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, capability)                      -- one config row per (org, capability); enables ON CONFLICT
);

CREATE INDEX idx_agents_config_org ON agents_config (org_id); -- buildos:lock-ok: fresh table created in same migration
```

`down.sql`:
```sql
-- buildos:destructive: rollback of the agents_config registry table (pre-prod / config only — no operational data lost)
DROP TABLE IF EXISTS agents_config;
```

No seeding — absence of a row = enabled-with-default. The `UNIQUE(org_id, capability)` constraint (not a partial index) is mandatory for `ON CONFLICT (org_id, capability)`.

---

## 5. Foresight tuning — port change detail (the one breaking interface change)

The `ForesightWorkspace` is a **single shared instance** (`cmd/worker/main.go:175` builds it once; `foresight.go` doc: "carries no per-org state, reused across invocations"), with thresholds baked into a `cfg ForesightThresholds` constructor field read at the two breach sites. Per-org tuning therefore **cannot** key off that field — it must arrive **per call**. So:

1. **Port:** `LoadForesightContext(ctx, in, tuning ForesightTuning)` (leaf signature change).
2. **Adapter (`service/foresight.go`):** read `tuning.ScheduleFloatDays` / `tuning.BudgetBurnPercent` where it read `w.cfg.*`. Delete the `cfg` field, the `defaultForesight*` consts, and the `ForesightThresholds` type. `NewForesightWorkspace` drops its `cfg` param.
3. **Sweep (`service/foresight_sweep.go`):** memoize per org per sweep:

```go
type orgForesightCfg struct { enabled bool; tuning agentic.ForesightTuning }
cfgByOrg := make(map[uuid.UUID]orgForesightCfg) // per-sweep memo (declared above the page loop)
// ... inside the per-project loop, before newOrch/RunForesight:
fc, ok := cfgByOrg[p.OrgID]
if !ok {
	cc, rErr := s.config.Resolve(ctx, p.OrgID, agentic.Foresight)
	if rErr != nil {
		return fmt.Errorf("foresight sweep: resolve config for org %s: %w", p.OrgID, rErr) // RETRYABLE — not the per-project continue bucket
	}
	fc = orgForesightCfg{enabled: cc.Enabled, tuning: agentic.ParseForesightTuning(cc.Config)}
	cfgByOrg[p.OrgID] = fc
	if !fc.enabled {
		log.InfoContext(ctx, "foresight sweep: capability disabled for org",
			slog.String("org_id", p.OrgID.String()), slog.String("reason", "capability_disabled"))
	}
}
if !fc.enabled { continue } // skip this org's projects
orch := s.newOrch(p.OrgID)
res, runErr := orch.RunForesight(ctx, agentic.ForesightInput{OrgID: p.OrgID, ProjectID: p.ID}, fc.tuning)
```

4. **Fakes/integration tests:** every `ForesightWorkspace` fake gains the `tuning` param; foresight integration tests assert tuning flows (default vs override).

`NewForesightSweepService(pool, projectStore, procChecker, resolver, newOrch, logger)` — resolver added (e.g. after `procChecker`).

---

## 6. RBAC / routing / API_CONTRACT

- **Route group:** `/api/v1/admin/agents` — a **new `/api/v1/admin/*` namespace** (does not exist yet; we commit to it as the operator-admin convention, documented in API_CONTRACT). Behind Auth + **SetupGate** (agent config is operational, not bootstrap — correctly 403s `SETUP_INCOMPLETE` pre-onboarding) + `RequireMinRole(admin)`. **Off** the `/api/v1/agents` pro-tier tree, so the kill-switch is reachable regardless of plan tier (and regardless of ESC-002).
- **Conditional mount:** `if cfg.AgentConfigService != nil { MountAgentConfigRoutes(r, agentConfigHandler) }` in `router.go`, inside the authenticated group (after SetupGate `r.Use`, before routes — chi panics if `Use` follows a route).
- **REST semantics:**
  - `GET /api/v1/admin/agents` → `200 {data:{agents:[EffectiveAgentConfig]}}`. Catalog ⟕ rows; `source` discriminates default vs override. Reads not audited.
  - `PUT /api/v1/admin/agents/{capability}` → **full-document upsert** (`enabled` + `config` both authoritative; omitted/`null` `config` ⇒ catalog default). `200 {data:{agent}}`. `404 NOT_FOUND` (capability ∉ catalog, validated **before** any DB write), `400 VALIDATION_ERROR` (config not an object / invalid foresight ints). **Not** PATCH — no partial merge. Last-writer-wins (documented; `updated_at`/`updated_by` are the future If-Match seam).
  - `DELETE /api/v1/admin/agents/{capability}` → reset to default. **Idempotent 204** whether or not an override row existed; `404` only for capability ∉ catalog. Audit `agent.config.reset` **only when a row was actually deleted** (no phantom-reset rows).
- **API_CONTRACT.md edits (required):** add the 3 endpoints + status codes, an "Agent config (`/admin/agents/*`)" RBAC matrix row (owner ✓ / admin ✓ / superintendent ✗ / field_worker ✗), and a one-line SetupGate note.

---

## 7. Audit / Composite Currency / PII / observability compliance

- **Audit:** `agent.config.updated` (PUT) / `agent.config.reset` (DELETE-when-row-existed) — `<singular-noun>.<resource>.<verb>`, matching `integration.credential.set`/`.deleted`. `ResourceType: AuditResourceAgentConfig` ("agent_config"), `ResourceID` = the row id (RETURNING / deleted-row id), `UserSub` from claims, `Metadata` = `{capability, enabled}` (+ `config` for `updated`). Written **inside the mutation tx** via `s.audit.Record(ctx, tx, …)`.
- **Composite Currency:** N/A — `agents_config` has **no monetary columns** (the foresight `budget_burn_percent` is an integer percent, not cents). The deterministic engine still owns all cents math; tuning only adjusts a threshold the engine compares against. No floats introduced.
- **PII:** config blobs are tuning scalars (ints/bools) — no PII, no secrets. `pii.ScrubJSON(Restricted)` at the store-audit layer is a harmless no-op on them. **Do not** store secrets in `agents_config` (that's the vault's job — `internal/cryptobox`); documented in the migration comment.
- **Observability:** worker disabled no-op logs at INFO with `reason="capability_disabled"` + `org_id`(+`capability`), **distinct** from the existing "no critical slip" / "no material signal" no-ops, so an accidental org-wide disable is visible (zero cards + zero errors otherwise looks identical to a quiet fleet).

---

## 8. Isolation compliance (`make lint-isolation` stays green)

- `internal/agentic/config.go` imports only `context`, `encoding/json`, `errors`, `github.com/google/uuid` — all already in the leaf's allowed set.
- The `ConfigResolver` fake used by `orchestrator_test.go`/`assistant_test.go`/`config_test.go` is a **local leaf-pure struct** in `internal/agentic/*_test.go` (Check 2 walks `.TestImports`). It must not import `pgx`/`service`/`store`.
- The service-side `AgentConfigService` imports `agentic` (adapter direction — allowed; arrow points inward). It must not leak a service/store type back across the `ConfigResolver` port (the port speaks only `CapabilityConfig`/`Capability`/`uuid`).
- Core (`physics`/`currency`) untouched → Check 1 stays green.

---

## 9. Inherited risk (surfaced, not silently built atop) — ESC-002

`RequirePlanTier(pro)` 402-walls the Experience HTTP endpoint for every self-minted token (`plan_tier=""`). **3a does not depend on that endpoint being reachable** (the config registry, `delay_cascade`, `foresight`, and the admin surface are all reachable). Filed as [ESC-002](./ESCALATION_LOG.md#esc-002) for an owner decision (populate `plan_tier` at mint vs drop the now-billing-less gate). Do **not** fold an ESC-002 fix into 3a.

---

## 10. Ordered implementation task breakdown (bottom-up; each step compiles)

1. **Leaf `config.go`** — `ConfigResolver`, `CapabilityConfig`, `ForesightTuning`(+`withDefaults`/`ParseForesightTuning`), `ErrCapabilityDisabled`, defaults. `config_test.go` for `withDefaults`/`ParseForesightTuning` + a local fake resolver. (Compiles; isolation-clean.)
2. **`registry.go`** — add `DefaultEnabled`/`DefaultConfig`; seed; update `registry_test.go`.
3. **Orchestrator gates** — `orchestrator.go` (+resolver param, `resolveEnabled`, gate), `assistant.go` (+resolver, +orgID, gate, `ErrCapabilityDisabled`), `foresight.go` (port +`tuning`, `RunForesight` +`tuning`). Update **all** agentic test call sites (pass `nil` resolver / `ForesightTuning{}` / orgID). Add the **nil-resolver ⇒ enabled-with-default** test. (Leaf compiles + `go test ./internal/agentic/...`.)
4. **Migration 016** + `make lint-migrations`.
5. **Model + store** (`agent_config.go`) + integration test.
6. **Service** (`agent_config.go`) — both faces; integration test (default/override/disabled/reset).
7. **Foresight adapter + sweep** — consume `tuning`; delete `cfg`/consts/`ForesightThresholds`; sweep memo + resolver; update foresight fakes/integration tests.
8. **Assistant service** — `+config`, thread orgID, propagate `ErrCapabilityDisabled`; update its tests.
9. **API** — handler + `writeAgentConfigError` + `MountAgentConfigRoutes`; `writeAIServiceError` `ErrCapabilityDisabled`→403 branch; handler tests.
10. **Router + wiring** — `RouterConfig.AgentConfigService`; `cmd/server` + `cmd/worker` build + inject the one shared `*AgentConfigService` (admin face to router, resolver face to assistant + cascade factory + sweep).
11. **API_CONTRACT.md** edits.
12. **Gates:** `make audit` (incl. `lint-isolation` 1+2, `lint-migrations`, `test`, `test-prod`, `bench-physics`) + `govulncheck` (both modes) + `make test-integration`.

**Constructor/port call-site blast radius (verified):** `NewOrchestrator` (cmd/worker `cascadeOrchestratorFactory.RunDelayCascade` + ~11 in `orchestrator_test.go`); `NewAssistant` + `Converse` (service `assistant.go` + ~9 in `agentic/assistant_test.go` + service/api assistant tests); `RunForesight`/`LoadForesightContext` (sweep + foresight adapter + foresight fakes/integration tests); `NewForesightWorkspace` (cmd/worker:175 + tests); `NewForesightSweepService` (cmd/worker:193 + tests); `NewAssistantService` (cmd/server:200 + tests). Treat as one mechanical pass — "preserving behavior" means **updating every call site**, not that they compile untouched.

---

## 11. Verification criteria (definition of done)

### 11.1 Integration tests (ephemeral PG via `testdb.NewPool`)
- **Store:** upsert→get round-trip; `ON CONFLICT` overwrite (second upsert updates, doesn't duplicate); `ListByOrg` ordering + org isolation (other org's rows invisible); delete returns rows-affected; `GetByCapability` miss → `store.ErrNotFound`.
- **Service resolver:** no-row → `{Enabled: true, Config: catalog default}`; row `enabled=false` → `{Enabled:false}`; row with `config` → returned verbatim; foresight default config parses to `{2,80}`.
- **Service CRUD:** `Set` unknown capability → `ErrNotFound`; `Set` malformed config → `ErrInvalidInput`; `Set` foresight negative threshold → `ErrInvalidInput`; `Set` valid → audit `agent.config.updated` present; `Reset` existing → row gone + audit `agent.config.reset`; `Reset` absent → idempotent nil + **no** audit row.
- **Foresight tuning flows:** an org with `{"budget_burn_percent":50}` surfaces a budget risk on a line at 60% burn that the default 80% would not; a disabled org's sweep produces zero cards (and the resolve happens once, not per project — assert via a counting fake resolver).
- **Sweep resolve-error is retryable:** a resolver that errors makes `RunForesightSweep` return non-nil (not swallowed).

### 11.2 Unit tests (no DB)
- `ForesightTuning.withDefaults` / `ParseForesightTuning` (garbage/partial/empty → defaults; never errors).
- **nil-resolver ⇒ enabled-with-default** for `Orchestrator` and `Assistant`.
- `Orchestrator`/`Assistant` with a fake resolver returning `enabled=false` → cascade no-ops; `Converse` → `ErrCapabilityDisabled`.
- Resolver error → orchestrator/Converse return a hard error (not soft-fail).
- Handler: RBAC (superintendent → 403); 404 unknown capability (PUT + DELETE); 400 malformed config; 204 idempotent reset; `ErrCapabilityDisabled` → 403 `CAPABILITY_DISABLED`.

### 11.3 Hard gates — all green
`make audit` (lint-isolation Check 1+2, lint-migrations + lint-migrations-test, test, test-prod, bench-physics ≤200/500ms) + `govulncheck` (default + `-tags=prod`) + full `make test-integration` (exit 0). `make test-prod` specifically proves the `Converse` signature change + resolver injection compile under `-tags=prod`.

### 11.4 Capability demonstrable end-to-end (VISION verification)
A `PUT /api/v1/admin/agents/foresight {"enabled":false}` (or a threshold change) **changes the next sweep's behavior with no redeploy** — covered by the integration tests above, satisfying VISION's "agents are enabled and tuned post-deploy via admin config — no redeploy" bullet for the agent half of Phase 3.

---

## 12. Top risks (carry into ultracode/ultrareview)
1. **Call-site blast radius** — the biggest mechanical risk; the signature changes break ~36 sites at compile time. The ultracode pipeline must compile after each step (bottom-up) and update test sites in lockstep.
2. **Isolation gate on `.TestImports`** — a `ConfigResolver` fake that imports `pgx`/`service` fails Check 2. Keep agentic test fakes leaf-pure.
3. **Foresight port change ripple** — every `ForesightWorkspace` fake/integration impl must add `tuning`; missing one fails compile.
4. **Default drift** — there must be exactly ONE home for the foresight defaults (leaf `DefaultForesightTuning`); the deleted service consts must not linger.
5. **Sweep error semantics** — resolve error must `return` (retryable), not `continue` (swallowed). Easy to get wrong by mirroring the per-project pattern.
6. **`make test-prod`** — verify the prod build tag compiles with the new signatures (auth_prod stub path).
7. **Don't fold ESC-002 in** — keep the plan-tier fix a separate decision.
