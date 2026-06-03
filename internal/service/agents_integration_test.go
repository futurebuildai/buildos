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
