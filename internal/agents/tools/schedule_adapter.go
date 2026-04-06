package tools

import (
	"context"

	"github.com/futurebuild/futurebuild-os/internal/futureshade/skills"
	"github.com/google/uuid"
)

// ScheduleRecalcAdapter bridges ScheduleEngine to the skills.ScheduleRecalcExecutor interface.
// This allows the FutureShade skills layer to invoke CPM recalculation without depending
// directly on the tools package's concrete types.
type ScheduleRecalcAdapter struct {
	engine *ScheduleEngine
}

// NewScheduleRecalcAdapter creates an adapter that wraps a ScheduleEngine.
func NewScheduleRecalcAdapter(engine *ScheduleEngine) *ScheduleRecalcAdapter {
	return &ScheduleRecalcAdapter{engine: engine}
}

// RecalculateSchedule implements skills.ScheduleRecalcExecutor by delegating
// to ScheduleEngine and converting the result type.
func (a *ScheduleRecalcAdapter) RecalculateSchedule(ctx context.Context, projectID, orgID uuid.UUID) (*skills.ScheduleRecalcResult, error) {
	result, err := a.engine.RecalculateSchedule(ctx, projectID, orgID)
	if err != nil {
		return nil, err
	}

	return &skills.ScheduleRecalcResult{
		TaskCount:         result.TaskCount,
		CriticalPathCount: result.CriticalPathCount,
		ProjectEnd:        result.ProjectEnd,
	}, nil
}
