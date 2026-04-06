package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// FeedCard represents a user-facing notification/action card.
// Matches the feed_cards table from migration 003 + migration 010 enhancements.
type FeedCard struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	ProjectID    *uuid.UUID `json:"project_id,omitempty"`
	CardType     string     `json:"card_type"`
	Title        string     `json:"title"`
	Body         string     `json:"body,omitempty"`
	Priority     string     `json:"priority"`
	TargetUserID *uuid.UUID `json:"target_user_id,omitempty"`
	TargetRole   string     `json:"target_role,omitempty"`
	Actions      []byte     `json:"actions,omitempty"`  // JSONB
	Status       string     `json:"status"`
	ActionedAt   *time.Time `json:"actioned_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`

	// Agent-related fields (migration 010)
	Headline    *string          `json:"headline,omitempty"`
	Consequence *string          `json:"consequence,omitempty"`
	Horizon     *string          `json:"horizon,omitempty"`
	AgentSource *string          `json:"agent_source,omitempty"`
	Deadline    *time.Time       `json:"deadline,omitempty"`
	EngineData  json.RawMessage  `json:"engine_data,omitempty"`
	TaskID      *uuid.UUID       `json:"task_id,omitempty"`
}

// FeedPriority constants matching the feed_cards.priority column.
const (
	PriorityCritical = "critical"
	PriorityUrgent   = "urgent"
	PriorityNormal   = "normal"
	PriorityLow      = "low"
)

// FeedStatus constants matching the feed_cards.status column.
const (
	FeedStatusActive    = "active"
	FeedStatusDismissed = "dismissed"
	FeedStatusActioned  = "actioned"
	FeedStatusExpired   = "expired"
)

// CardType constants for different agent-generated cards.
const (
	CardTypeBriefing        = "daily_briefing"
	CardTypeProcurement     = "procurement_alert"
	CardTypeWeatherAlert    = "weather_alert"
	CardTypeSubConfirmation = "sub_confirmation"
	CardTypeProgress        = "progress_update"
	CardTypeAgentApproval   = "agent_approval"
)

// AgentType identifies which autonomous agent generated an action.
type AgentType string

const (
	AgentDailyFocus  AgentType = "daily_focus"
	AgentProcurement AgentType = "procurement"
	AgentSubLiaison  AgentType = "sub_liaison"
)

// CommunicationLog records agent actions for audit.
// Matches communication_logs table from migration 003.
type CommunicationLog struct {
	ID             uuid.UUID  `json:"id"`
	ProjectID      uuid.UUID  `json:"project_id"`
	TaskID         uuid.UUID  `json:"task_id"`
	ContactName    string     `json:"contact_name"`
	ContactPhone   string     `json:"contact_phone,omitempty"`
	MessageType    string     `json:"message_type"`
	MessageBody    string     `json:"message_body"`
	Status         string     `json:"status"`
	ResponseBody   string     `json:"response_body,omitempty"`
	ResponseParsed string     `json:"response_parsed,omitempty"`
	IdempotencyKey *uuid.UUID `json:"idempotency_key,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	ResponseAt     *time.Time `json:"response_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// FeedFilter holds query parameters for feed card listing.
type FeedFilter struct {
	Priority string
	Status   string
	Limit    int
	Offset   int
}

// AgentPendingAction represents a human-in-the-loop approval record.
// Created when an agent recommends a state-changing action via create_approval_card.
// The action is executed when a human approves it.
type AgentPendingAction struct {
	ID          uuid.UUID       `json:"id"`
	OrgID       uuid.UUID       `json:"org_id"`
	CardID      *uuid.UUID      `json:"card_id,omitempty"`
	AgentSource string          `json:"agent_source"`
	ActionType  string          `json:"action_type"`
	ActionData  json.RawMessage `json:"action_data"`
	Status      string          `json:"status"`
	ResolvedBy  *string         `json:"resolved_by,omitempty"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// PendingAction status constants.
const (
	PendingActionPending  = "pending"
	PendingActionApproved = "approved"
	PendingActionRejected = "rejected"
	PendingActionExpired  = "expired"
)
