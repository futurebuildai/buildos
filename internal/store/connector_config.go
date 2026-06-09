package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// ConnectorConfigStore manages the connectors_config table: per-org, post-deploy
// configuration for the integration connectors (Phase 3b). One row per
// (org_id, connector_name); connectors are DEFAULT-OFF, so a row encodes an
// explicit opt-in. Mirrors AgentConfigStore.
//
// All methods take a pgx.Tx so the service composes a mutation with the audit
// write in one transaction. Stateless — safe to share.
type ConnectorConfigStore struct{}

// NewConnectorConfigStore constructs a ConnectorConfigStore.
func NewConnectorConfigStore() *ConnectorConfigStore { return &ConnectorConfigStore{} }

// UpsertConnectorConfigParams is the input for Upsert. Config is raw JSONB (a
// JSON object); nil is normalized to "{}". UpdatedBy is the caller's OIDC subject.
type UpsertConnectorConfigParams struct {
	OrgID         uuid.UUID
	ConnectorName string
	Kind          string // "" => keep existing on conflict / 'builtin' on insert
	Enabled       bool
	Config        []byte
	UpdatedBy     string
}

// Upsert writes the row for an (org_id, connector_name) pair, inserting or (on
// the UNIQUE conflict) fully replacing kind + enabled + config. Returns the
// written row. An empty Kind defaults to 'builtin' on insert and is preserved on
// conflict (COALESCE keeps the existing kind).
func (s *ConnectorConfigStore) Upsert(ctx context.Context, tx pgx.Tx, p UpsertConnectorConfigParams) (models.ConnectorConfig, error) {
	cfg := p.Config
	if cfg == nil {
		cfg = []byte("{}")
	}
	kind := p.Kind
	if kind == "" {
		kind = "builtin"
	}
	var c models.ConnectorConfig
	err := tx.QueryRow(ctx, `
		INSERT INTO connectors_config (org_id, connector_name, kind, enabled, config, updated_by)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		ON CONFLICT (org_id, connector_name) DO UPDATE SET
			kind       = EXCLUDED.kind,
			enabled    = EXCLUDED.enabled,
			config     = EXCLUDED.config,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
		RETURNING id, org_id, connector_name, kind, enabled, config, updated_by, created_at, updated_at`,
		p.OrgID, p.ConnectorName, kind, p.Enabled, cfg, p.UpdatedBy,
	).Scan(&c.ID, &c.OrgID, &c.ConnectorName, &c.Kind, &c.Enabled, &c.Config, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return models.ConnectorConfig{}, fmt.Errorf("upsert connector config: %w", err)
	}
	return c, nil
}

// GetByName returns the row for an (org_id, connector_name) pair, or ErrNotFound
// when no row exists (the resolver treats that as "default-OFF / disabled").
func (s *ConnectorConfigStore) GetByName(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, connectorName string) (models.ConnectorConfig, error) {
	var c models.ConnectorConfig
	err := tx.QueryRow(ctx, `
		SELECT id, org_id, connector_name, kind, enabled, config, updated_by, created_at, updated_at
		FROM connectors_config
		WHERE org_id = $1 AND connector_name = $2`,
		orgID, connectorName,
	).Scan(&c.ID, &c.OrgID, &c.ConnectorName, &c.Kind, &c.Enabled, &c.Config, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ConnectorConfig{}, ErrNotFound
		}
		return models.ConnectorConfig{}, fmt.Errorf("get connector config: %w", err)
	}
	return c, nil
}

// ListByOrg returns every connector row for an org, ordered by connector_name.
func (s *ConnectorConfigStore) ListByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]models.ConnectorConfig, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, connector_name, kind, enabled, config, updated_by, created_at, updated_at
		FROM connectors_config
		WHERE org_id = $1
		ORDER BY connector_name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list connector config: %w", err)
	}
	defer rows.Close()

	var out []models.ConnectorConfig
	for rows.Next() {
		var c models.ConnectorConfig
		if err := rows.Scan(&c.ID, &c.OrgID, &c.ConnectorName, &c.Kind, &c.Enabled, &c.Config, &c.UpdatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan connector config: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteByName removes the row for an (org_id, connector_name) pair (resetting to
// default-OFF) and returns the number of rows deleted (0 when none existed).
func (s *ConnectorConfigStore) DeleteByName(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, connectorName string) (int64, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM connectors_config
		WHERE org_id = $1 AND connector_name = $2`,
		orgID, connectorName)
	if err != nil {
		return 0, fmt.Errorf("delete connector config: %w", err)
	}
	return tag.RowsAffected(), nil
}
