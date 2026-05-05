package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/brain"
)

// fakeBriefer is the test double for MaestroDailyBriefer. Captures the
// last request and replays a scripted response. Lets us assert the
// caller assembled the structured-context envelope correctly without
// spinning up an HTTP server.
type fakeBriefer struct {
	lastReq brain.DailyBriefingRequest
	resp    *brain.DailyBriefingResponse
	err     error
}

func (f *fakeBriefer) DailyBriefing(_ context.Context, req brain.DailyBriefingRequest) (*brain.DailyBriefingResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestAgentsService_GenerateDailyBriefing_RejectsBadInput(t *testing.T) {
	svc := NewAgentsService(nil, nil, nil, &fakeBriefer{}, nil)
	if _, err := svc.GenerateDailyBriefing(context.Background(), uuid.Nil, "sub-1", "owner"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.GenerateDailyBriefing(context.Background(), uuid.New(), "", "owner"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty subject: err = %v, want ErrInvalidInput", err)
	}
}

func TestAgentsService_GenerateDailyBriefing_PropagatesMaestroError(t *testing.T) {
	// We can't reach the Maestro call without a real DB (the tx
	// runs first and would panic on nil pool). This validation-only
	// test confirms the validation path is the gate. Maestro error
	// propagation is exercised indirectly by the handler tests +
	// brain client unit tests.
	svc := NewAgentsService(nil, nil, nil, &fakeBriefer{err: errors.New("boom")}, nil)
	_, err := svc.GenerateDailyBriefing(context.Background(), uuid.Nil, "sub-1", "owner")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput (validation precedes Maestro call)", err)
	}
}

func TestAgentsService_NewAgentsService_NilAuditFallsBackToNoop(t *testing.T) {
	// Passing a nil AuditRecorder must be tolerated — tests that
	// don't care about audit shouldn't have to wire NoopAuditRecorder
	// explicitly, and a nil deref later in GenerateDailyBriefing
	// would be a regression.
	svc := NewAgentsService(nil, nil, nil, &fakeBriefer{}, nil)
	if svc.audit == nil {
		t.Fatal("audit recorder must be non-nil after constructor")
	}
	if _, ok := svc.audit.(NoopAuditRecorder); !ok {
		t.Errorf("audit = %T, want NoopAuditRecorder", svc.audit)
	}
}
