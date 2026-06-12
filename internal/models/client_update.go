package models

import (
	"time"

	"github.com/google/uuid"
)

// ClientUpdate lifecycle statuses.
const (
	ClientUpdateStatusDraft  = "draft"
	ClientUpdateStatusSent   = "sent"
	ClientUpdateStatusFailed = "failed"
)

// ClientUpdate is one homeowner-facing progress update (Chunk D —
// DAILY_REPORTS_CLIENT_UPDATES). It is the human-in-the-loop composer's row:
// an AI draft (Chunk C's ClientProgressUpdate) is persisted as 'draft', the
// operator edits the client-safe Subject/EditedBody and curates which photos
// the homeowner sees, then explicitly sends — the row flips to 'sent' and the
// email goes out via the existing Resend mailer post-commit. NEVER auto-sent.
//
// PII: RecipientEmail is Restricted — a snapshot of the homeowner address at
// send. It is OMITTED from list responses (json:"-") so it never appears in a
// portfolio/history payload; the send path uses it transiently and never logs
// it. AIDraft / EditedBody / Subject are Confidential (client-facing prose).
type ClientUpdate struct {
	ID            uuid.UUID   `json:"id"`
	OrgID         uuid.UUID   `json:"org_id"`
	ProjectID     uuid.UUID   `json:"project_id"`
	PeriodStart   time.Time   `json:"period_start"`
	PeriodEnd     time.Time   `json:"period_end"`
	Status        string      `json:"status"`
	AIDraft       *string     `json:"ai_draft,omitempty"`
	EditedBody    string      `json:"edited_body"`
	Subject       string      `json:"subject"`
	PhotoAssetIDs []uuid.UUID `json:"photo_asset_ids"`
	// RecipientEmail is the snapshot homeowner address at send. Restricted PII —
	// json:"-" keeps it out of every API response (the operator already knows
	// the address; the homeowner sees the email). Never logged.
	RecipientEmail *string    `json:"-"`
	CreatedBy      uuid.UUID  `json:"created_by"`
	SentBy         *uuid.UUID `json:"sent_by,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	SendError      *string    `json:"send_error,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}
