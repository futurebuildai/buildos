package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// IntegrationCredentialStore manages the integration_credentials table:
// per-org 3rd-party API keys held in the encrypted BYOK vault (WS3).
//
// All methods take a pgx.Tx so the service layer can compose a mutation
// with the audit-log write inside one transaction (matches the pattern
// in setup.go, schedule.go). Stateless — safe to share.
type IntegrationCredentialStore struct{}

// NewIntegrationCredentialStore constructs an IntegrationCredentialStore.
func NewIntegrationCredentialStore() *IntegrationCredentialStore {
	return &IntegrationCredentialStore{}
}

// UpsertActiveCredentialParams is the input for UpsertActive. Ciphertext,
// Nonce, and KeyVersion are produced by cryptobox.Seal; Last4 is the
// last 4 chars of the cleartext key (UI display only). CreatedBy is the
// caller's OIDC subject.
type UpsertActiveCredentialParams struct {
	OrgID      uuid.UUID
	Provider   string
	Label      string
	Ciphertext []byte
	Nonce      []byte
	KeyVersion int
	Last4      string
	CreatedBy  string
}

// UpsertActive atomically rotates the active credential for an
// (org_id, provider) pair: it first deactivates any existing active
// row, then inserts a fresh active row. Both happen inside the caller's
// tx, so the partial unique index integration_credentials_active_uidx
// never sees two active rows. Returns the newly-inserted row.
func (s *IntegrationCredentialStore) UpsertActive(ctx context.Context, tx pgx.Tx, p UpsertActiveCredentialParams) (models.IntegrationCredential, error) {
	// Deactivate the prior active row (if any) first so the partial
	// unique index permits the new active insert below.
	if _, err := tx.Exec(ctx, `
		UPDATE integration_credentials
		SET is_active = false, updated_at = now()
		WHERE org_id = $1 AND provider = $2 AND is_active`,
		p.OrgID, p.Provider,
	); err != nil {
		return models.IntegrationCredential{}, fmt.Errorf("deactivate prior credential: %w", err)
	}

	var c models.IntegrationCredential
	err := tx.QueryRow(ctx, `
		INSERT INTO integration_credentials
			(org_id, provider, label, ciphertext, nonce, key_version, last4, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8)
		RETURNING id, org_id, provider, label, ciphertext, nonce, key_version, last4, is_active, created_by, created_at, updated_at`,
		p.OrgID, p.Provider, p.Label, p.Ciphertext, p.Nonce, p.KeyVersion, p.Last4, p.CreatedBy,
	).Scan(
		&c.ID, &c.OrgID, &c.Provider, &c.Label, &c.Ciphertext, &c.Nonce,
		&c.KeyVersion, &c.Last4, &c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return models.IntegrationCredential{}, fmt.Errorf("insert credential: %w", err)
	}
	return c, nil
}

// GetActiveByProvider returns the single active credential for an
// (org_id, provider) pair. Returns ErrNotFound when none is active —
// the resolver path (AnthropicKey / ResendKey) treats that as a
// soft-fail (unconfigured), not an error.
func (s *IntegrationCredentialStore) GetActiveByProvider(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, provider string) (models.IntegrationCredential, error) {
	var c models.IntegrationCredential
	err := tx.QueryRow(ctx, `
		SELECT id, org_id, provider, label, ciphertext, nonce, key_version, last4, is_active, created_by, created_at, updated_at
		FROM integration_credentials
		WHERE org_id = $1 AND provider = $2 AND is_active`,
		orgID, provider,
	).Scan(
		&c.ID, &c.OrgID, &c.Provider, &c.Label, &c.Ciphertext, &c.Nonce,
		&c.KeyVersion, &c.Last4, &c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.IntegrationCredential{}, ErrNotFound
		}
		return models.IntegrationCredential{}, fmt.Errorf("get active credential: %w", err)
	}
	return c, nil
}

// ListByOrg returns every credential (active + inactive) for an org,
// newest first. Used by the admin list endpoint to render metadata
// (provider, label, last4, is_active, timestamps) — the secret bytes
// are scanned but never emitted (json:"-" on the model).
func (s *IntegrationCredentialStore) ListByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]models.IntegrationCredential, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, provider, label, ciphertext, nonce, key_version, last4, is_active, created_by, created_at, updated_at
		FROM integration_credentials
		WHERE org_id = $1
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		return nil, fmt.Errorf("list credentials: %w", err)
	}
	defer rows.Close()

	var out []models.IntegrationCredential
	for rows.Next() {
		var c models.IntegrationCredential
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.Provider, &c.Label, &c.Ciphertext, &c.Nonce,
			&c.KeyVersion, &c.Last4, &c.IsActive, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeactivateByProvider flips is_active=false on the active credential
// for an (org_id, provider) pair and returns the number of rows
// affected (0 when nothing was active). The service maps 0 rows to
// ErrNotFound.
func (s *IntegrationCredentialStore) DeactivateByProvider(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, provider string) (int64, error) {
	cmd, err := tx.Exec(ctx, `
		UPDATE integration_credentials
		SET is_active = false, updated_at = now()
		WHERE org_id = $1 AND provider = $2 AND is_active`,
		orgID, provider)
	if err != nil {
		return 0, fmt.Errorf("deactivate credential: %w", err)
	}
	return cmd.RowsAffected(), nil
}
