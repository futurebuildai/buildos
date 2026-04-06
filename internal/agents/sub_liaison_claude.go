package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
)

// subLiaisonClaudeSystemPrompt guides Claude for subcontractor message understanding.
const subLiaisonClaudeSystemPrompt = `You are a construction superintendent AI that understands subcontractor communications.

Your job: Parse inbound SMS/email messages from subcontractors and determine the correct action.

## Message Types You'll See
Subcontractors reply to automated confirmation requests with natural language. Examples:
- "Yeah we'll be there Monday" -> confirmation
- "Running about 2 hours behind" -> delay with estimate
- "We're at 75%, should wrap up tomorrow" -> progress update (75%)
- "Can't make it, truck broke down" -> delay, needs escalation
- "We're done, ready for inspection" -> progress update (100%)
- "Who do I talk to about the change order?" -> question, needs escalation

## Your Task
For each inbound message, analyze the intent and create appropriate feed cards.

Then, based on the situation:
1. For simple confirmations or progress updates: call write_feed_card with a low-priority informational card
2. For delays with schedule impact: call create_approval_card with an urgent card recommending follow-up action
3. For cancellations or serious issues: call create_approval_card with a critical card recommending immediate PM notification
4. For ambiguous messages or questions: call create_approval_card recommending the PM review the message

## Be construction-aware:
- "Running behind" usually means 1-4 hours, not days
- "Can't make it" for a scheduled task is more serious than a delay
- Weather-related delays are common and usually short-term
- Subcontractors often text informally — interpret generously
`

// parsedInbound represents Claude's structured understanding of an inbound message.
type parsedInbound struct {
	ProgressPercent *int   `json:"progress_percent,omitempty"`
	IsConfirmation  bool   `json:"is_confirmation"`
	IsDelay         bool   `json:"is_delay"`
	IsCancellation  bool   `json:"is_cancellation"`
	IsQuestion      bool   `json:"is_question"`
	EstimatedDelay  string `json:"estimated_delay,omitempty"`
	Summary         string `json:"summary"`
	Urgency         string `json:"urgency"` // "low", "medium", "high", "critical"
	SuggestedAction string `json:"suggested_action,omitempty"`
}

// handleInboundWithClaude uses Claude to understand a nuanced inbound message and take action.
// Falls back to keyword-based parsing on failure.
func (a *SubLiaisonAgent) handleInboundWithClaude(
	ctx context.Context,
	contactName string,
	contactCompany string,
	taskName string,
	taskID uuid.UUID,
	projectID uuid.UUID,
	orgID uuid.UUID,
	body string,
) error {
	userMessage := fmt.Sprintf(`Parse this inbound message from subcontractor %s (%s) regarding task "%s":

Message: "%s"

Determine the intent and create appropriate feed cards.`, contactName, contactCompany, taskName, body)

	projectCtx := ProjectContext{
		ProjectID: projectID,
		OrgID:     orgID,
		UserID:    uuid.Nil, // Agent user
	}

	result, err := a.claudeRunner.Run(ctx, subLiaisonClaudeSystemPrompt, userMessage, projectCtx)
	if err != nil {
		return fmt.Errorf("claude sub liaison reasoning failed: %w", err)
	}

	slog.Info("claude sub liaison reasoning completed",
		"task_id", taskID,
		"contact", contactName,
		"turns", result.Turns,
		"tools_used", result.ToolsUsed,
		"tokens", result.TotalTokens,
	)

	return nil
}

// draftFollowUpWithClaude uses Claude to generate a contextual follow-up message
// for a subcontractor who hasn't responded to a confirmation request.
func (a *SubLiaisonAgent) draftFollowUpWithClaude(
	ctx context.Context,
	contactName string,
	contactCompany string,
	taskName string,
	scheduledDate string,
	projectID uuid.UUID,
	orgID uuid.UUID,
) (string, error) {
	userMessage := fmt.Sprintf(`Draft a follow-up SMS to %s (%s) who hasn't responded to a confirmation request for task "%s" scheduled %s.

Keep it brief (under 160 chars for SMS), professional but friendly, and construction-appropriate.
Use the draft_message tool to create the follow-up.`,
		contactName, contactCompany, taskName, scheduledDate)

	projectCtx := ProjectContext{
		ProjectID: projectID,
		OrgID:     orgID,
		UserID:    uuid.Nil,
	}

	result, err := a.claudeRunner.Run(ctx, subLiaisonClaudeSystemPrompt, userMessage, projectCtx)
	if err != nil {
		return "", fmt.Errorf("claude follow-up draft failed: %w", err)
	}

	if result.Text != "" {
		return result.Text, nil
	}

	return "", fmt.Errorf("claude produced no follow-up text")
}

// normalizeBody lowercases a message body for keyword matching.
func normalizeBody(body string) string {
	b := []byte(body)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// containsAny checks if text contains any of the given keywords.
func containsAny(text string, keywords ...string) bool {
	for _, kw := range keywords {
		if len(kw) > len(text) {
			continue
		}
		for i := 0; i <= len(text)-len(kw); i++ {
			if text[i:i+len(kw)] == kw {
				return true
			}
		}
	}
	return false
}

// containsDelayIndicator checks for common delay-related keywords.
func containsDelayIndicator(text string) bool {
	return containsAny(text, "delay", "behind", "late", "can't make", "cant make", "not going to make", "push back")
}

// marshalParsedInbound serializes parsed inbound data for logging.
func marshalParsedInbound(p parsedInbound) string {
	b, err := json.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}
