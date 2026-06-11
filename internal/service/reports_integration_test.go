//go:build integration

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestReportsService_GetDailyReport_Derived seeds daily_logs + crew_checkins +
// task_progress and asserts GetProjectReport returns the joined, org-scoped
// derived report. A cross-org read 404s. Photo resolution is exercised with a
// nil resolver (storage off → count-only, no photo refs).
func TestReportsService_GetDailyReport_Derived(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	projA := uuid.New()
	userA := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")
	testdb.SeedProject(t, pool, projA, orgA, "Maple Duplex")
	testdb.SeedUser(t, pool, userA, orgA)

	day := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	if _, err := pool.Exec(ctx, `
		INSERT INTO daily_logs (org_id, project_id, reported_by, log_date, weather_conditions, work_summary, safety_incidents, photo_asset_ids, idempotency_key)
		VALUES ($1,$2,$3,$4::date,$5,$6,$7,$8,$9)`,
		orgA, projA, userA, "2026-06-09", "Sunny", "Framed second floor", "scaffold issue near C", nil, uuid.New()); err != nil {
		t.Fatalf("seed daily_log: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO crew_checkins (org_id, project_id, reported_by, crew_members, idempotency_key, reported_at)
		VALUES ($1,$2,$3,$4::jsonb,$5,$6)`,
		orgA, projA, userA, `[{"worker_id":"w1"},{"worker_id":"w2"}]`, uuid.New(), day.Add(8*time.Hour)); err != nil {
		t.Fatalf("seed crew_checkin: %v", err)
	}
	var taskA uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status)
		VALUES ($1,'2.0','Framing',5,'in_progress') RETURNING id`, projA).Scan(&taskA); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO task_progress (task_id, reported_by, percent_complete, reported_via, idempotency_key, reported_at)
		VALUES ($1,$2,$3,'web',$4,$5)`,
		taskA, userA, 60, uuid.New(), day.Add(9*time.Hour)); err != nil {
		t.Fatalf("seed task_progress: %v", err)
	}

	// nil PhotoResolver → reports work text-only (count from raw id list).
	svc := NewReportsService(pool, store.NewFieldStore(), store.NewProjectStore(), nil, nil, nil, nil)

	t.Run("joined derived report", func(t *testing.T) {
		r, err := svc.GetProjectReport(ctx, orgA, projA, day)
		if err != nil {
			t.Fatalf("GetProjectReport: %v", err)
		}
		if r.ProjectName != "Maple Duplex" {
			t.Errorf("project name = %q", r.ProjectName)
		}
		if r.WorkSummary != "Framed second floor" {
			t.Errorf("work summary = %q", r.WorkSummary)
		}
		// Internal operator surface: safety incident IS present.
		if r.SafetyIncidents != "scaffold issue near C" {
			t.Errorf("safety incident = %q", r.SafetyIncidents)
		}
		if r.CrewCount != 2 {
			t.Errorf("crew count = %d, want 2", r.CrewCount)
		}
		if len(r.TaskProgress) != 1 || r.TaskProgress[0].WBSCode != "2.0" {
			t.Errorf("task progress = %+v", r.TaskProgress)
		}
	})

	t.Run("cross-org 404", func(t *testing.T) {
		_, err := svc.GetProjectReport(ctx, orgB, projA, day)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-org GetProjectReport = %v, want ErrNotFound", err)
		}
	})

	t.Run("list returns the active date", func(t *testing.T) {
		// Pass an explicit window so the assertion doesn't depend on the wall
		// clock (the default window is last-14-days from time.Now()).
		summaries, err := svc.ListProjectReports(ctx, orgA, projA,
			time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("ListProjectReports (windowed): %v", err)
		}
		if len(summaries) != 1 {
			t.Fatalf("summaries = %d, want 1", len(summaries))
		}
		if !summaries[0].HasSafetyIncident {
			t.Error("summary should flag the safety incident")
		}
		if summaries[0].CrewCount != 2 || summaries[0].TaskProgressCount != 1 {
			t.Errorf("summary = %+v", summaries[0])
		}
	})
}
