package service

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
)

// TestNoopAuditRecorder_RecordIsNoOp confirms the test-time stand-in
// neither panics nor touches its (nil) tx — services wired with the
// no-op recorder must run their non-audit logic unhindered.
func TestNoopAuditRecorder_RecordIsNoOp(t *testing.T) {
	r := NewNoopAuditRecorder()
	// nil tx is safe precisely because the no-op never dereferences it.
	r.Record(context.Background(), nil, AuditEntry{
		OrgID:        uuid.New(),
		Action:       "x",
		ResourceType: "y",
		ResourceID:   uuid.New(),
	})
}

// TestAuditService_Record_InvalidEntrySkips covers the programmer-error
// guard: an entry missing any required field is logged at WARN and
// dropped before InsertAudit, so a nil tx never gets touched (no DB
// needed). The four sub-cases hit each missing-field branch.
func TestAuditService_Record_InvalidEntrySkips(t *testing.T) {
	svc := NewAuditService(store.NewAuditStore(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	good := AuditEntry{
		OrgID:        uuid.New(),
		Action:       "thing.created",
		ResourceType: "thing",
		ResourceID:   uuid.New(),
	}

	cases := map[string]func(AuditEntry) AuditEntry{
		"nil org":         func(e AuditEntry) AuditEntry { e.OrgID = uuid.Nil; return e },
		"empty action":    func(e AuditEntry) AuditEntry { e.Action = ""; return e },
		"empty resource":  func(e AuditEntry) AuditEntry { e.ResourceType = ""; return e },
		"nil resource id": func(e AuditEntry) AuditEntry { e.ResourceID = uuid.Nil; return e },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			// nil tx is safe: the guard returns before InsertAudit.
			svc.Record(ctx, nil, mutate(good))
		})
	}
}
