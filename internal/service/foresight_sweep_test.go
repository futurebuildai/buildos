package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
)

// fakeSweepResolver is a service-side test double for agentic.ConfigResolver.
type fakeSweepResolver struct {
	cfg   agentic.CapabilityConfig
	err   error
	calls map[uuid.UUID]int
}

func (f *fakeSweepResolver) Resolve(_ context.Context, orgID uuid.UUID, _ agentic.Capability) (agentic.CapabilityConfig, error) {
	if f.calls == nil {
		f.calls = map[uuid.UUID]int{}
	}
	f.calls[orgID]++
	if f.err != nil {
		return agentic.CapabilityConfig{}, f.err
	}
	return f.cfg, nil
}

func newSweepWithResolver(r agentic.ConfigResolver) *ForesightSweepService {
	return &ForesightSweepService{
		config: r,
		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
}

func TestResolveOrgConfig_NilResolver_EnabledWithDefault(t *testing.T) {
	s := newSweepWithResolver(nil)
	memo := map[uuid.UUID]orgForesightConfig{}
	fc, err := s.resolveOrgConfig(context.Background(), memo, uuid.New())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !fc.enabled || fc.tuning != agentic.DefaultForesightTuning() {
		t.Errorf("nil resolver => %+v, want enabled-with-default", fc)
	}
}

func TestResolveOrgConfig_ParsesTuning(t *testing.T) {
	r := &fakeSweepResolver{cfg: agentic.CapabilityConfig{Enabled: true, Config: json.RawMessage(`{"budget_burn_percent":50}`)}}
	s := newSweepWithResolver(r)
	fc, err := s.resolveOrgConfig(context.Background(), map[uuid.UUID]orgForesightConfig{}, uuid.New())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !fc.enabled || fc.tuning.BudgetBurnPercent != 50 || fc.tuning.ScheduleFloatDays != agentic.DefaultForesightTuning().ScheduleFloatDays {
		t.Errorf("resolved = %+v, want enabled + burn 50 + default float", fc)
	}
}

func TestResolveOrgConfig_Disabled(t *testing.T) {
	r := &fakeSweepResolver{cfg: agentic.CapabilityConfig{Enabled: false}}
	s := newSweepWithResolver(r)
	fc, err := s.resolveOrgConfig(context.Background(), map[uuid.UUID]orgForesightConfig{}, uuid.New())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if fc.enabled {
		t.Error("disabled config must report enabled=false")
	}
}

func TestResolveOrgConfig_ErrorPropagates(t *testing.T) {
	boom := errors.New("config db down")
	s := newSweepWithResolver(&fakeSweepResolver{err: boom})
	_, err := s.resolveOrgConfig(context.Background(), map[uuid.UUID]orgForesightConfig{}, uuid.New())
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap the resolver error (retryable)", err)
	}
}

func TestResolveOrgConfig_MemoizesOneResolvePerOrg(t *testing.T) {
	r := &fakeSweepResolver{cfg: agentic.CapabilityConfig{Enabled: true}}
	s := newSweepWithResolver(r)
	memo := map[uuid.UUID]orgForesightConfig{}
	org := uuid.New()

	for i := 0; i < 5; i++ {
		if _, err := s.resolveOrgConfig(context.Background(), memo, org); err != nil {
			t.Fatalf("err: %v", err)
		}
	}
	if r.calls[org] != 1 {
		t.Errorf("resolver called %d times for one org across 5 projects, want 1 (memoized)", r.calls[org])
	}
}
