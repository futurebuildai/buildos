package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// AgentConfigStore manages the agents_config table: per-org, post-deploy
// configuration for the agentic capabilities (Phase 3a). One row per
// (org_id, capability); a row encodes an OVERRIDE of the in-code catalog default
// (enabled + tuning). Absence of a row = enabled-with-default.
//
// All methods take a pgx.Tx so the service layer composes a mutation with the
// audit-log write inside one transaction (matches integration_credentials.go).
// Stateless — safe to share.
type AgentConfigStore struct{}

// NewAgentConfigStore constructs an AgentConfigStore.
func NewAgentConfigStore() *AgentConfigStore { return &AgentConfigStore{} }

// UpsertAgentConfigParams is the input for Upsert. Config is raw JSONB (a JSON
// object); nil is normalized to "{}". UpdatedBy is the caller's OIDC subject.
type UpsertAgentConfigParams struct {
	OrgID      uuid.UUID
	Capability string
	Enabled    bool
	Config     []byte
	UpdatedBy  string
}

// Upsert writes the override row for an (org_id, capability) pair, inserting or
// (on the UNIQUE(org_id, capability) conflict) fully replacing enabled + config.
// Full-document semantics: both enabled and config are authoritative. Returns
// the written row.
func (s *AgentConfigStore) Upsert(ctx context.Context, tx pgx.Tx, p UpsertAgentConfigParams) (models.AgentConfig, error) {
	cfg := p.Config
	if cfg == nil {
		cfg = []byte("{}")
	}
	var a models.AgentConfig
	err := tx.QueryRow(ctx, `
		INSERT INTO agents_config (org_id, capability, enabled, config, updated_by)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		ON CONFLICT (org_id, capability) DO UPDATE SET
			enabled    = EXCLUDED.enabled,
			config     = EXCLUDED.config,
			updated_by = EXCLUDED.updated_by,
			updated_at = now()
		RETURNING id, org_id, capability, enabled, config, updated_by, created_at, updated_at`,
		p.OrgID, p.Capability, p.Enabled, cfg, p.UpdatedBy,
	).Scan(&a.ID, &a.OrgID, &a.Capability, &a.Enabled, &a.Config, &a.UpdatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return models.AgentConfig{}, fmt.Errorf("upsert agent config: %w", err)
	}
	return a, nil
}

// GetByCapability returns the override row for an (org_id, capability) pair, or
// ErrNotFound when no row exists (the resolver treats that as "use the catalog
// default" — enabled-with-default).
func (s *AgentConfigStore) GetByCapability(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, capability string) (models.AgentConfig, error) {
	var a models.AgentConfig
	err := tx.QueryRow(ctx, `
		SELECT id, org_id, capability, enabled, config, updated_by, created_at, updated_at
		FROM agents_config
		WHERE org_id = $1 AND capability = $2`,
		orgID, capability,
	).Scan(&a.ID, &a.OrgID, &a.Capability, &a.Enabled, &a.Config, &a.UpdatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.AgentConfig{}, ErrNotFound
		}
		return models.AgentConfig{}, fmt.Errorf("get agent config: %w", err)
	}
	return a, nil
}

// ListByOrg returns every override row for an org, ordered by capability for a
// stable admin listing.
func (s *AgentConfigStore) ListByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]models.AgentConfig, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, capability, enabled, config, updated_by, created_at, updated_at
		FROM agents_config
		WHERE org_id = $1
		ORDER BY capability`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list agent config: %w", err)
	}
	defer rows.Close()

	var out []models.AgentConfig
	for rows.Next() {
		var a models.AgentConfig
		if err := rows.Scan(&a.ID, &a.OrgID, &a.Capability, &a.Enabled, &a.Config, &a.UpdatedBy, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan agent config: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteByCapability removes the override row for an (org_id, capability) pair
// (resetting the org to the catalog default) and returns the number of rows
// deleted (0 when no override existed — an idempotent reset).
func (s *AgentConfigStore) DeleteByCapability(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, capability string) (int64, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM agents_config
		WHERE org_id = $1 AND capability = $2`,
		orgID, capability)
	if err != nil {
		return 0, fmt.Errorf("delete agent config: %w", err)
	}
	return tag.RowsAffected(), nil
}
