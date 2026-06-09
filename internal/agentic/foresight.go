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

// ForesightPlan is the reasoner's advisory output: the set of material,
// materiality-judged risks to surface. It is judgment, not truth — the
// Workspace renders it into deduped feed cards + an audit trail; it never
// mutates the schedule or the money.
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

// ForesightReasoner is the AI-judgment port: it takes the deterministic,
// threshold-crossing snapshot and returns the materiality-judged risks to
// surface. Its sole implementation lives in internal/service over *ai.Client;
// a fake satisfies it in tests. A no-key org surfaces as ErrReasonerUnavailable.
type ForesightReasoner interface {
	JudgeRisks(ctx context.Context, c ForesightContext) (ForesightPlan, error)
}

// ForesightWorkspace is the data/effects port: deterministic per-project metric
// computation (read-only tx) and the deduped feed-card + audit apply (one write
// tx). Both implementations live in internal/service.
type ForesightWorkspace interface {
	// LoadForesightContext runs the deterministic per-project metric computation
	// in a read-only tx and returns ONLY threshold-crossing, not-already-carded
	// signals (dedup-before-AI cost gate).
	LoadForesightContext(ctx context.Context, in ForesightInput) (ForesightContext, error)
	// ApplyForesight renders the plan into DEDUPED feed cards + audit in one
	// write tx. A 23505 unique-violation on a card is a clean skip, not an error.
	ApplyForesight(ctx context.Context, in ForesightInput, plan ForesightPlan) (ForesightResult, error)
}

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

// NewForesightOrchestrator constructs a ForesightOrchestrator from the two
// foresight ports and a logger. A nil logger is replaced with slog.Default()
// so callers need not guard it. The capability registry is seeded in-code
// (NewRegistry, which now also registers Foresight); RunForesight consults it
// before dispatch, so a capability that isn't registered is refused — the seam
// Phase 3's configurable registry uses to disable a capability per deployment.
func NewForesightOrchestrator(r ForesightReasoner, w ForesightWorkspace, logger *slog.Logger) *ForesightOrchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &ForesightOrchestrator{
		reasoner:  r,
		workspace: w,
		registry:  NewRegistry(),
		logger:    logger,
	}
}

// RunForesight executes the foresight flow for one project. It mirrors
// RunDelayCascade's control flow, with a materiality GATE at step 3 (cost
// control) and soft-fail at step 4:
//
//  1. Confirm the foresight capability is registered (Phase-3 disable seam).
//  2. Load the deterministic, deduped context — a hard error is returned so
//     the River worker retries.
//  3. GATE: if no metric crossed its threshold (HasMaterialSignal false), log
//     and return a zero result, nil. NO AI CALL — the core cost gate.
//  4. Ask the Reasoner to judge materiality. If the reasoner is unavailable
//     (ErrReasonerUnavailable — e.g. no AI key), soft-fail: log and return a
//     zero result, nil. AI is advisory; its absence must not fail the job.
//  5. If the plan carries no risks (the AI judged everything immaterial), log
//     and return a zero result, nil.
//  6. Apply the plan (deduped feed cards + audit) via the Workspace in one tx
//     and log a summary.
//
// Hard failures from Load and Apply (real I/O / tx errors) are returned so the
// River worker can retry; only the advisory reasoner gap is swallowed.
func (o *ForesightOrchestrator) RunForesight(ctx context.Context, in ForesightInput) (ForesightResult, error) {
	log := o.logger.With(
		slog.String("flow", "foresight"),
		slog.String("org_id", in.OrgID.String()),
		slog.String("project_id", in.ProjectID.String()),
	)

	if _, ok := o.registry.Lookup(Foresight); !ok {
		// The capability isn't registered/enabled. In Phase 2b NewRegistry
		// always seeds foresight so this never trips; the gate is the seam
		// Phase 3's configurable registry uses to disable a capability per
		// deployment without a code change.
		return ForesightResult{}, fmt.Errorf("agentic: capability %q not registered", Foresight)
	}

	fc, err := o.workspace.LoadForesightContext(ctx, in)
	if err != nil {
		return ForesightResult{}, fmt.Errorf("agentic: load foresight context: %w", err)
	}

	if !fc.HasMaterialSignal() {
		log.InfoContext(ctx, "foresight skipped: no material signal",
			slog.Int("procurement", len(fc.Procurement)),
			slog.Int("schedule", len(fc.Schedule)),
			slog.Int("budget", len(fc.Budget)))
		return ForesightResult{}, nil
	}

	plan, err := o.reasoner.JudgeRisks(ctx, fc)
	if err != nil {
		if errors.Is(err, ErrReasonerUnavailable) {
			log.WarnContext(ctx, "foresight soft-failed: reasoner unavailable",
				slog.Any("error", err))
			return ForesightResult{}, nil
		}
		return ForesightResult{}, fmt.Errorf("agentic: judge foresight risks: %w", err)
	}

	if len(plan.Risks) == 0 {
		log.InfoContext(ctx, "foresight produced no risks",
			slog.Int("procurement", len(fc.Procurement)),
			slog.Int("schedule", len(fc.Schedule)),
			slog.Int("budget", len(fc.Budget)))
		return ForesightResult{}, nil
	}

	res, err := o.workspace.ApplyForesight(ctx, in, plan)
	if err != nil {
		return ForesightResult{}, fmt.Errorf("agentic: apply foresight: %w", err)
	}

	log.InfoContext(ctx, "foresight applied",
		slog.Int("risks", res.Risks),
		slog.Int("cards_created", res.CardsCreated),
		slog.Int("cards_skipped", res.CardsSkipped))
	return res, nil
}
