# Phase 2b — Foresight: Cross-Module Risk Agents (file-level implementation spec)

> **Status:** Spec ready for ultracode · **Author:** Lead architect (ultraplan) · **Date:** 2026-06-08
> **Companion:** [VISION.md](../../VISION.md) (north star) · [PHASES_2-4_ULTRALOOP_PLAN.md](PHASES_2-4_ULTRALOOP_PLAN.md) (chunk 2b row) · [PHASE_2A_INGESTION.md](PHASE_2A_INGESTION.md) (sibling chunk shape)
> **Chunk goal (verbatim from the plan):** cross-module risk/recommendation agents (procurement
> criticality, schedule-slip risk, budget burn) surfaced as feed cards. The harness **foresight**
> role: *proactively surface material standing risks the deterministic engine has computed, judged
> for materiality by AI, as actionable feed cards — without spamming a fresh card every sweep.*

---

## 0. Decision summary (read this first)

Phase 2b ships Foresight **in the agentic harness** as a new capability that **mirrors `delay_cascade`**:
a `Foresight` capability + `ForesightReasoner`/`ForesightWorkspace` ports in `internal/agentic` (leaf),
service-layer adapters in `internal/service`, a **periodic** River sweep that fans out per-project, and
a deduped feed-card + audit apply in one tx. This is the opposite call from 2a (which went
service-direct) — and deliberately so: **2a ingestion was a linear one-AI-call→one-write transform with
no cross-module judgment**, so the orchestrator machinery was unearned. **2b foresight IS cross-module
AI judgment over deterministic facts** (is this 81%-burn line material given the schedule? does this
low-float task matter?) — exactly what the Phase-1 ports were built for. The harness home is earned here.

### Why agentic-capability wins (and the runner-up tradeoff)

| | **Chosen: agentic capability (mirror delay_cascade)** | **Runner-up: deterministic-rules-first / service-direct** |
|---|---|---|
| Shape | `Foresight` capability + 2 ports + 2 adapters + `ForesightOrchestrator` + periodic sweep | rules compute metrics → cards directly; AI only "polishes" or is skipped |
| Fit | The materiality call ("is this breach worth a card?") + cross-dimension ranking is genuine AI **judgment**; the ports exist precisely to isolate that fuzzy leg from the deterministic engine. | Under-uses the harness the VISION makes top-priority; collapses to "another rule engine + chatbot." |
| Spam control | Same gate/dedup discipline applies; AI is the materiality filter that *prevents* rule-only spam | A pure rules-first design that emits a card per breach IS the over-firing failure mode delay_cascade taught us to avoid |
| Soft-fail | No key → no-op the AI judgment (deterministic legs still run); no rule-only cards (they'd reintroduce spam) | "Degrade to rule-only cards" sounds nice but contradicts "AI judges materiality" and re-opens the spam door |

**Runner-up's redeeming idea we GRAFT:** the runner-up correctly insists Foresight must **not duplicate
`procurement_check`'s rule logic**. We adopt that: the procurement dimension **reads the persisted
`procurement_items.status`** column (which `procurement_check` already computes) as the breach signal —
it does **not** re-derive the must-order threshold. See §6 (dimension 1) and §10 (ordering dependency).

### The two fatal flaws every candidate shipped — RESOLVED here (verified against the code)

1. **Migration `CREATE INDEX CONCURRENTLY` is dead-on-arrival.** `cmd/migrate/main.go:119-136` wraps
   **every** migration file in `pool.Begin → tx.Exec → tx.Commit`. Postgres rejects
   `CREATE INDEX CONCURRENTLY` inside a tx block (`25001`). **Verified:** zero of the 14 existing
   migrations use `CONCURRENTLY`; all use plain `CREATE INDEX` + `-- buildos:lock-ok:`. The lint Check 5
   (`scripts/lint-migrations.sh:268-280`) accepts `-- buildos:lock-ok: <reason>` as the same-line opt-out.
   **§9 ships a plain `CREATE UNIQUE INDEX` with a `lock-ok` annotation as the PRIMARY artifact**, not a
   fallback. The partial index is on the small per-fork `feed_cards` table — a brief ACCESS EXCLUSIVE is
   acceptable.

2. **The dedup unique index has a NULL hole.** Postgres treats `NULL` as distinct in a unique index, so
   a nullable `subject_code` would let the budget project-total case slip through with **zero**
   uniqueness enforcement — re-opening the spam bug exactly where it carves out `"total"`. **Resolved:**
   `subject_code` is `NOT NULL DEFAULT ''` (it has a concrete value for every risk card; the partial
   index's `card_type IN (...)` predicate — not nullability — is what excludes non-risk cards). And
   `ApplyForesight` treats a `23505` unique-violation on insert as a **clean skip**, not a hard error
   (so a concurrent-sweep race degrades to a skip, never a retry storm). See §5.

### Three more critique-driven decisions baked in

- **Separate `ForesightOrchestrator` type, NOT overloading `Orchestrator`.** `NewOrchestrator`
  (`orchestrator.go:34`) hard-builds a `*Registry` and holds concrete `reasoner Reasoner` +
  `workspace CascadeWorkspace` fields. Adding a second port pair would force the shipped, tested cascade
  factory (`cmd/worker/main.go:225`) to change. A separate `ForesightOrchestrator` (its own constructor,
  its own ports) leaves Phase-1 cascade untouched and keeps both leaf-clean. (We reject the
  "generic `Assessment[C,P]` driver" idea: refactoring the only working orchestration to DRY a ~50-line
  control flow is a regression risk not worth taking for instance #2; revisit at instance #3.)
- **Cost gate is real, not hand-waved.** Two compounding gates **before** any AI call: (a)
  `HasMaterialSignal()` — no breach → no AI; (b) **dedup-before-AI** — drop subjects that already have an
  active card while building the context, so a standing risk with a live card contributes **0 tokens** on
  subsequent sweeps. Without (b) the cost claim is off by ~30×. See §8.
- **Foresight-vs-cascade overlap is acknowledged and bounded** (§4): foresight is the *standing-risk
  safety net* on a 24h cadence; delay_cascade is the *real-time alarm* on critical-path change. They use
  different `card_type`s by design; the divergence is documented, not silently duplicated.

---

## 1. Package / file layout

### New files
| Path | Purpose |
|---|---|
| `internal/agentic/foresight.go` | **Leaf.** `Foresight` capability const; domain types (`ForesightInput`, deterministic metric carriers, `ForesightContext` + `HasMaterialSignal`, `ForesightRisk`, `ForesightPlan`, `ForesightResult`); the two ports (`ForesightReasoner`, `ForesightWorkspace`); `ForesightOrchestrator` + `NewForesightOrchestrator` + `RunForesight`. Imports: stdlib + `uuid` + `slog` only. |
| `internal/service/foresight.go` | Adapters: `ForesightReasoner` (AI judgment, typed-nil-safe) + `ForesightWorkspace` (deterministic load in read-only tx; deduped apply in one write tx + audit). Mirrors `internal/service/agentic.go`. |
| `internal/service/foresight_sweep.go` | `ForesightSweepService.RunForesightSweep(ctx)` — the `worker.ForesightRunner`. Recomputes procurement statuses first (§10 R3), keyset-paginates active projects across orgs, builds a per-org `ForesightOrchestrator`, calls `RunForesight` per project. |
| `internal/service/foresight_test.go` | Unit tests: deterministic metric math, `HasMaterialSignal` gate, soft-fail, dedup skip, fake `foresightReasonerAI`. |
| `internal/service/foresight_integration_test.go` | `//go:build integration`: ephemeral PG end-to-end (§12.1). |
| `migrations/015_foresight_dedup.up.sql` | additive: `feed_cards.subject_code` column + partial unique dedup index (plain `CREATE INDEX` + `lock-ok`). |
| `migrations/015_foresight_dedup.down.sql` | paired down (destructive header). |

### Edited files
| Path | Edit |
|---|---|
| `internal/agentic/registry.go` | add `const Foresight Capability = "foresight"`; `Register` it in `NewRegistry()` with a description. |
| `internal/ai/tasks.go` | add the `foresight_risk` task: req/resp types, `foresightRiskSystem`, `foresightRiskSchema`, `(*Client).ForesightRiskJudgment`. Mirrors `DelayCascadeReason`. |
| `internal/store/feed_cards.go` | add `SubjectCode string` to `CreateFeedCardParams` + the INSERT column list ($10); add `HasActiveRiskCard(ctx, tx, HasActiveRiskCardParams) (bool, error)`; map `23505` on insert → new sentinel `ErrDuplicateActiveRiskCard`. |
| `internal/store/projects.go` | add `ListActiveAcrossOrgsForSweep(ctx, tx, limit int, afterID uuid.UUID) ([]models.Project, error)` — keyset-paginated, `WHERE status='active' ORDER BY id`, **no org filter** (the sanctioned system-actor exception; see §10 R2). |
| `internal/service/audit.go` | add `AuditResourceForesight = "foresight"` constant alongside `AuditResourceCascade`. |
| `internal/worker/jobs.go` | add `ForesightSweepArgs{}` (empty), `ForesightRunner` consumer interface, `ForesightSweepWorker` + `NewForesightSweepWorker` (nil-panic), `Work`. Mirrors `ProcurementCheckWorker`. |
| `internal/worker/registry.go` | add `ForesightRunner` to `Dependencies`; `river.AddWorker(workers, NewForesightSweepWorker(deps.ForesightRunner))`; append a 24h `NewPeriodicJob(... ForesightSweepArgs{}, RunOnStart:false)`. |
| `cmd/worker/main.go` | build the `ForesightWorkspace` adapter (reuses the stores already constructed for cascade), a `foresightOrchestratorFactory` (per-org reasoner, mirrors `cascadeOrchestratorFactory`), wrap it in `ForesightSweepService`, pass as `Dependencies.ForesightRunner`. |

### Explicitly NOT touched (isolation proof)
- `internal/physics`, `internal/currency` — never imported by agentic; never import agentic. Untouched.
- `internal/agentic/cascade.go` + `orchestrator.go` — **unchanged** (separate `ForesightOrchestrator`).
- `internal/api/*` — no HTTP surface in 2b (foresight is worker-only; the dedup feed cards surface
  through the existing `GET /feed` listing). **`ListActiveAcrossOrgsForSweep` must never be wired behind
  an HTTP handler** (§10 R2).

---

## 2. Key Go types / interfaces / signatures

### 2a. `internal/agentic/foresight.go` (leaf — deterministic metric carriers + ports)

```go
package agentic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// Foresight is the cross-module standing-risk capability key. Matches the
// dedup card_type family and the River sweep's per-project flow name.
const Foresight Capability = "foresight"

// ForesightInput names one project to assess. The sweep calls RunForesight
// once per active (org, project) pair; River args carry no per-project data
// (the periodic job is org-wide and fans out inside the runner).
type ForesightInput struct {
	OrgID     uuid.UUID `json:"org_id"`
	ProjectID uuid.UUID `json:"project_id"`
}

// --- Deterministic metric carriers (the WORKSPACE computes every number;
//     agentic computes NONE of it — it only ferries facts to the reasoner). ---

// ProcurementMetric: one item's ordering pressure. Status is the engine-computed
// procurement_items.status (OK|WARNING|CRITICAL|ORDERED) — Foresight READS it,
// never re-derives it. DaysUntilMustOrder is whole days, context for the AI.
type ProcurementMetric struct {
	WBS                string `json:"wbs"`
	Description        string `json:"description"`
	Status             string `json:"status"`
	DaysUntilMustOrder int    `json:"days_until_must_order"` // (must_order_date - today) in whole days; <=0 = overdue
	Breached           bool   `json:"-"`                      // deterministic gate flag (Status WARNING/CRITICAL)
}

// ScheduleMetric: one low-float / critical task. RemainingFloatDays is read
// straight from the CPM-written total_float (integer days).
type ScheduleMetric struct {
	WBS                string `json:"wbs"`
	Name               string `json:"name"`
	RemainingFloatDays int    `json:"remaining_float_days"` // max(0, total_float); 0 = critical
	IsCritical         bool   `json:"is_critical"`
	PercentComplete    int    `json:"percent_complete"`
	Breached           bool   `json:"-"`
}

// BudgetMetric: one cost-coded budget line. Composite Currency — integer cents
// + currency_code, NO float64. BurnPercent is integer-percent (engine-computed).
type BudgetMetric struct {
	WBS            string `json:"wbs"`
	EstimatedCents int64  `json:"estimated_cents"`
	CommittedCents int64  `json:"committed_cents"`
	ActualCents    int64  `json:"actual_cents"`
	CurrencyCode   string `json:"currency_code"`
	BurnPercent    int    `json:"burn_percent"` // actual*100/estimated (int); -1 when estimated==0
	Breached       bool   `json:"-"`
}

// ForesightContext is the per-project deterministic snapshot — the sole input
// to the reasoner. The workspace populates it with ONLY threshold-crossing,
// not-already-carded metrics (the dedup-before-AI cost gate, §8). agentic
// computes none of it.
type ForesightContext struct {
	ProjectName string              `json:"project_name"`
	Procurement []ProcurementMetric `json:"procurement,omitempty"`
	Schedule    []ScheduleMetric    `json:"schedule,omitempty"`
	Budget      []BudgetMetric      `json:"budget,omitempty"`
}

// HasMaterialSignal reports whether ANY metric crossed its deterministic
// threshold (and is not already carded). The orchestrator no-ops (NO AI call)
// when false — the core cost gate. Because the workspace pre-filters deduped
// subjects, an all-standing-risks project returns false here and costs 0 tokens.
func (c ForesightContext) HasMaterialSignal() bool {
	for _, m := range c.Procurement {
		if m.Breached {
			return true
		}
	}
	for _, m := range c.Schedule {
		if m.Breached {
			return true
		}
	}
	for _, m := range c.Budget {
		if m.Breached {
			return true
		}
	}
	return false
}

// --- AI judgment output ---

// ForesightRisk is one judged, materiality-ranked risk. RiskType anchors the
// dedup card_type; SubjectCode (the WBS, or "total") anchors the dedup subject.
type ForesightRisk struct {
	RiskType          string `json:"risk_type"`    // "procurement_criticality" | "schedule_slip" | "budget_burn"
	SubjectCode       string `json:"subject_code"` // WBS; never empty (the apply layer enforces a non-empty subject)
	Severity          string `json:"severity"`     // critical | high | normal | low
	Title             string `json:"title"`
	Body              string `json:"body"`
	RecommendedAction string `json:"recommended_action"`
}

type ForesightPlan struct {
	Risks []ForesightRisk `json:"risks"`
}

// ForesightResult summarizes one project's run. CardsSkipped counts dedup hits
// (both the pre-AI pre-filter and any apply-time 23505 skip).
type ForesightResult struct {
	Risks        int `json:"risks"`
	CardsCreated int `json:"cards_created"`
	CardsSkipped int `json:"cards_skipped"`
}

// --- Ports (adapters live in internal/service) ---

type ForesightReasoner interface {
	JudgeRisks(ctx context.Context, c ForesightContext) (ForesightPlan, error)
}

type ForesightWorkspace interface {
	// LoadForesightContext runs the deterministic per-project metric computation
	// in a read-only tx and returns ONLY threshold-crossing, not-already-carded
	// signals (dedup-before-AI cost gate).
	LoadForesightContext(ctx context.Context, in ForesightInput) (ForesightContext, error)
	// ApplyForesight renders the plan into DEDUPED feed cards + audit in one
	// write tx. A 23505 unique-violation on a card is a clean skip, not an error.
	ApplyForesight(ctx context.Context, in ForesightInput, plan ForesightPlan) (ForesightResult, error)
}
```

> **Note — `ErrReasonerUnavailable` reuse:** the existing `agentic.ErrReasonerUnavailable`
> (`orchestrator.go:15`) is the shared soft-fail sentinel; the foresight reasoner wraps `ai.ErrUnconfigured`
> with it exactly as the cascade reasoner does. No new sentinel needed in agentic.

### 2b. `ForesightOrchestrator` (in `internal/agentic/foresight.go`)

```go
// ForesightOrchestrator runs the foresight flow over its own port pair. It is a
// SEPARATE type from Orchestrator (which serves delay_cascade) so the Phase-1
// cascade wiring is untouched. It holds no engine, store, or AI client — only
// the two ports, the shared capability registry, and a logger.
type ForesightOrchestrator struct {
	reasoner  ForesightReasoner
	workspace ForesightWorkspace
	registry  *Registry
	logger    *slog.Logger
}

func NewForesightOrchestrator(r ForesightReasoner, w ForesightWorkspace, logger *slog.Logger) *ForesightOrchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &ForesightOrchestrator{reasoner: r, workspace: w, registry: NewRegistry(), logger: logger}
}

// RunForesight mirrors RunDelayCascade's 5 steps, with a materiality GATE at
// step 3 (cost control) and soft-fail at step 4:
//   1. registry.Lookup(Foresight) gate (Phase-3 disable seam).
//   2. workspace.LoadForesightContext — hard error => return err (River retry).
//   3. GATE: if !ctx.HasMaterialSignal() => log, return zero, nil. NO AI CALL.
//   4. reasoner.JudgeRisks — errors.Is(ErrReasonerUnavailable) => WARN, zero, nil
//      (soft-fail); other err => return err (River retry).
//   5. if len(plan.Risks)==0 => log, return zero, nil.
//   6. workspace.ApplyForesight — hard error => return err (River retry).
func (o *ForesightOrchestrator) RunForesight(ctx context.Context, in ForesightInput) (ForesightResult, error)
```

### 2c. `internal/service/foresight.go` (adapters — mirror `agentic.go`)

```go
// foresightReasonerAI is the one-method consumer seam over *ai.Client, mirroring
// cascadeReasonerAI. Tests inject a fake without an HTTP server.
type foresightReasonerAI interface {
	ForesightRiskJudgment(ctx context.Context, req ai.ForesightRiskRequest) (*ai.ForesightRiskResponse, error)
}

type ForesightReasoner struct {
	ai    foresightReasonerAI
	orgID uuid.UUID
}

// NewForesightReasoner takes the concrete *ai.Client (not the interface) to dodge
// the typed-nil hazard — copies NewCascadeReasoner verbatim (agentic.go:69-78).
func NewForesightReasoner(client *ai.Client, orgID uuid.UUID) *ForesightReasoner

// JudgeRisks: maps agentic.ForesightContext -> ai.ForesightRiskRequest, stamps
// ai.ContextWithOrgID(ctx, orgID), dispatches, maps response back. r.ai == nil OR
// ai.ErrUnconfigured -> wrap agentic.ErrReasonerUnavailable. Other AI err -> wrap.
func (r *ForesightReasoner) JudgeRisks(ctx context.Context, c agentic.ForesightContext) (agentic.ForesightPlan, error)

type ForesightWorkspace struct {
	pool          *pgxpool.Pool
	scheduleStore *store.ScheduleStore
	procStore     *store.ProcurementStore
	finStore      *store.FinancialsStore
	projectStore  *store.ProjectStore
	feedStore     *store.FeedCardsStore
	audit         AuditRecorder
	cfg           ForesightThresholds // tuning dials; defaults below
}

// ForesightThresholds are the deterministic gate dials. Defaults are documented
// at wiring time; Phase 3's config registry makes them per-deployment tunable.
type ForesightThresholds struct {
	ScheduleFloatDays int // remaining float <= N => breached (default 2); is_critical always breaches
	BudgetBurnPercent int // burn >= N => breached (default 80)
	// Procurement breach reads the persisted status (WARNING/CRITICAL); no window const here.
}

func NewForesightWorkspace(pool *pgxpool.Pool, sched *store.ScheduleStore, proc *store.ProcurementStore,
	fin *store.FinancialsStore, proj *store.ProjectStore, feed *store.FeedCardsStore,
	audit AuditRecorder, cfg ForesightThresholds) *ForesightWorkspace

func (w *ForesightWorkspace) LoadForesightContext(ctx context.Context, in agentic.ForesightInput) (agentic.ForesightContext, error)
func (w *ForesightWorkspace) ApplyForesight(ctx context.Context, in agentic.ForesightInput, plan agentic.ForesightPlan) (agentic.ForesightResult, error)
```

### 2d. `internal/service/foresight_sweep.go`

```go
// ForesightSweepService satisfies worker.ForesightRunner. It owns the cross-org
// fan-out; per project it builds a per-org orchestrator (the AI key resolves
// per-org) and runs RunForesight. One bad project logs + continues (never aborts
// the fleet).
type ForesightSweepService struct {
	pool        *pgxpool.Pool
	projectStore *store.ProjectStore
	procChecker  ProcurementRecomputer // = *ProcurementService; RecomputeStatuses first (R3)
	newOrch      func(orgID uuid.UUID) *agentic.ForesightOrchestrator // per-org factory
	logger       *slog.Logger
}

// ProcurementRecomputer is the narrow seam for the pre-sweep status refresh.
type ProcurementRecomputer interface {
	RecomputeStatuses(ctx context.Context) (int64, error)
}

func (s *ForesightSweepService) RunForesightSweep(ctx context.Context) error
// 1. s.procChecker.RecomputeStatuses(ctx) — establishes happens-before so the
//    procurement dimension reads FRESH statuses (R3). Idempotent fleet sweep.
// 2. keyset-paginate projectStore.ListActiveAcrossOrgsForSweep(ctx, tx, limit, afterID).
// 3. per project: orch := s.newOrch(p.OrgID); orch.RunForesight(ctx, {p.OrgID, p.ID}).
//    Aggregate counts; per-project error -> log + continue. Return nil unless the
//    listing/recompute itself errored (those River-retry).
```

### 2e. `internal/ai/tasks.go` — the `foresight_risk` task (mirror `delay_cascade`)

```go
type ForesightProcurementRisk struct {
	WBS string `json:"wbs"`; Description string `json:"description"`; Status string `json:"status"`
	DaysUntilMustOrder int `json:"days_until_must_order"` // integer days, engine-computed
}
type ForesightScheduleRisk struct {
	WBS string `json:"wbs"`; Name string `json:"name"`
	RemainingFloatDays int `json:"remaining_float_days"`; IsCritical bool `json:"is_critical"`
	PercentComplete int `json:"percent_complete"`
}
type ForesightBudgetRisk struct {
	WBS string `json:"wbs"`
	EstimatedCents int64 `json:"estimated_cents"`; CommittedCents int64 `json:"committed_cents"`
	ActualCents int64 `json:"actual_cents"`; CurrencyCode string `json:"currency_code"`
	BurnPercent int `json:"burn_percent"` // engine-computed integer percent
}
type ForesightRiskRequest struct {
	ProjectName string                     `json:"project_name"`
	Procurement []ForesightProcurementRisk `json:"procurement,omitempty"`
	Schedule    []ForesightScheduleRisk    `json:"schedule,omitempty"`
	Budget      []ForesightBudgetRisk      `json:"budget,omitempty"`
}
type ForesightRiskItem struct {
	RiskType string `json:"risk_type"`; WBS string `json:"wbs"`; Severity string `json:"severity"`
	Title string `json:"title"`; Body string `json:"body"`; RecommendedAction string `json:"recommended_action"`
}
type ForesightRiskResponse struct {
	Risks []ForesightRiskItem `json:"risks"`
}

// ForesightRiskJudgment judges materiality of computed risks. Tool call, uses
// c.model (Opus) via c.callTool. Inherits ErrUnconfigured (no per-org key).
func (c *Client) ForesightRiskJudgment(ctx context.Context, req ForesightRiskRequest) (*ForesightRiskResponse, error)
```

`foresightRiskSystem` (the prompt, money/schedule-locked):

> *"You judge the materiality of standing cross-module risks on a residential construction project. You
> receive three categories the deterministic engine has ALREADY computed: procurement criticality (items
> whose ordering window the schedule puts at risk), schedule-slip risk (critical-path or low-float tasks),
> and budget burn (lines trending over estimate). You NEVER recompute a schedule date or a monetary total
> — every date, float day, and dollar figure (integer cents) is given. For each material risk return
> risk_type (exactly one of procurement_criticality / schedule_slip / budget_burn), wbs, severity
> (critical|high|normal|low), a short title, a concise body, and a recommended human next step. OMIT
> immaterial or well-managed risks — do not surface a card for every breach."*

`foresightRiskSchema` enum-locks `risk_type` and `severity`; required fields:
`risk_type, wbs, severity, title, body, recommended_action`. `ForesightRiskJudgment` returns an error if
all three input arrays are empty (the orchestrator's gate guarantees they aren't, but defend it).

### 2f. Store additions (`internal/store/feed_cards.go`)

```go
// ErrDuplicateActiveRiskCard signals a 23505 on the foresight dedup index — the
// service treats it as a clean skip (a concurrent sweep already carded this risk).
var ErrDuplicateActiveRiskCard = errors.New("feed_cards: active risk card already exists")

type HasActiveRiskCardParams struct {
	ProjectID   uuid.UUID
	CardType    string
	SubjectCode string // non-empty; "total" for project-level budget
}
func (s *FeedCardsStore) HasActiveRiskCard(ctx context.Context, tx pgx.Tx, p HasActiveRiskCardParams) (bool, error)
// SELECT EXISTS(... WHERE project_id=$1 AND card_type=$2 AND subject_code=$3 AND status='active')

// CreateFeedCardParams gains:
//   SubjectCode string // "" for non-risk cards (manual/cascade/invoice); concrete WBS/"total" for risk cards
// CreateFeedCard's INSERT adds subject_code ($10). Map 23505 on the dedup index
// -> ErrDuplicateActiveRiskCard (via the existing isUniqueViolation helper).
```

### 2g. Store additions (`internal/store/projects.go`)

```go
// ListActiveAcrossOrgsForSweep enumerates active projects across ALL orgs for
// the system-actor foresight sweep. This is the ONE store read that deliberately
// omits the org_id filter — sanctioned under ADR-002 (deployment = tenant), like
// the corporate rollup. It MUST NOT be reachable from any HTTP handler. Keyset
// pagination (afterID) bounds memory.
func (s *ProjectStore) ListActiveAcrossOrgsForSweep(ctx context.Context, tx pgx.Tx, limit int, afterID uuid.UUID) ([]models.Project, error)
// SELECT <projectColumns> FROM projects WHERE status='active' AND id > $1 ORDER BY id LIMIT $2
```

---

## 3. End-to-end flow (one project per RunForesight)

```
periodic foresight_sweep (24h) → ForesightSweepService.RunForesightSweep
  → RecomputeStatuses (refresh procurement status, R3)
  → for each active (org, project):
      ForesightOrchestrator.RunForesight(in)
        1. registry gate
        2. ForesightWorkspace.LoadForesightContext  (read-only tx; deterministic metrics; dedup pre-filter)
        3. HasMaterialSignal? no → no-op, NO AI
        4. ForesightReasoner.JudgeRisks  (1 batched Opus call; soft-fail on no key)
        5. empty plan → no-op
        6. ForesightWorkspace.ApplyForesight  (one write tx: deduped cards + audit)
```

`LoadForesightContext` (one read-only tx) does, in sequence, for the project:
1. `VerifyProjectInOrg(tx, projectID, orgID)` (defense-in-depth; ErrNotFound → upstream skip).
2. `projectStore.GetByID` → `ProjectName`.
3. For each of the three dimensions: load rows, compute the deterministic metric + `Breached`, AND
   **drop any subject that already has an active risk card** (`feedStore.HasActiveRiskCard`) — the
   dedup-before-AI cost gate (§8). Only breached, not-already-carded metrics land in the context.

---

## 4. The three dimensions (deterministic metric → AI judgment → deduped card + audit)

| Dim | Deterministic metric (LoadForesightContext, read-only tx) | Store call | Breach signal (deterministic) | card_type / dedup subject | target_role |
|---|---|---|---|---|---|
| **PROCUREMENT** | `DaysUntilMustOrder = floor((MustOrderDate − today)/24h)` (nil date → skip); **`Status` READ from DB** (set by `procurement_check`) | `ListProcurementItems(projectID, orgID)` | `Status ∈ {WARNING, CRITICAL}` (NOT re-derived; reuses `procurement_check`'s output — R3) | `procurement_criticality` / WBS | `superintendent` |
| **SCHEDULE** | `RemainingFloatDays = max(0, derefIntZero(TotalFloat))` (CPM-written) | `GetProjectTasks(projectID)` | `(IsCritical && Status != completed && PercentComplete < 100) \|\| RemainingFloatDays <= cfg.ScheduleFloatDays` | `schedule_slip` / WBS | `superintendent` |
| **BUDGET** | `BurnPercent = ActualCents*100/EstimatedCents` (**integer cents in, integer percent out — no float64**); `-1` when `EstimatedCents==0` | `ListProjectBudgets(projectID)` (per-WBS; three currency codes match per row by CHECK) | `BurnPercent >= cfg.BudgetBurnPercent` (a `-1` line never breaches) | `budget_burn` / WBS | `owner` |

**ApplyForesight (one write tx)** — per `ForesightRisk` in the plan:
1. `subject := risk.SubjectCode`; if empty, default to `"total"` (defensive — the schedule/procurement
   dims always carry a WBS; budget project-level fallback uses `"total"`).
2. `exists := feedStore.HasActiveRiskCard(tx, {projectID, risk.RiskType, subject})` — if true →
   `result.CardsSkipped++`, `continue`. (App-layer dedup; the partial unique index is the race backstop.)
3. `card := feedStore.CreateFeedCard(tx, {OrgID, ProjectID: &pid, CardType: risk.RiskType, Title, Body,
   Priority: foresightSeverityToPriority(risk.Severity), TargetRole: &role, SubjectCode: subject,
   Actions: marshalAudit([...review action...])})`.
   - On `store.ErrDuplicateActiveRiskCard` (a concurrent sweep won the race) → `result.CardsSkipped++`,
     `continue`. **NOT a hard error** — this is the fix for the 23505-retry-storm flaw.
4. `audit.Record(tx, {OrgID, Action: "agentic.foresight.risk_surfaced", ResourceType:
   AuditResourceForesight, ResourceID: projectID, Metadata: marshalAudit({risk_type, subject_code,
   severity, burn_percent / float_days / days_until_must_order, card_id: card.ID})})`.
5. `result.CardsCreated++`.

Card + audit commit or roll back together — identical guarantee to `ApplyCascade` (agentic.go:299-350).
`foresightSeverityToPriority` / `foresightTargetRole` reuse the cascade vocabulary (a shared helper or a
verbatim copy of `cascadeSeverityToPriority` / `cascadeTargetRole`; the budget→owner, else→superintendent
mapping is identical).

**Foresight vs. delay_cascade overlap (acknowledged):** a critical-path slip can produce a
`delay_cascade` card (event-driven, on recalc) AND a `schedule_slip` foresight card (periodic). These are
deliberately different `card_type`s: delay_cascade is the immediate alarm at the moment of slip;
foresight is the daily standing-risk net for risks that accrue with no single triggering write (float
erodes, burn climbs). Same-WBS double-surfacing across the two capabilities is a known, bounded behavior;
collapsing them is out of scope for 2b (would require a cross-capability dedup key — a Phase-3 item).

---

## 5. Dedup design — exact key, enforcement point, resolve-then-recur

**Mechanism: skip-if-active**, keyed `(project_id, card_type, subject_code)` over `status='active'`,
restricted to the three risk `card_type`s. This is the #1 anti-spam control (the delay_cascade
over-firing lesson).

- **Subject:** `feed_cards.subject_code VARCHAR(50) NOT NULL DEFAULT ''` = the WBS that anchors the risk
  (or `"total"` for a project-level budget card). `NOT NULL DEFAULT ''` so existing/non-risk cards get
  `''` and **the partial index's `card_type IN (...)` predicate** — not nullability — excludes them.
  This closes the NULL-distinctness hole (§0 fatal #2).
- **Enforced in TWO layers (belt + suspenders):**
  1. **Application (primary):** `HasActiveRiskCard` before each `CreateFeedCard` in the write tx (§4 step
     2). Auditable, testable, yields explicit skip counts.
  2. **DB partial unique index (race backstop):** see §9. Two concurrent sweeps that both pass the app
     check → the second `INSERT` raises `23505` → `CreateFeedCard` returns `ErrDuplicateActiveRiskCard` →
     `ApplyForesight` **treats it as a skip** (§4 step 3), not a hard error. No retry storm.
- **Dedup-before-AI (cost):** `LoadForesightContext` also drops already-carded subjects *before* the AI
  call (§8), so a standing risk with a live card costs **0 tokens** on subsequent sweeps.
- **Resolve-then-recur behavior (the critique's #1 weakness — explicitly handled):**
  - **Metric resolves (no longer breached):** the breach drops out of the context; no new card. The
    existing active card is **left as-is** — foresight only creates, it never auto-dismisses. *Documented
    limitation:* a resolved risk leaves a stale active card until a human dismisses it. A "reaper" that
    expires foresight cards whose subject is no longer material is an explicit Phase-3 follow-up, not 2b.
  - **Operator DISMISSES the card (acknowledges):** `DismissFeedCard` sets `status='dismissed'`, freeing
    the partial-index slot. **To avoid re-spamming an acknowledged-but-standing risk on the very next
    sweep, the dedup pre-check and the partial index both key on `status IN ('active','dismissed')`** —
    NOT just `'active'`. A dismissed risk card therefore suppresses re-creation. Re-surfacing a dismissed
    risk requires the operator to take a terminal action (`actioned`/`expired`) OR a future
    severity-escalation path (Phase 3) — it does NOT auto-recur daily. This is the corrected half of the
    spam problem the critiques flagged: **dismissal means "stop telling me," and the design honors it.**
  - **Operator ACTIONS the card:** `ActionFeedCard` sets `status='actioned'` (terminal), freeing the slot
    for both index states above → a genuinely-recurring risk can resurface on a later sweep. Desirable.

> **Index/key state = `('active','dismissed')`.** Rationale: `active` prevents duplicate live cards;
> `dismissed` prevents daily re-spam of an acknowledged standing risk. `actioned`/`expired` are terminal
> "handled" states that intentionally free the slot for true recurrence.

---

## 6. Exact / fuzzy split + Composite-Currency compliance

- **DETERMINISTIC (exact), in `ForesightWorkspace.LoadForesightContext`:** every number.
  `DaysUntilMustOrder` (date subtraction → whole days), `RemainingFloatDays` (read from CPM-written
  `total_float`), `BurnPercent = actual_cents*100/estimated_cents` (**integer cents in, integer percent
  out — zero `float64` in the money path**), the `Breached` flags, and the `HasMaterialSignal` gate.
  Procurement breach **reads** the engine-computed `status` (no re-derivation). Composite Currency:
  per-WBS budget lines only; **never summed across `currency_code`**; division-by-zero guarded (`-1`
  sentinel → never breaches, never rendered as "−1%").
- **FUZZY (AI), in `ForesightReasoner.JudgeRisks` only:** materiality judgment (is a breached metric
  worth a card?), ranking, severity, and the human-readable title/body/recommended_action. The AI
  receives the already-computed integers and is prompt + schema constrained to never recompute schedule
  or money (the response schema has **no** numeric metric fields). Mirrors the cascade split exactly.
- **Grep gate:** the new service/agentic files contain **no `float64`** in the money or metric path.
  Severity→priority and module→role are string maps.

---

## 7. Trigger + RBAC / card targeting

- **Trigger:** new River **periodic** job `foresight_sweep`, `PeriodicInterval(24*time.Hour)`,
  `RunOnStart:false` — same machinery as `procurement_check` / `corporate_rollup`. Registered in
  `registry.go`'s `periodicJobs` slice. Daily is right: the metrics it reads (`must_order_date`,
  `total_float`, burn) update on the daily `procurement_check` and on-recalc CPM; finer cadence buys
  nothing and only risks spend.
- **Scope:** `ForesightSweepArgs{}` is **empty** (org-wide). The single periodic tick fans out **inside**
  `RunForesightSweep` (one cron tick → N orchestrator runs, each ≤1 AI call). Per-project granularity
  lives in the service loop, not in River args — matches `procurement_check`.
- **RBAC / targeting:** cards use the existing `target_role` broadcast model (surfaced by `ListFeedCards`'
  `target_role = CallerRole` clause). Schedule + procurement → `superintendent`; budget → `owner` (money
  decision; mirrors `cascadeTargetRole`). No `target_user_id`. Cross-org isolation on the write side is
  the per-query `org_id` already enforced by `CreateFeedCard` / `ListFeedCards` — every card carries the
  loop's `OrgID`. The sweep's cross-org READ is the single sanctioned exception (§10 R2).

---

## 8. AI-cost bounding

Three compounding bounds, all stated at wiring time:

1. **Materiality gate before any AI call** (`RunForesight` step 3): `HasMaterialSignal()` false → no
   reasoner call. Healthy projects cost nothing.
2. **Dedup-before-AI** (`LoadForesightContext`): subjects with an active/dismissed risk card are dropped
   from the context *before* the AI request is assembled. A standing risk that already has a card
   contributes **0 tokens** on every subsequent sweep — this is what makes the steady-state cost low (the
   gate alone would re-judge standing risks daily; this closes that leak).
3. **One batched call per project per sweep:** all surviving breached signals across all three dimensions
   go in **one** `ForesightRiskRequest` (arrays) → **one** `ForesightRiskJudgment`. A project with 8
   breached procurement lines + 3 low-float tasks = 1 call.

**Expected volume (single-tenant fork, the real deployment model):** a fork runs a handful to low-tens of
active projects. Day 1: ~the fraction crossing the gate make 1 batched Opus call each. Steady state: far
fewer, because standing risks dedup to 0-token skips; only newly-breached or newly-uncarded subjects
trigger a call. Small engine-fact payloads → single-digit dollars/fork/month worst case. The
`ScheduleFloatDays` / `BudgetBurnPercent` dials (and the Phase-3 registry `Lookup(Foresight)` disable
seam) are the tuning knobs; document defaults (`2`, `80`) at wiring. (Honest note: a breach can yield 0
cards when the AI judges everything immaterial — a spend-with-no-output case; acceptable and bounded by
gates 1+2.)

---

## 9. Migration — additive, linter-clean (PLAIN index + lock-ok)

Latest migration is `014_invoice_ingestions` (2a), so this is **`015`**. No money columns, no `_cents`
(linter rules 1-2 inapplicable); rules 3 (paired up/down), 4 (destructive opt-in), 5 (index lock) apply.

```sql
-- migrations/015_foresight_dedup.up.sql
-- Foresight risk-card dedup: anchor an active/dismissed risk card to its subject
-- (WBS / "total") so the daily foresight sweep skips instead of spamming a new
-- card each run. subject_code is NOT NULL DEFAULT '' so the partial-index
-- card_type predicate (not nullability) excludes non-risk cards, and the unique
-- index has no NULL-distinctness hole.
ALTER TABLE feed_cards ADD COLUMN subject_code VARCHAR(50) NOT NULL DEFAULT '';

-- Plain CREATE INDEX (NOT CONCURRENTLY): the migrate runner wraps every migration
-- in a tx (cmd/migrate/main.go:119-136) and Postgres forbids CONCURRENTLY inside
-- a tx. Brief ACCESS EXCLUSIVE on the small per-fork feed_cards table is acceptable.
CREATE UNIQUE INDEX idx_feed_risk_dedup
  ON feed_cards (project_id, card_type, subject_code)
  WHERE status IN ('active', 'dismissed')
    AND card_type IN ('procurement_criticality', 'schedule_slip', 'budget_burn'); -- buildos:lock-ok: partial index on small per-fork feed_cards table; brief lock at deploy acceptable
```

```sql
-- migrations/015_foresight_dedup.down.sql
-- buildos:destructive: rollback of foresight risk-card dedup column + partial unique index
DROP INDEX IF EXISTS idx_feed_risk_dedup;
ALTER TABLE feed_cards DROP COLUMN subject_code;
```

**Linter compliance (5 rules):** (1) no forbidden money types ✓ (2) no `_cents` column ✓ (3) paired
up/down ✓ (4) `.down.sql` carries `-- buildos:destructive:` for the `DROP COLUMN` ✓ (5) the plain
`CREATE INDEX` carries `-- buildos:lock-ok:` on the same line ✓.

> No `foresight_risks` state table is needed: dedup state lives entirely in the `feed_cards.status`
> lifecycle, and the audit trail already records every surface event. (Unlike 2a's `invoice_ingestions`
> outbox, there's no external idempotency key to anchor.)

---

## 10. Ordered implementation task breakdown (bottom-up; each step compiles)

> Build inner→outer so every stage compiles before the next depends on it.

1. **Migration 015** — write `015_foresight_dedup.{up,down}.sql` (§9). Run `make lint-migrations` +
   `make migrate` + `make migrate-down` (local PG) to confirm apply/rollback. *(No Go yet.)*
2. **Store: feed_cards** — add `SubjectCode string` to `CreateFeedCardParams` + INSERT ($10); add
   `HasActiveRiskCardParams` + `HasActiveRiskCard`; add `ErrDuplicateActiveRiskCard` + map `23505` on the
   dedup index in `CreateFeedCard`. Update existing `CreateFeedCard` callers (cascade/2a) to pass
   `SubjectCode: ""`. `go build ./internal/store/...`.
3. **Store: projects** — add `ListActiveAcrossOrgsForSweep` (keyset, no org filter, loud comment).
   `go build ./internal/store/...`.
4. **Audit** — add `AuditResourceForesight = "foresight"`. `go build ./internal/service/...`.
5. **AI task** — add `foresight_risk` types + `foresightRiskSystem` + `foresightRiskSchema` +
   `(*Client).ForesightRiskJudgment` to `internal/ai/tasks.go`. `go build ./internal/ai/...`.
6. **Agentic leaf** — new `internal/agentic/foresight.go`: capability const (register in
   `registry.go`), metric carriers, `ForesightContext`+`HasMaterialSignal`, `ForesightRisk/Plan/Result`,
   the two ports, `ForesightOrchestrator`+`NewForesightOrchestrator`+`RunForesight`. `go build
   ./internal/agentic/...`. **Run `make lint-isolation` now** — agentic stays a leaf.
7. **Service adapters** — new `internal/service/foresight.go`: `foresightReasonerAI`,
   `ForesightReasoner`+`NewForesightReasoner` (typed-nil-safe), `ForesightThresholds`,
   `ForesightWorkspace`+`NewForesightWorkspace` (`LoadForesightContext` with dedup pre-filter;
   `ApplyForesight` with dedup + 23505-skip + audit), severity/role helpers. `go build
   ./internal/service/...`.
8. **Service sweep** — new `internal/service/foresight_sweep.go`: `ForesightSweepService`,
   `ProcurementRecomputer` seam, `RunForesightSweep` (recompute-first → keyset fan-out → per-project
   RunForesight, error-isolated). `go build ./internal/service/...`.
9. **Service unit tests** — `foresight_test.go`: metric math (incl. burn `-1` sentinel, integer-only),
   `HasMaterialSignal` gate, dedup skip, soft-fail (nil AI), fake `foresightReasonerAI`. `go test
   ./internal/service/...`.
10. **Worker** — `internal/worker/jobs.go`: `ForesightSweepArgs`, `ForesightRunner`,
    `ForesightSweepWorker`+`NewForesightSweepWorker`+`Work`. `internal/worker/registry.go`:
    `Dependencies.ForesightRunner`, `AddWorker`, periodic job. `go build ./internal/worker/...`.
11. **Wiring** — `cmd/worker/main.go`: build `ForesightWorkspace` (reuse cascade's stores), a
    `foresightOrchestratorFactory` (per-org reasoner, mirror `cascadeOrchestratorFactory`), wrap in
    `ForesightSweepService` (pass `procurementService` as `ProcurementRecomputer`), pass as
    `Dependencies.ForesightRunner`. `go build ./...`.
12. **Integration test + gates** — `foresight_integration_test.go` (§12.1). Run `make lint-isolation`,
    `make audit`, `make test-integration`.

### Resolved implementation hazards (carry into ultracode)
- **R1 — cross-org enumeration is genuinely new code.** `ProjectStore.ListByOrg` is org-scoped; there is
  no cross-org iterator today. `ListActiveAcrossOrgsForSweep` is the one new store read. Keyset-paginate;
  bound memory.
- **R2 — cross-org read is a sanctioned tenant-isolation exception, not a hole.** Per ADR-002
  (deployment = tenant) the worker is the system actor. Name it `...ForSweep`, comment it loudly, and
  **never** mount it behind an HTTP handler (the isolation lint does NOT defend tenant scoping — this is a
  review tripwire, enforce it in review).
- **R3 — procurement status staleness (ordering).** `foresight_sweep` and `procurement_check` both run at
  24h with no River ordering guarantee. **Resolved by construction:** `RunForesightSweep` calls
  `RecomputeStatuses` first (idempotent fleet sweep; harmless to run twice/day), establishing
  happens-before so the procurement dimension reads fresh statuses. Do NOT rely on interval staggering.

---

## 11. Isolation compliance (`make lint-isolation` stays green)

- **Check 1 (core ∌ agentic):** `internal/physics` + `internal/currency` are untouched and never import
  agentic. The new agentic file adds no edge back to them. ✓
- **Check 2 (agentic is a leaf):** `internal/agentic/foresight.go` imports only `context`, `errors`,
  `fmt`, `log/slog`, and `github.com/google/uuid` — same set as `cascade.go`/`orchestrator.go`. Ports are
  declared here; the DB/AI adapters live in `internal/service` (which already legitimately imports
  `ai`/`store`/`currency`). The script scans `Imports` AND `TestImports`, so any test fake for the ports
  must also stay leaf-clean (declare fakes in `internal/service` tests, not in `internal/agentic`). ✓

---

## 12. Verification criteria (definition of done)

### 12.1 Integration test (`internal/service/foresight_integration_test.go`, ephemeral PG)
Using `testdb.NewPool(t)` (freshly migrated pool, auto-cleanup) and a **fake** `foresightReasonerAI`:

- **`TestForesight_MetricCrossesThreshold_SurfacesOneCard`** — seed org+project with a budget line at
  ≥80% burn (and/or a critical task, and/or a WARNING procurement item). Run `RunForesight` with a fake
  reasoner returning one `ForesightRisk` for that subject. Assert: exactly **one** `feed_cards` row of the
  right `card_type`, `status='active'`, correct `target_role`, `subject_code` = the WBS; exactly **one**
  `audit_log` row `action='agentic.foresight.risk_surfaced'`, `resource_type='foresight'`,
  `resource_id=projectID`.
- **`TestForesight_SecondSweepSameStandingRisk_NoDuplicate`** — run `RunForesight` twice with the same
  breached metric + same fake plan. Assert still exactly **one** card and **one** audit row; second run's
  `result.CardsSkipped >= 1` (dedup hit); and assert the second run made **no AI call** (fake reasoner
  call-count stays 1 — proves dedup-before-AI). Then `DismissFeedCard` the card, run a third sweep, assert
  **still** no new card (dismissed-state suppression, §5).
- **`TestForesight_SoftFailNoKey`** — build `ForesightReasoner` with a **nil** `*ai.Client` (or a fake
  returning `ai.ErrUnconfigured`); seed a breached metric; run `RunForesight`. Assert it returns a zero
  result + **nil** error (soft-fail), and **zero** `feed_cards` / `audit_log` rows were written. (The
  deterministic load + gate still ran.)
- **`TestForesight_NoBreach_NoAICall_NoCard`** — seed a healthy project (no breach). Assert zero cards,
  zero audit, and the fake reasoner was **never** called (the `HasMaterialSignal` gate).
- **`TestForesight_BudgetZeroEstimate_NeverBreaches`** — a line with `EstimatedCents=0` → `BurnPercent=-1`
  → not breached → not in context, never rendered.

### 12.2 Unit tests (`foresight_test.go`, no DB)
Metric math (integer burn, `-1` sentinel, float-day deref, days-until-must-order); `HasMaterialSignal`
truth table; severity→priority + module→role maps; soft-fail translation
(`ai.ErrUnconfigured`→`ErrReasonerUnavailable`); dedup skip path; field mapping context↔ai request.

### 12.3 Hard gates — all must stay green
- `make lint-isolation` — **green** (agentic leaf intact; core untouched).
- `make audit` — **green** (`lint-migrations` + `lint-migrations-test` + `test` + `test-prod` +
  `bench-physics`). Migration 015 passes the 5 rules (§9); physics benches unaffected (no
  `internal/physics` change).
- `make test-integration` — **green** (the five tests above + existing suite).
- Composite Currency: grep the new files — **no `float64`** in the money/metric path; persisted/judged
  numbers are integer cents / integer percent.

### 12.4 Manual smoke (optional, post-merge)
On a fork with an Anthropic key + a project breaching a threshold: trigger the sweep (or wait the cadence)
→ a foresight feed card appears once; a second sweep adds none; dismiss it → a third sweep adds none. On a
fork with no key: sweep is a logged no-op (WARN), zero cards.

---

## 13. Top risks (carry into ultracode/ultrareview)

1. **Resolve-then-recur is half-handled by design, half-deferred.** Dismissal-state dedup (§5) defeats the
   daily re-spam of an *acknowledged* standing risk — the critiques' #1 weakness. But a *resolved* risk
   leaves a stale active card until a human dismisses it (foresight never auto-dismisses). The "expire
   cards whose subject is no longer material" reaper is an explicit Phase-3 follow-up. *State this to the
   owner — it is a conscious scope line, not an oversight.*
2. **`subject_code` stability.** Dedup correctness hinges on the WBS being the canonical, stable cost
   code (never the human label) and never renumbered mid-project. Budget project-level cards use `"total"`
   (stable). Document the invariant; a renumber causes transient double-cards that self-heal on dismiss.
3. **Silent no-op when AI is unconfigured.** A key-less fork gets zero foresight with only a WARN log —
   the safety net is silently off. The registry `Capabilities()` surface is the natural place to expose
   "foresight configured/active" on a health endpoint; promote that to a Phase-3 deliverable so
   "foresight silently disabled" is observable. (Out of scope for 2b's worker-only slice.)

---

## Appendix A — verified ground truth (load-bearing facts checked against the code)

| Claim | Verified |
|---|---|
| Migrate runner tx-wraps every migration (CONCURRENTLY would 25001) | `cmd/migrate/main.go:119-136` ✓ |
| lint Check 5 accepts `-- buildos:lock-ok:` same-line opt-out; zero existing migrations use CONCURRENTLY | `scripts/lint-migrations.sh:268-280` ✓ |
| `feed_cards`: `project_id` nullable, `card_type/status` TEXT, no `subject_code` today | `migrations/003_*.up.sql` + `internal/store/feed_cards.go` ✓ |
| `CreateFeedCardParams` non-pointer `OrgID`, `ProjectID *uuid.UUID`; INSERT has 9 cols | `internal/store/feed_cards.go:24-58` ✓ |
| `DismissFeedCard`→`status='dismissed'`; `ActionFeedCard`→`actioned`; both leave the row queryable | `internal/store/feed_cards.go:228-278` ✓ |
| `RecomputeStatuses` (store) is single-SQL, all projects/orgs; `must_order_date<now`→CRITICAL, `<now+window`→WARNING | `internal/store/procurement.go:311-345` ✓ |
| `DefaultProcurementWarningWindowDays = 7`; service `RecomputeStatuses(ctx)` wraps the store call | `internal/service/procurement.go:312,327` ✓ |
| `ProjectTask.TotalFloat *int` (days), `IsCritical bool`, `Status`, `PercentComplete` persisted | `internal/models/project.go` ✓ |
| `ProjectBudget` per-WBS, three `*CostCents int64` + matching `*CurrencyCode` (CHECK pairs them) | `internal/models/financials.go:16-23` + `ListProjectBudgets` ✓ |
| `ProjectStore.ListByOrg` is org-scoped; NO cross-org iterator exists | `internal/store/projects.go:74` ✓ |
| `projects.status` exists (`active`/`completed`/`archived`); `projectColumns` const for reuse | `migrations/001_*.up.sql` + `internal/store/projects.go:50` ✓ |
| `NewOrchestrator` hard-builds `*Registry` + concrete `Reasoner`/`CascadeWorkspace` (→ separate ForesightOrchestrator) | `internal/agentic/orchestrator.go:20-44` ✓ |
| `cascadeOrchestratorFactory` builds per-org reasoner + fresh orchestrator per invocation | `cmd/worker/main.go:213-236` ✓ |
| `NewCascadeReasoner` typed-nil-safe (stores only non-nil client); `ai.ErrUnconfigured`→`ErrReasonerUnavailable` | `internal/service/agentic.go:69-152` ✓ |
| `ApplyCascade` = one write tx, card + audit per impact, commit-or-rollback together | `internal/service/agentic.go:299-350` ✓ |
| `delay_cascade` enqueued event-driven on `criticalChanged` (foresight is the periodic complement) | `internal/service/schedule.go:186-193` ✓ |
| `ProcurementCheckWorker` periodic-job pattern (empty args, nil-panic ctor, thin Work) | `internal/worker/jobs.go:119-148` + `registry.go:60-66` ✓ |
| `DelayCascadeReason` task shape (typed req/resp, system prompt, enum-locked schema, callTool/Opus) to mirror | `internal/ai/tasks.go:467-585` ✓ |
| `AuditResourceCascade = "cascade"` (→ add `AuditResourceForesight = "foresight"`) | `internal/service/audit.go:33` ✓ |
| isolation lint = Check 1 (core ∌ agentic) + Check 2 (agentic leaf, Imports+TestImports) | `scripts/check-isolation.sh` ✓ |
