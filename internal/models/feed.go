package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// FeedCard is a single notification rendered in the contractor's feed.
// Cards land via three paths:
//
//  1. Background River jobs (Sprint 5: DailyBriefing, ProcurementCheck).
//  2. A2A webhooks from The Brain (Sprint 4: review_material_quote,
//     review_labor_bid, update_schedule, delivery_confirmation,
//     create_feed_card, localblue.lead_captured).
//  3. Direct service writes (e.g., Kanban→CPM transition emits a card
//     "project XYZ moved into construction").
//
// Targeting is either user-specific (TargetUserID) or role-broadcast
// (TargetRole). At least one must be set; service-layer validators
// enforce this.
type FeedCard struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	ProjectID    *uuid.UUID      `json:"project_id,omitempty"`
	CardType     string          `json:"card_type"` // weather_alert, procurement, sub_confirmation, progress, ...
	Title        string          `json:"title"`
	Body         string          `json:"body,omitempty"`
	Priority     string          `json:"priority"` // critical, urgent, normal, low
	TargetUserID *uuid.UUID      `json:"target_user_id,omitempty"`
	TargetRole   *string         `json:"target_role,omitempty"`
	Actions      json.RawMessage `json:"actions,omitempty"` // JSONB: [{label, action_type, payload}]
	Status       string          `json:"status"`            // active, dismissed, actioned, expired
	ActionedAt   *time.Time      `json:"actioned_at,omitempty"`
	ExpiresAt    *time.Time      `json:"expires_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}

// FeedCard priorities. Match the `priority` CHECK constraint in
// migration 003 (currently free-text but these are the only producers).
const (
	FeedPriorityCritical = "critical"
	FeedPriorityUrgent   = "urgent"
	FeedPriorityNormal   = "normal"
	FeedPriorityLow      = "low"
)

// IsValidFeedPriority reports whether s is one of the allowed values.
func IsValidFeedPriority(s string) bool {
	switch s {
	case FeedPriorityCritical, FeedPriorityUrgent, FeedPriorityNormal, FeedPriorityLow:
		return true
	default:
		return false
	}
}
