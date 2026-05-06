package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/store"
)

// ErrAgentsMaestroUnavailable is returned by agent flows when the
// service was constructed without a corresponding Maestro client (e.g.
// the worker binary doesn't wire DailyBriefer/ScheduleAdjuster). Lets
// handlers surface a clean 503 rather than panicking on a nil call.
// Mirrors procurement.ErrMaestroUnavailable but is package-private to
// the agents flows so the two error sites stay distinct in audit logs.
var ErrAgentsMaestroUnavailable = errors.New("agents: maestro client not configured")

// ErrAgentsScheduleServiceUnavailable is returned when the service was
// constructed without a ScheduleService — RecommendScheduleAdjustments
// needs it to re-validate CPM after applying duration deltas. The
// worker binary doesn't pass one (no caller path exercises agent flows
// from a job today).
var ErrAgentsScheduleServiceUnavailable = errors.New("agents: schedule service not configured")

// MaestroDailyBriefer is the consumer-side interface AgentsService
// needs from the Brain client. Defined here so tests can substitute a
// fake without spinning up an HTTP server, and so AgentsService
// doesn't transitively pin the entire brain.Client surface.
//
// Wraps the typed daily_briefing Maestro task (ADR-001 D5) shipped in
// PR #14. Replaces the earlier free-form MaestroChatter so callers get
// structured cost metadata back on every invocation.
type MaestroDailyBriefer interface {
	DailyBriefing(ctx context.Context, req brain.DailyBriefingRequest) (*brain.DailyBriefingResponse, error)
}

// MaestroScheduleAdjuster is the consumer-side interface
// AgentsService needs for the update_schedule flow (ADR-001 D5; S4
// Session 9.1). Brain reads the task graph snapshot, applies its
// scheduling heuristics, and returns recommended duration adjustments;
// BuildOS owns the CPM physics engine and re-validates every
// recommendation by re-running ForwardPass + BackwardPass.
//
// Constraint per ADR-003 (Gate 2 ratification 2026-05-06): the agent
// is permitted to consume the inbound A2A `update_schedule` and
// `delivery_confirmation` events only — those are the two payload
// schemas already aligned across BuildOS and Brain. This typed Maestro
// task is the OUTBOUND BuildOS → Brain call; the constraint applies
// to any future round-trip through the inbound A2A receiver, which
// this flow does not exercise (Brain's reply rides the synchronous
// HTTP response).
type MaestroScheduleAdjuster interface {
	UpdateSchedule(ctx context.Context, req brain.UpdateScheduleRequest) (*brain.UpdateScheduleResponse, error)
}

// AgentsService orchestrates Brain Maestro calls for BuildOS-side
// agent endpoints. Today: DailyBriefing, RecommendScheduleAdjustments.
// Future: SubLiaison, ProcurementNarrative, etc. — same shape, same
// orchestration.
type AgentsService struct {
	pool             *pgxpool.Pool
	fieldStore       *store.FieldStore
	feedStore        *store.FeedCardsStore
	scheduleStore    *store.ScheduleStore
	scheduleService  *ScheduleService
	maestro          MaestroDailyBriefer
	scheduleAdjuster MaestroScheduleAdjuster
	audit            AuditRecorder
}

// NewAgentsService wires the dependencies. The maestro client is
// invoked with the caller's Bearer token already in ctx (auth
// middleware stashes it). A nil AuditRecorder is replaced with the
// no-op so tests can omit it.
//
// scheduleStore + scheduleService + scheduleAdjuster may be nil — the
// worker binary doesn't expose the agent endpoints, so it's allowed to
// pass nil for the schedule trio. RecommendScheduleAdjustments returns
// ErrAgentsScheduleServiceUnavailable / ErrAgentsMaestroUnavailable
// on a nil call rather than panicking.
func NewAgentsService(
	pool *pgxpool.Pool,
	fields *store.FieldStore,
	feed *store.FeedCardsStore,
	scheduleStore *store.ScheduleStore,
	scheduleService *ScheduleService,
	maestro MaestroDailyBriefer,
	scheduleAdjuster MaestroScheduleAdjuster,
	audit AuditRecorder,
) *AgentsService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AgentsService{
		pool:             pool,
		fieldStore:       fields,
		feedStore:        feed,
		scheduleStore:    scheduleStore,
		scheduleService:  scheduleService,
		maestro:          maestro,
		scheduleAdjuster: scheduleAdjuster,
		audit:            audit,
	}
}

// DailyBriefing is the response from GenerateDailyBriefing — what the
// handler returns and the mobile/web client renders. SessionID is
// passed back so the client can continue the conversation if the user
// asks follow-up questions.
type DailyBriefing struct {
	Reply      string    `json:"reply"`
	SessionID  uuid.UUID `json:"session_id"`
	TaskCount  int       `json:"task_count"`
	AlertCount int       `json:"alert_count"`
}

// GenerateDailyBriefing assembles the caller's open tasks + recent
// active feed cards and dispatches the typed daily_briefing Maestro
// task. Brain owns the prompt template; BuildOS only assembles the
// structured context. The caller's plan tier is enforced by middleware
// (RequirePlanTier), not here — keeping the service free of HTTP
// concerns.
//
// On success, writes a billing-audit row carrying the metered cost
// (run_id, tokens_used, cost_cents, currency_code) — this is the
// per-call meter that rolls into the org's monthly AI usage line per
// ADR-001 D5. The audit write happens AFTER the Maestro call so failed
// invocations don't pollute the bill; if the audit insert itself
// fails, the briefing still returns to the caller (audit is logged at
// WARN by AuditService.Record but not propagated).
//
// The function does NOT persist the briefing as a feed card today;
// that's a follow-up PR once we want push-style delivery. For now the
// frontend caches the response client-side.
func (s *AgentsService) GenerateDailyBriefing(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject, callerRole string) (DailyBriefing, error) {
	if callerOrgID == uuid.Nil {
		return DailyBriefing{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if callerOIDCSubject == "" {
		return DailyBriefing{}, fmt.Errorf("%w: caller oidc subject is required", ErrInvalidInput)
	}

	type loadedContext struct {
		taskNames   []string
		alertTitles []string
	}

	var lc loadedContext
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		userID, err := s.fieldStore.LookupUserIDBySubject(ctx, tx, callerOIDCSubject, callerOrgID)
		if err != nil {
			return err
		}
		tasks, err := s.fieldStore.ListAssignedTasks(ctx, tx, store.ListAssignedTasksParams{
			UserID: userID,
			OrgID:  callerOrgID,
		})
		if err != nil {
			return err
		}
		lc.taskNames = make([]string, 0, len(tasks))
		for _, t := range tasks {
			lc.taskNames = append(lc.taskNames, t.WBSCode+" "+t.Name)
		}

		// Cards: only run the role/sub-targeted query when role is
		// non-empty. ListFeedCards needs both fields to evaluate the
		// targeting predicate.
		if callerRole != "" {
			res, err := s.feedStore.ListFeedCards(ctx, tx, store.ListFeedCardsParams{
				OrgID:             callerOrgID,
				CallerOIDCSubject: callerOIDCSubject,
				CallerRole:        callerRole,
				StatusFilter:      []string{"active"},
				PriorityFilter:    []string{"critical", "urgent"},
				Limit:             20,
			})
			if err != nil {
				return err
			}
			lc.alertTitles = make([]string, 0, len(res.Cards))
			for _, c := range res.Cards {
				lc.alertTitles = append(lc.alertTitles, "["+c.Priority+"] "+c.Title)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return DailyBriefing{}, ErrNotFound
		}
		return DailyBriefing{}, fmt.Errorf("daily briefing: load context: %w", err)
	}

	// Token plumbing happens via ctx — auth middleware stashed it.
	// Brain's daily_briefing task assembles the prompt from the
	// structured context; BuildOS no longer builds the prompt
	// itself.
	resp, err := s.maestro.DailyBriefing(ctx, brain.DailyBriefingRequest{
		Tasks:    lc.taskNames,
		Alerts:   lc.alertTitles,
		UserRole: callerRole,
	})
	if err != nil {
		return DailyBriefing{}, fmt.Errorf("daily briefing: maestro: %w", err)
	}

	s.recordDailyBriefingAudit(ctx, callerOrgID, callerOIDCSubject, resp, len(lc.taskNames), len(lc.alertTitles))

	return DailyBriefing{
		Reply:      resp.Reply,
		SessionID:  resp.SessionID,
		TaskCount:  len(lc.taskNames),
		AlertCount: len(lc.alertTitles),
	}, nil
}

// recordDailyBriefingAudit writes the per-call billing-audit row in a
// short standalone tx. Resource type is "ai_run" (a Maestro
// invocation, not a domain object); resource id is the Brain-issued
// run_id. Metadata carries the cost_cents / tokens_used block per
// ADR-001 D5. *_cents fields are Confidential-class per the PII
// catalog so the existing audit scrubber preserves them.
//
// A standalone tx is used (rather than wrapping the load-context tx
// or some other surrounding mutation) because daily_briefing is a
// read-only flow with no domain mutation to commit alongside. If the
// tx itself fails to begin, AuditService.Record still falls through
// to its log-and-swallow path; we never fail the user's briefing
// because audit couldn't be written.
func (s *AgentsService) recordDailyBriefingAudit(ctx context.Context, orgID uuid.UUID, userSub string, resp *brain.DailyBriefingResponse, taskCount, alertCount int) {
	metadata, err := json.Marshal(struct {
		RunID        uuid.UUID `json:"run_id"`
		SessionID    uuid.UUID `json:"session_id"`
		TokensUsed   int64     `json:"tokens_used"`
		CostCents    int64     `json:"cost_cents"`
		CurrencyCode string    `json:"currency_code"`
		TaskCount    int       `json:"task_count"`
		AlertCount   int       `json:"alert_count"`
		Task         string    `json:"task"`
	}{
		RunID:        resp.RunID,
		SessionID:    resp.SessionID,
		TokensUsed:   resp.TokensUsed,
		CostCents:    resp.CostCents,
		CurrencyCode: resp.CurrencyCode,
		TaskCount:    taskCount,
		AlertCount:   alertCount,
		Task:         "daily_briefing",
	})
	if err != nil {
		// Marshal failures here are programmer error (the struct
		// is fully composed of marshalable types). Log and skip
		// rather than returning; the briefing already succeeded.
		return
	}

	_ = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       "ai.daily_briefing.invoked",
			ResourceType: AuditResourceAIRun,
			ResourceID:   resp.RunID,
			Metadata:     metadata,
		})
		return nil
	})
}

// ScheduleAdjustmentSet is the response shape from
// RecommendScheduleAdjustments. AppliedDeltas counts only adjustments
// whose NewDurationDays was non-nil and successfully applied via
// UpdateTask. SkippedRationaleOnly counts adjustments whose
// NewDurationDays was nil (Brain returned a "review only" rationale
// without a numeric delta — the row is preserved in the audit metadata
// for transparency but no UPDATE fires). Cost block per ADR-001 D5.
type ScheduleAdjustmentSet struct {
	Adjustments          []brain.ScheduleAdjustment `json:"adjustments"`
	AppliedDeltas        int                        `json:"applied_deltas"`
	SkippedRationaleOnly int                        `json:"skipped_rationale_only"`
	RunID                uuid.UUID                  `json:"run_id"`
	TokensUsed           int64                      `json:"tokens_used"`
	CostCents            int64                      `json:"cost_cents"`
	CurrencyCode         string                     `json:"currency_code"`
}

// RecommendScheduleAdjustments runs the DailyFocusAgent's schedule-tuning
// flow (S4 Session 9.1, ADR-001 D5):
//
//  1. Loads the project's task graph + dependencies (ownership-checked
//     via VerifyProjectInOrg → ErrNotFound on cross-org access).
//  2. Calls Brain Maestro update_schedule with the snapshot.
//  3. Applies recommended duration deltas via ScheduleStore.UpdateTask
//     inside the same tx. Adjustments with nil NewDurationDays are
//     preserved in the audit metadata but not written.
//  4. Writes one batch audit row keyed by project_id with
//     Action="schedule.maestro_edit", ResourceType=AuditResourceSchedule,
//     metadata {run_id, tokens_used, cost_cents, currency_code,
//     recommended_delta_count, applied_deltas, skipped_rationale_only,
//     adjustments[]}. Per-adjustment audit would create N nearly-identical
//     rows per Maestro call with no extra signal.
//  5. Commits the deltas-and-audit tx.
//  6. Synchronously re-runs ScheduleService.RecalculateSchedule (its
//     own tx) so CPM physics re-validates with the new durations and
//     critical path / floats are recomputed. Recalc writes its own
//     "schedule.recalculated" audit row — both rows tell the truth:
//     "Maestro nudged durations" + "CPM re-evaluated".
//
// Failure modes:
//
//   - Maestro call inside tx1 returns an error → tx rolls back; durations
//     untouched; agent caller sees the wrapped error.
//   - tx1 commits but recalc tx2 fails → durations are persisted with
//     audit trail; CPM is stale. Caller receives the recalc error wrapped
//     so the next session knows to manually re-run /schedule/recalculate.
//     This is the same eventual-consistency pattern as DelayCascade
//     enqueues: applied state is preserved; physics catches up.
//
// callerUserSub is recorded on the audit row(s).
func (s *AgentsService) RecommendScheduleAdjustments(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID) (ScheduleAdjustmentSet, error) {
	if callerOrgID == uuid.Nil {
		return ScheduleAdjustmentSet{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if projectID == uuid.Nil {
		return ScheduleAdjustmentSet{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if s.scheduleAdjuster == nil {
		return ScheduleAdjustmentSet{}, ErrAgentsMaestroUnavailable
	}
	if s.scheduleStore == nil || s.scheduleService == nil {
		return ScheduleAdjustmentSet{}, ErrAgentsScheduleServiceUnavailable
	}

	var result ScheduleAdjustmentSet
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}

		tasks, err := s.scheduleStore.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load tasks: %w", err)
		}
		if len(tasks) == 0 {
			return fmt.Errorf("%w: project %s has no tasks", ErrInvalidInput, projectID)
		}
		deps, err := s.scheduleStore.GetProjectDependencies(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load dependencies: %w", err)
		}
		projectStart, err := s.scheduleStore.GetProjectStartDate(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load project start: %w", err)
		}

		taskSnaps := make([]brain.ScheduleTaskSnapshot, 0, len(tasks))
		for _, t := range tasks {
			taskSnaps = append(taskSnaps, brain.ScheduleTaskSnapshot{
				TaskID:          t.ID,
				WBSCode:         t.WBSCode,
				Name:            t.Name,
				DurationDays:    t.DurationDays,
				Status:          t.Status,
				PercentComplete: t.PercentComplete,
				IsCritical:      t.IsCritical,
			})
		}
		depSnaps := make([]brain.ScheduleDepSnapshot, 0, len(deps))
		for _, d := range deps {
			depSnaps = append(depSnaps, brain.ScheduleDepSnapshot{
				PredecessorID:  d.PredecessorID,
				SuccessorID:    d.SuccessorID,
				DependencyType: string(d.DependencyType),
				LagDays:        d.LagDays,
			})
		}

		// Maestro call inside the tx — a slow Brain response holds the
		// tx open briefly. Same trade-off as ProcurementService.RecommendVendors:
		// keeps the recommendation + audit + duration writes atomic so we
		// can't end up with a recommendation tracked in Brain's ai_runs but
		// missing on BuildOS, or a SQL update with no audit trail.
		resp, err := s.scheduleAdjuster.UpdateSchedule(ctx, brain.UpdateScheduleRequest{
			ProjectID:        projectID,
			ProjectStartDate: projectStart.Format("2006-01-02T15:04:05Z07:00"),
			Tasks:            taskSnaps,
			Dependencies:     depSnaps,
		})
		if err != nil {
			return fmt.Errorf("maestro update_schedule: %w", err)
		}
		if resp == nil {
			return fmt.Errorf("maestro update_schedule: nil response")
		}

		applied := 0
		skipped := 0
		for _, adj := range resp.Adjustments {
			if adj.NewDurationDays == nil {
				skipped++
				continue
			}
			if *adj.NewDurationDays < 0 {
				// Defensive — Brain shouldn't return negative durations,
				// but if it does we drop the row rather than violating the
				// CHECK constraint downstream and rolling back the whole
				// batch. Skipped rows still appear in audit metadata.
				skipped++
				continue
			}
			if _, err := s.scheduleStore.UpdateTask(ctx, tx, store.UpdateTaskParams{
				TaskID:       adj.TaskID,
				ProjectID:    projectID,
				DurationDays: adj.NewDurationDays,
			}); err != nil {
				return fmt.Errorf("apply duration delta for task %s: %w", adj.TaskID, err)
			}
			applied++
		}

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "schedule.maestro_edit",
			ResourceType: AuditResourceSchedule,
			ResourceID:   projectID,
			Metadata: marshalAudit(map[string]any{
				"run_id":                    resp.RunID,
				"tokens_used":               resp.TokensUsed,
				"cost_cents":                resp.CostCents,
				"currency_code":             resp.CurrencyCode,
				"recommended_delta_count":   len(resp.Adjustments),
				"applied_deltas":            applied,
				"skipped_rationale_only":    skipped,
				"adjustments":               resp.Adjustments,
			}),
		})

		result = ScheduleAdjustmentSet{
			Adjustments:          resp.Adjustments,
			AppliedDeltas:        applied,
			SkippedRationaleOnly: skipped,
			RunID:                resp.RunID,
			TokensUsed:           resp.TokensUsed,
			CostCents:            resp.CostCents,
			CurrencyCode:         resp.CurrencyCode,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ScheduleAdjustmentSet{}, ErrNotFound
		}
		return ScheduleAdjustmentSet{}, err
	}

	// CPM re-validation rides its own tx (a separate operation than the
	// agent's apply-deltas tx). Failure here means deltas are persisted
	// with audit trail but CPM is stale until the next manual recalc;
	// the wrapped error surfaces this state to the caller. Skip when
	// no deltas were actually applied — recomputing on a no-op edit
	// would double the audit row count for nothing.
	if result.AppliedDeltas > 0 {
		if _, _, recalcErr := s.scheduleService.RecalculateSchedule(ctx, projectID, callerOrgID, callerUserSub); recalcErr != nil {
			return result, fmt.Errorf("apply succeeded; recalc deferred: %w", recalcErr)
		}
	}
	return result, nil
}
