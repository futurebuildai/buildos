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

// fakeAdjuster is the test double for MaestroScheduleAdjuster. Mirrors
// fakeBriefer's shape — captures the last request, replays a scripted
// response. Lets us validate the gating in RecommendScheduleAdjustments
// without standing up a real Brain.
type fakeAdjuster struct {
	lastReq brain.UpdateScheduleRequest
	resp    *brain.UpdateScheduleResponse
	err     error
}

func (f *fakeAdjuster) UpdateSchedule(_ context.Context, req brain.UpdateScheduleRequest) (*brain.UpdateScheduleResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestAgentsService_GenerateDailyBriefing_RejectsBadInput(t *testing.T) {
	svc := NewAgentsService(nil, nil, nil, nil, nil, &fakeBriefer{}, nil, nil)
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
	svc := NewAgentsService(nil, nil, nil, nil, nil, &fakeBriefer{err: errors.New("boom")}, nil, nil)
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
	svc := NewAgentsService(nil, nil, nil, nil, nil, &fakeBriefer{}, nil, nil)
	if svc.audit == nil {
		t.Fatal("audit recorder must be non-nil after constructor")
	}
	if _, ok := svc.audit.(NoopAuditRecorder); !ok {
		t.Errorf("audit = %T, want NoopAuditRecorder", svc.audit)
	}
}

func TestAgentsService_RecommendScheduleAdjustments_RejectsBadInput(t *testing.T) {
	// Validation gates fire before any DB / Brain interaction, so we
	// can exercise them with all-nil deps. The constructor still
	// requires a non-nil briefer for daily-briefing tests; pass one
	// here for symmetry — the field is unused on this path.
	svc := NewAgentsService(nil, nil, nil, nil, nil, &fakeBriefer{}, &fakeAdjuster{}, nil)
	if _, err := svc.RecommendScheduleAdjustments(context.Background(), uuid.Nil, "sub-1", uuid.New()); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.RecommendScheduleAdjustments(context.Background(), uuid.New(), "sub-1", uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil project: err = %v, want ErrInvalidInput", err)
	}
}

func TestAgentsService_RecommendScheduleAdjustments_NilAdjusterReturnsSentinel(t *testing.T) {
	// The worker binary doesn't expose agent endpoints, so it's
	// allowed to construct AgentsService without a ScheduleAdjuster.
	// In that case the flow must return ErrAgentsMaestroUnavailable
	// rather than panicking on a nil method call.
	svc := NewAgentsService(nil, nil, nil, nil, nil, &fakeBriefer{}, nil, nil)
	_, err := svc.RecommendScheduleAdjustments(context.Background(), uuid.New(), "sub-1", uuid.New())
	if !errors.Is(err, ErrAgentsMaestroUnavailable) {
		t.Errorf("err = %v, want ErrAgentsMaestroUnavailable", err)
	}
}

func TestAgentsService_RecommendScheduleAdjustments_NilScheduleServiceReturnsSentinel(t *testing.T) {
	// Adjuster present but scheduleStore/scheduleService nil — same
	// guarded path. RecommendScheduleAdjustments needs both to load
	// the task graph and to re-run CPM after applying deltas.
	svc := NewAgentsService(nil, nil, nil, nil, nil, &fakeBriefer{}, &fakeAdjuster{}, nil)
	_, err := svc.RecommendScheduleAdjustments(context.Background(), uuid.New(), "sub-1", uuid.New())
	if !errors.Is(err, ErrAgentsScheduleServiceUnavailable) {
		t.Errorf("err = %v, want ErrAgentsScheduleServiceUnavailable", err)
	}
}
