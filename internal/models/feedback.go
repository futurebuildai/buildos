package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Feedback categories (the widget's selector). Mirrored by the CHECK
// constraint in migration 020 — keep the two in sync.
const (
	FeedbackCategoryBug      = "bug"
	FeedbackCategoryIdea     = "idea"
	FeedbackCategoryFriction = "friction"
	FeedbackCategoryOther    = "other"
)

// Feedback triage statuses. A row is born "new"; an admin moves it
// through the triage lifecycle in-app, and the buildos-operations
// command center PATCHes status back so the submitter sees progress.
// Mirrored by the CHECK constraint in migration 020.
const (
	FeedbackStatusNew      = "new"
	FeedbackStatusTriaged  = "triaged"
	FeedbackStatusPlanned  = "planned"
	FeedbackStatusShipped  = "shipped"
	FeedbackStatusDeclined = "declined"
)

// Feedback is one operator-filed report (Phase 0b): a bug/idea/friction
// note submitted from the web console widget, plus its triage state.
// Context carries the client-captured environment (route, role,
// app_version, user_agent, viewport) — non-secret JSONB, never
// credentials. Message and TriageNote are Confidential (internal/pii).
type Feedback struct {
	ID         uuid.UUID       `json:"id"`
	OrgID      uuid.UUID       `json:"org_id"`
	UserSub    string          `json:"user_sub"` // caller's JWT subject (TEXT — matches the updated_by convention)
	Category   string          `json:"category"`
	Message    string          `json:"message"`
	Context    json.RawMessage `json:"context"`
	Status     string          `json:"status"`
	TriageNote string          `json:"triage_note"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
