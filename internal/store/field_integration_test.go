//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedFieldTask inserts a project_tasks row with assigned_crew = [uid] and
// returns the task id.
func seedFieldTask(t *testing.T, ctx context.Context, pool poolExec, projectID, uid uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, assigned_crew, status)
		VALUES ($1, $2, $3, $4, ARRAY[$5]::uuid[], 'in_progress')
		RETURNING id`,
		projectID, name, name, 5, uid,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed task %s: %v", name, err)
	}
	return id
}

// poolExec is the minimal interface seed helpers need from a pool.
type poolExec interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func TestFieldStore_LookupUserIDBySubject(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	userA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedUser(t, pool, userA, orgA)

	t.Run("matching subject + org returns id", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			got, err := s.LookupUserIDBySubject(ctx, tx, userA.String(), orgA)
			if err != nil {
				return err
			}
			if got != userA {
				t.Errorf("got %s, want %s", got, userA)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("subject in different org returns ErrNotFound", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			_, err := s.LookupUserIDBySubject(ctx, tx, userA.String(), orgB)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestFieldStore_ListAssignedTasks(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")
	testdb.SeedUser(t, pool, userA, orgID)
	testdb.SeedUser(t, pool, userB, orgID)

	taskA := seedFieldTask(t, ctx, pool, projectID, userA, "task-A")
	taskOther := seedFieldTask(t, ctx, pool, projectID, userB, "task-B")

	// Mark one user-A task completed — it must NOT appear in the result.
	_, err := pool.Exec(ctx, `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, assigned_crew, status)
		VALUES ($1, 'task-A-done', 'task-A-done', 1, ARRAY[$2]::uuid[], 'completed')`,
		projectID, userA)
	if err != nil {
		t.Fatalf("seed completed: %v", err)
	}

	t.Run("returns only user A's open tasks", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAssignedTasks(ctx, tx, ListAssignedTasksParams{
				UserID: userA, OrgID: orgID,
			})
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("got %d rows, want 1", len(rows))
			}
			if rows[0].ID != taskA {
				t.Errorf("got task id %s, want %s", rows[0].ID, taskA)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("Since filter excludes pre-existing tasks", func(t *testing.T) {
		future := time.Now().UTC().Add(1 * time.Hour)
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAssignedTasks(ctx, tx, ListAssignedTasksParams{
				UserID: userA, OrgID: orgID, Since: future,
			})
			if err != nil {
				return err
			}
			if len(rows) != 0 {
				t.Errorf("got %d rows, want 0 (future Since)", len(rows))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("user B does not see user A's tasks", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAssignedTasks(ctx, tx, ListAssignedTasksParams{
				UserID: userB, OrgID: orgID,
			})
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0].ID != taskOther {
				t.Errorf("user B got %d rows, want exactly task-B", len(rows))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestFieldStore_ReportProgress_Idempotency(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "P")
	testdb.SeedUser(t, pool, userID, orgID)
	taskID := seedFieldTask(t, ctx, pool, projectID, userID, "task-1")

	key := uuid.New()

	// First insert.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.ReportProgress(ctx, tx, ReportProgressParams{
			TaskID: taskID, ReportedBy: userID, PercentComplete: 50,
			ReportedVia: "mobile", IdempotencyKey: key,
		})
		return err
	})
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}

	// Replay with same key — manual Begin/Rollback because the failed
	// INSERT poisons the tx for any subsequent COMMIT.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = s.ReportProgress(ctx, tx, ReportProgressParams{
		TaskID: taskID, ReportedBy: userID, PercentComplete: 75,
		ReportedVia: "mobile", IdempotencyKey: key,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Errorf("err = %v, want ErrIdempotencyConflict", err)
	}
}

func TestFieldStore_Checkin_Idempotency(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "P")
	testdb.SeedUser(t, pool, userID, orgID)

	key := uuid.New()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.Checkin(ctx, tx, CheckinParams{
			OrgID: orgID, ProjectID: projectID, ReportedBy: userID,
			CrewMembers:    json.RawMessage(`[{"worker_id":"abc"}]`),
			IdempotencyKey: key,
		})
		return err
	})
	if err != nil {
		t.Fatalf("first checkin: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = s.Checkin(ctx, tx, CheckinParams{
		OrgID: orgID, ProjectID: projectID, ReportedBy: userID,
		CrewMembers: json.RawMessage(`[]`), IdempotencyKey: key,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Errorf("err = %v, want ErrIdempotencyConflict", err)
	}
}

func TestFieldStore_DailyLog_Idempotency(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedProject(t, pool, projectID, orgID, "P")
	testdb.SeedUser(t, pool, userID, orgID)

	key := uuid.New()
	logDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.DailyLog(ctx, tx, DailyLogParams{
			OrgID: orgID, ProjectID: projectID, ReportedBy: userID,
			LogDate: logDate, WorkSummary: "ran framing",
			IdempotencyKey: key,
		})
		return err
	})
	if err != nil {
		t.Fatalf("first daily log: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = s.DailyLog(ctx, tx, DailyLogParams{
		OrgID: orgID, ProjectID: projectID, ReportedBy: userID,
		LogDate: logDate, WorkSummary: "different summary",
		IdempotencyKey: key,
	})
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Errorf("err = %v, want ErrIdempotencyConflict", err)
	}
}

func TestFieldStore_VerifyTaskInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projectA := uuid.New()
	userA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")
	testdb.SeedProject(t, pool, projectA, orgA, "P")
	testdb.SeedUser(t, pool, userA, orgA)
	taskA := seedFieldTask(t, ctx, pool, projectA, userA, "t")

	t.Run("task in org returns nil", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			return s.VerifyTaskInOrg(ctx, tx, taskA, orgA)
		})
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("cross-org returns ErrNotFound", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			err := s.VerifyTaskInOrg(ctx, tx, taskA, orgB)
			if !errors.Is(err, ErrNotFound) {
				t.Errorf("err = %v, want ErrNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
