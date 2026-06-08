package agentic

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// fakeReasoner is a test double for the Reasoner port.
type fakeReasoner struct {
	plan   CascadePlan
	err    error
	calls  int
	gotCtx CascadeContext
}

func (f *fakeReasoner) PlanCascade(_ context.Context, c CascadeContext) (CascadePlan, error) {
	f.calls++
	f.gotCtx = c
	return f.plan, f.err
}

// fakeWorkspace is a test double for the CascadeWorkspace port.
type fakeWorkspace struct {
	loadCtx  CascadeContext
	loadErr  error
	loadCall int

	applyRes   CascadeResult
	applyErr   error
	applyCall  int
	gotPlan    CascadePlan
	gotOrgID   uuid.UUID
	gotProject uuid.UUID
}

func (f *fakeWorkspace) LoadCascadeContext(_ context.Context, _, _ uuid.UUID) (CascadeContext, error) {
	f.loadCall++
	return f.loadCtx, f.loadErr
}

func (f *fakeWorkspace) ApplyCascade(_ context.Context, orgID, projectID uuid.UUID, plan CascadePlan) (CascadeResult, error) {
	f.applyCall++
	f.gotPlan = plan
	f.gotOrgID = orgID
	f.gotProject = projectID
	return f.applyRes, f.applyErr
}

func criticalContext() CascadeContext {
	return CascadeContext{
		ProjectName: "Maple St Rebuild",
		SlippedTasks: []CascadeSlippedTask{
			{WBS: "1.1", Name: "Foundation", FloatDays: 0, IsCritical: true},
			{WBS: "1.2", Name: "Framing", FloatDays: 3, IsCritical: false},
		},
	}
}

func TestRunDelayCascade_HappyPath(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	plan := CascadePlan{Impacts: []CascadeImpact{
		{Module: "procurement", Severity: "high", Title: "Order rebar now", Body: "b", RecommendedAction: "a"},
		{Module: "crew", Severity: "normal", Title: "Reslot framing crew", Body: "b", RecommendedAction: "a"},
	}}
	ws := &fakeWorkspace{
		loadCtx:  criticalContext(),
		applyRes: CascadeResult{CardsCreated: 2, Impacts: 2},
	}
	rsn := &fakeReasoner{plan: plan}

	o := NewOrchestrator(rsn, ws, nil)
	res, err := o.RunDelayCascade(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.CardsCreated != 2 || res.Impacts != 2 {
		t.Fatalf("got %+v, want {CardsCreated:2 Impacts:2}", res)
	}
	if rsn.calls != 1 {
		t.Fatalf("reasoner calls = %d, want 1", rsn.calls)
	}
	if ws.applyCall != 1 {
		t.Fatalf("apply calls = %d, want 1", ws.applyCall)
	}
	// The orchestrator must thread the plan and identifiers straight through.
	if len(ws.gotPlan.Impacts) != 2 {
		t.Fatalf("apply got %d impacts, want 2", len(ws.gotPlan.Impacts))
	}
	if ws.gotOrgID != in.OrgID || ws.gotProject != in.ProjectID {
		t.Fatalf("apply got org=%s project=%s, want org=%s project=%s",
			ws.gotOrgID, ws.gotProject, in.OrgID, in.ProjectID)
	}
	// And the loaded context must reach the reasoner unchanged.
	if rsn.gotCtx.ProjectName != "Maple St Rebuild" {
		t.Fatalf("reasoner got project %q, want %q", rsn.gotCtx.ProjectName, "Maple St Rebuild")
	}
}

func TestRunDelayCascade_NoCriticalPath_NoOp(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	// All slipped tasks have float — nothing critical.
	ws := &fakeWorkspace{loadCtx: CascadeContext{
		ProjectName: "Cedar Ct",
		SlippedTasks: []CascadeSlippedTask{
			{WBS: "2.1", Name: "Drywall", FloatDays: 5, IsCritical: false},
		},
	}}
	rsn := &fakeReasoner{}

	o := NewOrchestrator(rsn, ws, nil)
	res, err := o.RunDelayCascade(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (CascadeResult{}) {
		t.Fatalf("got %+v, want zero CascadeResult", res)
	}
	if rsn.calls != 0 {
		t.Fatalf("reasoner must not be called on a non-critical slip; calls = %d", rsn.calls)
	}
	if ws.applyCall != 0 {
		t.Fatalf("apply must not be called on a non-critical slip; calls = %d", ws.applyCall)
	}
}

func TestRunDelayCascade_EmptyContext_NoOp(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	// Completely empty context (no slipped tasks at all).
	ws := &fakeWorkspace{loadCtx: CascadeContext{}}
	rsn := &fakeReasoner{}

	o := NewOrchestrator(rsn, ws, nil)
	res, err := o.RunDelayCascade(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (CascadeResult{}) {
		t.Fatalf("got %+v, want zero CascadeResult", res)
	}
	if rsn.calls != 0 || ws.applyCall != 0 {
		t.Fatalf("empty context must no-op; reasoner=%d apply=%d", rsn.calls, ws.applyCall)
	}
}

func TestRunDelayCascade_ReasonerUnavailable_SoftFails(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	ws := &fakeWorkspace{loadCtx: criticalContext()}
	// Wrap the sentinel to prove errors.Is unwrapping works.
	rsn := &fakeReasoner{err: fmt.Errorf("no key for org: %w", ErrReasonerUnavailable)}

	o := NewOrchestrator(rsn, ws, nil)
	res, err := o.RunDelayCascade(context.Background(), in)
	if err != nil {
		t.Fatalf("reasoner-unavailable must soft-fail to nil error, got: %v", err)
	}
	if res != (CascadeResult{}) {
		t.Fatalf("got %+v, want zero CascadeResult on soft-fail", res)
	}
	if rsn.calls != 1 {
		t.Fatalf("reasoner calls = %d, want 1", rsn.calls)
	}
	if ws.applyCall != 0 {
		t.Fatalf("apply must not run when the reasoner is unavailable; calls = %d", ws.applyCall)
	}
}

func TestRunDelayCascade_ReasonerHardError_Propagates(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	ws := &fakeWorkspace{loadCtx: criticalContext()}
	boom := errors.New("anthropic 500")
	rsn := &fakeReasoner{err: boom}

	o := NewOrchestrator(rsn, ws, nil)
	_, err := o.RunDelayCascade(context.Background(), in)
	if err == nil {
		t.Fatal("expected a hard reasoner error to propagate, got nil")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error chain should wrap the reasoner error, got: %v", err)
	}
	if ws.applyCall != 0 {
		t.Fatalf("apply must not run after a hard reasoner error; calls = %d", ws.applyCall)
	}
}

func TestRunDelayCascade_LoadError_Propagates(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	boom := errors.New("db down")
	ws := &fakeWorkspace{loadErr: boom}
	rsn := &fakeReasoner{}

	o := NewOrchestrator(rsn, ws, nil)
	_, err := o.RunDelayCascade(context.Background(), in)
	if err == nil {
		t.Fatal("expected load error to propagate, got nil")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error chain should wrap the load error, got: %v", err)
	}
	if rsn.calls != 0 {
		t.Fatalf("reasoner must not run after a load error; calls = %d", rsn.calls)
	}
}

func TestRunDelayCascade_ApplyError_Propagates(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	boom := errors.New("tx rollback")
	ws := &fakeWorkspace{loadCtx: criticalContext(), applyErr: boom}
	rsn := &fakeReasoner{plan: CascadePlan{Impacts: []CascadeImpact{
		{Module: "schedule", Severity: "critical", Title: "t", Body: "b", RecommendedAction: "a"},
	}}}

	o := NewOrchestrator(rsn, ws, nil)
	_, err := o.RunDelayCascade(context.Background(), in)
	if err == nil {
		t.Fatal("expected apply error to propagate, got nil")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("error chain should wrap the apply error, got: %v", err)
	}
}

func TestRunDelayCascade_EmptyPlan_NoApply(t *testing.T) {
	in := DelayCascadeInput{OrgID: uuid.New(), ProjectID: uuid.New()}
	ws := &fakeWorkspace{loadCtx: criticalContext()}
	rsn := &fakeReasoner{plan: CascadePlan{Impacts: nil}}

	o := NewOrchestrator(rsn, ws, nil)
	res, err := o.RunDelayCascade(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != (CascadeResult{}) {
		t.Fatalf("got %+v, want zero CascadeResult when the plan is empty", res)
	}
	if ws.applyCall != 0 {
		t.Fatalf("apply must not run on an empty plan; calls = %d", ws.applyCall)
	}
}

func TestNewOrchestrator_NilLoggerDefaults(t *testing.T) {
	// A nil logger must not panic — it should fall back to slog.Default().
	o := NewOrchestrator(&fakeReasoner{}, &fakeWorkspace{}, nil)
	if o.logger == nil {
		t.Fatal("expected a non-nil logger after NewOrchestrator(nil)")
	}
}
