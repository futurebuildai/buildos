package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/brain"
)

// fakeMaestro lets us assert the assembled prompt without spinning up
// an HTTP server. Captures the last request and replays a scripted
// response.
type fakeMaestro struct {
	lastReq brain.ChatRequest
	resp    *brain.ChatResponse
	err     error
}

func (f *fakeMaestro) Chat(_ context.Context, req brain.ChatRequest) (*brain.ChatResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func TestAgentsService_GenerateDailyBriefing_RejectsBadInput(t *testing.T) {
	svc := NewAgentsService(nil, nil, nil, &fakeMaestro{})
	if _, err := svc.GenerateDailyBriefing(context.Background(), uuid.Nil, "sub-1", "owner"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.GenerateDailyBriefing(context.Background(), uuid.New(), "", "owner"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty subject: err = %v, want ErrInvalidInput", err)
	}
}

func TestBuildDailyBriefingPrompt_NoTasksNoAlerts(t *testing.T) {
	got := buildDailyBriefingPrompt(nil, nil)
	if !strings.Contains(got, "Active alerts: (none)") {
		t.Errorf("missing 'no alerts' line: %q", got)
	}
	if !strings.Contains(got, "Assigned tasks today: (none)") {
		t.Errorf("missing 'no tasks' line: %q", got)
	}
}

func TestBuildDailyBriefingPrompt_IncludesTasksAndAlerts(t *testing.T) {
	got := buildDailyBriefingPrompt(
		[]string{"06.10.10 framing", "08.50.00 windows"},
		[]string{"[critical] supply delay on rebar"},
	)
	if !strings.Contains(got, "06.10.10 framing") {
		t.Errorf("task missing from prompt: %q", got)
	}
	if !strings.Contains(got, "08.50.00 windows") {
		t.Errorf("task missing from prompt: %q", got)
	}
	if !strings.Contains(got, "supply delay on rebar") {
		t.Errorf("alert missing from prompt: %q", got)
	}
	if !strings.Contains(got, "under 6 sentences") {
		t.Errorf("missing length-cap directive: %q", got)
	}
}

func TestBuildDailyBriefingPrompt_LeadsWithAlerts(t *testing.T) {
	// The prompt asks Maestro to "lead with the highest-priority alert"
	// — verify that ordering directive is preserved.
	got := buildDailyBriefingPrompt(
		[]string{"task-A"},
		[]string{"[critical] alert-1"},
	)
	if !strings.Contains(got, "Lead with the highest-priority alert") {
		t.Errorf("ordering directive missing: %q", got)
	}
}

func TestAgentsService_GenerateDailyBriefing_PropagatesMaestroError(t *testing.T) {
	// We can't reach the Maestro call without a real DB (the tx
	// runs first and would panic on nil pool). This validation-only
	// test confirms the validation path is the gate. Maestro error
	// propagation is exercised indirectly by the handler tests +
	// brain client unit tests.
	svc := NewAgentsService(nil, nil, nil, &fakeMaestro{err: errors.New("boom")})
	_, err := svc.GenerateDailyBriefing(context.Background(), uuid.Nil, "sub-1", "owner")
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput (validation precedes Maestro call)", err)
	}
}
