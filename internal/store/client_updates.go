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

// ClientUpdateStore provides raw SQL for the client_updates table (Chunk D —
// DAILY_REPORTS_CLIENT_UPDATES). Stateless; every method takes a pgx.Tx so the
// service composes a mutation with its audit write inside one transaction
// (matches AssetStore / FieldStore). EVERY query filters by org_id — a
// cross-org id resolves to ErrNotFound, never another org's row.
type ClientUpdateStore struct{}

// NewClientUpdateStore constructs a ClientUpdateStore.
func NewClientUpdateStore() *ClientUpdateStore { return &ClientUpdateStore{} }

// clientUpdateColumns is the canonical column list + order. Shared by every
// query so scanClientUpdate stays in lockstep with the SELECT/RETURNING list.
const clientUpdateColumns = `id, org_id, project_id, period_start, period_end,
	status, ai_draft, edited_body, subject, recipient_email, photo_asset_ids,
	created_by, sent_by, sent_at, send_error, created_at, updated_at`

func scanClientUpdate(row pgx.Row) (models.ClientUpdate, error) {
	var c models.ClientUpdate
	if err := row.Scan(
		&c.ID, &c.OrgID, &c.ProjectID, &c.PeriodStart, &c.PeriodEnd,
		&c.Status, &c.AIDraft, &c.EditedBody, &c.Subject, &c.RecipientEmail, &c.PhotoAssetIDs,
		&c.CreatedBy, &c.SentBy, &c.SentAt, &c.SendError, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return models.ClientUpdate{}, err
	}
	return c, nil
}

// CreateClientUpdateParams is the insert payload for a draft. status defaults
// to 'draft' in the schema. EditedBody is seeded with the AI draft so the
// operator edits from the AI text rather than a blank box.
type CreateClientUpdateParams struct {
	OrgID         uuid.UUID
	ProjectID     uuid.UUID
	PeriodStart   time.Time
	PeriodEnd     time.Time
	AIDraft       *string
	EditedBody    string
	Subject       string
	PhotoAssetIDs []uuid.UUID
	CreatedBy     uuid.UUID
}

// Create inserts a new client_update in status 'draft' and returns it.
func (s *ClientUpdateStore) Create(ctx context.Context, tx pgx.Tx, p CreateClientUpdateParams) (models.ClientUpdate, error) {
	ids := p.PhotoAssetIDs
	if ids == nil {
		ids = []uuid.UUID{}
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO client_updates (
			org_id, project_id, period_start, period_end, status,
			ai_draft, edited_body, subject, photo_asset_ids, created_by
		) VALUES ($1, $2, $3, $4, 'draft', $5, $6, $7, $8, $9)
		RETURNING `+clientUpdateColumns,
		p.OrgID, p.ProjectID, p.PeriodStart, p.PeriodEnd,
		p.AIDraft, p.EditedBody, p.Subject, ids, p.CreatedBy,
	)
	c, err := scanClientUpdate(row)
	if err != nil {
		return models.ClientUpdate{}, fmt.Errorf("insert client_update: %w", err)
	}
	return c, nil
}

// GetByID returns a single client_update, org-scoped. ErrNotFound on miss OR a
// cross-org id (the org_id predicate makes the two indistinguishable —
// enumeration-defense).
func (s *ClientUpdateStore) GetByID(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (models.ClientUpdate, error) {
	row := tx.QueryRow(ctx, `
		SELECT `+clientUpdateColumns+`
		FROM client_updates WHERE id = $1 AND org_id = $2`,
		id, orgID,
	)
	c, err := scanClientUpdate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ClientUpdate{}, ErrNotFound
		}
		return models.ClientUpdate{}, fmt.Errorf("get client_update: %w", err)
	}
	return c, nil
}

// ListByProject returns a project's client updates newest-first, org-scoped.
func (s *ClientUpdateStore) ListByProject(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID) ([]models.ClientUpdate, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+clientUpdateColumns+`
		FROM client_updates
		WHERE org_id = $1 AND project_id = $2
		ORDER BY created_at DESC`,
		orgID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list client_updates by project: %w", err)
	}
	defer rows.Close()
	return scanClientUpdates(rows)
}

// UpdateDraftParams is the operator-edit payload. A 'draft' OR 'failed' row is
// editable (the WHERE clause enforces it — a 'sent' row matches no row and
// returns ErrNotFound, which the service maps to ErrAlreadySent after a
// status re-check). Editing a 'failed' row resets it to 'draft' and clears the
// stale send_error, so a failed send is fixed-then-resent through the normal
// draft lifecycle (review finding: failed updates were re-sendable but a PATCH
// 404'd, an asymmetric/surprising state).
type UpdateDraftParams struct {
	OrgID         uuid.UUID
	ID            uuid.UUID
	Subject       string
	EditedBody    string
	PhotoAssetIDs []uuid.UUID
}

// UpdateDraft applies the operator edit to a draft (or failed) row and returns
// it. Scoped by id + org. A 'failed' row is reset to 'draft' with send_error
// cleared; a 'sent' row matches no row (→ ErrNotFound).
func (s *ClientUpdateStore) UpdateDraft(ctx context.Context, tx pgx.Tx, p UpdateDraftParams) (models.ClientUpdate, error) {
	ids := p.PhotoAssetIDs
	if ids == nil {
		ids = []uuid.UUID{}
	}
	row := tx.QueryRow(ctx, `
		UPDATE client_updates
		SET subject = $3, edited_body = $4, photo_asset_ids = $5,
		    status = 'draft', send_error = NULL, updated_at = now()
		WHERE id = $1 AND org_id = $2 AND status IN ('draft', 'failed')
		RETURNING `+clientUpdateColumns,
		p.ID, p.OrgID, p.Subject, p.EditedBody, ids,
	)
	c, err := scanClientUpdate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ClientUpdate{}, ErrNotFound
		}
		return models.ClientUpdate{}, fmt.Errorf("update client_update draft: %w", err)
	}
	return c, nil
}

// MarkSentParams snapshots the send. RecipientEmail is the homeowner address at
// send (Restricted — never logged). SentBy is the operator's users.id.
type MarkSentParams struct {
	OrgID          uuid.UUID
	ID             uuid.UUID
	RecipientEmail string
	SentBy         uuid.UUID
}

// MarkSent flips a non-sent row to 'sent', snapshots the recipient, and clears
// any prior send_error. Scoped by id + org; only a row not already 'sent'
// transitions (replay/idempotency guard — a re-send of a 'sent' row matches no
// row → ErrNotFound, mapped to ErrAlreadySent by the service after re-check).
func (s *ClientUpdateStore) MarkSent(ctx context.Context, tx pgx.Tx, p MarkSentParams) (models.ClientUpdate, error) {
	row := tx.QueryRow(ctx, `
		UPDATE client_updates
		SET status = 'sent', recipient_email = $3, sent_by = $4,
		    sent_at = now(), send_error = NULL, updated_at = now()
		WHERE id = $1 AND org_id = $2 AND status <> 'sent'
		RETURNING `+clientUpdateColumns,
		p.ID, p.OrgID, p.RecipientEmail, p.SentBy,
	)
	c, err := scanClientUpdate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ClientUpdate{}, ErrNotFound
		}
		return models.ClientUpdate{}, fmt.Errorf("mark client_update sent: %w", err)
	}
	return c, nil
}

// MarkFailed records that a send failed (mailer unconfigured / rejected) so the
// operator knows it did not go out. send_error never carries the recipient.
// Scoped by id + org. Returns the updated row.
func (s *ClientUpdateStore) MarkFailed(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID, sendErr string) (models.ClientUpdate, error) {
	row := tx.QueryRow(ctx, `
		UPDATE client_updates
		SET status = 'failed', send_error = $3, updated_at = now()
		WHERE id = $1 AND org_id = $2
		RETURNING `+clientUpdateColumns,
		id, orgID, sendErr,
	)
	c, err := scanClientUpdate(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ClientUpdate{}, ErrNotFound
		}
		return models.ClientUpdate{}, fmt.Errorf("mark client_update failed: %w", err)
	}
	return c, nil
}

func scanClientUpdates(rows pgx.Rows) ([]models.ClientUpdate, error) {
	var out []models.ClientUpdate
	for rows.Next() {
		c, err := scanClientUpdate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan client_update: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
