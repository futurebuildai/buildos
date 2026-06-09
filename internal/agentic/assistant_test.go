package agentic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

// discardLogger is a leaf-clean *slog.Logger that writes nowhere.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakePlanner is a test double for the ChatPlanner port. It records what it was
// threaded and returns a canned result/error.
type fakePlanner struct {
	res    ChatResult
	err    error
	calls  int
	gotSys string
	gotIn  ChatInput
	gotReg *AssistantRegistry
	gotB   LoopBounds
}

func (p *fakePlanner) Plan(_ context.Context, sys string, in ChatInput, reg *AssistantRegistry, b LoopBounds) (ChatResult, error) {
	p.calls++
	p.gotSys = sys
	p.gotIn = in
	p.gotReg = reg
	p.gotB = b
	return p.res, p.err
}

// fakeExecutor is a leaf-clean ToolExecutor double.
type fakeExecutor struct {
	res   ToolResult
	err   error
	calls int
}

func (e *fakeExecutor) Execute(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	e.calls++
	return e.res, e.err
}

func TestNewAssistant_AppliesDefaultBounds(t *testing.T) {
	a := NewAssistant(&fakePlanner{}, nil, LoopBounds{}, nil)
	if a.bounds.MaxIterations != defaultMaxIterations {
		t.Errorf("MaxIterations = %d, want %d", a.bounds.MaxIterations, defaultMaxIterations)
	}
	if a.bounds.MaxToolCalls != defaultMaxToolCalls {
		t.Errorf("MaxToolCalls = %d, want %d", a.bounds.MaxToolCalls, defaultMaxToolCalls)
	}
	if a.bounds.MaxToolsPerTurn != defaultMaxToolsPerTurn {
		t.Errorf("MaxToolsPerTurn = %d, want %d", a.bounds.MaxToolsPerTurn, defaultMaxToolsPerTurn)
	}
	if a.bounds.MaxResultBytes != defaultMaxResultBytes {
		t.Errorf("MaxResultBytes = %d, want %d", a.bounds.MaxResultBytes, defaultMaxResultBytes)
	}
	if a.bounds.Timeout != defaultTimeout {
		t.Errorf("Timeout = %v, want %v", a.bounds.Timeout, defaultTimeout)
	}
	if a.logger == nil {
		t.Error("nil logger was not replaced with slog.Default()")
	}
}

func TestNewAssistant_PreservesExplicitBounds(t *testing.T) {
	want := LoopBounds{
		MaxIterations:   3,
		MaxToolCalls:    5,
		MaxToolsPerTurn: 2,
		MaxResultBytes:  1024,
		Timeout:         5 * time.Second,
	}
	a := NewAssistant(&fakePlanner{}, nil, want, nil)
	if a.bounds != want {
		t.Errorf("bounds = %+v, want %+v", a.bounds, want)
	}
}

func TestConverse_HappyPath_ThreadsRegistryAndBounds(t *testing.T) {
	reg := NewAssistantRegistry()
	reg.Add(Tool{
		Spec:     ToolSpec{Name: "list_projects", Description: "d", InputSchema: json.RawMessage(`{}`)},
		MinRole:  "superintendent",
		Executor: &fakeExecutor{},
	})

	planner := &fakePlanner{res: ChatResult{Reply: "grounded answer", Iterations: 2}}
	a := NewAssistant(planner, nil, LoopBounds{}, nil)

	in := ChatInput{Messages: []ChatTurn{{Role: "user", Text: "how many projects?"}}}
	res, err := a.Converse(context.Background(), uuid.New(), "system prompt", in, reg)
	if err != nil {
		t.Fatalf("Converse returned error: %v", err)
	}
	if res.Reply != "grounded answer" {
		t.Errorf("Reply = %q, want %q", res.Reply, "grounded answer")
	}
	if planner.calls != 1 {
		t.Fatalf("planner called %d times, want 1", planner.calls)
	}
	if planner.gotReg != reg {
		t.Error("planner did not receive the caller-scoped registry")
	}
	if planner.gotSys != "system prompt" {
		t.Errorf("planner got sys %q, want %q", planner.gotSys, "system prompt")
	}
	if len(planner.gotIn.Messages) != 1 || planner.gotIn.Messages[0].Text != "how many projects?" {
		t.Errorf("planner got input %+v", planner.gotIn)
	}
	if planner.gotB.MaxIterations != defaultMaxIterations {
		t.Errorf("planner got bounds %+v, want defaults", planner.gotB)
	}
}

func TestConverse_CapabilityGate_RefusesWhenUnregistered(t *testing.T) {
	planner := &fakePlanner{res: ChatResult{Reply: "should not run"}}
	// Construct directly with a registry that does NOT seed Experience to
	// exercise the Phase-3 disable seam.
	a := &Assistant{
		planner:  planner,
		registry: &Registry{descriptors: map[Capability]Descriptor{}},
		bounds:   LoopBounds{}.withDefaults(),
		logger:   discardLogger(),
	}

	_, err := a.Converse(context.Background(), uuid.New(), "sys", ChatInput{}, NewAssistantRegistry())
	if err == nil {
		t.Fatal("expected error when Experience capability is unregistered")
	}
	if planner.calls != 0 {
		t.Errorf("planner was called %d times despite capability gate refusal", planner.calls)
	}
}

func TestConverse_SoftFail_PropagatesErrAssistantUnavailable(t *testing.T) {
	planner := &fakePlanner{err: ErrAssistantUnavailable}
	a := NewAssistant(planner, nil, LoopBounds{}, discardLogger())

	_, err := a.Converse(context.Background(), uuid.New(), "sys", ChatInput{}, NewAssistantRegistry())
	if !errors.Is(err, ErrAssistantUnavailable) {
		t.Fatalf("err = %v, want ErrAssistantUnavailable", err)
	}
}

func TestConverse_ConfigDisabled_ReturnsErrCapabilityDisabled(t *testing.T) {
	planner := &fakePlanner{res: ChatResult{Reply: "should not run"}}
	a := NewAssistant(planner, &fakeConfigResolver{cfg: CapabilityConfig{Enabled: false}}, LoopBounds{}, discardLogger())

	_, err := a.Converse(context.Background(), uuid.New(), "sys", ChatInput{}, NewAssistantRegistry())
	if !errors.Is(err, ErrCapabilityDisabled) {
		t.Fatalf("err = %v, want ErrCapabilityDisabled", err)
	}
	if planner.calls != 0 {
		t.Errorf("planner ran %d times despite the capability being disabled", planner.calls)
	}
}

func TestConverse_ConfigResolveError_PropagatesHard(t *testing.T) {
	planner := &fakePlanner{}
	boom := errors.New("config db down")
	a := NewAssistant(planner, &fakeConfigResolver{err: boom}, LoopBounds{}, discardLogger())

	_, err := a.Converse(context.Background(), uuid.New(), "sys", ChatInput{}, NewAssistantRegistry())
	if err == nil {
		t.Fatal("a config read error must propagate hard")
	}
	if errors.Is(err, ErrCapabilityDisabled) || errors.Is(err, ErrAssistantUnavailable) {
		t.Errorf("a config read error must not masquerade as disabled/unavailable: %v", err)
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap the resolver error", err)
	}
	if planner.calls != 0 {
		t.Errorf("planner ran %d times despite a config read error", planner.calls)
	}
}

func TestConverse_WrappedPlannerErrorIsNotSoftFail(t *testing.T) {
	sentinel := errors.New("boom")
	planner := &fakePlanner{err: sentinel}
	a := NewAssistant(planner, nil, LoopBounds{}, discardLogger())

	_, err := a.Converse(context.Background(), uuid.New(), "sys", ChatInput{}, NewAssistantRegistry())
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrAssistantUnavailable) {
		t.Error("a generic planner error must NOT masquerade as ErrAssistantUnavailable")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want it to wrap the planner error", err)
	}
}

func TestNewRegistry_SeedsExperience(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup(Experience); !ok {
		t.Fatal("NewRegistry did not seed the Experience capability")
	}
}

func TestAssistantRegistry_AddSpecsExecutorLen(t *testing.T) {
	r := NewAssistantRegistry()
	if r.Len() != 0 {
		t.Fatalf("fresh registry Len = %d, want 0", r.Len())
	}
	ex := &fakeExecutor{res: ToolResult{Content: "ok"}}
	r.Add(Tool{Spec: ToolSpec{Name: "get_project"}, MinRole: "superintendent", Executor: ex})
	r.Add(Tool{Spec: ToolSpec{Name: "list_projects"}, MinRole: "superintendent", Executor: &fakeExecutor{}})

	if r.Len() != 2 {
		t.Fatalf("Len = %d, want 2", r.Len())
	}

	specs := r.Specs()
	if len(specs) != 2 || specs[0].Name != "get_project" || specs[1].Name != "list_projects" {
		t.Errorf("Specs not stable-sorted by name: %+v", specs)
	}

	got, ok := r.Executor("get_project")
	if !ok || got != ex {
		t.Errorf("Executor(get_project) = %v, %v; want the registered executor", got, ok)
	}
	if _, ok := r.Executor("nope"); ok {
		t.Error("Executor returned ok for an unregistered name")
	}
}

func TestAssistantRegistry_TryAddAndHas(t *testing.T) {
	r := NewAssistantRegistry()
	if r.Has("conn__x__t") {
		t.Error("Has on an empty registry should be false")
	}
	if !r.TryAdd(Tool{Spec: ToolSpec{Name: "conn__x__t"}, Executor: &fakeExecutor{}}) {
		t.Fatal("first TryAdd should succeed")
	}
	if !r.Has("conn__x__t") {
		t.Error("Has should report the added tool")
	}
	// Duplicate name → false, NO panic (the connector-merge guard).
	if r.TryAdd(Tool{Spec: ToolSpec{Name: "conn__x__t"}, Executor: &fakeExecutor{}}) {
		t.Error("TryAdd of a duplicate name must return false")
	}
	// Empty name → false, NO panic.
	if r.TryAdd(Tool{Spec: ToolSpec{Name: ""}, Executor: &fakeExecutor{}}) {
		t.Error("TryAdd of an empty name must return false")
	}
	if r.Len() != 1 {
		t.Fatalf("Len = %d, want 1 (the rejects must not have been added)", r.Len())
	}
}

func TestAssistantRegistry_AddPanicsOnEmptyName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Add with empty name did not panic")
		}
	}()
	NewAssistantRegistry().Add(Tool{Spec: ToolSpec{Name: ""}})
}

func TestAssistantRegistry_AddPanicsOnDuplicate(t *testing.T) {
	r := NewAssistantRegistry()
	r.Add(Tool{Spec: ToolSpec{Name: "dup"}, Executor: &fakeExecutor{}})
	defer func() {
		if recover() == nil {
			t.Error("Add with duplicate name did not panic")
		}
	}()
	r.Add(Tool{Spec: ToolSpec{Name: "dup"}, Executor: &fakeExecutor{}})
}
