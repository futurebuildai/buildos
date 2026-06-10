package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// jsonContainsNUL reports whether any string value or key in a JSON
// blob contains U+0000 — Postgres JSONB rejects the backslash-u-0000 escape
// (SQLSTATE 22P05), so it must be a 400 at validation time, not a 500
// at insert time. Decoding (rather than scanning the raw bytes for the
// escape sequence) avoids false positives on a literal backslash-u
// ("\\u0000") inside a value.
func jsonContainsNUL(raw []byte) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false // shape errors are caught by validateConfigObject
	}
	return anyContainsNUL(v)
}

func anyContainsNUL(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.ContainsRune(t, 0)
	case map[string]any:
		for k, val := range t {
			if strings.ContainsRune(k, 0) || anyContainsNUL(val) {
				return true
			}
		}
	case []any:
		for _, val := range t {
			if anyContainsNUL(val) {
				return true
			}
		}
	}
	return false
}

// Audit actions for the feedback loop (Phase 0b). Past-tense per the
// repo convention (agent.config.updated, feed.card.dismissed). The
// feedback → triage → ship loop reconstructs a report's history from
// these rows (audit metadata carries category/status, NEVER the
// message body — free-text content stays out of the audit log per the
// L-6 posture finding).
const (
	auditActionFeedbackSubmitted = "feedback.submitted"
	auditActionFeedbackTriaged   = "feedback.triaged"
)

// ErrRateLimited is the service-level throttle sentinel (handler maps
// it to 429 RATE_LIMITED + Retry-After). Distinct from the per-IP
// middleware limiter: this one is per-(org,user) and survives IP
// rotation.
var ErrRateLimited = errors.New("feedback: rate limited")

// Feedback validation bounds. Message/note caps mirror the CHECK
// constraints in migration 020; the context cap keeps a hostile client
// from stuffing megabytes of JSONB through the widget's side channel.
// The submit throttle bounds DB growth from a hostile authenticated
// flooder (the per-IP middleware limiter alone is too permissive for a
// write surface every role can reach).
const (
	feedbackMessageMaxChars  = 4000
	feedbackContextMaxBytes  = 4096
	feedbackListDefaultPage  = 100
	feedbackListMaxPerPage   = 500
	feedbackSubmitWindow     = time.Hour
	feedbackSubmitMaxPerHour = 20
)

// validFeedbackCategories / validFeedbackStatuses mirror migration
// 020's CHECK constraints so bad input 400s before any DB write.
var validFeedbackCategories = map[string]bool{
	models.FeedbackCategoryBug:      true,
	models.FeedbackCategoryIdea:     true,
	models.FeedbackCategoryFriction: true,
	models.FeedbackCategoryOther:    true,
}

var validFeedbackStatuses = map[string]bool{
	models.FeedbackStatusNew:      true,
	models.FeedbackStatusTriaged:  true,
	models.FeedbackStatusPlanned:  true,
	models.FeedbackStatusShipped:  true,
	models.FeedbackStatusDeclined: true,
}

// FeedbackService owns the feedback loop's write paths (Phase 0b):
// any authenticated user submits via the web-console widget; admins
// (and the buildos-operations command center, holding an admin token)
// list + triage. One tx per mutation + audit, matching setup.go.
type FeedbackService struct {
	pool  *pgxpool.Pool
	store *store.FeedbackStore
	audit AuditRecorder
}

// NewFeedbackService wires the store + audit. A nil AuditRecorder
// falls back to the no-op (matches NewAgentConfigService).
func NewFeedbackService(pool *pgxpool.Pool, st *store.FeedbackStore, audit AuditRecorder) *FeedbackService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &FeedbackService{pool: pool, store: st, audit: audit}
}

// SubmitFeedbackInput is the validated input for Submit. OrgID +
// UserSub come from JWT claims (never the request body). Context is
// the widget's client-captured environment blob.
type SubmitFeedbackInput struct {
	OrgID    uuid.UUID
	UserSub  string
	Category string
	Message  string
	Context  json.RawMessage
}

// Submit validates and persists one feedback report (status "new") and
// audits feedback.submitted, in one tx.
func (s *FeedbackService) Submit(ctx context.Context, in SubmitFeedbackInput) (models.Feedback, error) {
	if in.OrgID == uuid.Nil {
		return models.Feedback{}, fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	if !validFeedbackCategories[in.Category] {
		return models.Feedback{}, fmt.Errorf("%w: unknown category %q", ErrInvalidInput, in.Category)
	}
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		return models.Feedback{}, fmt.Errorf("%w: message is required", ErrInvalidInput)
	}
	if len([]rune(msg)) > feedbackMessageMaxChars {
		return models.Feedback{}, fmt.Errorf("%w: message exceeds %d characters", ErrInvalidInput, feedbackMessageMaxChars)
	}
	// Postgres TEXT/JSONB cannot store U+0000 (SQLSTATE 22021/22P05) —
	// reject it here so a well-formed-JSON-with-NUL body is a 400, not
	// a 500 with Sentry noise.
	if strings.ContainsRune(msg, 0) {
		return models.Feedback{}, fmt.Errorf("%w: message must not contain NUL", ErrInvalidInput)
	}
	cctx, err := validateConfigObject(in.Context)
	if err != nil {
		return models.Feedback{}, fmt.Errorf("%w: context must be a JSON object", ErrInvalidInput)
	}
	if len(cctx) > feedbackContextMaxBytes {
		return models.Feedback{}, fmt.Errorf("%w: context exceeds %d bytes", ErrInvalidInput, feedbackContextMaxBytes)
	}
	if jsonContainsNUL(cctx) {
		return models.Feedback{}, fmt.Errorf("%w: context must not contain NUL", ErrInvalidInput)
	}

	var out models.Feedback
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Per-(org,user) submit throttle, read in the same tx as the
		// insert. Advisory (concurrent submits can nudge past the cap)
		// — the goal is bounding flood growth, not strict quota.
		recent, qErr := s.store.CountRecentByUser(ctx, tx, in.OrgID, in.UserSub, feedbackSubmitWindow)
		if qErr != nil {
			return qErr
		}
		if recent >= feedbackSubmitMaxPerHour {
			return fmt.Errorf("%w: more than %d submissions in the last hour", ErrRateLimited, feedbackSubmitMaxPerHour)
		}
		row, qErr := s.store.Insert(ctx, tx, store.InsertFeedbackParams{
			OrgID:    in.OrgID,
			UserSub:  in.UserSub,
			Category: in.Category,
			Message:  msg,
			Context:  cctx,
		})
		if qErr != nil {
			return qErr
		}
		out = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       auditActionFeedbackSubmitted,
			ResourceType: AuditResourceFeedback,
			ResourceID:   row.ID,
			// Category only — the message body is Confidential free
			// text and stays out of audit metadata (posture L-6).
			Metadata: marshalAudit(map[string]any{"category": in.Category}),
		})
		return nil
	})
	if err != nil {
		return models.Feedback{}, fmt.Errorf("submit feedback: %w", err)
	}
	return out, nil
}

// ListFeedbackInput controls a paginated feedback listing (mirrors
// ListProspectsInput).
type ListFeedbackInput struct {
	OrgID   uuid.UUID
	Status  string // optional; must be a valid status when non-empty
	Page    int    // 1-based; defaults to 1 when <1
	PerPage int    // defaults to 100; clamped to [1,500]
}

// ListForAdmin returns one page of the org's feedback, newest first,
// optionally filtered by status, with the total count — the harvest
// surface the buildos-operations command center pages through (no
// silent truncation: Total/TotalPages tell the poller when to keep
// going).
func (s *FeedbackService) ListForAdmin(ctx context.Context, in ListFeedbackInput) (store.FeedbackPage, error) {
	if in.Status != "" && !validFeedbackStatuses[in.Status] {
		return store.FeedbackPage{}, fmt.Errorf("%w: unknown status %q", ErrInvalidInput, in.Status)
	}
	if in.PerPage < 1 {
		in.PerPage = feedbackListDefaultPage
	}
	if in.PerPage > feedbackListMaxPerPage {
		in.PerPage = feedbackListMaxPerPage
	}
	var page store.FeedbackPage
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		p, qErr := s.store.ListByOrg(ctx, tx, in.OrgID, in.Status, in.Page, in.PerPage)
		if qErr != nil {
			return qErr
		}
		page = p
		return nil
	})
	if err != nil {
		return store.FeedbackPage{}, fmt.Errorf("list feedback for admin: %w", err)
	}
	return page, nil
}

// TriageFeedbackInput is the validated input for Triage. A nil
// TriageNote keeps the existing note; a non-nil one replaces it.
type TriageFeedbackInput struct {
	OrgID      uuid.UUID
	ID         uuid.UUID
	Status     string
	TriageNote *string
	UserSub    string
}

// Triage moves a report through the lifecycle (org-scoped; a foreign
// row is ErrNotFound) and audits feedback.triaged, in one tx.
func (s *FeedbackService) Triage(ctx context.Context, in TriageFeedbackInput) (models.Feedback, error) {
	if !validFeedbackStatuses[in.Status] {
		return models.Feedback{}, fmt.Errorf("%w: unknown status %q", ErrInvalidInput, in.Status)
	}
	if in.TriageNote != nil && len([]rune(*in.TriageNote)) > feedbackMessageMaxChars {
		return models.Feedback{}, fmt.Errorf("%w: triage_note exceeds %d characters", ErrInvalidInput, feedbackMessageMaxChars)
	}
	if in.TriageNote != nil && strings.ContainsRune(*in.TriageNote, 0) {
		return models.Feedback{}, fmt.Errorf("%w: triage_note must not contain NUL", ErrInvalidInput)
	}

	var out models.Feedback
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, qErr := s.store.UpdateStatus(ctx, tx, in.OrgID, in.ID, in.Status, in.TriageNote)
		if qErr != nil {
			return qErr
		}
		out = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.UserSub,
			Action:       auditActionFeedbackTriaged,
			ResourceType: AuditResourceFeedback,
			ResourceID:   row.ID,
			// Status transition only — the note is Confidential free
			// text and stays out of audit metadata (posture L-6).
			Metadata: marshalAudit(map[string]any{
				"status":   in.Status,
				"has_note": in.TriageNote != nil && *in.TriageNote != "",
			}),
		})
		return nil
	})
	if err != nil {
		// Surface the service sentinel for the handler's 404 leg
		// (foreign-org or unknown id — indistinguishable on purpose).
		if errors.Is(err, store.ErrNotFound) {
			return models.Feedback{}, fmt.Errorf("triage feedback: %w", ErrNotFound)
		}
		return models.Feedback{}, fmt.Errorf("triage feedback: %w", err)
	}
	return out, nil
}
