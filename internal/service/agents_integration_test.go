//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestAgentsService_RecordDailyBriefingAudit_WritesEntry exercises the
// standalone audit tx that GenerateDailyBriefing fires after a
// successful AI call. The unit tests in agents_test.go pass a nil pool
// and stop at the AI boundary, so this helper read 0% — here we call it
// directly against a real pool (BeginTxFunc needs one) with a capturing
// recorder, asserting the audit entry's labels + metadata shape.
func TestAgentsService_RecordDailyBriefingAudit_WritesEntry(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := &capturingAuditRecorder{}
	svc := NewAgentsService(pool, nil, nil, nil, nil, &fakeBriefer{}, nil, rec)

	orgID := uuid.New()
	sessionID := uuid.New()
	resp := &ai.DailyBriefingResponse{SessionID: sessionID, Reply: "ok"}

	svc.recordDailyBriefingAudit(context.Background(), orgID, "sub-123", resp, 3, 2)

	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(rec.entries))
	}
	e := rec.entries[0]
	if e.Action != "ai.daily_briefing.invoked" {
		t.Errorf("Action = %q, want ai.daily_briefing.invoked", e.Action)
	}
	if e.ResourceType != AuditResourceAIRun {
		t.Errorf("ResourceType = %q, want %q", e.ResourceType, AuditResourceAIRun)
	}
	if e.ResourceID != sessionID {
		t.Errorf("ResourceID = %s, want session id %s", e.ResourceID, sessionID)
	}
	if e.OrgID != orgID || e.UserSub != "sub-123" {
		t.Errorf("org/sub = %s/%q, want %s/sub-123", e.OrgID, e.UserSub, orgID)
	}

	// Metadata carries the task/alert counts + session id + the fixed
	// "daily_briefing" task tag (the shape audit consumers parse).
	var meta struct {
		SessionID  uuid.UUID `json:"session_id"`
		TaskCount  int       `json:"task_count"`
		AlertCount int       `json:"alert_count"`
		Task       string    `json:"task"`
	}
	if err := json.Unmarshal(e.Metadata, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta.SessionID != sessionID || meta.TaskCount != 3 || meta.AlertCount != 2 || meta.Task != "daily_briefing" {
		t.Errorf("metadata = %+v, want {%s 3 2 daily_briefing}", meta, sessionID)
	}
}

// newAgentsServiceFixture wires an AgentsService over a fresh pool with
// the field + feed stores and a capturing audit recorder, plus a seeded
// org/user/project. The briefer is injected by the caller so each test
// can script its own response/error. Returns the seeded ids the daily
// briefing assembles its structured context from.
func newAgentsServiceFixture(t *testing.T, briefer DailyBriefer) (*AgentsService, *capturingAuditRecorder, *pgxpool.Pool, uuid.UUID, string, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	rec := &capturingAuditRecorder{}
	svc := NewAgentsService(pool, store.NewFieldStore(), store.NewFeedCardsStore(), nil, nil, briefer, nil, rec)

	orgID := uuid.New()
	userID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	return svc, rec, pool, orgID, userID.String(), projectID
}

// TestAgentsService_GenerateDailyBriefing drives the read-only
// context-load tx end-to-end: the happy path assembles the caller's
// assigned tasks + critical/urgent feed cards into the AI request and
// returns the scripted reply (asserting the request envelope, the
// returned counts, and the audit write), then the unknown-subject,
// nil-briefer, and AI-error legs.
func TestAgentsService_GenerateDailyBriefing(t *testing.T) {
	sessionID := uuid.New()
	briefer := &fakeBriefer{resp: &ai.DailyBriefingResponse{SessionID: sessionID, Reply: "Here is your day."}}
	svc, rec, pool, orgID, subject, projectID := newAgentsServiceFixture(t, briefer)
	ctx := context.Background()

	uid := uuid.MustParse(subject)
	seedFieldServiceTask(t, pool, projectID, uid, "framing")
	seedFeedCard(t, pool, store.CreateFeedCardParams{
		OrgID: orgID, CardType: "alert", Title: "permit overdue", Body: "b",
		Priority: "urgent", TargetUserID: &uid, Actions: json.RawMessage(`null`),
	})

	t.Run("assembles context, calls the briefer, audits", func(t *testing.T) {
		got, err := svc.GenerateDailyBriefing(ctx, orgID, subject, "field_worker")
		if err != nil {
			t.Fatalf("GenerateDailyBriefing: %v", err)
		}
		if got.Reply != "Here is your day." || got.SessionID != sessionID {
			t.Errorf("response = %+v, want scripted reply + session %s", got, sessionID)
		}
		if got.TaskCount != 1 || got.AlertCount != 1 {
			t.Errorf("counts = task %d / alert %d, want 1 / 1", got.TaskCount, got.AlertCount)
		}
		// The briefer saw the assembled envelope: one task ("framing
		// framing" — wbs+name), one "[urgent] ..." alert, the role.
		if len(briefer.lastReq.Tasks) != 1 || len(briefer.lastReq.Alerts) != 1 {
			t.Fatalf("req = %d tasks / %d alerts, want 1 / 1", len(briefer.lastReq.Tasks), len(briefer.lastReq.Alerts))
		}
		if briefer.lastReq.Alerts[0] != "[urgent] permit overdue" {
			t.Errorf("alert = %q, want [urgent] permit overdue", briefer.lastReq.Alerts[0])
		}
		if briefer.lastReq.UserRole != "field_worker" {
			t.Errorf("role = %q, want field_worker", briefer.lastReq.UserRole)
		}
		if len(rec.entries) == 0 || rec.entries[len(rec.entries)-1].Action != "ai.daily_briefing.invoked" {
			t.Errorf("audit not recorded: %+v", rec.entries)
		}
	})

	t.Run("unknown subject surfaces ErrNotFound", func(t *testing.T) {
		_, err := svc.GenerateDailyBriefing(ctx, orgID, uuid.NewString(), "field_worker")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

// TestAgentsService_GenerateDailyBriefing_NilBriefer confirms the
// load-context tx still runs (a real pool is required) but the flow
// returns ErrAgentsAIUnavailable before any AI dispatch when no briefer
// was wired — the worker-binary construction path.
func TestAgentsService_GenerateDailyBriefing_NilBriefer(t *testing.T) {
	svc, _, _, orgID, subject, _ := newAgentsServiceFixture(t, nil)
	_, err := svc.GenerateDailyBriefing(context.Background(), orgID, subject, "field_worker")
	if !errors.Is(err, ErrAgentsAIUnavailable) {
		t.Fatalf("err = %v, want ErrAgentsAIUnavailable", err)
	}
}

// TestAgentsService_GenerateDailyBriefing_AIError confirms a briefer
// error is wrapped and propagated (no audit row written, since the
// audit fires only after a successful AI call).
func TestAgentsService_GenerateDailyBriefing_AIError(t *testing.T) {
	briefer := &fakeBriefer{err: errors.New("anthropic down")}
	svc, rec, _, orgID, subject, _ := newAgentsServiceFixture(t, briefer)

	_, err := svc.GenerateDailyBriefing(context.Background(), orgID, subject, "field_worker")
	if err == nil || !strings.Contains(err.Error(), "anthropic down") {
		t.Fatalf("err = %v, want wrapped briefer error", err)
	}
	if len(rec.entries) != 0 {
		t.Errorf("audit entries = %d, want 0 (no audit on AI failure)", len(rec.entries))
	}
}

// taskDuration reads a single task's stored duration_days — used to
// prove RecommendScheduleAdjustments persisted the applied delta.
func taskDuration(t *testing.T, pool *pgxpool.Pool, taskID uuid.UUID) int {
	t.Helper()
	var d int
	if err := pool.QueryRow(context.Background(),
		`SELECT duration_days FROM project_tasks WHERE id = $1`, taskID).Scan(&d); err != nil {
		t.Fatalf("read task duration: %v", err)
	}
	return d
}

// TestAgentsService_RecommendScheduleAdjustments drives the full
// AI-tuning flow against a real pool: load the task graph → call the
// (faked) adjuster → apply duration deltas + audit in tx1 → re-run CPM
// in tx2. Reuses the schedule fixture (River tables + an insert-only
// client) so the post-apply RecalculateSchedule can enqueue its
// delay_cascade follow-up.
func TestAgentsService_RecommendScheduleAdjustments(t *testing.T) {
	sched, fx := newScheduleService(t)
	ctx := context.Background()

	a := seedSchedTask(t, sched.pool, fx.projectID, "1.0", "Foundation", 5)
	b := seedSchedTask(t, sched.pool, fx.projectID, "2.0", "Framing", 3)
	seedSchedDep(t, sched.pool, fx.projectID, a, b)

	newDur := 9
	adjuster := &fakeAdjuster{resp: &ai.UpdateScheduleResponse{Adjustments: []ai.ScheduleAdjustment{
		{TaskID: a, NewDurationDays: &newDur, Rationale: "extend foundation cure"},
		{TaskID: b, NewDurationDays: nil, Rationale: "monitor only"}, // rationale-only → skipped
	}}}
	rec := &capturingAuditRecorder{}
	agents := NewAgentsService(sched.pool, nil, nil, store.NewScheduleStore(), sched, nil, adjuster, rec)

	t.Run("applies deltas, audits, and re-runs CPM", func(t *testing.T) {
		got, err := agents.RecommendScheduleAdjustments(ctx, fx.orgID, "owner-sub", fx.projectID)
		if err != nil {
			t.Fatalf("RecommendScheduleAdjustments: %v", err)
		}
		if got.AppliedDeltas != 1 || got.SkippedRationaleOnly != 1 || len(got.Adjustments) != 2 {
			t.Errorf("result = %+v, want applied 1 / skipped 1 / 2 adjustments", got)
		}
		// The adjuster saw the loaded snapshot (both tasks + the dep).
		if len(adjuster.lastReq.Tasks) != 2 || len(adjuster.lastReq.Dependencies) != 1 {
			t.Errorf("req = %d tasks / %d deps, want 2 / 1", len(adjuster.lastReq.Tasks), len(adjuster.lastReq.Dependencies))
		}
		// The applied delta is persisted; the rationale-only task is untouched.
		if d := taskDuration(t, sched.pool, a); d != newDur {
			t.Errorf("task a duration = %d, want %d (delta applied)", d, newDur)
		}
		if d := taskDuration(t, sched.pool, b); d != 3 {
			t.Errorf("task b duration = %d, want 3 (rationale-only, untouched)", d)
		}
		// tx1 wrote the maestro_edit audit; tx2's recalc wrote its own
		// schedule.recalculated row + enqueued a delay_cascade job.
		if len(rec.entries) == 0 || rec.entries[len(rec.entries)-1].Action != "schedule.maestro_edit" {
			t.Errorf("maestro_edit audit not recorded: %+v", rec.entries)
		}
		if got := scheduleAuditCount(t, sched, fx.orgID, "schedule.recalculated"); got != 1 {
			t.Errorf("schedule.recalculated rows = %d, want 1 (recalc ran)", got)
		}
		if got := delayCascadeJobCount(t, sched); got != 1 {
			t.Errorf("delay_cascade jobs = %d, want 1 (recalc enqueued)", got)
		}
	})

	t.Run("cross-org project surfaces ErrNotFound", func(t *testing.T) {
		if _, err := agents.RecommendScheduleAdjustments(ctx, uuid.New(), "intruder", fx.projectID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

// An over-cap model duration (> the duration CHECK from migration 019) must be
// SKIPPED, not violate the constraint and roll back the whole batch — the valid
// adjustment in the same response must still apply.
func TestAgentsService_RecommendScheduleAdjustments_SkipsOutOfRangeDuration(t *testing.T) {
	sched, fx := newScheduleService(t)
	ctx := context.Background()

	a := seedSchedTask(t, sched.pool, fx.projectID, "1.0", "Foundation", 5)
	b := seedSchedTask(t, sched.pool, fx.projectID, "2.0", "Framing", 3)

	huge := maxTaskDurationDays + 1 // would violate project_tasks_duration_days_sane
	good := 7
	adjuster := &fakeAdjuster{resp: &ai.UpdateScheduleResponse{Adjustments: []ai.ScheduleAdjustment{
		{TaskID: a, NewDurationDays: &huge, Rationale: "hallucinated huge duration"},
		{TaskID: b, NewDurationDays: &good, Rationale: "ok"},
	}}}
	agents := NewAgentsService(sched.pool, nil, nil, store.NewScheduleStore(), sched, nil, adjuster, &capturingAuditRecorder{})

	got, err := agents.RecommendScheduleAdjustments(ctx, fx.orgID, "owner-sub", fx.projectID)
	if err != nil {
		t.Fatalf("over-cap duration must be skipped, not roll back the batch: %v", err)
	}
	if got.AppliedDeltas != 1 {
		t.Errorf("AppliedDeltas = %d, want 1 (over-cap skipped, valid applied)", got.AppliedDeltas)
	}
	if d := taskDuration(t, sched.pool, a); d != 5 {
		t.Errorf("task a duration = %d, want 5 (over-cap row untouched)", d)
	}
	if d := taskDuration(t, sched.pool, b); d != good {
		t.Errorf("task b duration = %d, want %d (valid delta applied)", d, good)
	}
}

// TestAgentsService_RecommendScheduleAdjustments_NoTasks covers the
// empty-graph guard: a valid project with no tasks fails the flow with
// ErrInvalidInput before any AI dispatch.
func TestAgentsService_RecommendScheduleAdjustments_NoTasks(t *testing.T) {
	sched, fx := newScheduleService(t)
	adjuster := &fakeAdjuster{resp: &ai.UpdateScheduleResponse{}}
	agents := NewAgentsService(sched.pool, nil, nil, store.NewScheduleStore(), sched, nil, adjuster, &capturingAuditRecorder{})

	_, err := agents.RecommendScheduleAdjustments(context.Background(), fx.orgID, "owner-sub", fx.projectID)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}
