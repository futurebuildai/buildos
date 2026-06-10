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

// FeedbackStore manages the feedback table (Phase 0b): operator-filed
// bug/idea/friction reports and their triage state.
//
// All methods take a pgx.Tx so the service layer composes a mutation
// with the audit-log write inside one transaction (matches
// agent_config.go). Stateless — safe to share.
type FeedbackStore struct{}

// NewFeedbackStore constructs a FeedbackStore.
func NewFeedbackStore() *FeedbackStore { return &FeedbackStore{} }

const feedbackColumns = `id, org_id, user_sub, category, message, context, status, triage_note, created_at, updated_at`

func scanFeedback(row pgx.Row) (models.Feedback, error) {
	var f models.Feedback
	err := row.Scan(&f.ID, &f.OrgID, &f.UserSub, &f.Category, &f.Message,
		&f.Context, &f.Status, &f.TriageNote, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

// InsertFeedbackParams is the input for Insert. Context is raw JSONB (a
// JSON object); nil is normalized to "{}". UserSub is the caller's JWT
// subject. Status always starts at "new" — triage is a separate verb.
type InsertFeedbackParams struct {
	OrgID    uuid.UUID
	UserSub  string
	Category string
	Message  string
	Context  []byte
}

// Insert writes a new feedback row (status "new") and returns it.
func (s *FeedbackStore) Insert(ctx context.Context, tx pgx.Tx, p InsertFeedbackParams) (models.Feedback, error) {
	cctx := p.Context
	if cctx == nil {
		cctx = []byte("{}")
	}
	f, err := scanFeedback(tx.QueryRow(ctx, `
		INSERT INTO feedback (org_id, user_sub, category, message, context)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING `+feedbackColumns,
		p.OrgID, p.UserSub, p.Category, p.Message, cctx))
	if err != nil {
		return models.Feedback{}, fmt.Errorf("insert feedback: %w", err)
	}
	return f, nil
}

// FeedbackPage bundles one page of feedback with the total count for
// pagination (mirrors ProspectsPage). Total is computed in the same
// query (window function) so the count and the page are consistent —
// the harvest poller can drain past any backlog without a truncation
// blind spot.
type FeedbackPage struct {
	Feedback   []models.Feedback
	Total      int
	Page       int
	PerPage    int
	TotalPages int
}

// ListByOrg returns one page of an org's feedback, newest first,
// optionally filtered by status ("" = all statuses).
func (s *FeedbackStore) ListByOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, status string, page, perPage int) (FeedbackPage, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 1
	}
	offset := (page - 1) * perPage

	rows, err := tx.Query(ctx, `
		SELECT `+feedbackColumns+`, COUNT(*) OVER() AS total_count
		FROM feedback
		WHERE org_id = $1
		  AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4`,
		orgID, status, perPage, offset)
	if err != nil {
		return FeedbackPage{}, fmt.Errorf("list feedback: %w", err)
	}
	defer rows.Close()

	out := FeedbackPage{Page: page, PerPage: perPage}
	for rows.Next() {
		var f models.Feedback
		if err := rows.Scan(&f.ID, &f.OrgID, &f.UserSub, &f.Category, &f.Message,
			&f.Context, &f.Status, &f.TriageNote, &f.CreatedAt, &f.UpdatedAt,
			&out.Total); err != nil {
			return FeedbackPage{}, fmt.Errorf("scan feedback: %w", err)
		}
		out.Feedback = append(out.Feedback, f)
	}
	if err := rows.Err(); err != nil {
		return FeedbackPage{}, fmt.Errorf("list feedback rows: %w", err)
	}
	if out.Total > 0 {
		out.TotalPages = (out.Total + perPage - 1) / perPage
	}
	return out, nil
}

// CountRecentByUser counts a user's submissions in the trailing window
// — the per-(org,user) submit throttle's read. The (org_id, status)
// index prefix bounds the scan to the org's rows.
func (s *FeedbackStore) CountRecentByUser(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, userSub string, window time.Duration) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM feedback
		WHERE org_id = $1 AND user_sub = $2 AND created_at > now() - make_interval(secs => $3)`,
		orgID, userSub, window.Seconds()).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count recent feedback: %w", err)
	}
	return n, nil
}

// UpdateStatus moves a feedback row through the triage lifecycle,
// org-scoped (a row belonging to another org is ErrNotFound, never
// touched). A nil triageNote keeps the existing note; a non-nil one
// replaces it (empty string clears it).
func (s *FeedbackStore) UpdateStatus(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID, status string, triageNote *string) (models.Feedback, error) {
	f, err := scanFeedback(tx.QueryRow(ctx, `
		UPDATE feedback SET
			status      = $3,
			triage_note = COALESCE($4, triage_note),
			updated_at  = now()
		WHERE org_id = $1 AND id = $2
		RETURNING `+feedbackColumns,
		orgID, id, status, triageNote))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.Feedback{}, ErrNotFound
		}
		return models.Feedback{}, fmt.Errorf("update feedback status: %w", err)
	}
	return f, nil
}
