package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// maxTaskDurationDays mirrors the project_tasks.duration_days CHECK added in
// migration 019 (0..36500). The AI duration-adjuster skips any out-of-range
// model output rather than letting it violate the constraint and roll back the
// whole apply tx.
const maxTaskDurationDays = 36500

// ErrAgentsAIUnavailable is returned by agent flows when the service
// was constructed without a corresponding AI client (e.g. the worker
// binary doesn't wire DailyBriefer/ScheduleAdjuster). Lets handlers
// surface a clean 503 rather than panicking on a nil call. Mirrors
// procurement.ErrAIUnavailable but is package-private to the agents
// flows so the two error sites stay distinct in audit logs.
var ErrAgentsAIUnavailable = errors.New("agents: ai client not configured")

// ErrAgentsScheduleServiceUnavailable is returned when the service was
// constructed without a ScheduleService — RecommendScheduleAdjustments
// needs it to re-validate CPM after applying duration deltas. The
// worker binary doesn't pass one (no caller path exercises agent flows
// from a job today).
var ErrAgentsScheduleServiceUnavailable = errors.New("agents: schedule service not configured")

// DailyBriefer is the consumer-side interface AgentsService needs from
// the native AI client. Defined here so tests can substitute a fake
// without spinning up an HTTP server, and so AgentsService doesn't
// transitively pin the entire ai.Client surface.
//
// Wraps the typed daily_briefing task dispatched natively to Anthropic
// (internal/ai). The per-org Anthropic key is resolved from the
// context (ai.ContextWithOrgID), which the caller sets before the call.
type DailyBriefer interface {
	DailyBriefing(ctx context.Context, req ai.DailyBriefingRequest) (*ai.DailyBriefingResponse, error)
}

// ScheduleAdjuster is the consumer-side interface AgentsService needs
// for the update_schedule flow (S4 Session 9.1). The model reads the
// task graph snapshot, applies its scheduling heuristics, and returns
// recommended duration adjustments; BuildOS owns the CPM physics
// engine and re-validates every recommendation by re-running
// ForwardPass + BackwardPass.
type ScheduleAdjuster interface {
	UpdateSchedule(ctx context.Context, req ai.UpdateScheduleRequest) (*ai.UpdateScheduleResponse, error)
}

// AgentsService orchestrates native AI calls for BuildOS-side agent
// endpoints. Today: DailyBriefing, RecommendScheduleAdjustments.
// Future: SubLiaison, ProcurementNarrative, etc. — same shape, same
// orchestration.
type AgentsService struct {
	pool             *pgxpool.Pool
	fieldStore       *store.FieldStore
	feedStore        *store.FeedCardsStore
	scheduleStore    *store.ScheduleStore
	scheduleService  *ScheduleService
	briefer          DailyBriefer
	scheduleAdjuster ScheduleAdjuster
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
// ErrAgentsScheduleServiceUnavailable / ErrAgentsAIUnavailable on a nil
// call rather than panicking.
func NewAgentsService(
	pool *pgxpool.Pool,
	fields *store.FieldStore,
	feed *store.FeedCardsStore,
	scheduleStore *store.ScheduleStore,
	scheduleService *ScheduleService,
	briefer DailyBriefer,
	scheduleAdjuster ScheduleAdjuster,
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
		briefer:          briefer,
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
// task. BuildOS assembles the structured context and owns the prompt.
// Role-based access is enforced at the route (RequireMinRole), not here
// — keeping the service free of HTTP concerns. (The pro-tier plan gate
// was removed in ESC-002.)
//
// On success, writes an audit row recording the invocation. The audit
// write happens AFTER the AI call so failed invocations don't leave a
// trace; if the audit insert itself fails, the briefing still returns
// to the caller (audit is logged at WARN by AuditService.Record but
// not propagated).
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

	if s.briefer == nil {
		return DailyBriefing{}, ErrAgentsAIUnavailable
	}

	// The per-org Anthropic key is resolved from the context; stash
	// the org id so the ai client's KeyResolver can find it.
	aiCtx := ai.ContextWithOrgID(ctx, callerOrgID.String())
	resp, err := s.briefer.DailyBriefing(aiCtx, ai.DailyBriefingRequest{
		Tasks:    lc.taskNames,
		Alerts:   lc.alertTitles,
		UserRole: callerRole,
	})
	if err != nil {
		return DailyBriefing{}, fmt.Errorf("daily briefing: ai: %w", err)
	}

	s.recordDailyBriefingAudit(ctx, callerOrgID, callerOIDCSubject, resp, len(lc.taskNames), len(lc.alertTitles))

	return DailyBriefing{
		Reply:      resp.Reply,
		SessionID:  resp.SessionID,
		TaskCount:  len(lc.taskNames),
		AlertCount: len(lc.alertTitles),
	}, nil
}

// recordDailyBriefingAudit writes the per-call audit row in a short
// standalone tx. Resource type is "ai_run" (an AI invocation, not a
// domain object); resource id is the response session id (the native
// client is single-shot — there is no server-issued run id).
//
// A standalone tx is used (rather than wrapping the load-context tx
// or some other surrounding mutation) because daily_briefing is a
// read-only flow with no domain mutation to commit alongside. If the
// tx itself fails to begin, AuditService.Record still falls through
// to its log-and-swallow path; we never fail the user's briefing
// because audit couldn't be written.
func (s *AgentsService) recordDailyBriefingAudit(ctx context.Context, orgID uuid.UUID, userSub string, resp *ai.DailyBriefingResponse, taskCount, alertCount int) {
	metadata, err := json.Marshal(struct {
		SessionID  uuid.UUID `json:"session_id"`
		TaskCount  int       `json:"task_count"`
		AlertCount int       `json:"alert_count"`
		Task       string    `json:"task"`
	}{
		SessionID:  resp.SessionID,
		TaskCount:  taskCount,
		AlertCount: alertCount,
		Task:       "daily_briefing",
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
			ResourceID:   resp.SessionID,
			Metadata:     metadata,
		})
		return nil
	})
}

// ScheduleProposal is one enriched, per-row adjustment in a
// ScheduleAdjustmentSet. Unlike the raw ai.ScheduleAdjustment (which
// carries only {task_id, new_duration_days, rationale}), the proposal
// is joined against the loaded task so the UI can show what each change
// is and let the user accept/reject it per row:
//
//   - WBSCode / Name      — the row's identity (from the loaded task)
//   - OldDurationDays      — the current duration (from the loaded task)
//   - NewDurationDays      — the model's proposed duration (nil = monitor-only)
//   - Rationale            — the model's free-text justification
//   - IsCritical           — whether the task is currently on the critical path
//   - ProposedChange       — true iff NewDurationDays is set AND differs from old
//     (a real duration change to propose); false = advisory / monitor-only
//   - Applied              — true iff this row was written via UpdateTask on the
//     real (non-dry-run) apply path. Always false on the dry-run preview.
type ScheduleProposal struct {
	TaskID          uuid.UUID `json:"task_id"`
	WBSCode         string    `json:"wbs_code"`
	Name            string    `json:"name"`
	OldDurationDays int       `json:"old_duration_days"`
	NewDurationDays *int      `json:"new_duration_days,omitempty"`
	Rationale       string    `json:"rationale,omitempty"`
	IsCritical      bool      `json:"is_critical"`
	ProposedChange  bool      `json:"proposed_change"`
	Applied         bool      `json:"applied"`
}

// ScheduleAdjustmentSet is the response shape from
// RecommendScheduleAdjustments. Adjustments carries the enriched
// per-row proposals (wbs/name/old/new/rationale/critical). DryRun is
// true when the set was a preview (no writes, no recalc). On the
// dry-run preview, ProposedChanges counts rows the user could apply and
// AppliedDeltas is 0; on the real apply path, AppliedDeltas counts rows
// actually written. AdvisoryCount counts monitor-only rows (no proposed
// duration change).
type ScheduleAdjustmentSet struct {
	Adjustments        []ScheduleProposal `json:"adjustments"`
	DryRun             bool               `json:"dry_run"`
	ProposedChanges    int                `json:"proposed_changes"`
	AdvisoryCount      int                `json:"advisory_count"`
	AppliedDeltas      int                `json:"applied_deltas"`
	CriticalRecomputed bool               `json:"critical_recomputed"`
	// SkippedRationaleOnly is retained for wire-compat with the prior
	// response shape; it equals AdvisoryCount (monitor-only rows).
	SkippedRationaleOnly int `json:"skipped_rationale_only"`
}

// RecommendScheduleAdjustments runs the schedule-tuning AI flow and
// returns enriched per-row proposals. PREVIEW-FIRST (ESC-AUX-01):
//
//   - dryRun=true (the "Suggest adjustments" UI path): loads the task
//     graph, calls the AI update_schedule task, and returns the proposals
//     joined against the loaded tasks (wbs/name/old/new/rationale/critical).
//     It MUTATES NOTHING — no UpdateTask, no recalc, no audit. The human
//     then commits the rows they want via ApplyScheduleAdjustments.
//   - dryRun=false (legacy auto-apply path, retained for callers that
//     still want a one-shot apply): also applies every in-range numeric
//     delta via UpdateTask in the load tx, writes a schedule.maestro_edit
//     audit row, then re-runs CPM in its own tx.
//
// Both paths are ownership-checked via VerifyProjectInOrg (ErrNotFound on
// cross-org access). callerUserSub is recorded on the audit row (real path).
//
// Failure modes (real path only):
//
//   - AI call inside the load tx errors → tx rolls back; durations untouched.
//   - tx1 commits but recalc tx2 fails → durations are persisted with the
//     audit trail; CPM is stale. Caller receives the recalc error wrapped so
//     the next /schedule/recalculate catches up (same eventual-consistency
//     pattern as DelayCascade enqueues).
func (s *AgentsService) RecommendScheduleAdjustments(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID, dryRun bool) (ScheduleAdjustmentSet, error) {
	if callerOrgID == uuid.Nil {
		return ScheduleAdjustmentSet{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if projectID == uuid.Nil {
		return ScheduleAdjustmentSet{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if s.scheduleAdjuster == nil {
		return ScheduleAdjustmentSet{}, ErrAgentsAIUnavailable
	}
	if s.scheduleStore == nil || s.scheduleService == nil {
		return ScheduleAdjustmentSet{}, ErrAgentsScheduleServiceUnavailable
	}

	// A dry-run preview must not hold a writable tx; use read-only so an
	// accidental write would fail loudly rather than silently mutating.
	txOpts := pgx.TxOptions{}
	if dryRun {
		txOpts.AccessMode = pgx.ReadOnly
	}

	var result ScheduleAdjustmentSet
	err := pgx.BeginTxFunc(ctx, s.pool, txOpts, func(tx pgx.Tx) error {
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

		// Index the loaded tasks by id so we can join each AI proposal
		// against its current duration / wbs / name / critical flag.
		byID := make(map[uuid.UUID]int, len(tasks))
		for i := range tasks {
			byID[tasks[i].ID] = i
		}

		taskSnaps := make([]ai.ScheduleTaskSnapshot, 0, len(tasks))
		for _, t := range tasks {
			taskSnaps = append(taskSnaps, ai.ScheduleTaskSnapshot{
				TaskID:          t.ID,
				WBSCode:         t.WBSCode,
				Name:            t.Name,
				DurationDays:    t.DurationDays,
				Status:          t.Status,
				PercentComplete: t.PercentComplete,
				IsCritical:      t.IsCritical,
			})
		}
		depSnaps := make([]ai.ScheduleDepSnapshot, 0, len(deps))
		for _, d := range deps {
			depSnaps = append(depSnaps, ai.ScheduleDepSnapshot{
				PredecessorID:  d.PredecessorID,
				SuccessorID:    d.SuccessorID,
				DependencyType: string(d.DependencyType),
				LagDays:        d.LagDays,
			})
		}

		// AI call inside the tx — a slow model response holds the tx open
		// briefly. The per-org Anthropic key is resolved from the context.
		aiCtx := ai.ContextWithOrgID(ctx, callerOrgID.String())
		resp, err := s.scheduleAdjuster.UpdateSchedule(aiCtx, ai.UpdateScheduleRequest{
			ProjectID:        projectID,
			ProjectStartDate: projectStart.Format("2006-01-02T15:04:05Z07:00"),
			Tasks:            taskSnaps,
			Dependencies:     depSnaps,
		})
		if err != nil {
			return fmt.Errorf("ai update_schedule: %w", err)
		}
		if resp == nil {
			return fmt.Errorf("ai update_schedule: nil response")
		}

		proposals := make([]ScheduleProposal, 0, len(resp.Adjustments))
		applied := 0
		advisory := 0
		proposed := 0
		for _, adj := range resp.Adjustments {
			ti, known := byID[adj.TaskID]
			if !known {
				// The model returned a task id that isn't in this project's
				// graph — drop it rather than fabricating identity.
				continue
			}
			t := tasks[ti]
			p := ScheduleProposal{
				TaskID:          t.ID,
				WBSCode:         t.WBSCode,
				Name:            t.Name,
				OldDurationDays: t.DurationDays,
				Rationale:       adj.Rationale,
				IsCritical:      t.IsCritical,
			}

			// Monitor-only: nil delta, or an out-of-range value, or a delta
			// equal to the current duration → advisory (no change to propose).
			inRange := adj.NewDurationDays != nil && *adj.NewDurationDays >= 1 && *adj.NewDurationDays <= maxTaskDurationDays
			isChange := inRange && *adj.NewDurationDays != t.DurationDays
			if isChange {
				p.NewDurationDays = adj.NewDurationDays
				p.ProposedChange = true
				proposed++
			} else {
				advisory++
			}
			proposals = append(proposals, p)

			// On the real (non-dry-run) path, apply the in-range change now.
			if !dryRun && isChange {
				if _, err := s.scheduleStore.UpdateTask(ctx, tx, store.UpdateTaskParams{
					TaskID:       t.ID,
					ProjectID:    projectID,
					DurationDays: adj.NewDurationDays,
				}); err != nil {
					return fmt.Errorf("apply duration delta for task %s: %w", t.ID, err)
				}
				proposals[len(proposals)-1].Applied = true
				applied++
			}
		}

		if !dryRun {
			// Per-row deltas only (no free-text rationale in audit metadata).
			s.audit.Record(ctx, tx, AuditEntry{
				OrgID:        callerOrgID,
				UserSub:      callerUserSub,
				Action:       "schedule.maestro_edit",
				ResourceType: AuditResourceSchedule,
				ResourceID:   projectID,
				Metadata: marshalAudit(map[string]any{
					"recommended_delta_count": len(resp.Adjustments),
					"applied_deltas":          applied,
					"skipped_rationale_only":  advisory,
					"deltas":                  scheduleDeltaAudit(proposals),
				}),
			})
		}

		result = ScheduleAdjustmentSet{
			Adjustments:          proposals,
			DryRun:               dryRun,
			ProposedChanges:      proposed,
			AdvisoryCount:        advisory,
			AppliedDeltas:        applied,
			SkippedRationaleOnly: advisory,
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ScheduleAdjustmentSet{}, ErrNotFound
		}
		return ScheduleAdjustmentSet{}, err
	}

	// CPM re-validation rides its own tx (real path only). Failure here
	// means deltas are persisted with audit trail but CPM is stale until
	// the next manual recalc; the wrapped error surfaces this state.
	if !dryRun && result.AppliedDeltas > 0 {
		if _, _, recalcErr := s.scheduleService.RecalculateSchedule(ctx, projectID, callerOrgID, callerUserSub); recalcErr != nil {
			return result, fmt.Errorf("apply succeeded; recalc deferred: %w", recalcErr)
		}
		result.CriticalRecomputed = true
	}
	return result, nil
}

// ScheduleAdjustmentApply is one user-selected duration change to commit
// via ApplyScheduleAdjustments. The user picks rows from a dry-run
// preview; WBSCode identifies the task within the project and
// NewDurationDays is the duration to write.
type ScheduleAdjustmentApply struct {
	WBSCode         string
	NewDurationDays int
}

// ScheduleApplyResult is the response from ApplyScheduleAdjustments.
type ScheduleApplyResult struct {
	AppliedDeltas      int  `json:"applied_deltas"`
	CriticalRecomputed bool `json:"critical_recomputed"`
}

// ApplyScheduleAdjustments commits a set of user-selected duration
// changes (PREVIEW-FIRST, ESC-AUX-01 — AI proposes, human commits). The
// caller supplies the rows it accepted from a RecommendScheduleAdjustments
// dry-run preview as {wbs_code, new_duration_days} pairs. The flow:
//
//  1. Loads the project's tasks (ownership-checked via VerifyProjectInOrg
//     → ErrNotFound cross-org). Indexes them by wbs_code.
//  2. Validates each row: the wbs must exist in the project, and the new
//     duration must be in [1, maxTaskDurationDays]. A bad row fails the
//     WHOLE batch with ErrInvalidInput (all-or-nothing; the user committed
//     a coherent set).
//  3. Updates each duration via ScheduleStore.UpdateTask in one tx.
//  4. Writes one schedule.adjustments.applied audit row with the per-task
//     deltas {wbs, old, new} — NO free-text rationale in the metadata.
//  5. Commits, then re-runs CPM in its own tx so floats / critical path
//     recompute (same deferred-recalc semantics as the auto-apply path).
//
// callerUserSub is recorded on the audit row.
func (s *AgentsService) ApplyScheduleAdjustments(ctx context.Context, callerOrgID uuid.UUID, callerUserSub string, projectID uuid.UUID, applies []ScheduleAdjustmentApply) (ScheduleApplyResult, error) {
	if callerOrgID == uuid.Nil {
		return ScheduleApplyResult{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if projectID == uuid.Nil {
		return ScheduleApplyResult{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if len(applies) == 0 {
		return ScheduleApplyResult{}, fmt.Errorf("%w: at least one adjustment is required", ErrInvalidInput)
	}
	if s.scheduleStore == nil || s.scheduleService == nil {
		return ScheduleApplyResult{}, ErrAgentsScheduleServiceUnavailable
	}

	var result ScheduleApplyResult
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}
		tasks, err := s.scheduleStore.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return fmt.Errorf("load tasks: %w", err)
		}
		byWBS := make(map[string]models.ProjectTask, len(tasks))
		for _, t := range tasks {
			byWBS[t.WBSCode] = t
		}

		// Validate the whole batch up-front so a single bad row doesn't
		// leave a partial apply (the tx would roll back anyway, but a clean
		// 400 beats a confusing constraint error).
		seen := make(map[string]bool, len(applies))
		type delta struct {
			task models.ProjectTask
			next int
		}
		deltas := make([]delta, 0, len(applies))
		for _, a := range applies {
			if a.WBSCode == "" {
				return fmt.Errorf("%w: wbs_code is required", ErrInvalidInput)
			}
			if seen[a.WBSCode] {
				return fmt.Errorf("%w: duplicate wbs_code %q", ErrInvalidInput, a.WBSCode)
			}
			seen[a.WBSCode] = true
			if a.NewDurationDays < 1 || a.NewDurationDays > maxTaskDurationDays {
				return fmt.Errorf("%w: new_duration_days for %q must be in [1, %d]", ErrInvalidInput, a.WBSCode, maxTaskDurationDays)
			}
			t, ok := byWBS[a.WBSCode]
			if !ok {
				return fmt.Errorf("%w: task %q not found in project", ErrInvalidInput, a.WBSCode)
			}
			deltas = append(deltas, delta{task: t, next: a.NewDurationDays})
		}

		applied := 0
		auditDeltas := make([]map[string]any, 0, len(deltas))
		for _, d := range deltas {
			next := d.next
			if _, err := s.scheduleStore.UpdateTask(ctx, tx, store.UpdateTaskParams{
				TaskID:       d.task.ID,
				ProjectID:    projectID,
				DurationDays: &next,
			}); err != nil {
				return fmt.Errorf("apply duration for %q: %w", d.task.WBSCode, mapStoreError(err))
			}
			auditDeltas = append(auditDeltas, map[string]any{
				"wbs": d.task.WBSCode,
				"old": d.task.DurationDays,
				"new": next,
			})
			applied++
		}

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "schedule.adjustments.applied",
			ResourceType: AuditResourceSchedule,
			ResourceID:   projectID,
			Metadata: marshalAudit(map[string]any{
				"applied_deltas": applied,
				"deltas":         auditDeltas,
			}),
		})

		result.AppliedDeltas = applied
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ScheduleApplyResult{}, ErrNotFound
		}
		return ScheduleApplyResult{}, err
	}

	if result.AppliedDeltas > 0 {
		if _, _, recalcErr := s.scheduleService.RecalculateSchedule(ctx, projectID, callerOrgID, callerUserSub); recalcErr != nil {
			return result, fmt.Errorf("apply succeeded; recalc deferred: %w", recalcErr)
		}
		result.CriticalRecomputed = true
	}
	return result, nil
}

// scheduleDeltaAudit projects the proposals that were actually applied
// into the per-row {wbs, old, new} shape the audit metadata records — no
// free-text rationale leaks into the audit row.
func scheduleDeltaAudit(proposals []ScheduleProposal) []map[string]any {
	out := make([]map[string]any, 0, len(proposals))
	for _, p := range proposals {
		if !p.Applied || p.NewDurationDays == nil {
			continue
		}
		out = append(out, map[string]any{
			"wbs": p.WBSCode,
			"old": p.OldDurationDays,
			"new": *p.NewDurationDays,
		})
	}
	return out
}
