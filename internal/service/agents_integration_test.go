//go:build integration

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
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
