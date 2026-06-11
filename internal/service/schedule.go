package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/physics"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/worker"
)

// ScheduleService orchestrates CPM scheduling with transactional guarantees.
// Implements the pattern from ARCHITECTURE.md Section 4:
// CPM calculation + schedule persistence + River job insertion in a single transaction.
type ScheduleService struct {
	pool          *pgxpool.Pool
	scheduleStore *store.ScheduleStore
	riverClient   *river.Client[pgx.Tx]
	audit         AuditRecorder
}

// NewScheduleService creates a new ScheduleService. audit may be nil;
// nil falls back to a no-op recorder so partial-wiring deployments
// (e.g. the worker daemon, integration tests) compile without an
// AuditService. Production wiring in cmd/server passes a real recorder.
func NewScheduleService(pool *pgxpool.Pool, scheduleStore *store.ScheduleStore, riverClient *river.Client[pgx.Tx], audit AuditRecorder) *ScheduleService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ScheduleService{
		pool:          pool,
		scheduleStore: scheduleStore,
		riverClient:   riverClient,
		audit:         audit,
	}
}

// RecalculateSchedule runs the CPM physics engine and persists results.
// All operations execute within a single pgx.BeginTxFunc transaction to prevent
// "phantom jobs" that reference data not yet committed. Cross-tenant guard:
// returns ErrNotFound if the project doesn't belong to callerOrgID.
//
// callerUserSub is the OIDC subject of the user triggering the recalc;
// recorded on the audit row. May be empty for system-triggered recalcs
// (e.g. River DelayCascade jobs); the AuditRecorder treats an empty
// UserSub as "system actor" rather than rejecting it.
func (s *ScheduleService) RecalculateSchedule(ctx context.Context, projectID, callerOrgID uuid.UUID, callerUserSub string) (*physics.CPMResult, time.Duration, error) {
	var cpmResult *physics.CPMResult
	var computeTime time.Duration

	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}
		res, took, err := s.recalcOnTx(ctx, tx, projectID, callerOrgID, callerUserSub)
		if err != nil {
			return err
		}
		cpmResult = res
		computeTime = took
		return nil
	})

	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, 0, ErrNotFound
		}
		return nil, 0, err
	}

	return cpmResult, computeTime, nil
}

// recalcOnTx runs the full CPM pipeline on an already-open tx: load
// tasks/deps → ForwardPass/BackwardPass → persist results → audit →
// enqueue a DelayCascade iff the critical set changed. It is the single
// CPM-on-tx body shared by RecalculateSchedule (which opens its own tx and
// verifies project ownership first) and ImportSchedule (which calls it after
// inserting the imported rows in the SAME tx, so GetProjectTasks sees them).
// The caller owns tx lifecycle and the project-ownership guard; this never
// commits, rolls back, or re-verifies the org.
func (s *ScheduleService) recalcOnTx(ctx context.Context, tx pgx.Tx, projectID, callerOrgID uuid.UUID, callerUserSub string) (*physics.CPMResult, time.Duration, error) {
	// 1. Load tasks and dependencies from DB
	tasks, err := s.scheduleStore.GetProjectTasks(ctx, tx, projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("load tasks: %w", err)
	}
	if len(tasks) == 0 {
		return nil, 0, fmt.Errorf("no tasks found for project %s", projectID)
	}

	deps, err := s.scheduleStore.GetProjectDependencies(ctx, tx, projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("load dependencies: %w", err)
	}

	projectStart, err := s.scheduleStore.GetProjectStartDate(ctx, tx, projectID)
	if err != nil {
		return nil, 0, fmt.Errorf("load project start: %w", err)
	}

	// 2. Run CPM ForwardPass + BackwardPass
	computeStart := time.Now()

	graph := physics.BuildDependencyGraph(tasks, deps)
	cal := &physics.StandardCalendar{}

	schedule, err := physics.ForwardPass(graph, projectStart, cal, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("forward pass: %w", err)
	}

	criticalPath, err := physics.BackwardPass(graph, schedule, cal, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("backward pass: %w", err)
	}

	computeTime := time.Since(computeStart)

	// Find project end date
	var projectEnd time.Time
	first := true
	for _, sched := range schedule {
		if first || sched.EarlyFinish.After(projectEnd) {
			projectEnd = sched.EarlyFinish
			first = false
		}
	}

	// Build result
	taskSchedules := make([]physics.TaskSchedule, 0, len(schedule))
	for _, ts := range schedule {
		taskSchedules = append(taskSchedules, ts)
	}

	cpmResult := &physics.CPMResult{
		Tasks:        taskSchedules,
		CriticalPath: criticalPath,
		ProjectEnd:   projectEnd,
	}

	// 3. Persist schedule results
	scheduleResults := make(map[uuid.UUID]store.ScheduleResult, len(schedule))
	for taskID, ts := range schedule {
		scheduleResults[taskID] = store.ScheduleResult{
			EarlyStart:  ts.EarlyStart,
			EarlyFinish: ts.EarlyFinish,
			LateStart:   ts.LateStart,
			LateFinish:  ts.LateFinish,
			TotalFloat:  int(math.Round(ts.TotalFloat)),
			IsCritical:  ts.IsCritical,
		}
	}

	if err := s.scheduleStore.UpdateSchedule(ctx, tx, tasks, scheduleResults); err != nil {
		return nil, 0, fmt.Errorf("persist schedule: %w", err)
	}

	// 4. Record audit row inside the same tx — if any later step
	//    fails, the audit row rolls back with the schedule write.
	s.audit.Record(ctx, tx, AuditEntry{
		OrgID:        callerOrgID,
		UserSub:      callerUserSub,
		Action:       "schedule.recalculated",
		ResourceType: AuditResourceSchedule,
		ResourceID:   projectID,
		Metadata: marshalAudit(map[string]any{
			"task_count":         len(tasks),
			"critical_path_size": len(criticalPath),
			"compute_ms":         computeTime.Milliseconds(),
			"project_end":        projectEnd,
		}),
	})

	// 5. Enqueue a delay cascade ONLY when the critical-path SET actually
	//    changed (SAME TRANSACTION). A recalc that leaves the same tasks
	//    critical — e.g. a duration tweak on a task that has float — must
	//    not fan out an AI-reasoned cascade: that would cost an Opus call
	//    plus a feed-card stream on every routine recalc. Compare the prior
	//    critical set (the loaded `tasks`, pre-overwrite) against the
	//    freshly computed set.
	priorCritical := make(map[uuid.UUID]bool, len(tasks))
	priorCount := 0
	for _, t := range tasks {
		if t.IsCritical {
			priorCritical[t.ID] = true
			priorCount++
		}
	}
	newCount := 0
	criticalChanged := false
	for id, sr := range scheduleResults {
		if sr.IsCritical {
			newCount++
			if !priorCritical[id] {
				criticalChanged = true
			}
		}
	}
	if newCount != priorCount {
		criticalChanged = true
	}
	cpmResult.CriticalPathChanged = criticalChanged

	if criticalChanged {
		if _, err := s.riverClient.InsertTx(ctx, tx, &worker.DelayCascadeArgs{
			OrgID:     callerOrgID,
			ProjectID: projectID,
		}, nil); err != nil {
			return nil, 0, fmt.Errorf("enqueue delay cascade: %w", err)
		}
	}

	return cpmResult, computeTime, nil
}

// GanttView is the response shape for GET /schedule/gantt: the full task
// list, the IDs of critical-path tasks (in WBS order), and the project
// end date computed from stored CPM results. Critical path is read from
// is_critical column rather than re-running the physics engine — the
// engine results are persisted by RecalculateSchedule.
type GanttView struct {
	Tasks        []models.ProjectTask `json:"tasks"`
	CriticalPath []uuid.UUID          `json:"critical_path"`
	ProjectEnd   time.Time            `json:"project_end"`
}

// GetGantt returns the Gantt-shaped view of a project's stored schedule.
// Returns ErrNotFound for tenant mismatches and missing projects.
func (s *ScheduleService) GetGantt(ctx context.Context, projectID, callerOrgID uuid.UUID) (GanttView, error) {
	var view GanttView
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}
		tasks, err := s.scheduleStore.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		view = ganttFromTasks(tasks)
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return GanttView{}, ErrNotFound
		}
		return GanttView{}, err
	}
	return view, nil
}

// ganttFromTasks assembles the GanttView from a task slice, picking out
// is_critical=true tasks (in WBS order — input is pre-sorted) and the
// project end as the max early_finish across all tasks. A project that
// has never been recalculated (no CPM results yet) returns zero ProjectEnd
// and an empty critical path; the frontend can detect this and prompt
// the user to run /recalculate.
func ganttFromTasks(tasks []models.ProjectTask) GanttView {
	view := GanttView{Tasks: tasks, CriticalPath: make([]uuid.UUID, 0)}
	for _, t := range tasks {
		if t.IsCritical {
			view.CriticalPath = append(view.CriticalPath, t.ID)
		}
		if t.EarlyFinish != nil && t.EarlyFinish.After(view.ProjectEnd) {
			view.ProjectEnd = *t.EarlyFinish
		}
	}
	return view
}

// ListProjectTasksInput controls a task listing.
type ListProjectTasksInput struct {
	ProjectID  uuid.UUID
	OrgID      uuid.UUID
	Status     string // optional
	IsCritical *bool  // optional
}

// ListProjectTasks returns tasks for a project with optional filters.
// Validates status if provided.
func (s *ScheduleService) ListProjectTasks(ctx context.Context, in ListProjectTasksInput) ([]models.ProjectTask, error) {
	if in.Status != "" && !isValidTaskStatus(in.Status) {
		return nil, fmt.Errorf("%w: status %q is not one of {pending, in_progress, completed}", ErrInvalidInput, in.Status)
	}
	var out []models.ProjectTask
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, in.OrgID); err != nil {
			return err
		}
		var qErr error
		out, qErr = s.scheduleStore.ListProjectTasks(ctx, tx, store.ListTasksParams{
			ProjectID:  in.ProjectID,
			Status:     in.Status,
			IsCritical: in.IsCritical,
		})
		return qErr
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return out, nil
}

// UpdateTaskInput is the service-layer input for partial-updating a task.
// AssignedCrew uses a pointer-to-slice so callers can distinguish:
//
//	nil          → no change
//	&[]uuid.UUID{} → clear crew
//	&[]uuid.UUID{ids...} → replace crew
type UpdateTaskInput struct {
	TaskID          uuid.UUID
	ProjectID       uuid.UUID
	OrgID           uuid.UUID
	PercentComplete *int
	Status          *string
	AssignedCrew    *[]uuid.UUID
}

// UpdateTask modifies a task. Validates status and percent_complete
// ranges. Does NOT trigger CPM recalculation — caller must POST
// /schedule/recalculate to reshuffle the critical path.
func (s *ScheduleService) UpdateTask(ctx context.Context, in UpdateTaskInput) (models.ProjectTask, error) {
	if in.PercentComplete != nil && (*in.PercentComplete < 0 || *in.PercentComplete > 100) {
		return models.ProjectTask{}, fmt.Errorf("%w: percent_complete must be 0..100", ErrInvalidInput)
	}
	if in.Status != nil && !isValidTaskStatus(*in.Status) {
		return models.ProjectTask{}, fmt.Errorf("%w: status %q is not one of {pending, in_progress, completed}", ErrInvalidInput, *in.Status)
	}

	var task models.ProjectTask
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, in.OrgID); err != nil {
			return err
		}
		updated, err := s.scheduleStore.UpdateTask(ctx, tx, store.UpdateTaskParams{
			TaskID:          in.TaskID,
			ProjectID:       in.ProjectID,
			PercentComplete: in.PercentComplete,
			Status:          in.Status,
			AssignedCrew:    in.AssignedCrew,
		})
		if err != nil {
			return err
		}
		task = updated
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return models.ProjectTask{}, ErrNotFound
		}
		return models.ProjectTask{}, err
	}
	return task, nil
}

// isValidTaskStatus is the schedule-side analogue of
// models.IsValidInvoiceStatus. Project_tasks status enum: pending,
// in_progress, completed. (Schema CHECK constraint enforces same set.)
func isValidTaskStatus(s string) bool {
	switch s {
	case "pending", "in_progress", "completed":
		return true
	default:
		return false
	}
}
