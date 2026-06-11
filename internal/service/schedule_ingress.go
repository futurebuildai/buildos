package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/models/types"
	"github.com/futurebuildai/buildos/internal/physics"
	"github.com/futurebuildai/buildos/internal/store"
)

// Ingress bounds. duration_days lower bound is 1 (NOT 0): physics.getTaskDuration
// rejects DurationDays==0 with ErrInvalidTaskDuration (the float override fields
// are json:"-", never loaded on ingress, so DurationDays is the only effective
// duration). migration 019's CHECK (0..36500) is the DB backstop; we enforce the
// tighter [1, 36500] service-side so an imported task can never break the next
// recalc. lag_days is bounded ±10y (CHECK-less column; magnitude has the same
// CPU-loop DoS shape that motivated the duration cap).
// maxTaskDurationDays is reused from agents.go (the migration-019 cap, 36500).
const (
	minTaskDurationDays = 1
	minLagDays          = -3650
	maxLagDays          = 3650
)

// uuidNamespaceIngress is a fixed namespace for synthesizing stable
// placeholder task UUIDs (keyed by wbs_code) for the pre-persist cycle graph.
// The real DB UUIDs are assigned at insert; these only need to be internally
// consistent within one validation pass.
var uuidNamespaceIngress = uuid.MustParse("6f9619ff-8b86-d011-b42d-00cf4fc964ff")

// ImportTaskInput is one task in a schedule import. CPM-output fields are
// not accepted (ignored if present on the wire). Status/PercentComplete
// carry their defaults from the handler when omitted.
type ImportTaskInput struct {
	WBSCode         string
	Name            string
	DurationDays    int
	Status          string
	PercentComplete int
	AssignedCrew    []uuid.UUID
}

// ImportDependencyInput is one wbs_code-keyed dependency. The client does
// not know server-assigned task UUIDs, so deps reference tasks[].WBSCode in
// the same batch; the service resolves codes to UUIDs after insert.
type ImportDependencyInput struct {
	PredecessorCode string
	SuccessorCode   string
	DependencyType  string
	LagDays         int
}

// ImportScheduleInput is the validated input for ImportSchedule.
type ImportScheduleInput struct {
	Tasks        []ImportTaskInput
	Dependencies []ImportDependencyInput
	Recalculate  bool
}

// ImportScheduleResult is the outcome of an import: the persisted tasks
// (with server IDs; CPM columns null until recalc), the dependency count,
// and — when Recalculate was true — the CPM result + compute time.
type ImportScheduleResult struct {
	Tasks           []models.ProjectTask
	DependencyCount int
	CPMResult       *physics.CPMResult
	RecalcDuration  time.Duration
}

// ImportSchedule authors a whole task graph (tasks + dependencies) for a
// project atomically: validate everything (incl. acyclicity) BEFORE any
// write, insert tasks RETURNING ids, resolve wbs_code→UUID for deps, insert
// deps, then — when Recalculate is true — run CPM on the SAME tx via
// recalcOnTx so the Gantt is populated in the same request. A rejected
// import leaves zero state (validation is pre-tx; cycle/self-loop rejected
// before graph construction so gonum never panics).
//
// Cross-tenant guard: ErrNotFound if the project doesn't belong to callerOrgID.
// A 23505 (UNIQUE(project_id, wbs_code) against existing rows, or duplicate
// dep pair) maps to ErrInvalidInput (400).
func (s *ScheduleService) ImportSchedule(ctx context.Context, projectID, callerOrgID uuid.UUID, callerUserSub string, in ImportScheduleInput) (*ImportScheduleResult, error) {
	if callerOrgID == uuid.Nil {
		return nil, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}

	// ---- Validation: ALL of it BEFORE the tx (no partial state) ----

	if len(in.Tasks) == 0 {
		return nil, fmt.Errorf("%w: tasks is required", ErrInvalidInput)
	}

	taskParams := make([]store.InsertTaskParams, 0, len(in.Tasks))
	seenWBS := make(map[string]struct{}, len(in.Tasks))
	for i := range in.Tasks {
		t := in.Tasks[i]
		wbs := strings.TrimSpace(t.WBSCode)
		if wbs == "" {
			return nil, fmt.Errorf("%w: task wbs_code is required", ErrInvalidInput)
		}
		if _, dup := seenWBS[wbs]; dup {
			return nil, fmt.Errorf("%w: duplicate wbs_code %q in batch", ErrInvalidInput, wbs)
		}
		seenWBS[wbs] = struct{}{}

		name := strings.TrimSpace(t.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: task %q name is required", ErrInvalidInput, wbs)
		}
		if t.DurationDays < minTaskDurationDays || t.DurationDays > maxTaskDurationDays {
			return nil, fmt.Errorf("%w: task %q duration_days must be %d..%d", ErrInvalidInput, wbs, minTaskDurationDays, maxTaskDurationDays)
		}
		status := t.Status
		if status == "" {
			status = "pending"
		}
		if !isValidTaskStatus(status) {
			return nil, fmt.Errorf("%w: task %q status %q is not one of {pending, in_progress, completed}", ErrInvalidInput, wbs, status)
		}
		if t.PercentComplete < 0 || t.PercentComplete > 100 {
			return nil, fmt.Errorf("%w: task %q percent_complete must be 0..100", ErrInvalidInput, wbs)
		}

		taskParams = append(taskParams, store.InsertTaskParams{
			ProjectID:       projectID,
			WBSCode:         wbs,
			Name:            name,
			Status:          status,
			DurationDays:    t.DurationDays,
			PercentComplete: t.PercentComplete,
			AssignedCrew:    t.AssignedCrew,
		})
	}

	// Dependency validation. Build the proposed-graph rows as we go so we can
	// run DetectCycle BEFORE the tx. Self-loops are rejected here, before any
	// graph construction, because gonum's SetEdge PANICS on a self-edge.
	type depRow struct {
		predCode, succCode, depType string
		lagDays                     int
	}
	depRows := make([]depRow, 0, len(in.Dependencies))
	seenPairs := make(map[[2]string]struct{}, len(in.Dependencies))
	for i := range in.Dependencies {
		d := in.Dependencies[i]
		pred := strings.TrimSpace(d.PredecessorCode)
		succ := strings.TrimSpace(d.SuccessorCode)
		if pred == "" || succ == "" {
			return nil, fmt.Errorf("%w: dependency predecessor_code and successor_code are required", ErrInvalidInput)
		}
		if pred == succ {
			return nil, fmt.Errorf("%w: dependency self-loop on wbs_code %q is not allowed", ErrInvalidInput, pred)
		}
		if _, ok := seenWBS[pred]; !ok {
			return nil, fmt.Errorf("%w: dependency references unknown wbs_code %q", ErrInvalidInput, pred)
		}
		if _, ok := seenWBS[succ]; !ok {
			return nil, fmt.Errorf("%w: dependency references unknown wbs_code %q", ErrInvalidInput, succ)
		}
		pair := [2]string{pred, succ}
		if _, dup := seenPairs[pair]; dup {
			return nil, fmt.Errorf("%w: duplicate dependency %q->%q in batch", ErrInvalidInput, pred, succ)
		}
		seenPairs[pair] = struct{}{}

		depType := d.DependencyType
		if depType == "" {
			depType = string(types.DependencyTypeFS)
		}
		if !isValidDependencyType(depType) {
			return nil, fmt.Errorf("%w: dependency %q->%q dependency_type %q is not one of {FS, SS, FF, SF}", ErrInvalidInput, pred, succ, depType)
		}
		if d.LagDays < minLagDays || d.LagDays > maxLagDays {
			return nil, fmt.Errorf("%w: dependency %q->%q lag_days must be %d..%d", ErrInvalidInput, pred, succ, minLagDays, maxLagDays)
		}

		depRows = append(depRows, depRow{predCode: pred, succCode: succ, depType: depType, lagDays: d.LagDays})
	}

	// Cycle rejection BEFORE any write. Synthesize ProjectTask/TaskDependency
	// with deterministic placeholder IDs keyed by wbs_code, build the in-memory
	// graph, and run DetectCycle. Self-loops already rejected above, so no
	// edge here is a self-edge and SetEdge cannot panic.
	if len(depRows) > 0 {
		codeToID := make(map[string]uuid.UUID, len(taskParams))
		proposedTasks := make([]models.ProjectTask, 0, len(taskParams))
		for _, tp := range taskParams {
			id := uuid.NewSHA1(uuidNamespaceIngress, []byte(tp.WBSCode))
			codeToID[tp.WBSCode] = id
			proposedTasks = append(proposedTasks, models.ProjectTask{ID: id, WBSCode: tp.WBSCode})
		}
		proposedDeps := make([]models.TaskDependency, 0, len(depRows))
		for _, dr := range depRows {
			proposedDeps = append(proposedDeps, models.TaskDependency{
				PredecessorID:  codeToID[dr.predCode],
				SuccessorID:    codeToID[dr.succCode],
				DependencyType: types.DependencyType(dr.depType),
				LagDays:        dr.lagDays,
			})
		}
		graph := physics.BuildDependencyGraph(proposedTasks, proposedDeps)
		if err := physics.DetectCycle(graph); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
	}

	// ---- Persist + recalc in ONE tx ----

	result := &ImportScheduleResult{DependencyCount: len(depRows)}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID); err != nil {
			return err
		}

		inserted, err := s.scheduleStore.InsertTasks(ctx, tx, taskParams)
		if err != nil {
			return err
		}
		result.Tasks = inserted

		// Map wbs_code → server-assigned UUID for dependency resolution.
		codeToID := make(map[string]uuid.UUID, len(inserted))
		for _, t := range inserted {
			codeToID[t.WBSCode] = t.ID
		}
		if len(depRows) > 0 {
			depParams := make([]store.InsertDependencyParams, 0, len(depRows))
			for _, dr := range depRows {
				depParams = append(depParams, store.InsertDependencyParams{
					ProjectID:      projectID,
					PredecessorID:  codeToID[dr.predCode],
					SuccessorID:    codeToID[dr.succCode],
					DependencyType: dr.depType,
					LagDays:        dr.lagDays,
				})
			}
			if err := s.scheduleStore.InsertDependencies(ctx, tx, depParams); err != nil {
				return err
			}
		}

		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "schedule.imported",
			ResourceType: AuditResourceSchedule,
			ResourceID:   projectID,
			Metadata: marshalAudit(map[string]any{
				"task_count":       len(inserted),
				"dependency_count": len(depRows),
				"recalculated":     in.Recalculate,
			}),
		})

		if in.Recalculate {
			// Same-tx recalc: GetProjectTasks/GetProjectDependencies see the
			// just-inserted rows. recalcOnTx persists CPM cols + enqueues a
			// DelayCascade (the critical set goes ∅ → non-empty on first import).
			cpm, took, rerr := s.recalcOnTx(ctx, tx, projectID, callerOrgID, callerUserSub)
			if rerr != nil {
				return rerr
			}
			result.CPMResult = cpm
			result.RecalcDuration = took

			// Re-read so the returned tasks carry the freshly persisted CPM
			// columns (early_start/late_finish/is_critical), not the
			// pre-recalc null state from the insert RETURNING.
			refreshed, rerr := s.scheduleStore.GetProjectTasks(ctx, tx, projectID)
			if rerr != nil {
				return rerr
			}
			result.Tasks = refreshed
		}
		return nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return result, nil
}

// CreateTaskInput is the validated input for a single-task create. No
// dependencies, no auto-recalc (mirrors UpdateTask). CallerUserSub is
// recorded on the audit row.
type CreateTaskInput struct {
	ProjectID       uuid.UUID
	OrgID           uuid.UUID
	CallerUserSub   string
	WBSCode         string
	Name            string
	DurationDays    int
	Status          string
	PercentComplete int
	AssignedCrew    []uuid.UUID
}

// CreateTask inserts a single task (reusing the batch InsertTasks store call
// with a 1-element slice). Validates duration/status/percent like the import
// path. Does NOT trigger CPM recalc — the operator POSTs /schedule/recalculate
// afterward (matches UpdateTask).
func (s *ScheduleService) CreateTask(ctx context.Context, in CreateTaskInput) (models.ProjectTask, error) {
	if in.OrgID == uuid.Nil {
		return models.ProjectTask{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	wbs := strings.TrimSpace(in.WBSCode)
	if wbs == "" {
		return models.ProjectTask{}, fmt.Errorf("%w: wbs_code is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return models.ProjectTask{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if in.DurationDays < minTaskDurationDays || in.DurationDays > maxTaskDurationDays {
		return models.ProjectTask{}, fmt.Errorf("%w: duration_days must be %d..%d", ErrInvalidInput, minTaskDurationDays, maxTaskDurationDays)
	}
	status := in.Status
	if status == "" {
		status = "pending"
	}
	if !isValidTaskStatus(status) {
		return models.ProjectTask{}, fmt.Errorf("%w: status %q is not one of {pending, in_progress, completed}", ErrInvalidInput, status)
	}
	if in.PercentComplete < 0 || in.PercentComplete > 100 {
		return models.ProjectTask{}, fmt.Errorf("%w: percent_complete must be 0..100", ErrInvalidInput)
	}

	var task models.ProjectTask
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, in.OrgID); err != nil {
			return err
		}
		inserted, err := s.scheduleStore.InsertTasks(ctx, tx, []store.InsertTaskParams{{
			ProjectID:       in.ProjectID,
			WBSCode:         wbs,
			Name:            name,
			Status:          status,
			DurationDays:    in.DurationDays,
			PercentComplete: in.PercentComplete,
			AssignedCrew:    in.AssignedCrew,
		}})
		if err != nil {
			return err
		}
		task = inserted[0]
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.CallerUserSub,
			Action:       "task.created",
			ResourceType: AuditResourceProjectTask,
			ResourceID:   task.ID,
			After:        marshalAudit(task),
			Metadata: marshalAudit(map[string]any{
				"project_id": in.ProjectID,
				"wbs_code":   task.WBSCode,
			}),
		})
		return nil
	})
	if err != nil {
		return models.ProjectTask{}, mapStoreError(err)
	}
	return task, nil
}

// isValidDependencyType reports whether s is one of the four CPM dependency
// relationships. types.DependencyType has no .Valid() method, so this checks
// against the four consts directly — an invalid value would otherwise fall
// through to FS in calculateConstraintDate's default branch.
func isValidDependencyType(s string) bool {
	switch types.DependencyType(s) {
	case types.DependencyTypeFS, types.DependencyTypeSS, types.DependencyTypeFF, types.DependencyTypeSF:
		return true
	default:
		return false
	}
}
