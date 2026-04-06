package skills

import (
	"context"
	"fmt"
)

// ProcurementExecutor defines the interface for the procurement agent.
// This avoids a direct dependency on the agents package.
// TODO: Wire to agents.ProcurementAgent when integration is ready.
type ProcurementExecutor interface {
	RunCheck(ctx context.Context) error
}

// ProcurementSyncSkill wraps ProcurementAgent.RunCheck for Tribunal-triggered execution.
type ProcurementSyncSkill struct {
	executor ProcurementExecutor
}

// NewProcurementSyncSkill creates a new ProcurementSyncSkill.
func NewProcurementSyncSkill(executor ProcurementExecutor) *ProcurementSyncSkill {
	return &ProcurementSyncSkill{executor: executor}
}

// ID returns the skill identifier.
func (s *ProcurementSyncSkill) ID() string {
	return "procurement_sync"
}

// Execute runs the procurement agent.
func (s *ProcurementSyncSkill) Execute(ctx context.Context, params map[string]any) (Result, error) {
	err := s.executor.RunCheck(ctx)
	if err != nil {
		return Result{
			Success: false,
			Summary: fmt.Sprintf("Procurement sync failed: %v", err),
		}, err
	}

	return Result{
		Success: true,
		Summary: "Procurement sync completed successfully",
	}, nil
}
