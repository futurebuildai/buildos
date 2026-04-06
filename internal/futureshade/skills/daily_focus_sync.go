package skills

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// DailyFocusExecutor defines the interface for the daily focus agent.
// This avoids a direct dependency on the agents package, allowing for
// circular dependency avoidance and easier testing.
// TODO: Wire to agents.DailyFocusAgent when integration is ready.
type DailyFocusExecutor interface {
	GenerateBriefings(ctx context.Context, orgID uuid.UUID) error
}

// DailyFocusSyncSkill wraps DailyFocusAgent.GenerateBriefings for Tribunal-triggered execution.
type DailyFocusSyncSkill struct {
	executor DailyFocusExecutor
}

// NewDailyFocusSyncSkill creates a new DailyFocusSyncSkill.
func NewDailyFocusSyncSkill(executor DailyFocusExecutor) *DailyFocusSyncSkill {
	return &DailyFocusSyncSkill{executor: executor}
}

// ID returns the skill identifier.
func (s *DailyFocusSyncSkill) ID() string {
	return "daily_focus_sync"
}

// Execute runs the daily focus agent.
// Required params: org_id (string UUID)
func (s *DailyFocusSyncSkill) Execute(ctx context.Context, params map[string]any) (Result, error) {
	orgIDStr, ok := params["org_id"].(string)
	if !ok || orgIDStr == "" {
		return Result{
			Success: false,
			Summary: "Missing required parameter: org_id",
		}, fmt.Errorf("missing required parameter: org_id")
	}

	orgID, err := uuid.Parse(orgIDStr)
	if err != nil {
		return Result{
			Success: false,
			Summary: fmt.Sprintf("Invalid org_id format: %v", err),
		}, fmt.Errorf("invalid org_id: %w", err)
	}

	err = s.executor.GenerateBriefings(ctx, orgID)
	if err != nil {
		return Result{
			Success: false,
			Summary: fmt.Sprintf("Daily focus sync failed: %v", err),
		}, err
	}

	return Result{
		Success: true,
		Summary: "Daily focus sync completed successfully",
	}, nil
}
