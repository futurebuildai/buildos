package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// FeedCardWriter allows tools to write cards into the portfolio feed.
// Mirrors agents.FeedWriter to avoid import cycles. Go's implicit interfaces
// mean *service.FeedService satisfies both (via CreateCard).
type FeedCardWriter interface {
	CreateCard(ctx context.Context, card *models.FeedCard) (uuid.UUID, error)
}

// RegisterFeedTools registers feed card and approval-related tools.
func RegisterFeedTools(r *Registry, feedWriter FeedCardWriter) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "write_feed_card",
			Description: "Create an informational feed card that appears in the user's portfolio feed. Use for status updates, notifications, and non-actionable information.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"card_type":{"type":"string","description":"Type of card (e.g., daily_briefing, weather_alert, milestone)"},"headline":{"type":"string","description":"Short headline (max 120 chars)"},"body":{"type":"string","description":"Card body with details"},"priority":{"type":"string","enum":["critical","urgent","normal","low"],"description":"Priority level"},"horizon":{"type":"string","enum":["today","this_week","horizon"],"description":"When this is relevant"}},"required":["card_type","headline","body","priority","horizon"]}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var params struct {
				CardType string `json:"card_type"`
				Headline string `json:"headline"`
				Body     string `json:"body"`
				Priority string `json:"priority"`
				Horizon  string `json:"horizon"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			agentSource := "ChatAgent"
			card := &models.FeedCard{
				OrgID:       scope.OrgID,
				ProjectID:   &scope.ProjectID,
				CardType:    params.CardType,
				Title:       params.Headline,
				Body:        params.Body,
				Priority:    params.Priority,
				Headline:    &params.Headline,
				Horizon:     &params.Horizon,
				AgentSource: &agentSource,
				Status:      models.FeedStatusActive,
			}
			cardID, err := feedWriter.CreateCard(ctx, card)
			if err != nil {
				return "", fmt.Errorf("write feed card: %w", err)
			}
			return fmt.Sprintf(`{"success":true,"card_id":"%s"}`, cardID), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name: "create_approval_card",
			Description: `Create an approval card that requires human confirmation before an action is executed. Use this for ANY state-changing operation: updating task status, sending notifications, making schedule changes, approving change orders, etc.

The card appears in the user's feed with Approve/Reject buttons. When approved, the stored action is executed automatically.

Examples:
- "Recommend delaying framing start by 2 days due to rain" -> action_type: "delay_task", action_payload: {...}
- "Send confirmation request to electrician" -> action_type: "send_sms", action_payload: {...}
- "Order custom windows now (lead time closing)" -> action_type: "mark_ordered", action_payload: {...}`,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"headline":{"type":"string","description":"Clear description of what action is being recommended"},"body":{"type":"string","description":"Detailed reasoning: why this action is recommended, what happens if ignored"},"consequence":{"type":"string","description":"What happens if this action is NOT taken (e.g., '2-day critical path slip')"},"priority":{"type":"string","enum":["critical","urgent","normal","low"],"description":"Priority level"},"action_type":{"type":"string","description":"The tool/action to execute on approval (e.g., 'update_task_status', 'send_email', 'send_sms', 'recalculate_schedule')"},"action_payload":{"type":"object","description":"The exact parameters to pass to the action tool when approved"},"expires_hours":{"type":"integer","description":"Hours until this approval expires (default: 24)","default":24}},"required":["headline","body","priority","action_type","action_payload"]}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			var params struct {
				Headline      string          `json:"headline"`
				Body          string          `json:"body"`
				Consequence   string          `json:"consequence"`
				Priority      string          `json:"priority"`
				ActionType    string          `json:"action_type"`
				ActionPayload json.RawMessage `json:"action_payload"`
				ExpiresHours  int             `json:"expires_hours"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			if params.ExpiresHours <= 0 {
				params.ExpiresHours = 24
			}
			expiresAt := time.Now().UTC().Add(time.Duration(params.ExpiresHours) * time.Hour)

			agentSource := "ChatAgent"
			var consequence *string
			if params.Consequence != "" {
				consequence = &params.Consequence
			}

			// Store action_type + action_payload in engine_data for the feed card
			engineData, _ := json.Marshal(map[string]interface{}{
				"action_type":    params.ActionType,
				"action_payload": params.ActionPayload,
			})

			actionsJSON, _ := json.Marshal([]map[string]string{
				{"label": "Approve", "action_type": "approve_agent_action"},
				{"label": "Reject", "action_type": "reject_agent_action"},
				{"label": "Modify", "action_type": "modify"},
			})

			card := &models.FeedCard{
				ID:          uuid.New(),
				OrgID:       scope.OrgID,
				ProjectID:   &scope.ProjectID,
				CardType:    models.CardTypeAgentApproval,
				Title:       params.Headline,
				Body:        params.Body,
				Priority:    params.Priority,
				Headline:    &params.Headline,
				Consequence: consequence,
				AgentSource: &agentSource,
				ExpiresAt:   &expiresAt,
				EngineData:  engineData,
				Actions:     actionsJSON,
				Status:      models.FeedStatusActive,
			}

			cardID, err := feedWriter.CreateCard(ctx, card)
			if err != nil {
				return "", fmt.Errorf("write approval card: %w", err)
			}

			slog.Info("approval card created",
				"card_id", cardID,
				"action_type", params.ActionType,
				"expires_at", expiresAt,
			)

			return fmt.Sprintf(`{"success":true,"approval_card_id":"%s","action_type":"%s","expires_at":"%s"}`,
				cardID, params.ActionType, expiresAt.Format(time.RFC3339)), nil
		},
	})
}
