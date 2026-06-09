package agentic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// fakeConfigResolver is a leaf-pure test double for the ConfigResolver port.
// It MUST NOT import internal/store, internal/service, or pgx — the isolation
// gate (scripts/check-isolation.sh, Check 2) walks TestImports too.
type fakeConfigResolver struct {
	cfg    CapabilityConfig
	err    error
	calls  int
	gotOrg uuid.UUID
	gotCap Capability
	byOrg  map[uuid.UUID]CapabilityConfig // optional per-org overrides
}

func (f *fakeConfigResolver) Resolve(_ context.Context, orgID uuid.UUID, c Capability) (CapabilityConfig, error) {
	f.calls++
	f.gotOrg = orgID
	f.gotCap = c
	if f.err != nil {
		return CapabilityConfig{}, f.err
	}
	if f.byOrg != nil {
		if cc, ok := f.byOrg[orgID]; ok {
			return cc, nil
		}
	}
	return f.cfg, nil
}

func TestForesightTuning_WithDefaults(t *testing.T) {
	cases := []struct {
		name string
		in   ForesightTuning
		want ForesightTuning
	}{
		{"zero -> defaults", ForesightTuning{}, ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
		{"negative -> defaults", ForesightTuning{ScheduleFloatDays: -3, BudgetBurnPercent: -1}, ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
		{"partial -> fill missing", ForesightTuning{BudgetBurnPercent: 50}, ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 50}},
		{"both set -> unchanged", ForesightTuning{ScheduleFloatDays: 5, BudgetBurnPercent: 90}, ForesightTuning{ScheduleFloatDays: 5, BudgetBurnPercent: 90}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.WithDefaults(); got != tc.want {
				t.Fatalf("WithDefaults() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestParseForesightTuning(t *testing.T) {
	cases := []struct {
		name string
		in   json.RawMessage
		want ForesightTuning
	}{
		{"empty -> defaults", nil, ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
		{"empty object -> defaults", json.RawMessage("{}"), ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
		{"override burn -> default float", json.RawMessage(`{"budget_burn_percent":50}`), ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 50}},
		{"both -> verbatim", json.RawMessage(`{"schedule_float_days":4,"budget_burn_percent":70}`), ForesightTuning{ScheduleFloatDays: 4, BudgetBurnPercent: 70}},
		{"garbage type -> defaults (never errors)", json.RawMessage(`{"schedule_float_days":"two"}`), ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
		{"malformed json -> defaults (never errors)", json.RawMessage(`{not json`), ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
		{"negative -> defaults", json.RawMessage(`{"schedule_float_days":-1,"budget_burn_percent":-9}`), ForesightTuning{ScheduleFloatDays: 2, BudgetBurnPercent: 80}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseForesightTuning(tc.in); got != tc.want {
				t.Fatalf("ParseForesightTuning(%s) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestDefaultForesightTuning_MatchesCatalogDefaultConfig(t *testing.T) {
	// The foresight catalog DefaultConfig must parse back to the typed default —
	// proving the single-source-of-truth (no drift between the JSON seeded into
	// the descriptor and the typed DefaultForesightTuning).
	d, ok := NewRegistry().Lookup(Foresight)
	if !ok {
		t.Fatal("foresight must be in the catalog")
	}
	if got := ParseForesightTuning(d.DefaultConfig); got != DefaultForesightTuning() {
		t.Fatalf("foresight DefaultConfig parsed to %+v, want %+v", got, DefaultForesightTuning())
	}
}
