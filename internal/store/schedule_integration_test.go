//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/testdb"
)

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
