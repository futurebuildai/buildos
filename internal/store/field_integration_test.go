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

// seedFleetAsset inserts a fleet_assets row and returns its id.
func seedFleetAsset(t *testing.T, ctx context.Context, pool poolExec, orgID uuid.UUID, name, assetType string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO fleet_assets (org_id, name, asset_type, serial_number, status)
		VALUES ($1, $2, $3, $4, 'available') RETURNING id`,
		orgID, name, assetType, name+"-SN",
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed fleet asset %s: %v", name, err)
	}
	return id
}

// seedAllocation inserts an equipment_allocations row for [start, end).
func seedAllocation(t *testing.T, ctx context.Context, pool poolExec, assetID, projectID uuid.UUID, start, end time.Time) {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO equipment_allocations (asset_id, project_id, start_date, end_date)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		assetID, projectID, start, end,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed allocation: %v", err)
	}
}

func TestFieldStore_ListAllocatedEquipment(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	today := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	active := func() (time.Time, time.Time) {
		return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	}

	orgA := uuid.New()
	orgB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedUser(t, pool, userA, orgA)
	testdb.SeedUser(t, pool, userB, orgA)

	projMine := uuid.New()      // A: userA has an open task
	projNotMine := uuid.New()   // A: only userB has a task
	projCompleted := uuid.New() // A: userA's only task is completed
	projB := uuid.New()         // B: cross-org
	testdb.SeedProject(t, pool, projMine, orgA, "Mine")
	testdb.SeedProject(t, pool, projNotMine, orgA, "Not Mine")
	testdb.SeedProject(t, pool, projCompleted, orgA, "Completed")
	testdb.SeedProject(t, pool, projB, orgB, "Cross Org")

	seedFieldTask(t, ctx, pool, projMine, userA, "mine-open")
	seedFieldTask(t, ctx, pool, projNotMine, userB, "notmine-open")
	// userA assigned to projCompleted but the task is completed → not an active site.
	if _, err := pool.Exec(ctx, `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, assigned_crew, status)
		VALUES ($1, 'done', 'done', 1, ARRAY[$2]::uuid[], 'completed')`, projCompleted, userA); err != nil {
		t.Fatalf("seed completed task: %v", err)
	}
	// A STALE cross-org assignment: userA's id sits in a task's assigned_crew in
	// Org B. The org filter MUST keep Org B's equipment out of Org A's result.
	seedFieldTask(t, ctx, pool, projB, userA, "crossorg-open")

	start, end := active()
	assetMine := seedFleetAsset(t, ctx, pool, orgA, "Excavator", "excavator")
	seedAllocation(t, ctx, pool, assetMine, projMine, start, end) // SHOWN

	assetNotMine := seedFleetAsset(t, ctx, pool, orgA, "Grader", "grader")
	seedAllocation(t, ctx, pool, assetNotMine, projNotMine, start, end) // not my project

	assetCompleted := seedFleetAsset(t, ctx, pool, orgA, "Crane", "crane")
	seedAllocation(t, ctx, pool, assetCompleted, projCompleted, start, end) // my task completed

	assetPast := seedFleetAsset(t, ctx, pool, orgA, "Compactor", "compactor")
	seedAllocation(t, ctx, pool, assetPast, projMine,
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)) // ended (end exclusive, today=15)

	assetFuture := seedFleetAsset(t, ctx, pool, orgA, "Loader", "loader")
	seedAllocation(t, ctx, pool, assetFuture, projMine,
		time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)) // not started

	assetOrgB := seedFleetAsset(t, ctx, pool, orgB, "B-Excavator", "excavator")
	seedAllocation(t, ctx, pool, assetOrgB, projB, start, end) // other org

	list := func(uid uuid.UUID) []string {
		var names []string
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAllocatedEquipment(ctx, tx, ListAllocatedEquipmentParams{
				UserID: uid, OrgID: orgA, Today: today,
			})
			if err != nil {
				return err
			}
			for _, e := range rows {
				names = append(names, e.Name)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("list equipment: %v", err)
		}
		return names
	}

	t.Run("userA sees only the active asset on their own project", func(t *testing.T) {
		got := list(userA)
		if len(got) != 1 || got[0] != "Excavator" {
			t.Errorf("userA equipment = %v, want [Excavator] (excludes not-mine, completed-site, past, future, cross-org)", got)
		}
	})

	t.Run("userB sees only the asset on their own project", func(t *testing.T) {
		got := list(userB)
		if len(got) != 1 || got[0] != "Grader" {
			t.Errorf("userB equipment = %v, want [Grader]", got)
		}
	})

	t.Run("field-safe projection carries the allocation window, not org_id", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListAllocatedEquipment(ctx, tx, ListAllocatedEquipmentParams{UserID: userA, OrgID: orgA, Today: today})
			if err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("want 1 row")
			}
			e := rows[0]
			if e.ID != assetMine {
				t.Errorf("id = %s, want %s", e.ID, assetMine)
			}
			if e.AssetType != "excavator" || e.Status != "available" {
				t.Errorf("unexpected type/status: %s/%s", e.AssetType, e.Status)
			}
			if !e.StartDate.Equal(start) || !e.EndDate.Equal(end) {
				t.Errorf("window = [%s,%s), want [%s,%s)", e.StartDate, e.EndDate, start, end)
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

func TestFieldStore_Checkin_NilCrewNormalizesToEmptyArray(t *testing.T) {
	// CrewMembers nil (len 0) must normalize to a JSONB `[]` — the
	// idempotency test always passes a non-empty array (or an explicit
	// `[]`, len 2), so the len(crew)==0 branch is otherwise unreached.
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Crewless Co")
	testdb.SeedProject(t, pool, projectID, orgID, "P")
	testdb.SeedUser(t, pool, userID, orgID)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, err := s.Checkin(ctx, tx, CheckinParams{
			OrgID: orgID, ProjectID: projectID, ReportedBy: userID,
			CrewMembers:    nil, // exercises the len()==0 normalization
			IdempotencyKey: uuid.New(),
		})
		if err != nil {
			return err
		}
		var arr []any
		if err := json.Unmarshal(c.CrewMembers, &arr); err != nil {
			t.Errorf("crew_members unmarshal: %v", err)
		}
		if len(arr) != 0 {
			t.Errorf("nil CrewMembers should normalize to [], got %s", string(c.CrewMembers))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("checkin: %v", err)
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
