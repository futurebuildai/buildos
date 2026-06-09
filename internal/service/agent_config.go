package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Audit actions for the agent config registry (Phase 3a). Singular-noun.resource
// .verb, matching the integration.credential.* convention.
const (
	auditActionAgentConfigUpdated = "agent.config.updated"
	auditActionAgentConfigReset   = "agent.config.reset"
)

// Effective-config source discriminators surfaced by ListEffective.
const (
	agentConfigSourceDefault  = "default"  // no override row — the catalog default
	agentConfigSourceOverride = "override" // an explicit per-org row
)

// AgentConfigService is the agent config registry (Phase 3a). It has TWO faces
// over the same agents_config table:
//
//  1. Admin CRUD (ListEffective / Set / Reset) — the operator surface behind
//     /api/v1/admin/agents, one-tx-per-mutation + audit.
//  2. agentic.ConfigResolver (Resolve) — the runtime read the orchestrators /
//     sweep consult to gate + tune each capability per org.
//
// The in-code catalog (agentic.Registry) is the existence authority: Set/Reset
// reject a capability the binary cannot run (404), and Resolve falls back to the
// catalog default when an org has no override row.
type AgentConfigService struct {
	pool    *pgxpool.Pool
	store   *store.AgentConfigStore
	catalog *agentic.Registry
	audit   AuditRecorder
	logger  *slog.Logger
}

// NewAgentConfigService wires the store + the in-code catalog + audit. A nil
// AuditRecorder falls back to the no-op; a nil logger becomes slog.Default().
func NewAgentConfigService(pool *pgxpool.Pool, st *store.AgentConfigStore, audit AuditRecorder, logger *slog.Logger) *AgentConfigService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentConfigService{
		pool:    pool,
		store:   st,
		catalog: agentic.NewRegistry(),
		audit:   audit,
		logger:  logger,
	}
}

// ---- Face 1: admin CRUD ------------------------------------------------

// EffectiveAgentConfig is one capability's effective config for an org: the
// catalog metadata merged with any override row. Source discriminates an
// explicit override from the implicit catalog default.
type EffectiveAgentConfig struct {
	Capability  string          `json:"capability"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config"`
	Source      string          `json:"source"` // "default" | "override"
	UpdatedBy   string          `json:"updated_by,omitempty"`
	UpdatedAt   *time.Time      `json:"updated_at,omitempty"`
}

// SetAgentConfigInput is the validated input for Set. OrgID + UserSub come from
// JWT claims (never the request body). Config is the raw tuning blob.
type SetAgentConfigInput struct {
	OrgID      uuid.UUID
	Capability string
	Enabled    bool
	Config     json.RawMessage
	UserSub    string
}

// ListEffective returns every catalog capability with its effective config for
// the org: the override row when present, else the catalog default. Driven by
// the catalog (the existence authority), so an orphaned override row for a
// capability the binary no longer ships is simply not listed.
func (s *AgentConfigService) ListEffective(ctx context.Context, orgID uuid.UUID) ([]EffectiveAgentConfig, error) {
	var rows []models.AgentConfig
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.ListByOrg(ctx, tx, orgID)
		if qErr != nil {
			return qErr
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list effective agent config: %w", err)
	}

	byCap := make(map[string]models.AgentConfig, len(rows))
	for _, r := range rows {
		byCap[r.Capability] = r
	}

	descriptors := s.catalog.Capabilities() // sorted by capability key
	out := make([]EffectiveAgentConfig, 0, len(descriptors))
	for _, d := range descriptors {
		eff := EffectiveAgentConfig{
			Capability:  d.Capability.String(),
			Description: d.Description,
			Enabled:     d.DefaultEnabled,
			Config:      defaultConfigJSON(d.DefaultConfig),
			Source:      agentConfigSourceDefault,
		}
		if row, ok := byCap[d.Capability.String()]; ok {
			eff.Enabled = row.Enabled
			eff.Config = defaultConfigJSON(row.Config)
			eff.Source = agentConfigSourceOverride
			eff.UpdatedBy = row.UpdatedBy
			ts := row.UpdatedAt
			eff.UpdatedAt = &ts
		}
		out = append(out, eff)
	}
	return out, nil
}

// Set upserts the override (enabled + config) for a capability, full-document
// semantics. It validates the capability is in the catalog (ErrNotFound -> 404)
// and the config is a well-formed tuning object (ErrInvalidInput -> 400) BEFORE
// any DB write, then upserts + audits in one tx.
func (s *AgentConfigService) Set(ctx context.Context, in SetAgentConfigInput) (models.AgentConfig, error) {
	if in.OrgID == uuid.Nil {
		return models.AgentConfig{}, fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	if _, ok := s.catalog.Lookup(agentic.Capability(in.Capability)); !ok {
		return models.AgentConfig{}, fmt.Errorf("%w: unknown capability %q", ErrNotFound, in.Capability)
	}
	cfg, err := validateAgentConfig(in.Capability, in.Config)
	if err != nil {
		return models.AgentConfig{}, err
	}

	var out models.AgentConfig
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, qErr := s.store.Upsert(ctx, tx, store.UpsertAgentConfigParams{
			OrgID:      in.OrgID,
			Capability: in.Capability,
			Enabled:    in.Enabled,
			Config:     cfg,
			UpdatedBy:  in.UserSub,
		})
		if qErr != nil {
			return qErr
		}
		out = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       auditActionAgentConfigUpdated,
			ResourceType: AuditResourceAgentConfig,
			ResourceID:   row.ID,
			Metadata: marshalAudit(map[string]any{
				"capability": in.Capability,
				"enabled":    in.Enabled,
				"config":     json.RawMessage(cfg),
			}),
		})
		return nil
	})
	if err != nil {
		return models.AgentConfig{}, fmt.Errorf("set agent config: %w", err)
	}
	return out, nil
}

// Reset removes the override row for a capability, resetting the org to the
// catalog default. Idempotent: returns nil whether or not a row existed; only
// audits (agent.config.reset) when a row was ACTUALLY deleted (no phantom-reset
// rows). Validates the capability is in the catalog (ErrNotFound -> 404).
func (s *AgentConfigService) Reset(ctx context.Context, orgID uuid.UUID, capability, userSub string) error {
	if orgID == uuid.Nil {
		return fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	if _, ok := s.catalog.Lookup(agentic.Capability(capability)); !ok {
		return fmt.Errorf("%w: unknown capability %q", ErrNotFound, capability)
	}

	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		affected, qErr := s.store.DeleteByCapability(ctx, tx, orgID, capability)
		if qErr != nil {
			return qErr
		}
		if affected == 0 {
			return nil // idempotent reset: nothing to delete, nothing to audit
		}
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       auditActionAgentConfigReset,
			ResourceType: AuditResourceAgentConfig,
			ResourceID:   orgID, // the override row is gone; key the audit to the org
			Metadata:     marshalAudit(map[string]any{"capability": capability}),
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("reset agent config: %w", err)
	}
	return nil
}

// ---- Face 2: agentic.ConfigResolver ------------------------------------

// Resolve returns the per-org CapabilityConfig: the override row when present,
// else the in-code catalog default (enabled-with-default). A read error is
// returned (the caller treats it as a hard infrastructure failure — never a
// soft-fail).
func (s *AgentConfigService) Resolve(ctx context.Context, orgID uuid.UUID, c agentic.Capability) (agentic.CapabilityConfig, error) {
	d, known := s.catalog.Lookup(c)
	if !known {
		// A capability the binary cannot run. Treat as disabled rather than
		// erroring — the orchestrator's own catalog Lookup guards this too.
		return agentic.CapabilityConfig{Enabled: false}, nil
	}

	var row models.AgentConfig
	found := true
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.GetByCapability(ctx, tx, orgID, c.String())
		if qErr != nil {
			if errors.Is(qErr, store.ErrNotFound) {
				found = false
				return nil
			}
			return qErr
		}
		row = r
		return nil
	})
	if err != nil {
		return agentic.CapabilityConfig{}, fmt.Errorf("resolve agent config: %w", err)
	}
	if !found {
		return agentic.CapabilityConfig{Enabled: d.DefaultEnabled, Config: defaultConfigJSON(d.DefaultConfig)}, nil
	}
	return agentic.CapabilityConfig{Enabled: row.Enabled, Config: defaultConfigJSON(row.Config)}, nil
}

// ---- helpers -----------------------------------------------------------

// validateAgentConfig enforces that the config is a JSON OBJECT and, for
// capabilities with a typed shape (foresight), that its tuning fields are
// well-formed. Returns the normalized raw bytes ("{}" for an empty/nil blob).
func validateAgentConfig(capability string, raw json.RawMessage) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []byte("{}"), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("%w: config must be valid JSON", ErrInvalidInput)
	}
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] != '{' {
		return nil, fmt.Errorf("%w: config must be a JSON object", ErrInvalidInput)
	}

	if capability == agentic.Foresight.String() {
		// Reject non-integer / negative thresholds at write time so the admin
		// cannot persist config the engine would silently discard. The runtime
		// parse (agentic.ParseForesightTuning) stays defensive regardless.
		var probe struct {
			ScheduleFloatDays *int `json:"schedule_float_days"`
			BudgetBurnPercent *int `json:"budget_burn_percent"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return nil, fmt.Errorf("%w: foresight tuning must be integers", ErrInvalidInput)
		}
		if probe.ScheduleFloatDays != nil && *probe.ScheduleFloatDays < 0 {
			return nil, fmt.Errorf("%w: schedule_float_days must be >= 0", ErrInvalidInput)
		}
		if probe.BudgetBurnPercent != nil && *probe.BudgetBurnPercent < 0 {
			return nil, fmt.Errorf("%w: budget_burn_percent must be >= 0", ErrInvalidInput)
		}
	}
	return raw, nil
}

// defaultConfigJSON normalizes a possibly-empty raw config to a non-empty JSON
// object so the wire form and the resolver never hand back nil/empty bytes.
func defaultConfigJSON(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}
