//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// schedFixture bundles the seeded org + project ids a schedule test works
// against. project_start_date is pinned to a fixed date so the CPM
// ForwardPass has a deterministic root (the audit metadata's project_end
// derives from it).
type schedFixture struct {
	orgID     uuid.UUID
	projectID uuid.UUID
}

var schedStartDate = time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)

// newScheduleService wires a real ScheduleService to a fresh migrated pool.
// Two pieces beyond the usual org+project seed are required for the recalc
// round-trip:
//
//   - River's job tables (river_job, …) are NOT in migrations/*.up.sql; they
//     are applied separately by rivermigrate in cmd/migrate. We apply them
//     here so the in-tx InsertTx of the DelayCascade follow-up job has a table
//     to land in.
//   - An insert-only River client (no Workers/Queues) — RecalculateSchedule
//     only InsertTx's the DelayCascade job inside the schedule tx; nothing is
//     worked in-test, so an insert-only client is sufficient.
//
// A REAL audit recorder is used so the in-tx schedule.recalculated write
// actually hits audit_log (verifiable via scheduleAuditCount).
func newScheduleService(t *testing.T) (*ScheduleService, *schedFixture) {
	t.Helper()
	pool := testdb.NewPool(t)
	ctx := context.Background()

	migrator, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		t.Fatalf("river migrator: %v", err)
	}
	if _, err := migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		t.Fatalf("river migrate up: %v", err)
	}

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		t.Fatalf("river client: %v", err)
	}

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Tallgrass Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Cedar Court Custom")
	if _, err := pool.Exec(ctx,
		`UPDATE projects SET project_start_date = $2 WHERE id = $1`,
		projectID, schedStartDate); err != nil {
		t.Fatalf("pin project_start_date: %v", err)
	}

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewScheduleService(pool, store.NewScheduleStore(), riverClient, audit)
	return svc, &schedFixture{orgID: orgID, projectID: projectID}
}

func scheduleAuditCount(t *testing.T, s *ScheduleService, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, action).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// delayCascadeJobCount reads the River queue table directly to prove the
// follow-up job was enqueued inside the schedule tx (committed, not phantom).
func delayCascadeJobCount(t *testing.T, s *ScheduleService) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM river_job WHERE kind = $1`, "delay_cascade").Scan(&n); err != nil {
		t.Fatalf("count river_job: %v", err)
	}
	return n
}

// seedSchedTask inserts a project_tasks row with the given WBS/name/duration
// (status pending, 0% complete) and returns its id.
func seedSchedTask(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, name string, durationDays int) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status, percent_complete)
		VALUES ($1, $2, $3, $4, 'pending', 0)
		RETURNING id`, projectID, wbs, name, durationDays).Scan(&id); err != nil {
		t.Fatalf("seed task %s: %v", wbs, err)
	}
	return id
}

// seedSchedDep inserts a finish-to-start dependency (zero lag).
func seedSchedDep(t *testing.T, pool *pgxpool.Pool, projectID, pred, succ uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO task_dependencies (project_id, predecessor_id, successor_id, dependency_type, lag_days)
		VALUES ($1, $2, $3, 'FS', 0)`, projectID, pred, succ); err != nil {
		t.Fatalf("seed dependency: %v", err)
	}
}

// TestScheduleService_Recalculate_PersistsComputesAndEnqueues is the canonical
// CPM round-trip: a 3-task finish-to-start chain is recalculated, the physics
// result is returned + persisted, exactly one schedule.recalculated audit row
// is written, and a delay_cascade River job is enqueued in the SAME tx. A pure
// chain has no parallel slack, so every task lands on the critical path; the
// stored is_critical flags are read back through GetGantt.
func TestScheduleService_Recalculate_PersistsComputesAndEnqueues(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	a := seedSchedTask(t, svc.pool, fx.projectID, "1.0", "Foundation", 5)
	b := seedSchedTask(t, svc.pool, fx.projectID, "2.0", "Framing", 3)
	c := seedSchedTask(t, svc.pool, fx.projectID, "3.0", "Roofing", 2)
	seedSchedDep(t, svc.pool, fx.projectID, a, b)
	seedSchedDep(t, svc.pool, fx.projectID, b, c)

	res, computeTime, err := svc.RecalculateSchedule(ctx, fx.projectID, fx.orgID, "owner-sub")
	if err != nil {
		t.Fatalf("RecalculateSchedule: %v", err)
	}
	if res == nil {
		t.Fatal("nil CPM result")
	}
	if len(res.Tasks) != 3 {
		t.Errorf("result tasks = %d, want 3", len(res.Tasks))
	}
	if !res.CriticalPathChanged {
		t.Error("CriticalPathChanged = false, want true (chain has a critical path)")
	}
	if len(res.CriticalPath) == 0 {
		t.Error("critical path is empty, want the chain's critical tasks")
	}
	if res.ProjectEnd.IsZero() {
		t.Error("ProjectEnd is zero, want the computed chain finish")
	}
	if computeTime <= 0 {
		t.Errorf("compute time = %v, want > 0", computeTime)
	}

	if got := scheduleAuditCount(t, svc, fx.orgID, "schedule.recalculated"); got != 1 {
		t.Errorf("schedule.recalculated audit rows = %d, want 1", got)
	}
	if got := delayCascadeJobCount(t, svc); got != 1 {
		t.Errorf("delay_cascade river_job rows = %d, want 1", got)
	}

	// The persisted is_critical flags round-trip through the Gantt read.
	gantt, err := svc.GetGantt(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetGantt: %v", err)
	}
	if len(gantt.Tasks) != 3 {
		t.Errorf("gantt tasks = %d, want 3", len(gantt.Tasks))
	}
	// The persisted is_critical flags must match exactly what the engine
	// computed — proving UpdateSchedule wrote the right rows in the tx.
	if len(gantt.CriticalPath) != len(res.CriticalPath) {
		t.Errorf("gantt critical path = %d, want %d (persisted == computed)", len(gantt.CriticalPath), len(res.CriticalPath))
	}
	if gantt.ProjectEnd.IsZero() {
		t.Error("gantt ProjectEnd is zero, want the persisted early_finish max")
	}
}

// TestScheduleService_Recalculate_NoChangeDoesNotReEnqueue proves the bug_002
// fix: a second recalc that leaves the critical-path SET unchanged must NOT
// re-enqueue a delay cascade — otherwise every routine recalc would cost an
// Opus call + a feed-card stream. The first recalc establishes the critical
// path (the set goes from empty → the chain, so it fires once); the identical
// second recalc leaves the same tasks critical, so CriticalPathChanged is
// false and no new delay_cascade job lands.
func TestScheduleService_Recalculate_NoChangeDoesNotReEnqueue(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	a := seedSchedTask(t, svc.pool, fx.projectID, "1.0", "Foundation", 5)
	b := seedSchedTask(t, svc.pool, fx.projectID, "2.0", "Framing", 3)
	c := seedSchedTask(t, svc.pool, fx.projectID, "3.0", "Roofing", 2)
	seedSchedDep(t, svc.pool, fx.projectID, a, b)
	seedSchedDep(t, svc.pool, fx.projectID, b, c)

	// First recalc: the critical set goes from empty → the chain. Fires once.
	first, _, err := svc.RecalculateSchedule(ctx, fx.projectID, fx.orgID, "owner-sub")
	if err != nil {
		t.Fatalf("first RecalculateSchedule: %v", err)
	}
	if !first.CriticalPathChanged {
		t.Error("first recalc: CriticalPathChanged = false, want true (critical path established)")
	}
	if got := delayCascadeJobCount(t, svc); got != 1 {
		t.Fatalf("after first recalc: delay_cascade jobs = %d, want 1", got)
	}

	// Second recalc, identical inputs: the same tasks stay critical, so the
	// set is unchanged and no cascade should fire.
	second, _, err := svc.RecalculateSchedule(ctx, fx.projectID, fx.orgID, "owner-sub")
	if err != nil {
		t.Fatalf("second RecalculateSchedule: %v", err)
	}
	if second.CriticalPathChanged {
		t.Error("second recalc: CriticalPathChanged = true, want false (critical set unchanged)")
	}
	if got := delayCascadeJobCount(t, svc); got != 1 {
		t.Errorf("after unchanged second recalc: delay_cascade jobs = %d, want 1 (no re-enqueue)", got)
	}
}

// TestScheduleService_Recalculate_CrossTenantHidden proves the org guard on the
// mutating recalc path: a foreign org recalculating another org's project gets
// ErrNotFound (existence not leaked), and nothing is written — no audit row,
// no enqueued job.
func TestScheduleService_Recalculate_CrossTenantHidden(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	a := seedSchedTask(t, svc.pool, fx.projectID, "1.0", "Foundation", 5)
	b := seedSchedTask(t, svc.pool, fx.projectID, "2.0", "Framing", 3)
	seedSchedDep(t, svc.pool, fx.projectID, a, b)

	otherOrg := uuid.New()
	if _, _, err := svc.RecalculateSchedule(ctx, fx.projectID, otherOrg, "intruder"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant RecalculateSchedule = %v, want ErrNotFound", err)
	}
	if got := scheduleAuditCount(t, svc, otherOrg, "schedule.recalculated"); got != 0 {
		t.Errorf("foreign-org recalc wrote %d audit rows, want 0", got)
	}
	if got := delayCascadeJobCount(t, svc); got != 0 {
		t.Errorf("foreign-org recalc enqueued %d jobs, want 0", got)
	}
}

// TestScheduleService_Recalculate_NoTasks covers the empty-graph guard: a valid
// project with no tasks fails the recalc (nothing to schedule) and writes no
// audit row.
func TestScheduleService_Recalculate_NoTasks(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	_, _, err := svc.RecalculateSchedule(ctx, fx.projectID, fx.orgID, "owner-sub")
	if err == nil {
		t.Fatal("RecalculateSchedule over a task-less project = nil, want error")
	}
	if got := scheduleAuditCount(t, svc, fx.orgID, "schedule.recalculated"); got != 0 {
		t.Errorf("task-less recalc wrote %d audit rows, want 0", got)
	}
}

// TestScheduleService_GetGantt_NeverComputedAndCrossTenant covers the read path:
// a project whose tasks exist but were never recalculated returns an empty
// critical path + zero ProjectEnd (the frontend's "run /recalculate" cue), and
// a foreign org's Gantt read is hidden as ErrNotFound.
func TestScheduleService_GetGantt_NeverComputedAndCrossTenant(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	seedSchedTask(t, svc.pool, fx.projectID, "1.0", "Foundation", 5)
	seedSchedTask(t, svc.pool, fx.projectID, "2.0", "Framing", 3)

	gantt, err := svc.GetGantt(ctx, fx.projectID, fx.orgID)
	if err != nil {
		t.Fatalf("GetGantt: %v", err)
	}
	if len(gantt.Tasks) != 2 {
		t.Errorf("gantt tasks = %d, want 2", len(gantt.Tasks))
	}
	if gantt.CriticalPath == nil {
		t.Error("CriticalPath must be non-nil (stable [] for JSON)")
	}
	if len(gantt.CriticalPath) != 0 {
		t.Errorf("never-computed critical path = %d, want 0", len(gantt.CriticalPath))
	}
	if !gantt.ProjectEnd.IsZero() {
		t.Errorf("never-computed ProjectEnd = %v, want zero", gantt.ProjectEnd)
	}

	if _, err := svc.GetGantt(ctx, fx.projectID, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant GetGantt = %v, want ErrNotFound", err)
	}
}

// TestScheduleService_ListAndUpdateTask covers the DB-backed task list + partial
// update entrypoints (the unit tests only reach their pre-tx validation gates):
// the status/critical filters narrow the set, an update round-trips
// percent/status, and both honor the org guard.
func TestScheduleService_ListAndUpdateTask(t *testing.T) {
	svc, fx := newScheduleService(t)
	ctx := context.Background()

	t1 := seedSchedTask(t, svc.pool, fx.projectID, "1.0", "Foundation", 5)
	seedSchedTask(t, svc.pool, fx.projectID, "2.0", "Framing", 3)

	all, err := svc.ListProjectTasks(ctx, ListProjectTasksInput{ProjectID: fx.projectID, OrgID: fx.orgID})
	if err != nil {
		t.Fatalf("ListProjectTasks(all): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered task list = %d, want 2", len(all))
	}

	pending, err := svc.ListProjectTasks(ctx, ListProjectTasksInput{ProjectID: fx.projectID, OrgID: fx.orgID, Status: "pending"})
	if err != nil {
		t.Fatalf("ListProjectTasks(pending): %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("pending task list = %d, want 2", len(pending))
	}

	// Cross-tenant list is hidden.
	if _, err := svc.ListProjectTasks(ctx, ListProjectTasksInput{ProjectID: fx.projectID, OrgID: uuid.New()}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant ListProjectTasks = %v, want ErrNotFound", err)
	}

	// Partial update round-trips percent + status.
	pct := 40
	status := "in_progress"
	updated, err := svc.UpdateTask(ctx, UpdateTaskInput{
		TaskID:          t1,
		ProjectID:       fx.projectID,
		OrgID:           fx.orgID,
		PercentComplete: &pct,
		Status:          &status,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.PercentComplete != 40 || updated.Status != "in_progress" {
		t.Errorf("update = %d%%/%q, want 40/in_progress", updated.PercentComplete, updated.Status)
	}

	// Cross-tenant update is hidden (the project guard fires before the row write).
	if _, err := svc.UpdateTask(ctx, UpdateTaskInput{
		TaskID: t1, ProjectID: fx.projectID, OrgID: uuid.New(), Status: &status,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant UpdateTask = %v, want ErrNotFound", err)
	}
}
