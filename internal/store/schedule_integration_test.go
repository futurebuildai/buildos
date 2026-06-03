//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedDependency inserts a task_dependencies row (FS, lag 0) wiring a
// predecessor to a successor within a project.
func seedDependency(t *testing.T, pool *pgxpool.Pool, projectID, predecessorID, successorID uuid.UUID) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO task_dependencies (project_id, predecessor_id, successor_id, dependency_type, lag_days)
		VALUES ($1, $2, $3, 'FS', 0)`, projectID, predecessorID, successorID)
	if err != nil {
		t.Fatalf("seed dependency: %v", err)
	}
}

// seedTask inserts a project_tasks row with sensible defaults so each
// test can populate just the fields it cares about. Returns the task id.
func seedTask(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbsCode, name, status string, isCritical bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, is_critical, status, percent_complete)
		VALUES ($1, $2, $3, 1, $4, $5, 0)
		RETURNING id`, projectID, wbsCode, name, isCritical, status,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return id
}

func TestScheduleStore_ListProjectTasks_WithFilters(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "House")

	// 3 tasks: one critical/pending, one critical/completed, one non-critical/pending.
	seedTask(t, pool, projectID, "1.0", "Foundation", "pending", true)
	seedTask(t, pool, projectID, "2.0", "Framing", "completed", true)
	seedTask(t, pool, projectID, "3.0", "Painting", "pending", false)

	cases := []struct {
		name       string
		status     string
		isCritical *bool
		wantCount  int
	}{
		{"no filters", "", nil, 3},
		{"status=pending", "pending", nil, 2},
		{"is_critical=true", "", boolPtr(true), 2},
		{"is_critical=false", "", boolPtr(false), 1},
		{"both filters", "pending", boolPtr(true), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
				got, err := s.ListProjectTasks(ctx, tx, ListTasksParams{
					ProjectID:  projectID,
					Status:     c.status,
					IsCritical: c.isCritical,
				})
				if err != nil {
					return err
				}
				if len(got) != c.wantCount {
					t.Errorf("got %d tasks, want %d", len(got), c.wantCount)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("list tasks: %v", err)
			}
		})
	}
}

func TestScheduleStore_UpdateTask_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "House")
	taskID := seedTask(t, pool, projectID, "1.0", "Foundation", "pending", true)

	pct := 50
	status := "in_progress"

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.UpdateTask(ctx, tx, UpdateTaskParams{
			TaskID:          taskID,
			ProjectID:       projectID,
			PercentComplete: &pct,
			Status:          &status,
		})
		if err != nil {
			return err
		}
		if got.PercentComplete != 50 {
			t.Errorf("percent_complete = %d, want 50", got.PercentComplete)
		}
		if got.Status != "in_progress" {
			t.Errorf("status = %s, want in_progress", got.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
}

func TestScheduleStore_UpdateTask_NilCrewPreservesExisting(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "House")
	taskID := seedTask(t, pool, projectID, "1.0", "Foundation", "pending", false)

	// Pre-populate assigned_crew with one UUID.
	crewMember := uuid.New()
	_, err := pool.Exec(ctx, `UPDATE project_tasks SET assigned_crew = $1 WHERE id = $2`,
		[]uuid.UUID{crewMember}, taskID)
	if err != nil {
		t.Fatalf("seed crew: %v", err)
	}

	// Update without touching AssignedCrew (nil) — should preserve the seed.
	status := "in_progress"
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.UpdateTask(ctx, tx, UpdateTaskParams{
			TaskID:    taskID,
			ProjectID: projectID,
			Status:    &status,
			// AssignedCrew explicitly nil
		})
		if err != nil {
			return err
		}
		if len(got.AssignedCrew) != 1 || got.AssignedCrew[0] != crewMember {
			t.Errorf("crew should be preserved; got %v", got.AssignedCrew)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestScheduleStore_UpdateTask_EmptyCrewClears(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "House")
	taskID := seedTask(t, pool, projectID, "1.0", "Foundation", "pending", false)

	crewMember := uuid.New()
	_, err := pool.Exec(ctx, `UPDATE project_tasks SET assigned_crew = $1 WHERE id = $2`,
		[]uuid.UUID{crewMember}, taskID)
	if err != nil {
		t.Fatalf("seed crew: %v", err)
	}

	// Pointer to empty slice → clear the column.
	empty := []uuid.UUID{}
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.UpdateTask(ctx, tx, UpdateTaskParams{
			TaskID:       taskID,
			ProjectID:    projectID,
			AssignedCrew: &empty,
		})
		if err != nil {
			return err
		}
		if len(got.AssignedCrew) != 0 {
			t.Errorf("crew should be cleared; got %v", got.AssignedCrew)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
}

func TestScheduleStore_UpdateTask_NotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	missingTask := uuid.New()
	missingProject := uuid.New()
	pct := 50
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.UpdateTask(ctx, tx, UpdateTaskParams{
			TaskID:          missingTask,
			ProjectID:       missingProject,
			PercentComplete: &pct,
		})
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestScheduleStore_GetTaskInProject_ScopedToProject(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectA := uuid.New()
	projectB := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectA, orgID, "A")
	testdb.SeedProject(t, pool, projectB, orgID, "B")
	taskInA := seedTask(t, pool, projectA, "1.0", "Foundation", "pending", true)

	// Same project: ok.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		_, err := s.GetTaskInProject(ctx, tx, taskInA, projectA)
		return err
	})
	if err != nil {
		t.Errorf("same-project get failed: %v", err)
	}

	// Wrong project: ErrNotFound.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		_, err := s.GetTaskInProject(ctx, tx, taskInA, projectB)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-project get returned %v, want ErrNotFound", err)
	}
}

func boolPtr(b bool) *bool { return &b }

// TestScheduleStore_GetProjectTasksAndDependencies covers the two CPM
// graph-load queries: GetProjectTasks (all tasks, WBS-ordered) and
// GetProjectDependencies (the edge set).
func TestScheduleStore_GetProjectTasksAndDependencies(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "House")

	// Insert out of WBS order to prove the ORDER BY.
	framing := seedTask(t, pool, projectID, "2.0", "Framing", "pending", false)
	foundation := seedTask(t, pool, projectID, "1.0", "Foundation", "pending", true)
	seedDependency(t, pool, projectID, foundation, framing)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		tasks, err := s.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(tasks) != 2 {
			t.Fatalf("GetProjectTasks = %d, want 2", len(tasks))
		}
		// WBS-ordered: 1.0 before 2.0.
		if tasks[0].WBSCode != "1.0" || tasks[1].WBSCode != "2.0" {
			t.Errorf("tasks not WBS-ordered: %q then %q", tasks[0].WBSCode, tasks[1].WBSCode)
		}

		deps, err := s.GetProjectDependencies(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if len(deps) != 1 {
			t.Fatalf("GetProjectDependencies = %d, want 1", len(deps))
		}
		if deps[0].PredecessorID != foundation || deps[0].SuccessorID != framing {
			t.Errorf("dependency wiring wrong: pred=%v succ=%v", deps[0].PredecessorID, deps[0].SuccessorID)
		}
		if string(deps[0].DependencyType) != "FS" {
			t.Errorf("dependency_type = %q, want FS", deps[0].DependencyType)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

// TestScheduleStore_GetProjectStartDate covers the COALESCE anchor: a
// pinned project_start_date wins; otherwise it falls back to created_at
// (always set), so the returned time is never zero.
func TestScheduleStore_GetProjectStartDate(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	pinned := uuid.New()
	fallback := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, pinned, orgID, "Pinned")
	testdb.SeedProject(t, pool, fallback, orgID, "Fallback")

	want := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `UPDATE projects SET project_start_date = $2 WHERE id = $1`, pinned, want); err != nil {
		t.Fatalf("pin start date: %v", err)
	}

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, err := s.GetProjectStartDate(ctx, tx, pinned)
		if err != nil {
			return err
		}
		if !got.Equal(want) {
			t.Errorf("pinned start date = %v, want %v", got, want)
		}

		// No project_start_date / permit_issued_date → COALESCE falls to created_at.
		fb, err := s.GetProjectStartDate(ctx, tx, fallback)
		if err != nil {
			return err
		}
		if fb.IsZero() {
			t.Errorf("fallback start date is zero, want created_at")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

// TestScheduleStore_UpdateSchedule_PersistsCPMFields proves the CPM
// write-back: only tasks present in the results map are updated; the
// early/late/float/is_critical fields round-trip, and a task absent from
// the map is left untouched.
func TestScheduleStore_UpdateSchedule_PersistsCPMFields(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewScheduleStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "House")

	computed := seedTask(t, pool, projectID, "1.0", "Foundation", "pending", false)
	untouched := seedTask(t, pool, projectID, "2.0", "Framing", "pending", false)

	es := time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC)
	ef := es.Add(48 * time.Hour)
	results := map[uuid.UUID]ScheduleResult{
		computed: {
			EarlyStart: es, EarlyFinish: ef,
			LateStart: es, LateFinish: ef,
			TotalFloat: 0, IsCritical: true,
		},
		// `untouched` deliberately absent → skipped by the `ok` guard.
	}

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		tasks := []models.ProjectTask{{ID: computed}, {ID: untouched}}
		return s.UpdateSchedule(ctx, tx, tasks, results)
	})
	if err != nil {
		t.Fatalf("update schedule: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, err := s.GetProjectTasks(ctx, tx, projectID)
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]models.ProjectTask{}
		for _, tk := range got {
			byID[tk.ID] = tk
		}
		c := byID[computed]
		if c.EarlyStart == nil || !c.EarlyStart.Equal(es) {
			t.Errorf("computed early_start = %v, want %v", c.EarlyStart, es)
		}
		if !c.IsCritical {
			t.Errorf("computed is_critical = false, want true")
		}
		if c.TotalFloat == nil || *c.TotalFloat != 0 {
			t.Errorf("computed total_float = %v, want 0", c.TotalFloat)
		}
		// untouched task keeps its NULL schedule fields.
		u := byID[untouched]
		if u.EarlyStart != nil {
			t.Errorf("untouched early_start = %v, want nil (skipped)", u.EarlyStart)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}
