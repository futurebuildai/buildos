package agentic

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// ErrReasonerUnavailable signals that no AI reasoner is available for this
// request — e.g. the org has no Anthropic key configured. Reasoner
// implementations wrap their provider's "unconfigured" sentinel with this so
// the orchestrator can soft-fail the cascade (log + return a zero result and a
// nil error) instead of treating an advisory gap as a hard failure.
var ErrReasonerUnavailable = errors.New("agentic: reasoner unavailable")

// Orchestrator runs cross-module agentic flows over the Reasoner (judgment)
// and CascadeWorkspace (data/effects) ports. It holds no engine, no store, and
// no AI client — only the two ports, the capability registry, and a logger.
type Orchestrator struct {
	reasoner  Reasoner
	workspace CascadeWorkspace
	registry  *Registry
	logger    *slog.Logger
}

// NewOrchestrator constructs an Orchestrator from the two ports and a logger.
// A nil logger is replaced with slog.Default() so callers need not guard it.
// The capability registry is seeded in-code (NewRegistry); RunDelayCascade
// consults it before dispatch, so a capability that isn't registered is
// refused. Phase 3 swaps the in-code registry for a DB-backed, per-deployment
// configurable one — at which point this same gate disables a capability
// without any code change.
func NewOrchestrator(reasoner Reasoner, workspace CascadeWorkspace, logger *slog.Logger) *Orchestrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Orchestrator{
		reasoner:  reasoner,
		workspace: workspace,
		registry:  NewRegistry(),
		logger:    logger,
	}
}

// RunDelayCascade executes the delay-cascade flow:
//
//  1. Confirm the delay_cascade capability is registered (enabled).
//  2. Load the engine-computed context for the slipped project.
//  3. If the context carries no critical slipped tasks (a non-critical slip
//     that absorbs into float), log and return a zero result, nil — nothing to
//     surface.
//  4. Ask the Reasoner to plan the cross-module impacts. If the reasoner is
//     unavailable (ErrReasonerUnavailable — e.g. no AI key), soft-fail: log
//     and return a zero result, nil. AI is advisory; its absence must not
//     fail the job.
//  5. Apply the plan (feed cards + audit) via the Workspace in one tx and log
//     a summary.
//
// Hard failures from Load and Apply (real I/O / tx errors) are returned as
// errors so the River worker can retry; only the advisory reasoner gap is
// swallowed.
func (o *Orchestrator) RunDelayCascade(ctx context.Context, in DelayCascadeInput) (CascadeResult, error) {
	log := o.logger.With(
		slog.String("flow", "delay_cascade"),
		slog.String("org_id", in.OrgID.String()),
		slog.String("project_id", in.ProjectID.String()),
	)

	if _, ok := o.registry.Lookup(DelayCascade); !ok {
		// The capability isn't registered/enabled. In Phase 1 the in-code
		// registry always seeds delay_cascade so this never trips; the gate
		// is the seam Phase 3's configurable registry uses to disable a
		// capability per deployment without a code change.
		return CascadeResult{}, fmt.Errorf("agentic: capability %q not registered", DelayCascade)
	}

	cc, err := o.workspace.LoadCascadeContext(ctx, in.OrgID, in.ProjectID)
	if err != nil {
		return CascadeResult{}, fmt.Errorf("agentic: load cascade context: %w", err)
	}

	if !cc.HasCriticalPath() {
		log.InfoContext(ctx, "delay cascade skipped: no critical-path slip",
			slog.Int("slipped_tasks", len(cc.SlippedTasks)))
		return CascadeResult{}, nil
	}

	plan, err := o.reasoner.PlanCascade(ctx, cc)
	if err != nil {
		if errors.Is(err, ErrReasonerUnavailable) {
			log.WarnContext(ctx, "delay cascade soft-failed: reasoner unavailable",
				slog.Any("error", err))
			return CascadeResult{}, nil
		}
		return CascadeResult{}, fmt.Errorf("agentic: plan cascade: %w", err)
	}

	if len(plan.Impacts) == 0 {
		log.InfoContext(ctx, "delay cascade produced no impacts",
			slog.Int("slipped_tasks", len(cc.SlippedTasks)))
		return CascadeResult{}, nil
	}

	res, err := o.workspace.ApplyCascade(ctx, in.OrgID, in.ProjectID, plan)
	if err != nil {
		return CascadeResult{}, fmt.Errorf("agentic: apply cascade: %w", err)
	}

	log.InfoContext(ctx, "delay cascade applied",
		slog.Int("impacts", res.Impacts),
		slog.Int("cards_created", res.CardsCreated))
	return res, nil
}
