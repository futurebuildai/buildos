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

// This file holds the internal/service adapters that satisfy the
// internal/agentic ports. The agentic package is an isolated leaf
// (stdlib + uuid + slog only): it declares the Reasoner and
// CascadeWorkspace port interfaces and never imports the stores, the
// AI client, or pgx. These adapters are the seam — they translate the
// real BuildOS dependencies into the domain-neutral agentic types and
// own the transaction boundary so agentic stays effect-free.
//
// EXACT/FUZZY split: CascadeReasoner is the only fuzzy leg (it asks the
// model for judgment). CascadeWorkspace is purely deterministic — it
// reads engine-computed facts and renders the advisory plan into feed
// cards + audit. Neither leg ever mutates the schedule or the money;
// the CPM physics engine stays authoritative.

// cascadeWeekdayDateLayout is the wire-form date the cascade context
// ships to the model — YYYY-MM-DD, matching the layout the other AI
// tasks use for engine-computed dates.
const cascadeWireDateLayout = "2006-01-02"

// ---- CascadeReasoner (agentic.Reasoner) --------------------------------

// cascadeReasonerAI is the consumer-side slice of the native AI client
// the reasoner needs. Defined here (mirroring DailyBriefer /
// ScheduleAdjuster) so the reasoner doesn't pin the whole ai.Client
// surface and tests can substitute a fake without an HTTP server.
type cascadeReasonerAI interface {
	DelayCascadeReason(ctx context.Context, req ai.DelayCascadeReasonRequest) (*ai.DelayCascadeReasonResponse, error)
}

// CascadeReasoner adapts the native AI delay_cascade task to the
// agentic.Reasoner port. It maps the domain-neutral agentic.CascadeContext
// onto the ai wire request field-for-field, sets the per-org Anthropic key
// resolution context (ai.ContextWithOrgID), and — critically — translates
// ai.ErrUnconfigured into agentic.ErrReasonerUnavailable so a fork with no
// Anthropic key configured soft-fails the cascade rather than erroring the
// River job.
type CascadeReasoner struct {
	ai    cascadeReasonerAI
	orgID uuid.UUID
}

// NewCascadeReasoner wires the reasoner to the AI client. orgID is the
// org whose Anthropic key resolves the call; it is stamped into the
// context the AI client reads (ai.ContextWithOrgID). A nil ai client is
// allowed — PlanCascade then returns agentic.ErrReasonerUnavailable so
// the orchestrator soft-fails (a worker built without AI wiring still
// runs the deterministic legs and no-ops the reasoning).
func NewCascadeReasoner(client cascadeReasonerAI, orgID uuid.UUID) *CascadeReasoner {
	return &CascadeReasoner{ai: client, orgID: orgID}
}

// PlanCascade satisfies agentic.Reasoner. Maps the agentic context to the
// ai request, dispatches the typed delay_cascade task, and maps the
// response back. ai.ErrUnconfigured (no key for the org) is wrapped as
// agentic.ErrReasonerUnavailable; any other AI error is returned wrapped
// for River retry.
func (r *CascadeReasoner) PlanCascade(ctx context.Context, c agentic.CascadeContext) (agentic.CascadePlan, error) {
	if r.ai == nil {
		return agentic.CascadePlan{}, fmt.Errorf("cascade reasoner: ai client not configured: %w", agentic.ErrReasonerUnavailable)
	}

	req := ai.DelayCascadeReasonRequest{
		ProjectName:  c.ProjectName,
		SlippedTasks: make([]ai.DelayCascadeSlippedTask, 0, len(c.SlippedTasks)),
		Procurement:  make([]ai.DelayCascadeProcurement, 0, len(c.Procurement)),
		Budget:       make([]ai.DelayCascadeBudget, 0, len(c.Budget)),
	}
	for _, t := range c.SlippedTasks {
		req.SlippedTasks = append(req.SlippedTasks, ai.DelayCascadeSlippedTask{
			WBS:         t.WBS,
			Name:        t.Name,
			EarlyFinish: t.EarlyFinish,
			LateFinish:  t.LateFinish,
			FloatDays:   t.FloatDays,
			IsCritical:  t.IsCritical,
		})
	}
	for _, p := range c.Procurement {
		req.Procurement = append(req.Procurement, ai.DelayCascadeProcurement{
			Description:  p.Description,
			Status:       p.Status,
			LeadTimeDays: p.LeadTimeDays,
			MustOrderBy:  p.MustOrderBy,
		})
	}
	for _, b := range c.Budget {
		req.Budget = append(req.Budget, ai.DelayCascadeBudget{
			WBS:            b.WBS,
			EstimatedCents: b.EstimatedCents,
			CommittedCents: b.CommittedCents,
			ActualCents:    b.ActualCents,
			CurrencyCode:   b.CurrencyCode,
		})
	}

	// Stamp the org id so the AI client's KeyResolver finds the per-org
	// Anthropic key (same pattern as AgentsService).
	aiCtx := ai.ContextWithOrgID(ctx, r.orgID.String())
	resp, err := r.ai.DelayCascadeReason(aiCtx, req)
	if err != nil {
		if errors.Is(err, ai.ErrUnconfigured) {
			// Soft-fail: no Anthropic key for this org. Translate to the
			// port's sentinel so the orchestrator skips the cascade.
			return agentic.CascadePlan{}, fmt.Errorf("cascade reasoner: %w", agentic.ErrReasonerUnavailable)
		}
		return agentic.CascadePlan{}, fmt.Errorf("cascade reasoner: ai delay_cascade: %w", err)
	}
	if resp == nil {
		return agentic.CascadePlan{}, fmt.Errorf("cascade reasoner: ai delay_cascade: nil response")
	}

	plan := agentic.CascadePlan{Impacts: make([]agentic.CascadeImpact, 0, len(resp.Impacts))}
	for _, im := range resp.Impacts {
		plan.Impacts = append(plan.Impacts, agentic.CascadeImpact{
			Module:            im.Module,
			Severity:          im.Severity,
			Title:             im.Title,
			Body:              im.Body,
			RecommendedAction: im.RecommendedAction,
		})
	}
	return plan, nil
}

// ---- CascadeWorkspace (agentic.CascadeWorkspace) -----------------------

// CascadeWorkspace adapts the real stores to the agentic.CascadeWorkspace
// port. It owns the pgx transaction boundary so the agentic package stays
// free of any DB coupling: LoadCascadeContext runs a read-only tx and
// projects engine-computed rows into the domain-neutral context;
// ApplyCascade runs a single write tx that renders the advisory plan into
// feed cards + audit atomically. Modeled on
// AgentsService.RecommendScheduleAdjustments.
type CascadeWorkspace struct {
	pool          *pgxpool.Pool
	scheduleStore *store.ScheduleStore
	procStore     *store.ProcurementStore
	finStore      *store.FinancialsStore
	projectStore  *store.ProjectStore
	feedStore     *store.FeedCardsStore
	audit         AuditRecorder
}

// NewCascadeWorkspace wires the stores + audit recorder. A nil
// AuditRecorder is replaced with the no-op so the worker / tests can omit
// it.
func NewCascadeWorkspace(
	pool *pgxpool.Pool,
	scheduleStore *store.ScheduleStore,
	procStore *store.ProcurementStore,
	finStore *store.FinancialsStore,
	projectStore *store.ProjectStore,
	feedStore *store.FeedCardsStore,
	audit AuditRecorder,
) *CascadeWorkspace {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &CascadeWorkspace{
		pool:          pool,
		scheduleStore: scheduleStore,
		procStore:     procStore,
		finStore:      finStore,
		projectStore:  projectStore,
		feedStore:     feedStore,
		audit:         audit,
	}
}

// LoadCascadeContext gathers the engine-computed snapshot for the slipped
// project inside one read-only tx. Ownership is checked via
// VerifyProjectInOrg (cross-org reads surface as ErrNotFound upstream).
// Only critical tasks are projected as slipped tasks — a non-critical slip
// absorbs into float and the orchestrator no-ops on a context with no
// critical path, so there's no value loading the rest.
func (w *CascadeWorkspace) LoadCascadeContext(ctx context.Context, orgID, projectID uuid.UUID) (agentic.CascadeContext, error) {
	var out agentic.CascadeContext
	err := pgx.BeginTxFunc(ctx, w.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, orgID); err != nil {
			return err
		}

		project, err := w.projectStore.GetByID(ctx, tx, projectID, orgID)
		if err != nil {
			return fmt.Errorf("load project: %w", err)
		}
		out.ProjectName = project.Name

		tasks, err := w.scheduleStore.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load tasks: %w", err)
		}
		for _, t := range tasks {
			if !t.IsCritical {
				continue
			}
			out.SlippedTasks = append(out.SlippedTasks, agentic.CascadeSlippedTask{
				WBS:         t.WBSCode,
				Name:        t.Name,
				EarlyFinish: formatCascadeDate(t.EarlyFinish),
				LateFinish:  formatCascadeDate(t.LateFinish),
				FloatDays:   derefIntZero(t.TotalFloat),
				IsCritical:  t.IsCritical,
			})
		}

		items, err := w.procStore.ListProcurementItems(ctx, tx, store.ListProcurementItemsParams{
			ProjectID: projectID,
			OrgID:     orgID,
		})
		if err != nil {
			return fmt.Errorf("load procurement: %w", err)
		}
		for _, it := range items {
			out.Procurement = append(out.Procurement, agentic.CascadeProcurement{
				Description:  it.Name,
				Status:       it.Status,
				LeadTimeDays: it.LeadTimeDays,
				MustOrderBy:  formatCascadeDate(it.MustOrderDate),
			})
		}

		budgets, err := w.finStore.ListProjectBudgets(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load budgets: %w", err)
		}
		for _, b := range budgets {
			out.Budget = append(out.Budget, agentic.CascadeBudget{
				WBS:            b.WBSCode,
				EstimatedCents: b.EstimatedCostCents,
				CommittedCents: b.CommittedCostCents,
				ActualCents:    b.ActualCostCents,
				CurrencyCode:   b.EstimatedCostCurrencyCode,
			})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return agentic.CascadeContext{}, ErrNotFound
		}
		return agentic.CascadeContext{}, fmt.Errorf("cascade workspace: load context: %w", err)
	}
	return out, nil
}

// cascadeReviewAction is one feed-card action carrying the reasoner's
// recommended next step. Matches the feed_cards.actions JSONB shape
// ([{label, action_type, payload}]) the existing producers use.
type cascadeReviewAction struct {
	Label      string                     `json:"label"`
	ActionType string                     `json:"action_type"`
	Payload    cascadeReviewActionPayload `json:"payload"`
}

// cascadeReviewActionPayload carries the structured impact detail behind
// the review action so the client can render the recommended action.
type cascadeReviewActionPayload struct {
	Module            string `json:"module"`
	Severity          string `json:"severity"`
	RecommendedAction string `json:"recommended_action"`
}

// ApplyCascade renders the advisory plan into feed cards + audit inside a
// single write tx. Per impact: one feed card (CardType "delay_cascade",
// priority mapped from severity, target role mapped from module) plus one
// audit row (Action "agentic.delay_cascade.impact", ResourceType cascade,
// ResourceID = projectID). Feed card + audit commit or roll back together.
func (w *CascadeWorkspace) ApplyCascade(ctx context.Context, orgID, projectID uuid.UUID, plan agentic.CascadePlan) (agentic.CascadeResult, error) {
	var result agentic.CascadeResult
	result.Impacts = len(plan.Impacts)

	err := pgx.BeginTxFunc(ctx, w.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		pid := projectID
		for _, im := range plan.Impacts {
			targetRole := cascadeTargetRole(im.Module)
			actions := marshalAudit([]cascadeReviewAction{{
				Label:      "Review impact",
				ActionType: "review_cascade_impact",
				Payload: cascadeReviewActionPayload{
					Module:            im.Module,
					Severity:          im.Severity,
					RecommendedAction: im.RecommendedAction,
				},
			}})

			if _, err := w.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
				OrgID:      orgID,
				ProjectID:  &pid,
				CardType:   "delay_cascade",
				Title:      im.Title,
				Body:       im.Body,
				Priority:   cascadeSeverityToPriority(im.Severity),
				TargetRole: &targetRole,
				Actions:    actions,
			}); err != nil {
				return fmt.Errorf("create cascade feed card: %w", err)
			}
			result.CardsCreated++

			w.audit.Record(ctx, tx, AuditEntry{
				OrgID:        orgID,
				Action:       "agentic.delay_cascade.impact",
				ResourceType: AuditResourceCascade,
				ResourceID:   projectID,
				Metadata: marshalAudit(map[string]any{
					"module":             im.Module,
					"severity":           im.Severity,
					"title":              im.Title,
					"recommended_action": im.RecommendedAction,
				}),
			})
		}
		return nil
	})
	if err != nil {
		return agentic.CascadeResult{}, fmt.Errorf("cascade workspace: apply: %w", err)
	}
	return result, nil
}

// cascadeSeverityToPriority maps a reasoner severity (critical | high |
// normal | low) onto a feed-card priority (critical | urgent | normal |
// low). "high" has no feed-priority equivalent, so it maps to "urgent";
// anything unrecognized falls back to normal.
func cascadeSeverityToPriority(severity string) string {
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

// cascadeTargetRole routes a cascade impact to the right RBAC role:
// schedule / procurement / crew impacts go to the superintendent (the
// operational owner of the field plan); budget impacts go to the owner (a
// money decision). Role strings are the RBAC vocabulary; the literals
// avoid importing the api/middleware constants into the service layer.
func cascadeTargetRole(module string) string {
	if module == "budget" {
		return "owner"
	}
	return "superintendent"
}

// formatCascadeDate renders an optional engine date as the wire-form
// YYYY-MM-DD string, or "" when the date is nil (unknown / not yet
// computed).
func formatCascadeDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(cascadeWireDateLayout)
}

// derefIntZero returns *p, or 0 when p is nil.
func derefIntZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
