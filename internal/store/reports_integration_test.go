//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

func TestFieldStore_DailyReport_DerivedReads(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFieldStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projA := uuid.New()
	projB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projA, orgA, "Maple Duplex")
	testdb.SeedProject(t, pool, projB, orgB, "Cross Org")
	testdb.SeedUser(t, pool, userA, orgA)
	testdb.SeedUser(t, pool, userB, orgB)

	day := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	// Daily log on day for projA (org A).
	if _, err := pool.Exec(ctx, `
		INSERT INTO daily_logs (org_id, project_id, reported_by, log_date, weather_conditions, work_summary, safety_incidents, photo_asset_ids, idempotency_key)
		VALUES ($1,$2,$3,$4::date,$5,$6,$7,$8,$9)`,
		orgA, projA, userA, "2026-06-09", "Sunny", "Framed second floor", "scaffold issue near C", nil, uuid.New()); err != nil {
		t.Fatalf("seed daily_log A: %v", err)
	}
	// Crew check-in with 3 members on day for projA.
	if _, err := pool.Exec(ctx, `
		INSERT INTO crew_checkins (org_id, project_id, reported_by, crew_members, idempotency_key, reported_at)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6)`,
		orgA, projA, userA, `[{"worker_id":"w1"},{"worker_id":"w2"},{"worker_id":"w3"}]`, uuid.New(),
		day.Add(8*time.Hour)); err != nil {
		t.Fatalf("seed crew_checkin A: %v", err)
	}
	// A task + progress on day for projA (task_progress has no org_id → join).
	var taskA uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status)
		VALUES ($1,'2.0','Framing',5,'in_progress') RETURNING id`, projA).Scan(&taskA); err != nil {
		t.Fatalf("seed task A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_progress (task_id, reported_by, percent_complete, reported_via, idempotency_key, reported_at)
		VALUES ($1,$2,$3,'web',$4,$5)`,
		taskA, userA, 60, uuid.New(), day.Add(9*time.Hour)); err != nil {
		t.Fatalf("seed task_progress A: %v", err)
	}

	// Cross-org noise: a task + progress for projB (org B) on the same day. Must
	// NOT appear in org A's reads.
	var taskB uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status)
		VALUES ($1,'9.9','Other',5,'in_progress') RETURNING id`, projB).Scan(&taskB); err != nil {
		t.Fatalf("seed task B: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_progress (task_id, reported_by, percent_complete, reported_via, idempotency_key, reported_at)
		VALUES ($1,$2,$3,'web',$4,$5)`,
		taskB, userB, 99, uuid.New(), day.Add(9*time.Hour)); err != nil {
		t.Fatalf("seed task_progress B: %v", err)
	}

	t.Run("dates list returns the active day for the project", func(t *testing.T) {
		_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			dates, err := s.ListDailyReportDates(ctx, tx, orgA, projA, 0)
			if err != nil {
				t.Fatalf("ListDailyReportDates: %v", err)
			}
			if len(dates) != 1 {
				t.Fatalf("dates = %v, want exactly 2026-06-09", dates)
			}
			if dates[0].Format("2006-01-02") != "2026-06-09" {
				t.Errorf("date = %s", dates[0].Format("2006-01-02"))
			}
			return nil
		})
	})

	t.Run("daily log fields are org-scoped", func(t *testing.T) {
		_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			f, ok, err := s.DailyLogFieldsByProjectDate(ctx, tx, orgA, projA, day)
			if err != nil {
				t.Fatalf("DailyLogFields: %v", err)
			}
			if !ok {
				t.Fatal("want a daily log for org A")
			}
			if f.WorkSummary != "Framed second floor" || f.SafetyIncidents != "scaffold issue near C" {
				t.Errorf("fields = %+v", f)
			}
			// Cross-org: org B sees no log on projA.
			_, okB, err := s.DailyLogFieldsByProjectDate(ctx, tx, orgB, projA, day)
			if err != nil {
				t.Fatalf("cross-org DailyLogFields: %v", err)
			}
			if okB {
				t.Error("org B must not see org A's daily log")
			}
			return nil
		})
	})

	t.Run("crew count sums JSONB array lengths", func(t *testing.T) {
		_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			n, err := s.CrewCountByProjectDate(ctx, tx, orgA, projA, day)
			if err != nil {
				t.Fatalf("CrewCount: %v", err)
			}
			if n != 3 {
				t.Errorf("crew count = %d, want 3", n)
			}
			return nil
		})
	})

	t.Run("task progress joins through project_tasks with org isolation", func(t *testing.T) {
		_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			lines, err := s.TaskProgressByProjectDate(ctx, tx, orgA, projA, day)
			if err != nil {
				t.Fatalf("TaskProgress: %v", err)
			}
			if len(lines) != 1 || lines[0].WBSCode != "2.0" || lines[0].PercentComplete != 60 {
				t.Fatalf("lines = %+v, want one 2.0 @ 60%%", lines)
			}
			// Cross-org: org B's progress for projB must not surface under org A/projA.
			cross, err := s.TaskProgressByProjectDate(ctx, tx, orgA, projB, day)
			if err != nil {
				t.Fatalf("cross-org TaskProgress: %v", err)
			}
			if len(cross) != 0 {
				t.Errorf("org A querying projB progress = %d rows, want 0 (cross-org leak)", len(cross))
			}
			return nil
		})
	})
}
