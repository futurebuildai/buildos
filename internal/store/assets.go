package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// AssetStore provides raw SQL for the assets table: the org/project-scoped
// registry of blobs in the per-fork object store (Chunk A). Stateless; every
// method takes a pgx.Tx so the service layer composes a mutation with its audit
// write inside one transaction (matches IntegrationCredentialStore / FieldStore).
//
// EVERY query filters by org_id (tenant isolation is per-query, per the store
// contract) — a cross-org id resolves to ErrNotFound, never another org's row.
type AssetStore struct{}

// NewAssetStore constructs an AssetStore.
func NewAssetStore() *AssetStore { return &AssetStore{} }

// InsertAssetParams is the input for Create (a pending asset row). ProjectID is
// optional (org-level assets are allowed). StorageKey is the opaque object key
// the service generated; UploadedBy is the caller's JWT sub.
type InsertAssetParams struct {
	OrgID       uuid.UUID
	ProjectID   *uuid.UUID
	StorageKey  string
	ContentType string
	SizeBytes   int64
	UploadedBy  string
}

// Create inserts a new asset row in status 'pending' and returns it.
func (s *AssetStore) Create(ctx context.Context, tx pgx.Tx, p InsertAssetParams) (models.Asset, error) {
	var a models.Asset
	err := tx.QueryRow(ctx, `
		INSERT INTO assets (org_id, project_id, storage_key, content_type, size_bytes, uploaded_by, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending')
		RETURNING id, org_id, project_id, storage_key, content_type, size_bytes, status, uploaded_by, checksum_sha256, created_at, confirmed_at`,
		p.OrgID, p.ProjectID, p.StorageKey, p.ContentType, p.SizeBytes, p.UploadedBy,
	).Scan(
		&a.ID, &a.OrgID, &a.ProjectID, &a.StorageKey, &a.ContentType, &a.SizeBytes,
		&a.Status, &a.UploadedBy, &a.ChecksumSHA256, &a.CreatedAt, &a.ConfirmedAt,
	)
	if err != nil {
		return models.Asset{}, fmt.Errorf("insert asset: %w", err)
	}
	return a, nil
}

// MarkReady transitions a pending asset to 'ready' (sets confirmed_at = now and
// the optional checksum). Org-scoped: a cross-org id matches no row and returns
// ErrNotFound. Only a 'pending' row transitions (idempotency/replay guard).
func (s *AssetStore) MarkReady(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID, checksum *string) (models.Asset, error) {
	var a models.Asset
	err := tx.QueryRow(ctx, `
		UPDATE assets
		SET status = 'ready', confirmed_at = now(), checksum_sha256 = COALESCE($3, checksum_sha256)
		WHERE id = $1 AND org_id = $2 AND status = 'pending'
		RETURNING id, org_id, project_id, storage_key, content_type, size_bytes, status, uploaded_by, checksum_sha256, created_at, confirmed_at`,
		id, orgID, checksum,
	).Scan(
		&a.ID, &a.OrgID, &a.ProjectID, &a.StorageKey, &a.ContentType, &a.SizeBytes,
		&a.Status, &a.UploadedBy, &a.ChecksumSHA256, &a.CreatedAt, &a.ConfirmedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Asset{}, ErrNotFound
		}
		return models.Asset{}, fmt.Errorf("mark asset ready: %w", err)
	}
	return a, nil
}

// MarkFailed transitions a pending asset to 'failed'. Org-scoped.
func (s *AssetStore) MarkFailed(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE assets SET status = 'failed'
		WHERE id = $1 AND org_id = $2 AND status = 'pending'`,
		id, orgID)
	if err != nil {
		return fmt.Errorf("mark asset failed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetByID returns a single asset by id, org-scoped. ErrNotFound on miss OR on a
// cross-org id (the org_id predicate makes the two indistinguishable, which is
// the desired enumeration-defense behavior).
func (s *AssetStore) GetByID(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (models.Asset, error) {
	var a models.Asset
	err := tx.QueryRow(ctx, `
		SELECT id, org_id, project_id, storage_key, content_type, size_bytes, status, uploaded_by, checksum_sha256, created_at, confirmed_at
		FROM assets
		WHERE id = $1 AND org_id = $2`,
		id, orgID,
	).Scan(
		&a.ID, &a.OrgID, &a.ProjectID, &a.StorageKey, &a.ContentType, &a.SizeBytes,
		&a.Status, &a.UploadedBy, &a.ChecksumSHA256, &a.CreatedAt, &a.ConfirmedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Asset{}, ErrNotFound
		}
		return models.Asset{}, fmt.Errorf("get asset: %w", err)
	}
	return a, nil
}

// ListByProject returns an org's assets for a project, newest-first. When
// readyOnly is true only 'ready' assets are returned (the default gallery view).
func (s *AssetStore) ListByProject(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, readyOnly bool) ([]models.Asset, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, project_id, storage_key, content_type, size_bytes, status, uploaded_by, checksum_sha256, created_at, confirmed_at
		FROM assets
		WHERE org_id = $1 AND project_id = $2
		  AND ($3::bool = false OR status = 'ready')
		ORDER BY created_at DESC`,
		orgID, projectID, readyOnly)
	if err != nil {
		return nil, fmt.Errorf("list assets by project: %w", err)
	}
	defer rows.Close()
	return scanAssets(rows)
}

// ListByIDs returns the ready, org-owned assets among ids (used by daily-report
// / client-update photo resolution in later chunks). Unknown / cross-org /
// non-ready ids are silently dropped — the caller compares len to detect gaps.
func (s *AssetStore) ListByIDs(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, ids []uuid.UUID) ([]models.Asset, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, project_id, storage_key, content_type, size_bytes, status, uploaded_by, checksum_sha256, created_at, confirmed_at
		FROM assets
		WHERE org_id = $1 AND id = ANY($2) AND status = 'ready'
		ORDER BY created_at DESC`,
		orgID, ids)
	if err != nil {
		return nil, fmt.Errorf("list assets by ids: %w", err)
	}
	defer rows.Close()
	return scanAssets(rows)
}

func scanAssets(rows pgx.Rows) ([]models.Asset, error) {
	var out []models.Asset
	for rows.Next() {
		var a models.Asset
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.ProjectID, &a.StorageKey, &a.ContentType, &a.SizeBytes,
			&a.Status, &a.UploadedBy, &a.ChecksumSHA256, &a.CreatedAt, &a.ConfirmedAt,
		); err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
