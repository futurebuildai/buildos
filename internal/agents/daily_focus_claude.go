package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/google/uuid"
)

// dailyFocusSystemPrompt is the system prompt for Claude-powered daily focus.
const dailyFocusSystemPrompt = `You are the AI superintendent analyzing a construction project for the daily morning briefing.

Your job: identify the TOP 3-5 things requiring human attention today, ranked by urgency.

## For each item you identify, do one of the following:
1. If it requires a human decision or approval -> call create_approval_card with specific recommended action
2. If it's informational but important -> call write_feed_card

## Categories to analyze:
- **Weather Impact**: Check the weather forecast. If rain/storms are expected, recommend delays for exterior work or protective measures.
- **Critical Path Tasks**: Tasks on the critical path that start today or this week. Any blockers?
- **Procurement Deadlines**: Items with order deadlines approaching. Recommend ordering if lead time windows are closing.
- **Subcontractor Confirmations**: Subs that haven't confirmed for upcoming tasks. Recommend resending or finding alternatives.
- **Schedule Risks**: Tasks that are falling behind or have dependencies at risk.

## Output Format:
After creating the relevant cards, provide a brief summary paragraph for the daily briefing email.
Keep it direct and construction-industry appropriate. No fluff.

## Important:
- Always use create_approval_card for actions that modify project state
- Be specific in your recommendations (include names, dates, dollar amounts when available)
- Flag the single most critical item as priority "critical"
`

// WithClaudeRunner sets the AgentRunner for Claude-powered reasoning.
// When set, processProject uses Claude to generate actionable approval cards
// in addition to the text briefing.
func (a *DailyFocusAgent) WithClaudeRunner(runner *AgentRunner) *DailyFocusAgent {
	a.claudeRunner = runner
	return a
}

// processProjectWithClaude uses the AgentRunner to generate intelligent
// daily focus cards with actionable recommendations.
func (a *DailyFocusAgent) processProjectWithClaude(ctx context.Context, projectID uuid.UUID, projectName string) (*BriefingResult, error) {
	now := time.Now().UTC()

	userMessage := fmt.Sprintf(`Analyze today's priorities for project "%s" (ID: %s).
Date: %s

Please use the available tools to gather project data, then analyze the situation
and create the appropriate feed cards for the most important items.
Finally, provide a brief summary paragraph for the daily briefing email.`,
		projectName, projectID, now.Format("Monday, Jan 02, 2006"))

	projectCtx := ProjectContext{
		ProjectID: projectID,
		OrgID:     uuid.Nil, // Will be set by GenerateBriefings caller context
		UserID:    uuid.Nil, // Agent user
	}

	result, err := a.claudeRunner.Run(ctx, dailyFocusSystemPrompt, userMessage, projectCtx)
	if err != nil {
		return nil, fmt.Errorf("claude daily focus failed: %w", err)
	}

	slog.Info("claude daily focus completed",
		"project_id", projectID,
		"turns", result.Turns,
		"tools_used", result.ToolsUsed,
		"tokens", result.TotalTokens,
	)

	// Parse Claude's text response into a briefing result
	return parseBriefingResponse(projectID, projectName, result.Text), nil
}

// parseBriefingResponse converts Claude's text summary into a structured BriefingResult.
// Claude may return structured JSON or plain text — we handle both gracefully.
func parseBriefingResponse(projectID uuid.UUID, projectName, text string) *BriefingResult {
	// Try JSON parsing first (Claude sometimes returns structured output)
	var jsonBriefing struct {
		Bullets  []string `json:"bullets"`
		Priority string   `json:"priority"`
	}
	if err := json.Unmarshal([]byte(text), &jsonBriefing); err == nil && len(jsonBriefing.Bullets) > 0 {
		priority := jsonBriefing.Priority
		if priority == "" {
			priority = models.PriorityNormal
		}
		return &BriefingResult{
			ProjectID:   projectID,
			ProjectName: projectName,
			Title:       fmt.Sprintf("Daily Briefing: %s", projectName),
			Bullets:     jsonBriefing.Bullets,
			Priority:    priority,
		}
	}

	// Plain text — use as a single bullet
	return &BriefingResult{
		ProjectID:   projectID,
		ProjectName: projectName,
		Title:       fmt.Sprintf("Daily Briefing: %s", projectName),
		Bullets:     []string{text},
		Priority:    models.PriorityNormal,
	}
}
