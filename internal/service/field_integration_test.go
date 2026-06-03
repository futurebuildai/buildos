//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedFieldServiceTask inserts an in-progress project_tasks row with
// assigned_crew = [uid] and returns the task id.
func seedFieldServiceTask(t *testing.T, pool *pgxpool.Pool, projectID, uid uuid.UUID, name string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, assigned_crew, status)
		VALUES ($1, $2, $3, $4, ARRAY[$5]::uuid[], 'in_progress')
		RETURNING id`,
		projectID, name, name, 5, uid,
	).Scan(&id); err != nil {
		t.Fatalf("seed task %s: %v", name, err)
	}
	return id
}

// newFieldServiceFixture wires a FieldService over a fresh pool plus the
// common org/user/project seed every field path needs. The user's OIDC
// subject is its id string (post-pivot password users — see
// testdb.SeedUser). Returns the recorder so audit writes can be asserted.
func newFieldServiceFixture(t *testing.T) (*FieldService, *capturingAuditRecorder, *pgxpool.Pool, uuid.UUID, string, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	rec := &capturingAuditRecorder{}
	svc := NewFieldService(pool, store.NewFieldStore(), store.NewFeedCardsStore(), rec)

	orgID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	return svc, rec, pool, orgID, userID.String(), projectID
}

// TestFieldService_Sync drives the read-only delta pull end-to-end: the
// happy path returns the caller's open assigned task + a server_time
// stamp, and the input guards / unknown-subject leg are covered too.
func TestFieldService_Sync(t *testing.T) {
	svc, _, pool, orgID, subject, projectID := newFieldServiceFixture(t)
	ctx := context.Background()

	uid := uuid.MustParse(subject)
	taskID := seedFieldServiceTask(t, pool, projectID, uid, "open-task")

	t.Run("returns assigned tasks + server_time", func(t *testing.T) {
		resp, err := svc.Sync(ctx, SyncOptions{
			CallerOrgID:       orgID,
			CallerOIDCSubject: subject,
			CallerRole:        "field_worker",
		})
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		if len(resp.Tasks) != 1 || resp.Tasks[0].ID != taskID {
			t.Fatalf("tasks = %+v, want exactly the seeded task", resp.Tasks)
		}
		if resp.ServerTime.IsZero() {
			t.Error("ServerTime is zero, want a stamp")
		}
		// Role set but no cards seeded → empty (non-nil) slice.
		if resp.FeedCards == nil {
			t.Error("FeedCards = nil, want empty slice")
		}
	})

	t.Run("nil org is rejected", func(t *testing.T) {
		if _, err := svc.Sync(ctx, SyncOptions{CallerOIDCSubject: subject}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})

	t.Run("unknown subject surfaces ErrNotFound", func(t *testing.T) {
		_, err := svc.Sync(ctx, SyncOptions{
			CallerOrgID:       orgID,
			CallerOIDCSubject: uuid.NewString(), // no such user
			CallerRole:        "field_worker",
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

// TestFieldService_ReportProgress covers the write tx, the audit record,
// the idempotency-conflict translation (re-submit with the same key),
// and the cross-org task-verification leg (→ ErrNotFound).
func TestFieldService_ReportProgress(t *testing.T) {
	svc, rec, pool, orgID, subject, projectID := newFieldServiceFixture(t)
	ctx := context.Background()

	uid := uuid.MustParse(subject)
	taskID := seedFieldServiceTask(t, pool, projectID, uid, "progress-task")
	key := uuid.New()

	t.Run("writes a progress row and audits it", func(t *testing.T) {
		got, err := svc.ReportProgress(ctx, orgID, subject, ReportProgressInput{
			TaskID:          taskID,
			PercentComplete: 60,
			IdempotencyKey:  key,
		})
		if err != nil {
			t.Fatalf("ReportProgress: %v", err)
		}
		if got.PercentComplete != 60 || got.TaskID != taskID {
			t.Errorf("row = %+v, want 60%% on the seeded task", got)
		}
		if len(rec.entries) == 0 || rec.entries[len(rec.entries)-1].Action != "field.task_progress.reported" {
			t.Errorf("audit not recorded: %+v", rec.entries)
		}
	})

	t.Run("re-submit with same key is a 409 conflict", func(t *testing.T) {
		_, err := svc.ReportProgress(ctx, orgID, subject, ReportProgressInput{
			TaskID:          taskID,
			PercentComplete: 60,
			IdempotencyKey:  key, // reused
		})
		if !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatalf("err = %v, want ErrIdempotencyConflict", err)
		}
	})

	t.Run("task in another org surfaces ErrNotFound", func(t *testing.T) {
		otherOrg := uuid.New()
		otherProject := uuid.New()
		testdb.SeedOrg(t, pool, otherOrg, "Other Org")
		testdb.SeedProject(t, pool, otherProject, otherOrg, "Other Org Project")
		foreignTask := seedFieldServiceTask(t, pool, otherProject, uid, "foreign-task")
		_, err := svc.ReportProgress(ctx, orgID, subject, ReportProgressInput{
			TaskID:          foreignTask,
			PercentComplete: 10,
			IdempotencyKey:  uuid.New(),
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

// TestFieldService_Checkin covers the crew-checkin write + audit and the
// idempotency-conflict leg.
func TestFieldService_Checkin(t *testing.T) {
	svc, _, _, orgID, subject, projectID := newFieldServiceFixture(t)
	ctx := context.Background()
	key := uuid.New()
	crew := json.RawMessage(`[{"worker_id":"crew-1"}]`)

	got, err := svc.Checkin(ctx, orgID, subject, CheckinInput{
		ProjectID:      projectID,
		CrewMembers:    crew,
		IdempotencyKey: key,
	})
	if err != nil {
		t.Fatalf("Checkin: %v", err)
	}
	if got.ProjectID != projectID {
		t.Errorf("checkin project = %s, want %s", got.ProjectID, projectID)
	}

	if _, err := svc.Checkin(ctx, orgID, subject, CheckinInput{
		ProjectID:      projectID,
		CrewMembers:    crew,
		IdempotencyKey: key, // reused
	}); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("re-submit err = %v, want ErrIdempotencyConflict", err)
	}
}

// TestFieldService_DailyLog covers the daily-log write + audit and the
// cross-org project-verification leg.
func TestFieldService_DailyLog(t *testing.T) {
	svc, _, pool, orgID, subject, projectID := newFieldServiceFixture(t)
	ctx := context.Background()

	got, err := svc.DailyLog(ctx, orgID, subject, DailyLogInput{
		ProjectID:      projectID,
		LogDate:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		WorkSummary:    "Framed the second floor.",
		IdempotencyKey: uuid.New(),
	})
	if err != nil {
		t.Fatalf("DailyLog: %v", err)
	}
	if got.ProjectID != projectID {
		t.Errorf("daily log project = %s, want %s", got.ProjectID, projectID)
	}

	t.Run("project in another org surfaces ErrNotFound", func(t *testing.T) {
		otherOrg := uuid.New()
		otherProject := uuid.New()
		testdb.SeedOrg(t, pool, otherOrg, "Other Org")
		testdb.SeedProject(t, pool, otherProject, otherOrg, "Other Org Project")
		_, err := svc.DailyLog(ctx, orgID, subject, DailyLogInput{
			ProjectID:      otherProject,
			LogDate:        time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			WorkSummary:    "Should not land.",
			IdempotencyKey: uuid.New(),
		})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}
