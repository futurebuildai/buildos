//go:build integration

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestScheduleService_ImportSchedule_KeystoneRoundTrip is the keystone proof:
// importing a 3-task finish-to-start chain (with recalculate=true) inserts the
// tasks+deps in one tx, runs CPM on the SAME tx, and persists a REAL critical
// path — verified by reading the populated early_start/late_finish/is_critical
// columns back through GetGantt. A pure FS chain has no slack, so every task
// lands on the critical path. Exactly one schedule.imported + one
// schedule.recalculated audit row are written, and a delay_cascade job is
// enqueued (∅ → non-empty critical set).
func TestScheduleService_ImportSchedule_KeystoneRoundTrip(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	res, err := svc.ImportSchedule(ctx, fx.projectID, fx.orgID, "owner-sub", ImportScheduleInput{
		Tasks: []ImportTaskInput{
			{WBSCode: "01-00", Name: "Site Prep", DurationDays: 3},
			{WBSCode: "03-30", Name: "Foundation", DurationDays: 5},
			{WBSCode: "06-10", Name: "Framing", DurationDays: 8},
		},
		Dependencies: []ImportDependencyInput{
			{PredecessorCode: "01-00", SuccessorCode: "03-30", DependencyType: "FS"},
			{PredecessorCode: "03-30", SuccessorCode: "06-10", DependencyType: "FS"},
		},
		Recalculate: true,
	})
	if err != nil {
		t.Fatalf("ImportSchedule: %v", err)
	}
	if len(res.Tasks) != 3 {
		t.Fatalf("imported tasks = %d, want 3", len(res.Tasks))
	}
	if res.DependencyCount != 2 {
		t.Errorf("dependency_count = %d, want 2", res.DependencyCount)
	}
	if res.CPMResult == nil {
		t.Fatal("CPMResult is nil, want a populated result (recalculate=true)")
	}
	if len(res.CPMResult.CriticalPath) == 0 {
		t.Error("critical path is empty, want the chain's critical tasks")
	}
	if !res.CPMResult.CriticalPathChanged {
		t.Error("CriticalPathChanged = false, want true on first import (∅ → chain)")
	}
	if res.CPMResult.ProjectEnd.IsZero() {
		t.Error("ProjectEnd is zero, want the computed chain finish")
	}

	// The returned tasks carry the freshly persisted CPM columns (re-read in-tx).
	populated := 0
	for _, tk := range res.Tasks {
		if tk.EarlyStart != nil && tk.EarlyFinish != nil && tk.LateStart != nil && tk.LateFinish != nil {
			populated++
		}
	}
	if populated != 3 {
		t.Errorf("tasks with populated CPM columns = %d, want 3", populated)
	}

	// Audit: one import row + one recalc row (recalcOnTx writes the latter).
	if got := scheduleAuditCount(t, svc, fx.orgID, "schedule.imported"); got != 1 {
		t.Errorf("schedule.imported audit rows = %d, want 1", got)
	}
	if got := scheduleAuditCount(t, svc, fx.orgID, "schedule.recalculated"); got != 1 {
		t.Errorf("schedule.recalculated audit rows = %d, want 1", got)
	}
	if got := delayCascadeJobCount(t, svc); got != 1 {
		t.Errorf("delay_cascade river_job rows = %d, want 1", got)
	}

	// GetProjectTasks (via GetGantt) reflects the persisted critical path —
	// proving the import flowed through the identical engine path and wrote
	// the right is_critical flags.
	gantt, err := svc.GetGantt(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetGantt: %v", err)
	}
	if len(gantt.Tasks) != 3 {
		t.Errorf("gantt tasks = %d, want 3", len(gantt.Tasks))
	}
	if len(gantt.CriticalPath) != len(res.CPMResult.CriticalPath) {
		t.Errorf("gantt critical path = %d, want %d (persisted == computed)", len(gantt.CriticalPath), len(res.CPMResult.CriticalPath))
	}
	if gantt.ProjectEnd.IsZero() {
		t.Error("gantt ProjectEnd is zero, want the persisted early_finish max")
	}
}

// TestScheduleService_ImportSchedule_NoRecalcLeavesNullCPM proves recalculate=false
// lands tasks with NULL CPM columns (operator runs /recalculate later) and
// enqueues NO cascade.
func TestScheduleService_ImportSchedule_NoRecalcLeavesNullCPM(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	res, err := svc.ImportSchedule(ctx, fx.projectID, fx.orgID, "owner-sub", ImportScheduleInput{
		Tasks: []ImportTaskInput{
			{WBSCode: "01-00", Name: "Site Prep", DurationDays: 3},
			{WBSCode: "03-30", Name: "Foundation", DurationDays: 5},
		},
		Dependencies: []ImportDependencyInput{
			{PredecessorCode: "01-00", SuccessorCode: "03-30"},
		},
		Recalculate: false,
	})
	if err != nil {
		t.Fatalf("ImportSchedule: %v", err)
	}
	if res.CPMResult != nil {
		t.Error("CPMResult is non-nil, want nil when recalculate=false")
	}
	for _, tk := range res.Tasks {
		if tk.EarlyStart != nil || tk.IsCritical {
			t.Errorf("task %s has CPM data without recalc", tk.WBSCode)
		}
	}
	if got := scheduleAuditCount(t, svc, fx.orgID, "schedule.recalculated"); got != 0 {
		t.Errorf("recalculated audit rows = %d, want 0 (no recalc)", got)
	}
	if got := delayCascadeJobCount(t, svc); got != 0 {
		t.Errorf("delay_cascade jobs = %d, want 0 (no recalc)", got)
	}
}

// TestScheduleService_ImportSchedule_CrossTenantHidden proves the org guard:
// a foreign org importing into another org's project gets ErrNotFound and
// writes nothing.
func TestScheduleService_ImportSchedule_CrossTenantHidden(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()
	foreignOrg := uuid.New()

	_, err := svc.ImportSchedule(ctx, fx.projectID, foreignOrg, "intruder", ImportScheduleInput{
		Tasks:       []ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 3}},
		Recalculate: true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (cross-tenant)", err)
	}
	// Nothing written: the real org sees no tasks.
	gantt, err := svc.GetGantt(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetGantt: %v", err)
	}
	if len(gantt.Tasks) != 0 {
		t.Errorf("tasks after rejected cross-tenant import = %d, want 0", len(gantt.Tasks))
	}
}

// TestScheduleService_ImportSchedule_DuplicateWBSAgainstExisting proves a second
// import re-using a wbs_code (UNIQUE(project_id, wbs_code)) maps 23505 →
// ErrInvalidInput, and rolls back (the second import's tasks don't land).
func TestScheduleService_ImportSchedule_DuplicateWBSAgainstExisting(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	if _, err := svc.ImportSchedule(ctx, fx.projectID, fx.orgID, "owner", ImportScheduleInput{
		Tasks:       []ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 3}},
		Recalculate: false,
	}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	// Re-import the same wbs_code → 23505 → ErrInvalidInput.
	_, err := svc.ImportSchedule(ctx, fx.projectID, fx.orgID, "owner", ImportScheduleInput{
		Tasks:       []ImportTaskInput{{WBSCode: "01-00", Name: "Site Again", DurationDays: 4}},
		Recalculate: false,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duplicate wbs import: err = %v, want ErrInvalidInput", err)
	}
	gantt, err := svc.GetGantt(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetGantt: %v", err)
	}
	if len(gantt.Tasks) != 1 {
		t.Errorf("tasks after rejected dup import = %d, want 1 (rollback)", len(gantt.Tasks))
	}
}

// TestScheduleService_CreateTask_PersistsNoRecalc proves the single-task create
// lands a row (NULL CPM cols) without enqueuing a cascade.
func TestScheduleService_CreateTask_PersistsNoRecalc(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, CreateTaskInput{
		ProjectID:    fx.projectID,
		OrgID:        fx.orgID,
		WBSCode:      "01-00",
		Name:         "Site Prep",
		DurationDays: 3,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.ID == uuid.Nil || task.WBSCode != "01-00" {
		t.Errorf("task = %+v, want a persisted 01-00 row", task)
	}
	if task.EarlyStart != nil || task.IsCritical {
		t.Error("new task has CPM data without recalc")
	}
	if got := delayCascadeJobCount(t, svc); got != 0 {
		t.Errorf("delay_cascade jobs = %d, want 0", got)
	}
}
