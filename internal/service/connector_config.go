package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/authz"
	"github.com/futurebuildai/buildos/internal/connectors"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Audit actions for the connector registry (Phase 3b). Singular-noun.resource.verb.
const (
	auditActionConnectorConfigUpdated = "connector.config.updated"
	auditActionConnectorConfigReset   = "connector.config.reset"
)

// Effective-config source discriminators surfaced by ListEffective.
const (
	connectorSourceDefault  = "default"  // no override row (disabled)
	connectorSourceOverride = "override" // an explicit per-org row
)

// ConnectorService is the integration connector registry (Phase 3b). Two faces
// over the connectors_config table + the in-code built-in connector catalog:
//
//  1. Admin CRUD (ListEffective / Set / Reset) — the operator surface behind
//     /api/v1/admin/connectors, one-tx-per-mutation + audit.
//  2. ToolsFor — the per-request merge the Experience flow's buildRegistry
//     consults: the ENABLED connectors' tools, namespaced and MinRole-floored at
//     admin, ready to mount.
//
// The built-in catalog is the existence authority: Set/Reset reject an unknown
// connector (404); ToolsFor only ever mounts catalog connectors. Connectors are
// DEFAULT-OFF: absence of a row ⇒ disabled.
type ConnectorService struct {
	pool    *pgxpool.Pool
	store   *store.ConnectorConfigStore
	catalog map[string]connectors.Connector // by Name()
	order   []string                        // catalog names, stable order
	audit   AuditRecorder
	logger  *slog.Logger
}

// NewConnectorService wires the store + the built-in connector catalog + audit.
// A nil AuditRecorder falls back to the no-op; a nil logger becomes slog.Default().
func NewConnectorService(pool *pgxpool.Pool, st *store.ConnectorConfigStore, audit AuditRecorder, logger *slog.Logger) *ConnectorService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	catalog := make(map[string]connectors.Connector)
	var order []string
	for _, c := range connectors.Builtins() {
		catalog[c.Name()] = c
		order = append(order, c.Name())
	}
	return &ConnectorService{
		pool:    pool,
		store:   st,
		catalog: catalog,
		order:   order,
		audit:   audit,
		logger:  logger,
	}
}

// ---- Face 1: admin CRUD ------------------------------------------------

// EffectiveConnector is one connector's effective config for an org: the built-in
// catalog metadata merged with any override row. Connectors are default-OFF, so
// Enabled is false unless an explicit row enables it.
type EffectiveConnector struct {
	Connector   string          `json:"connector"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config"`
	Source      string          `json:"source"` // "default" (no row) | "override"
	UpdatedBy   string          `json:"updated_by,omitempty"`
	UpdatedAt   *time.Time      `json:"updated_at,omitempty"`
}

// SetConnectorInput is the validated input for Set. OrgID + UserSub come from
// JWT claims (never the request body).
type SetConnectorInput struct {
	OrgID         uuid.UUID
	ConnectorName string
	Enabled       bool
	Config        json.RawMessage
	UserSub       string
}

// ListEffective returns every built-in connector with its effective config for
// the org (default-OFF, or an override row). Catalog-driven, so an orphaned row
// for a connector the binary no longer ships is simply not listed.
func (s *ConnectorService) ListEffective(ctx context.Context, orgID uuid.UUID) ([]EffectiveConnector, error) {
	var rows []models.ConnectorConfig
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.ListByOrg(ctx, tx, orgID)
		if qErr != nil {
			return qErr
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list effective connectors: %w", err)
	}

	byName := make(map[string]models.ConnectorConfig, len(rows))
	for _, r := range rows {
		byName[r.ConnectorName] = r
	}

	out := make([]EffectiveConnector, 0, len(s.order))
	for _, name := range s.order {
		c := s.catalog[name]
		eff := EffectiveConnector{
			Connector:   name,
			Description: c.Description(),
			Enabled:     false, // DEFAULT-OFF
			Config:      json.RawMessage("{}"),
			Source:      connectorSourceDefault,
		}
		if row, ok := byName[name]; ok {
			eff.Enabled = row.Enabled
			eff.Config = defaultConfigJSON(row.Config)
			eff.Source = connectorSourceOverride
			eff.UpdatedBy = row.UpdatedBy
			ts := row.UpdatedAt
			eff.UpdatedAt = &ts
		}
		out = append(out, eff)
	}
	return out, nil
}

// Set upserts the override (enabled + config) for a connector. Validates the
// connector is in the catalog (ErrNotFound → 404) and the config is a JSON object
// (ErrInvalidInput → 400) BEFORE any DB write, then upserts + audits in one tx.
func (s *ConnectorService) Set(ctx context.Context, in SetConnectorInput) (models.ConnectorConfig, error) {
	if in.OrgID == uuid.Nil {
		return models.ConnectorConfig{}, fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	if _, ok := s.catalog[in.ConnectorName]; !ok {
		return models.ConnectorConfig{}, fmt.Errorf("%w: unknown connector %q", ErrNotFound, in.ConnectorName)
	}
	cfg, err := validateConnectorConfig(in.Config)
	if err != nil {
		return models.ConnectorConfig{}, err
	}

	var out models.ConnectorConfig
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, qErr := s.store.Upsert(ctx, tx, store.UpsertConnectorConfigParams{
			OrgID:         in.OrgID,
			ConnectorName: in.ConnectorName,
			Enabled:       in.Enabled,
			Config:        cfg,
			UpdatedBy:     in.UserSub,
		})
		if qErr != nil {
			return qErr
		}
		out = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       auditActionConnectorConfigUpdated,
			ResourceType: AuditResourceConnectorConfig,
			ResourceID:   row.ID,
			Metadata:     marshalAudit(map[string]any{"connector": in.ConnectorName, "enabled": in.Enabled, "config": json.RawMessage(cfg)}),
		})
		return nil
	})
	if err != nil {
		return models.ConnectorConfig{}, fmt.Errorf("set connector config: %w", err)
	}
	return out, nil
}

// Reset removes the override row for a connector, resetting it to default-OFF.
// Idempotent: nil whether or not a row existed; audits (connector.config.reset)
// only when a row was actually deleted. Validates the connector is in the catalog.
func (s *ConnectorService) Reset(ctx context.Context, orgID uuid.UUID, connectorName, userSub string) error {
	if orgID == uuid.Nil {
		return fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	if _, ok := s.catalog[connectorName]; !ok {
		return fmt.Errorf("%w: unknown connector %q", ErrNotFound, connectorName)
	}
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		affected, qErr := s.store.DeleteByName(ctx, tx, orgID, connectorName)
		if qErr != nil {
			return qErr
		}
		if affected == 0 {
			return nil // idempotent: nothing to delete, nothing to audit
		}
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       auditActionConnectorConfigReset,
			ResourceType: AuditResourceConnectorConfig,
			ResourceID:   orgID,
			Metadata:     marshalAudit(map[string]any{"connector": connectorName}),
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("reset connector config: %w", err)
	}
	return nil
}

// ---- Face 2: the assistant merge --------------------------------------

// ToolsFor returns the ENABLED connectors' tools for a caller, namespaced and
// MinRole-floored at admin, ready to merge into the per-request registry. A
// connectors_config read error is returned so buildRegistry can fail closed
// (mount zero connector tools); a single connector whose BuildTools errors is
// logged and skipped. Returns ONLY connector tools (never internal/agentic ones).
func (s *ConnectorService) ToolsFor(ctx context.Context, c connectors.Caller) ([]agentic.Tool, error) {
	var rows []models.ConnectorConfig
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.ListByOrg(ctx, tx, c.OrgID)
		if qErr != nil {
			return qErr
		}
		rows = r
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("resolve enabled connectors: %w", err)
	}

	enabled := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.Enabled {
			enabled[r.ConnectorName] = true
		}
	}

	var out []agentic.Tool
	for _, name := range s.order {
		if !enabled[name] {
			continue // default-OFF / disabled
		}
		conn := s.catalog[name]
		tools, bErr := conn.BuildTools(ctx, c)
		if bErr != nil {
			// One bad connector must not take down the whole leg.
			s.logger.WarnContext(ctx, "connector BuildTools failed; skipping connector",
				slog.String("connector", name), slog.Any("error", bErr))
			continue
		}
		for _, t := range tools {
			// Defense-in-depth for the connector seam (3b-ii external connectors):
			// a tool with a nil executor or an empty/illegal name must never be
			// mounted (it would panic on invocation / advertise a junk name).
			if t.Executor == nil {
				s.logger.WarnContext(ctx, "connector tool has a nil executor; dropping",
					slog.String("connector", name), slog.String("tool", t.Spec.Name))
				continue
			}
			if t.Spec.Name == "" {
				s.logger.WarnContext(ctx, "connector tool has an empty name; dropping",
					slog.String("connector", name))
				continue
			}
			namespaced := connectors.NamespaceToolName(name, t.Spec.Name)
			if !connectors.ValidToolName(namespaced) {
				s.logger.WarnContext(ctx, "connector tool name invalid after namespacing; dropping",
					slog.String("connector", name), slog.String("tool", t.Spec.Name))
				continue
			}
			t.Spec.Name = namespaced
			t.MinRole = floorConnectorMinRole(t.MinRole)
			out = append(out, t)
		}
	}
	return out, nil
}

// ---- helpers -----------------------------------------------------------

// floorConnectorMinRole raises a connector tool's MinRole to admin when it
// declares anything lower (or an unknown/empty role). A connector tool is never
// available below admin in 3b (BuildOS cannot vouch for a connector's effects).
func floorConnectorMinRole(declared string) string {
	if authz.RoleRank(declared) < authz.RoleRank(authz.RoleAdmin) {
		return authz.RoleAdmin
	}
	return declared
}

// validateConnectorConfig enforces that the config is a JSON OBJECT (or empty),
// returning the normalized raw bytes. 3b-i stores but does not interpret connector
// config (forward-compat for 3b-ii), so this is just the shared object check.
func validateConnectorConfig(raw json.RawMessage) ([]byte, error) {
	return validateConfigObject(raw)
}
