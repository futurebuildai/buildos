package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// ShareLinkStore provides raw SQL for the client_update_share_links table
// (Chunk E — DAILY_REPORTS_CLIENT_UPDATES). Stateless; every mutation takes a
// pgx.Tx so the service composes it with its audit write inside one
// transaction (matches SetupStore's bootstrap-token methods). EVERY query
// filters by org_id on the operator surface — a cross-org id resolves to
// ErrNotFound, never another org's row.
//
// The PUBLIC resolution path (GetActiveByHash) is the one place a query is NOT
// org-scoped: the public route has no authenticated org. Security comes from
// the 256-bit token preimage + the uniform not-found, exactly like the
// bootstrap-token redemption path. The org_id is READ OUT of the resolved row,
// never supplied by the (unauthenticated) caller.
type ShareLinkStore struct{}

// NewShareLinkStore constructs a ShareLinkStore.
func NewShareLinkStore() *ShareLinkStore { return &ShareLinkStore{} }

const shareLinkColumns = `id, org_id, client_update_id, token_hash, expires_at,
	revoked_at, created_by, last_viewed_at, view_count, created_at`

func scanShareLink(row pgx.Row) (models.ShareLink, error) {
	var l models.ShareLink
	if err := row.Scan(
		&l.ID, &l.OrgID, &l.ClientUpdateID, &l.TokenHash, &l.ExpiresAt,
		&l.RevokedAt, &l.CreatedBy, &l.LastViewedAt, &l.ViewCount, &l.CreatedAt,
	); err != nil {
		return models.ShareLink{}, err
	}
	return l, nil
}

// CreateShareLinkParams stores a pre-hashed token. The cleartext is generated
// by the service and shown ONCE; only the hash lands here.
type CreateShareLinkParams struct {
	OrgID          uuid.UUID
	ClientUpdateID uuid.UUID
	TokenHash      string
	ExpiresAt      time.Time
	CreatedBy      uuid.UUID
}

// Create inserts a share link and returns it.
func (s *ShareLinkStore) Create(ctx context.Context, tx pgx.Tx, p CreateShareLinkParams) (models.ShareLink, error) {
	row := tx.QueryRow(ctx, `
		INSERT INTO client_update_share_links (
			org_id, client_update_id, token_hash, expires_at, created_by
		) VALUES ($1, $2, $3, $4, $5)
		RETURNING `+shareLinkColumns,
		p.OrgID, p.ClientUpdateID, p.TokenHash, p.ExpiresAt, p.CreatedBy,
	)
	l, err := scanShareLink(row)
	if err != nil {
		return models.ShareLink{}, fmt.Errorf("insert share link: %w", err)
	}
	return l, nil
}

// GetActiveByHash resolves the PUBLIC route's token: an unrevoked, unexpired
// link by its hash. Returns ErrNotFound when no row matches — even if the link
// exists but is revoked or expired — so the public handler emits a UNIFORM 404
// across all failure reasons (enumeration defense, mirroring the bootstrap
// token's GetActiveBootstrapTokenByHash). NOT org-scoped: the public caller has
// no org; the org_id is read out of the resolved row.
func (s *ShareLinkStore) GetActiveByHash(ctx context.Context, tx pgx.Tx, hash string, now time.Time) (models.ShareLink, error) {
	row := tx.QueryRow(ctx, `
		SELECT `+shareLinkColumns+`
		FROM client_update_share_links
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > $2`,
		hash, now,
	)
	l, err := scanShareLink(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ShareLink{}, ErrNotFound
		}
		return models.ShareLink{}, fmt.Errorf("get active share link: %w", err)
	}
	return l, nil
}

// GetByID returns a single share link, org-scoped. ErrNotFound on miss OR a
// cross-org id (the org_id predicate makes the two indistinguishable).
func (s *ShareLinkStore) GetByID(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (models.ShareLink, error) {
	row := tx.QueryRow(ctx, `
		SELECT `+shareLinkColumns+`
		FROM client_update_share_links
		WHERE id = $1 AND org_id = $2`,
		id, orgID,
	)
	l, err := scanShareLink(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ShareLink{}, ErrNotFound
		}
		return models.ShareLink{}, fmt.Errorf("get share link: %w", err)
	}
	return l, nil
}

// ListByClientUpdate returns a client update's share links newest-first,
// org-scoped (active/expired/revoked — the operator sees all of them).
func (s *ShareLinkStore) ListByClientUpdate(ctx context.Context, tx pgx.Tx, orgID, clientUpdateID uuid.UUID) ([]models.ShareLink, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+shareLinkColumns+`
		FROM client_update_share_links
		WHERE org_id = $1 AND client_update_id = $2
		ORDER BY created_at DESC`,
		orgID, clientUpdateID)
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	defer rows.Close()
	var out []models.ShareLink
	for rows.Next() {
		l, err := scanShareLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan share link: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// Revoke sets revoked_at on a link, org-scoped. The WHERE double-checks
// revoked_at IS NULL so a re-revoke is a no-op (RowsAffected=0 → ErrNotFound,
// which the service treats idempotently after a status re-check). Returns the
// updated row.
func (s *ShareLinkStore) Revoke(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID, now time.Time) (models.ShareLink, error) {
	row := tx.QueryRow(ctx, `
		UPDATE client_update_share_links
		SET revoked_at = $3
		WHERE id = $1 AND org_id = $2 AND revoked_at IS NULL
		RETURNING `+shareLinkColumns,
		id, orgID, now,
	)
	l, err := scanShareLink(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ShareLink{}, ErrNotFound
		}
		return models.ShareLink{}, fmt.Errorf("revoke share link: %w", err)
	}
	return l, nil
}

// TouchView records a best-effort page view: bumps view_count and stamps
// last_viewed_at. Carries NO PII (no IP, no UA). It runs in its OWN tx on the
// public read path (the read must succeed even if telemetry fails) so the
// service swallows any error from it.
func (s *ShareLinkStore) TouchView(ctx context.Context, tx pgx.Tx, id uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE client_update_share_links
		SET view_count = view_count + 1, last_viewed_at = $2
		WHERE id = $1`,
		id, now)
	if err != nil {
		return fmt.Errorf("touch share link view: %w", err)
	}
	return nil
}
