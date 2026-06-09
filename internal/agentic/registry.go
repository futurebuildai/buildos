package agentic

import (
	"encoding/json"
	"sort"
)

// Capability identifies a registered agentic flow by stable string key.
type Capability string

// DelayCascade is the schedule-slip cross-module cascade flow — the first
// real agentic capability. Its key matches the River job kind so the worker
// and the registry agree on a single name.
const DelayCascade Capability = "delay_cascade"

// Descriptor is the in-code metadata for one agentic capability — the STATIC
// catalog of what the binary can run (a compile-time fact). The per-org,
// post-deploy ENABLED/tuning state (a runtime fact) lives in the DB and is read
// through the ConfigResolver port (Phase 3a). DefaultEnabled / DefaultConfig are
// the catalog fallback the resolver returns when an org has no config row.
type Descriptor struct {
	Capability  Capability `json:"capability"`
	Description string     `json:"description"`
	// DefaultEnabled is the no-row enabled fallback. true today (every
	// capability runs unconditionally pre-3a); a future expensive/destructive
	// capability can ship default-OFF by seeding this false.
	DefaultEnabled bool `json:"default_enabled"`
	// DefaultConfig is the no-row tuning fallback (a JSON object; "{}" for
	// capabilities with no tuning yet). For foresight it carries the default
	// thresholds, derived from DefaultForesightTuning so it cannot drift.
	DefaultConfig json.RawMessage `json:"default_config"`
}

// Registry is the in-code catalog of agentic capabilities. It is a simple
// keyed map with Register / Lookup / Capabilities; there is no DB backing in
// Phase 1. It is not safe for concurrent mutation — populate it once at wiring
// time, then treat it as read-only.
type Registry struct {
	descriptors map[Capability]Descriptor
}

// NewRegistry returns a Registry seeded with the built-in capabilities (today:
// delay_cascade, foresight).
func NewRegistry() *Registry {
	r := &Registry{descriptors: make(map[Capability]Descriptor)}
	r.Register(Descriptor{
		Capability:     DelayCascade,
		Description:    "Reason about the cross-module blast radius of a schedule slip and surface impacts as feed cards.",
		DefaultEnabled: true,
		DefaultConfig:  json.RawMessage("{}"), // no tuning in 3a (enable/disable only)
	})
	r.Register(Descriptor{
		Capability:     Foresight,
		Description:    "Periodically surface material standing cross-module risks (procurement criticality, schedule slip, budget burn) the deterministic engine has computed, judged for materiality by AI, as deduped feed cards.",
		DefaultEnabled: true,
		DefaultConfig:  foresightDefaultConfigJSON, // {"schedule_float_days":2,"budget_burn_percent":80}
	})
	r.Register(Descriptor{
		Capability:     Experience,
		Description:    "Answer operator questions in natural language by planning which read-only ERP tools to call, executing them scoped to the caller's org and role, and synthesizing a grounded answer from the deterministic engine facts.",
		DefaultEnabled: true,
		DefaultConfig:  json.RawMessage("{}"), // no tuning in 3a (enable/disable only)
	})
	return r
}

// Register adds (or overwrites) a capability descriptor. It panics on an empty
// capability key, since that is a programmer error at wiring time.
func (r *Registry) Register(d Descriptor) {
	if d.Capability == "" {
		panic("agentic: Register called with empty capability")
	}
	r.descriptors[d.Capability] = d
}

// Lookup returns the descriptor for a capability and whether it was found.
func (r *Registry) Lookup(c Capability) (Descriptor, bool) {
	d, ok := r.descriptors[c]
	return d, ok
}

// Capabilities returns all registered descriptors, sorted by capability key
// for stable, deterministic output (e.g. for a /capabilities surface).
func (r *Registry) Capabilities() []Descriptor {
	out := make([]Descriptor, 0, len(r.descriptors))
	for _, d := range r.descriptors {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Capability < out[j].Capability
	})
	return out
}

// String renders a capability as its underlying string key.
func (c Capability) String() string { return string(c) }
