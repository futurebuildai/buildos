package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// This file holds the internal/service adapters that satisfy the foresight
// ports declared in internal/agentic (a leaf: stdlib + uuid + slog only). It
// mirrors agentic.go (the delay_cascade adapters) — the agentic package never
// imports the stores, the AI client, or pgx; these adapters are the seam and
// own the transaction boundary so agentic stays effect-free.
//
// EXACT/FUZZY split (Phase 2b): the WORKSPACE computes EVERY metric
// deterministically (integer cents in, integer percent out — no float64 in the
// money path) and decides Breached purely from thresholds; the REASONER is the
// only fuzzy leg — it judges materiality/severity/phrasing and its response
// carries NO numeric metric fields. Neither leg ever mutates the schedule or
// the money; the CPM physics engine + the persisted ledger stay authoritative.

// ---- ForesightThresholds -----------------------------------------------

// ForesightThresholds are the deterministic gate dials. Defaults are applied at
// construction (NewForesightWorkspace) so a zero-value cfg behaves sanely;
// Phase 3's config registry makes them per-deployment tunable.
type ForesightThresholds struct {
	// ScheduleFloatDays: a task with remaining float <= N is breached
	// (default 2). A critical, not-yet-complete task always breaches
	// regardless of this dial.
	ScheduleFloatDays int
	// BudgetBurnPercent: a budget line with burn >= N is breached (default
	// 80). A line with EstimatedCents==0 has BurnPercent -1 and never breaches.
	BudgetBurnPercent int
	// Procurement breach reads the persisted procurement_items.status
	// (WARNING/CRITICAL, set by procurement_check) — no window const here.
}

// defaultForesightScheduleFloatDays / defaultForesightBudgetBurnPercent are the
// documented Phase-2b defaults (see spec §4 / §8).
const (
	defaultForesightScheduleFloatDays = 2
	defaultForesightBudgetBurnPercent = 80
)

// withForesightDefaults fills any unset (<=0) dial with its documented default.
func (c ForesightThresholds) withDefaults() ForesightThresholds {
	if c.ScheduleFloatDays <= 0 {
		c.ScheduleFloatDays = defaultForesightScheduleFloatDays
	}
	if c.BudgetBurnPercent <= 0 {
		c.BudgetBurnPercent = defaultForesightBudgetBurnPercent
	}
	return c
}

// ---- ForesightReasoner (agentic.ForesightReasoner) ---------------------

// foresightReasonerAI is the consumer-side slice of the native AI client the
// reasoner needs — the one-method seam over *ai.Client (mirrors
// cascadeReasonerAI). Tests inject a fake without an HTTP server.
type foresightReasonerAI interface {
	ForesightRiskJudgment(ctx context.Context, req ai.ForesightRiskRequest) (*ai.ForesightRiskResponse, error)
}

// ForesightReasoner adapts the native AI foresight_risk task to the
// agentic.ForesightReasoner port. It maps the domain-neutral
// agentic.ForesightContext onto the ai wire request, sets the per-org Anthropic
// key resolution context (ai.ContextWithOrgID), and — critically — translates
// ai.ErrUnconfigured into agentic.ErrReasonerUnavailable so a fork with no
// Anthropic key configured soft-fails foresight rather than erroring the River
// job.
type ForesightReasoner struct {
	ai    foresightReasonerAI
	orgID uuid.UUID
}

// NewForesightReasoner wires the reasoner to the native AI client. orgID is the
// org whose Anthropic key resolves the call; it is stamped into the context the
// AI client reads (ai.ContextWithOrgID). It takes the concrete *ai.Client (not
// the foresightReasonerAI interface) precisely to dodge the typed-nil interface
// hazard — copies NewCascadeReasoner verbatim: a nil *ai.Client leaves the
// internal client unset and JudgeRisks returns agentic.ErrReasonerUnavailable so
// the orchestrator soft-fails.
func NewForesightReasoner(client *ai.Client, orgID uuid.UUID) *ForesightReasoner {
	r := &ForesightReasoner{orgID: orgID}
	// Assign only a non-nil client. Storing a nil *ai.Client straight into the
	// foresightReasonerAI interface field would make r.ai a non-nil interface
	// wrapping a nil pointer, defeating the `r.ai == nil` guard in JudgeRisks.
	if client != nil {
		r.ai = client
	}
	return r
}

// JudgeRisks satisfies agentic.ForesightReasoner. Maps the agentic context to
// the ai request, dispatches the typed foresight_risk task, and maps the
// response back. ai.ErrUnconfigured (no key for the org) is wrapped as
// agentic.ErrReasonerUnavailable; any other AI error is returned wrapped for
// River retry.
func (r *ForesightReasoner) JudgeRisks(ctx context.Context, c agentic.ForesightContext) (agentic.ForesightPlan, error) {
	if r.ai == nil {
		return agentic.ForesightPlan{}, fmt.Errorf("foresight reasoner: ai client not configured: %w", agentic.ErrReasonerUnavailable)
	}

	req := ai.ForesightRiskRequest{
		ProjectName: c.ProjectName,
		Procurement: make([]ai.ForesightProcurementRisk, 0, len(c.Procurement)),
		Schedule:    make([]ai.ForesightScheduleRisk, 0, len(c.Schedule)),
		Budget:      make([]ai.ForesightBudgetRisk, 0, len(c.Budget)),
	}
	for _, p := range c.Procurement {
		req.Procurement = append(req.Procurement, ai.ForesightProcurementRisk{
			WBS:                p.WBS,
			Description:        p.Description,
			Status:             p.Status,
			DaysUntilMustOrder: p.DaysUntilMustOrder,
		})
	}
	for _, s := range c.Schedule {
		req.Schedule = append(req.Schedule, ai.ForesightScheduleRisk{
			WBS:                s.WBS,
			Name:               s.Name,
			RemainingFloatDays: s.RemainingFloatDays,
			IsCritical:         s.IsCritical,
			PercentComplete:    s.PercentComplete,
		})
	}
	for _, b := range c.Budget {
		req.Budget = append(req.Budget, ai.ForesightBudgetRisk{
			WBS:            b.WBS,
			EstimatedCents: b.EstimatedCents,
			CommittedCents: b.CommittedCents,
			ActualCents:    b.ActualCents,
			CurrencyCode:   b.CurrencyCode,
			BurnPercent:    b.BurnPercent,
		})
	}

	// Stamp the org id so the AI client's KeyResolver finds the per-org
	// Anthropic key (same pattern as CascadeReasoner / AgentsService).
	aiCtx := ai.ContextWithOrgID(ctx, r.orgID.String())
	resp, err := r.ai.ForesightRiskJudgment(aiCtx, req)
	if err != nil {
		if errors.Is(err, ai.ErrUnconfigured) {
			// Soft-fail: no Anthropic key for this org. Translate to the port's
			// sentinel so the orchestrator skips foresight without erroring.
			return agentic.ForesightPlan{}, fmt.Errorf("foresight reasoner: %w", agentic.ErrReasonerUnavailable)
		}
		return agentic.ForesightPlan{}, fmt.Errorf("foresight reasoner: ai foresight_risk: %w", err)
	}
	if resp == nil {
		return agentic.ForesightPlan{}, fmt.Errorf("foresight reasoner: ai foresight_risk: nil response")
	}

	plan := agentic.ForesightPlan{Risks: make([]agentic.ForesightRisk, 0, len(resp.Risks))}
	for _, it := range resp.Risks {
		plan.Risks = append(plan.Risks, agentic.ForesightRisk{
			RiskType:          it.RiskType,
			SubjectCode:       it.WBS, // the ai-layer WBS maps onto the dedup subject
			Severity:          it.Severity,
			Title:             it.Title,
			Body:              it.Body,
			RecommendedAction: it.RecommendedAction,
		})
	}
	return plan, nil
}

// ---- ForesightWorkspace (agentic.ForesightWorkspace) -------------------

// ForesightWorkspace adapts the real stores to the agentic.ForesightWorkspace
// port. It owns the pgx transaction boundary so the agentic package stays free
// of any DB coupling: LoadForesightContext runs a read-only tx (deterministic
// metrics + dedup pre-filter), ApplyForesight runs a single write tx (deduped
// feed cards + audit atomically). Mirrors CascadeWorkspace.
type ForesightWorkspace struct {
	pool          *pgxpool.Pool
	scheduleStore *store.ScheduleStore
	procStore     *store.ProcurementStore
	finStore      *store.FinancialsStore
	projectStore  *store.ProjectStore
	feedStore     *store.FeedCardsStore
	audit         AuditRecorder
	cfg           ForesightThresholds
}

// NewForesightWorkspace wires the stores + audit recorder + threshold dials. A
// nil AuditRecorder is replaced with the no-op; a zero-value cfg gets the
// documented defaults.
func NewForesightWorkspace(
	pool *pgxpool.Pool,
	sched *store.ScheduleStore,
	proc *store.ProcurementStore,
	fin *store.FinancialsStore,
	proj *store.ProjectStore,
	feed *store.FeedCardsStore,
	audit AuditRecorder,
	cfg ForesightThresholds,
) *ForesightWorkspace {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &ForesightWorkspace{
		pool:          pool,
		scheduleStore: sched,
		procStore:     proc,
		finStore:      fin,
		projectStore:  proj,
		feedStore:     feed,
		audit:         audit,
		cfg:           cfg.withDefaults(),
	}
}

// Card-type constants — the three foresight risk families. These are both the
// feed_cards.card_type and the dedup partial-index predicate values
// (migration 015).
const (
	foresightCardProcurement = "procurement_criticality"
	foresightCardSchedule    = "schedule_slip"
	foresightCardBudget      = "budget_burn"
)

// foresightBudgetTotalSubject is the dedup subject used when a budget risk
// carries no WBS (a project-level rollup card). Stable across sweeps.
const foresightBudgetTotalSubject = "total"

// hoursPerDay is the divisor for converting a date delta to whole days.
const foresightHoursPerDay = 24

// LoadForesightContext gathers the deterministic per-project snapshot inside one
// read-only tx and returns ONLY threshold-crossing, not-already-carded signals
// (the dedup-before-AI cost gate, §8). Steps, in order:
//  1. VerifyProjectInOrg (defense-in-depth; cross-org reads surface as
//     ErrNotFound upstream).
//  2. projectStore.GetByID -> ProjectName.
//  3. For each of the three dimensions: load rows, compute the deterministic
//     metric + Breached, AND drop any subject that already has an active/
//     dismissed risk card (feedStore.HasActiveRiskCard). Only breached,
//     not-already-carded metrics land in the context.
func (w *ForesightWorkspace) LoadForesightContext(ctx context.Context, in agentic.ForesightInput) (agentic.ForesightContext, error) {
	var out agentic.ForesightContext
	now := time.Now().UTC()

	err := pgx.BeginTxFunc(ctx, w.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, in.OrgID); err != nil {
			return err
		}

		project, err := w.projectStore.GetByID(ctx, tx, in.ProjectID, in.OrgID)
		if err != nil {
			return fmt.Errorf("load project: %w", err)
		}
		out.ProjectName = project.Name

		// --- PROCUREMENT: breach reads the engine-computed status; never re-derived.
		items, err := w.procStore.ListProcurementItems(ctx, tx, store.ListProcurementItemsParams{
			ProjectID: in.ProjectID,
			OrgID:     in.OrgID,
		})
		if err != nil {
			return fmt.Errorf("load procurement: %w", err)
		}
		for _, it := range items {
			breached := it.Status == models.ProcurementStatusWarning || it.Status == models.ProcurementStatusCritical
			if !breached {
				continue
			}
			carded, err := w.feedStore.HasActiveRiskCard(ctx, tx, store.HasActiveRiskCardParams{
				ProjectID:   in.ProjectID,
				CardType:    foresightCardProcurement,
				SubjectCode: it.WBSCode,
			})
			if err != nil {
				return fmt.Errorf("dedup pre-check procurement: %w", err)
			}
			if carded {
				continue
			}
			out.Procurement = append(out.Procurement, agentic.ProcurementMetric{
				WBS:                it.WBSCode,
				Description:        it.Name,
				Status:             it.Status,
				DaysUntilMustOrder: foresightDaysUntil(it.MustOrderDate, now),
				Breached:           true,
			})
		}

		// --- SCHEDULE: RemainingFloatDays read from CPM-written total_float.
		tasks, err := w.scheduleStore.GetProjectTasks(ctx, tx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("load tasks: %w", err)
		}
		for _, t := range tasks {
			floatDays := foresightRemainingFloatDays(t.TotalFloat)
			criticalActive := t.IsCritical && t.Status != "completed" && t.PercentComplete < 100
			breached := criticalActive || floatDays <= w.cfg.ScheduleFloatDays
			if !breached {
				continue
			}
			carded, err := w.feedStore.HasActiveRiskCard(ctx, tx, store.HasActiveRiskCardParams{
				ProjectID:   in.ProjectID,
				CardType:    foresightCardSchedule,
				SubjectCode: t.WBSCode,
			})
			if err != nil {
				return fmt.Errorf("dedup pre-check schedule: %w", err)
			}
			if carded {
				continue
			}
			out.Schedule = append(out.Schedule, agentic.ScheduleMetric{
				WBS:                t.WBSCode,
				Name:               t.Name,
				RemainingFloatDays: floatDays,
				IsCritical:         t.IsCritical,
				PercentComplete:    t.PercentComplete,
				Breached:           true,
			})
		}

		// --- BUDGET: integer-cents burn; -1 sentinel never breaches.
		budgets, err := w.finStore.ListProjectBudgets(ctx, tx, in.ProjectID)
		if err != nil {
			return fmt.Errorf("load budgets: %w", err)
		}
		for _, b := range budgets {
			burn := foresightBurnPercent(b.ActualCostCents, b.EstimatedCostCents)
			// A -1 line (zero estimate) never breaches.
			breached := burn >= 0 && burn >= w.cfg.BudgetBurnPercent
			if !breached {
				continue
			}
			carded, err := w.feedStore.HasActiveRiskCard(ctx, tx, store.HasActiveRiskCardParams{
				ProjectID:   in.ProjectID,
				CardType:    foresightCardBudget,
				SubjectCode: b.WBSCode,
			})
			if err != nil {
				return fmt.Errorf("dedup pre-check budget: %w", err)
			}
			if carded {
				continue
			}
			out.Budget = append(out.Budget, agentic.BudgetMetric{
				WBS:            b.WBSCode,
				EstimatedCents: b.EstimatedCostCents,
				CommittedCents: b.CommittedCostCents,
				ActualCents:    b.ActualCostCents,
				CurrencyCode:   b.EstimatedCostCurrencyCode,
				BurnPercent:    burn,
				Breached:       true,
			})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return agentic.ForesightContext{}, ErrNotFound
		}
		return agentic.ForesightContext{}, fmt.Errorf("foresight workspace: load context: %w", err)
	}
	return out, nil
}

// foresightReviewAction is one feed-card action carrying the reasoner's
// recommended next step. Matches the feed_cards.actions JSONB shape
// ([{label, action_type, payload}]) the existing producers use.
type foresightReviewAction struct {
	Label      string                       `json:"label"`
	ActionType string                       `json:"action_type"`
	Payload    foresightReviewActionPayload `json:"payload"`
}

// foresightReviewActionPayload carries the structured risk detail behind the
// review action so the client can render the recommended action.
type foresightReviewActionPayload struct {
	RiskType          string `json:"risk_type"`
	SubjectCode       string `json:"subject_code"`
	Severity          string `json:"severity"`
	RecommendedAction string `json:"recommended_action"`
}

// ApplyForesight renders the plan into DEDUPED feed cards + audit inside a
// single write tx. Per ForesightRisk in the plan (spec §4):
//  1. subject := risk.SubjectCode; if empty, default to "total".
//  2. HasActiveRiskCard skip (app-layer dedup) -> CardsSkipped++, continue.
//  3. CreateFeedCard (card_type = risk.RiskType, priority from severity, role
//     from card type, subject_code = subject). A store.ErrDuplicateActiveRiskCard
//     (a concurrent sweep won the race, 23505) is a CLEAN SKIP -> CardsSkipped++,
//     continue — NOT a hard error (the 23505-retry-storm fix).
//  4. audit.Record (action "agentic.foresight.risk_surfaced", resource foresight,
//     resource_id = projectID).
//
// Card + audit commit or roll back together — same guarantee as ApplyCascade.
func (w *ForesightWorkspace) ApplyForesight(ctx context.Context, in agentic.ForesightInput, plan agentic.ForesightPlan) (agentic.ForesightResult, error) {
	var result agentic.ForesightResult
	result.Risks = len(plan.Risks)

	err := pgx.BeginTxFunc(ctx, w.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		pid := in.ProjectID
		for _, risk := range plan.Risks {
			subject := risk.SubjectCode
			if subject == "" {
				subject = foresightBudgetTotalSubject
			}

			exists, err := w.feedStore.HasActiveRiskCard(ctx, tx, store.HasActiveRiskCardParams{
				ProjectID:   in.ProjectID,
				CardType:    risk.RiskType,
				SubjectCode: subject,
			})
			if err != nil {
				return fmt.Errorf("apply dedup pre-check: %w", err)
			}
			if exists {
				result.CardsSkipped++
				continue
			}

			targetRole := foresightTargetRole(risk.RiskType)
			actions := marshalAudit([]foresightReviewAction{{
				Label:      "Review risk",
				ActionType: "review_foresight_risk",
				Payload: foresightReviewActionPayload{
					RiskType:          risk.RiskType,
					SubjectCode:       subject,
					Severity:          risk.Severity,
					RecommendedAction: risk.RecommendedAction,
				},
			}})

			card, err := w.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
				OrgID:       in.OrgID,
				ProjectID:   &pid,
				CardType:    risk.RiskType,
				Title:       risk.Title,
				Body:        risk.Body,
				Priority:    foresightSeverityToPriority(risk.Severity),
				SubjectCode: subject,
				TargetRole:  &targetRole,
				Actions:     actions,
			})
			if err != nil {
				if errors.Is(err, store.ErrDuplicateActiveRiskCard) {
					// A concurrent sweep won the race (23505 on the dedup index).
					// Clean skip, not a hard error — the 23505-retry-storm fix.
					result.CardsSkipped++
					continue
				}
				return fmt.Errorf("create foresight feed card: %w", err)
			}

			w.audit.Record(ctx, tx, AuditEntry{
				OrgID:        in.OrgID,
				Action:       "agentic.foresight.risk_surfaced",
				ResourceType: AuditResourceForesight,
				ResourceID:   in.ProjectID,
				Metadata: marshalAudit(map[string]any{
					"risk_type":    risk.RiskType,
					"subject_code": subject,
					"severity":     risk.Severity,
					"title":        risk.Title,
					"card_id":      card.ID,
				}),
			})
			result.CardsCreated++
		}
		return nil
	})
	if err != nil {
		return agentic.ForesightResult{}, fmt.Errorf("foresight workspace: apply: %w", err)
	}
	return result, nil
}

// ---- deterministic metric helpers (integer-only; no float64) -----------

// foresightBurnPercent computes integer-percent burn = actual*100/estimated
// using integer cents in and integer percent out — NO float64 in the money
// path. Returns -1 (sentinel) when estimated == 0 so a divide-by-zero line
// never breaches and is never rendered as "-1%".
func foresightBurnPercent(actualCents, estimatedCents int64) int {
	if estimatedCents == 0 {
		return -1
	}
	return int(actualCents * 100 / estimatedCents)
}

// foresightRemainingFloatDays reads the CPM-written total_float (nil -> 0) and
// clamps to >= 0. 0 means no slack (effectively critical).
func foresightRemainingFloatDays(totalFloat *int) int {
	v := derefIntZero(totalFloat)
	if v < 0 {
		return 0
	}
	return v
}

// foresightDaysUntil returns whole days from now until t (floor of the hour
// delta / 24). A nil date returns 0 (no ordering pressure context). <=0 means
// the ordering window has closed (overdue).
func foresightDaysUntil(t *time.Time, now time.Time) int {
	if t == nil {
		return 0
	}
	return int(t.Sub(now).Hours()) / foresightHoursPerDay
}

// foresightSeverityToPriority maps a reasoner severity (critical | high |
// normal | low) onto a feed-card priority (critical | urgent | normal | low) —
// identical vocabulary to cascadeSeverityToPriority.
func foresightSeverityToPriority(severity string) string {
	switch severity {
	case "critical":
		return models.FeedPriorityCritical
	case "high":
		return models.FeedPriorityUrgent
	case "low":
		return models.FeedPriorityLow
	default:
		return models.FeedPriorityNormal
	}
}

// foresightTargetRole routes a foresight risk to the right RBAC role: budget
// risks go to the owner (a money decision); schedule + procurement go to the
// superintendent (the operational owner of the field plan). Same budget->owner,
// else->superintendent mapping as cascadeTargetRole, keyed on card_type.
func foresightTargetRole(cardType string) string {
	if cardType == foresightCardBudget {
		return "owner"
	}
	return "superintendent"
}
