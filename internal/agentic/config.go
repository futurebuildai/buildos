package agentic

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// ErrCapabilityDisabled signals that an org has explicitly disabled a capability
// via admin config. Synchronous flows (experience) propagate it so the handler
// maps a deliberate admin state to a clean 403 — distinct from the 503 a missing
// AI key produces (ErrAssistantUnavailable). Worker flows do NOT surface it:
// they clean no-op (zero result, nil) so River does not retry a deliberate
// disable.
var ErrCapabilityDisabled = errors.New("agentic: capability disabled by config")

// CapabilityConfig is the resolved, per-org runtime config for one capability.
// Enabled gates the flow; Config is the capability-specific tuning blob (a JSON
// object; "{}" when untuned). The leaf never interprets Config's shape except
// through the typed helpers below — interpretation is the caller's job. This is
// the runtime (per-org, DB-backed) counterpart to the static in-code Descriptor.
type CapabilityConfig struct {
	Enabled bool
	Config  json.RawMessage
}

// ConfigResolver resolves per-org capability config at runtime. The sole adapter
// lives in internal/service: it reads the agents_config table and falls back to
// the in-code catalog default (Descriptor.DefaultEnabled / DefaultConfig) when no
// row exists. A read error is an INFRASTRUCTURE failure the caller must treat as
// hard (worker retry / HTTP 5xx) — never a soft-fail, because soft-failing a
// failed config read would silently run a possibly-disabled capability.
//
// orgID is the TENANT key only. It must never influence RBAC or tool scoping —
// that stays structural (per-request sealed registries / Input-carried scoping).
type ConfigResolver interface {
	Resolve(ctx context.Context, orgID uuid.UUID, c Capability) (CapabilityConfig, error)
}

// ---- foresight tuning (the one capability whose Config drives behavior in 3a) ----

const (
	// defaultForesightScheduleFloatDays: a schedule task with remaining float
	// <= this is breached (a critical, not-yet-complete task always breaches).
	defaultForesightScheduleFloatDays = 2
	// defaultForesightBudgetBurnPercent: a budget line with integer burn% >=
	// this is breached (a zero-estimate line has burn -1 and never breaches).
	defaultForesightBudgetBurnPercent = 80
)

// ForesightTuning is the typed, per-org foresight knobs parsed from a
// CapabilityConfig.Config blob. The deterministic workspace receives THIS (typed
// integer thresholds), never raw JSON — the leaf does selection/plumbing, never
// schedule or money math. It is the single in-code home for the default dials
// (NewRegistry marshals DefaultForesightTuning into the foresight descriptor's
// DefaultConfig, so the catalog default and the engine default can never drift).
type ForesightTuning struct {
	ScheduleFloatDays int `json:"schedule_float_days"`
	BudgetBurnPercent int `json:"budget_burn_percent"`
}

// DefaultForesightTuning is the single source of truth for the default
// thresholds. The registry marshals it into the foresight Descriptor.DefaultConfig.
func DefaultForesightTuning() ForesightTuning {
	return ForesightTuning{
		ScheduleFloatDays: defaultForesightScheduleFloatDays,
		BudgetBurnPercent: defaultForesightBudgetBurnPercent,
	}
}

// WithDefaults returns a copy with any non-positive field replaced by its
// documented default, so a partial or garbage config still yields fully-valid
// thresholds. Exported so the deterministic workspace can apply it defensively to
// whatever tuning it is handed.
func (t ForesightTuning) WithDefaults() ForesightTuning {
	if t.ScheduleFloatDays <= 0 {
		t.ScheduleFloatDays = defaultForesightScheduleFloatDays
	}
	if t.BudgetBurnPercent <= 0 {
		t.BudgetBurnPercent = defaultForesightBudgetBurnPercent
	}
	return t
}

// ParseForesightTuning totally-parses a config blob into thresholds: a malformed,
// partial, or empty blob yields defaults, NEVER an error. A config that passed
// the admin write-time object check must not be able to fail a worker job at read
// time (defense in depth — the write path validates, the read path forgives).
func ParseForesightTuning(cfg json.RawMessage) ForesightTuning {
	var t ForesightTuning
	if len(cfg) > 0 {
		_ = json.Unmarshal(cfg, &t) // ignore error: defensive, WithDefaults coerces
	}
	return t.WithDefaults()
}

// foresightDefaultConfigJSON is the marshaled DefaultForesightTuning, seeded into
// the foresight Descriptor.DefaultConfig by NewRegistry. Derived from the typed
// default so the JSON and the typed value can never disagree.
var foresightDefaultConfigJSON = mustJSON(DefaultForesightTuning())

// mustJSON marshals a value known to be marshalable (a plain struct of ints);
// it panics on failure, which can only be a programmer error.
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("agentic: marshal default config: " + err.Error())
	}
	return b
}
