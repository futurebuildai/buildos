package agents

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/google/uuid"
)

// procurementClaudeSystemPrompt guides Claude for procurement reasoning.
const procurementClaudeSystemPrompt = `You are a construction procurement specialist AI analyzing long-lead items for a project.

Your job: For each procurement item that has reached Warning or Critical status, analyze the situation and create actionable feed cards.

## For each item, you should:
1. Assess the urgency based on lead time, calculated order date, and critical path impact
2. Create an approval card (create_approval_card) recommending the specific action to take:
   - "Order Now" for critical items where the order window is closing
   - "Escalate to PM" for items with vendor issues or significant budget impact
   - "Find Alternative" for items that may be unavailable or overpriced
3. Include the consequence of inaction (e.g., "3-day critical path slip if not ordered by Friday")

## Be specific:
- Include item names, deadlines, and dollar amounts when available
- Recommend specific actions (not just "review this")
- Flag items that affect the critical path as highest priority
`

// WithClaudeRunner sets the AgentRunner for Claude-powered procurement reasoning.
func (a *ProcurementAgent) WithClaudeRunner(runner *AgentRunner) *ProcurementAgent {
	a.claudeRunner = runner
	return a
}

// runCheckWithClaude uses Claude to analyze procurement status changes and generate
// intelligent recommendations with approval cards. Falls back to template cards on failure.
func (a *ProcurementAgent) runCheckWithClaude(ctx context.Context, orgID uuid.UUID, changes []service.StatusChange) {
	if len(changes) == 0 {
		return
	}

	// Build a summary of all items needing attention
	var itemSummary string
	for i, ch := range changes {
		orderDate := "N/A"
		if ch.Item.MustOrderDate != nil {
			orderDate = ch.Item.MustOrderDate.Format("2006-01-02")
		}
		itemSummary += fmt.Sprintf("%d. %s (Status: %s -> %s, Order by: %s, Days left: %d, Cost: %d cents %s)\n",
			i+1, ch.Item.Description, ch.OldStatus, ch.NewStatus,
			orderDate, ch.DaysLeft,
			ch.Item.EstimatedCostCents, ch.Item.EstimatedCostCurrencyCode)
	}

	userMessage := fmt.Sprintf(`Analyze these procurement items that need attention:

%s

Create appropriate approval cards for each item. The most critical items should get priority "critical".`, itemSummary)

	projectCtx := ProjectContext{
		ProjectID: changes[0].ProjectID,
		OrgID:     orgID,
		UserID:    uuid.Nil, // Agent user
	}

	result, err := a.claudeRunner.Run(ctx, procurementClaudeSystemPrompt, userMessage, projectCtx)
	if err != nil {
		slog.Warn("claude procurement reasoning failed, falling back to template cards",
			"org_id", orgID, "error", err)
		return
	}

	slog.Info("claude procurement reasoning completed",
		"org_id", orgID,
		"items", len(changes),
		"turns", result.Turns,
		"tools_used", result.ToolsUsed,
		"tokens", result.TotalTokens,
	)
}
